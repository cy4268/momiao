package platform

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNicknameTaken       = errors.New("nickname taken")
	ErrStaleProfileVersion = errors.New("stale profile version")
	ErrRenameCooldown      = errors.New("rename cooldown")
	ErrInvalidAvatar       = errors.New("invalid avatar")
	ErrInvalidProfile      = errors.New("invalid profile mutation")
)

type ProfileAvatar struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Source string `json:"source"`
}
type Profile struct {
	UserID            int64           `json:"user_id,string"`
	ShortAccountID    string          `json:"short_account_id"`
	Status            string          `json:"status"`
	DisplayName       string          `json:"display_name"`
	AvatarID          string          `json:"avatar_id"`
	ProfileVersion    int64           `json:"profile_version,string"`
	NicknameChangedAt *time.Time      `json:"nickname_changed_at"`
	NextRenameAt      *time.Time      `json:"next_rename_at"`
	SuggestedName     string          `json:"suggested_name"`
	Avatars           []ProfileAvatar `json:"avatars"`
}
type ProfilePatch struct {
	ExpectedVersion int64
	DisplayName     *string
	AvatarID        *string
}

func profileView(p Profile) Profile {
	p.ShortAccountID = ShortAccountID(p.UserID)
	p.SuggestedName = "Master-" + p.ShortAccountID
	p.Status = "INCOMPLETE"
	if p.ProfileVersion > 0 {
		p.Status = "COMPLETE"
	}
	p.AvatarID = "system-default"
	p.Avatars = []ProfileAvatar{{ID: "system-default", Label: "系统默认头像", Source: "SYSTEM"}}
	if p.NicknameChangedAt != nil {
		changed := p.NicknameChangedAt.UTC()
		next := changed.Add(7 * 24 * time.Hour)
		p.NicknameChangedAt = &changed
		p.NextRenameAt = &next
	}
	return p
}

const profileColumns = "newapi_user_id,display_name,avatar_id,profile_version,nickname_changed_at"

func scanProfile(row pgx.Row) (Profile, error) {
	var p Profile
	err := row.Scan(&p.UserID, &p.DisplayName, &p.AvatarID, &p.ProfileVersion, &p.NicknameChangedAt)
	if err != nil {
		return Profile{}, err
	}
	return profileView(p), nil
}

// ReadProfile projects an absent row without creating any identity or wallet.
func (s *Store) ReadProfile(ctx context.Context, userID int64) (Profile, error) {
	if userID <= 0 {
		return Profile{}, ErrInvalidProfile
	}
	p, err := scanProfile(s.pool.QueryRow(ctx, "SELECT "+profileColumns+" FROM identity.master_profiles WHERE newapi_user_id=$1", userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return profileView(Profile{UserID: userID}), nil
	}
	return p, err
}

func profileDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "master_profiles_normalized_name_key" {
		return ErrNicknameTaken
	}
	return err
}

func appendProfileName(ctx context.Context, tx pgx.Tx, p Profile, normalized string) error {
	_, err := tx.Exec(ctx, `INSERT INTO identity.master_profile_name_history(newapi_user_id,profile_version,display_name,normalized_name) VALUES($1,$2,$3,$4)`, p.UserID, p.ProfileVersion, p.DisplayName, normalized)
	return err
}

// InitializeProfile atomically creates only the external identity anchor,
// complete profile and first history entry. Replay is limited to unchanged v1.
func (s *Store) InitializeProfile(ctx context.Context, userID, expected int64, display, avatar string) (Profile, error) {
	if userID <= 0 || expected != 0 {
		return Profile{}, ErrInvalidProfile
	}
	if avatar != "system-default" {
		return Profile{}, ErrInvalidAvatar
	}
	display, normalized, err := ValidateNickname(display)
	if err != nil {
		return Profile{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Profile{}, err
	}
	defer rollback(tx)
	// An absent profile has no row to lock. Serialize same-user initialization;
	// cross-user nickname conflicts remain enforced by the database UNIQUE key.
	if err = lockIdentity(ctx, tx, "master-profile.initialize.v1", userID); err != nil {
		return Profile{}, err
	}
	existing, err := scanProfile(tx.QueryRow(ctx, "SELECT "+profileColumns+" FROM identity.master_profiles WHERE newapi_user_id=$1 FOR UPDATE", userID))
	if err == nil {
		if existing.ProfileVersion != 1 || existing.DisplayName != display || existing.AvatarID != avatar {
			return Profile{}, ErrStaleProfileVersion
		}
		if err = tx.Commit(ctx); err != nil {
			return Profile{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO identity.account_refs(newapi_user_id) VALUES($1) ON CONFLICT DO NOTHING", userID); err != nil {
		return Profile{}, err
	}
	p, err := scanProfile(tx.QueryRow(ctx, `INSERT INTO identity.master_profiles(newapi_user_id,display_name,normalized_name,avatar_id) VALUES($1,$2,$3,$4) RETURNING `+profileColumns, userID, display, normalized, avatar))
	if err != nil {
		return Profile{}, profileDBError(err)
	}
	if err = appendProfileName(ctx, tx, p, normalized); err != nil {
		return Profile{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Profile{}, profileDBError(err)
	}
	return p, nil
}

// UpdateProfile uses optimistic versioning in addition to a row lock. An exact
// no-op verifies the version but does not consume cooldown or create history.
// Commit errors are not retried; clients must reconcile with a successful GET.
func (s *Store) UpdateProfile(ctx context.Context, userID int64, patch ProfilePatch) (Profile, error) {
	if userID <= 0 || patch.ExpectedVersion < 1 || (patch.DisplayName == nil && patch.AvatarID == nil) {
		return Profile{}, ErrInvalidProfile
	}
	if patch.AvatarID != nil && *patch.AvatarID != "system-default" {
		return Profile{}, ErrInvalidAvatar
	}
	var display, normalized string
	var err error
	if patch.DisplayName != nil {
		display, normalized, err = ValidateNickname(*patch.DisplayName)
		if err != nil {
			return Profile{}, err
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Profile{}, err
	}
	defer rollback(tx)
	p, err := scanProfile(tx.QueryRow(ctx, "SELECT "+profileColumns+" FROM identity.master_profiles WHERE newapi_user_id=$1 FOR UPDATE", userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrStaleProfileVersion
	}
	if err != nil {
		return Profile{}, err
	}
	if p.ProfileVersion != patch.ExpectedVersion {
		return Profile{}, ErrStaleProfileVersion
	}
	rename := patch.DisplayName != nil && display != p.DisplayName
	// Only one avatar exists in this slice. Its valid avatar-only request is an
	// exact no-op, including during the rename cooldown.
	if !rename {
		if err = tx.Commit(ctx); err != nil {
			return Profile{}, err
		}
		return p, nil
	}
	if p.ProfileVersion == math.MaxInt64 {
		return Profile{}, ErrInvalidProfile
	}
	var now time.Time
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return Profile{}, err
	}
	if p.NextRenameAt != nil && now.Before(*p.NextRenameAt) {
		return Profile{}, ErrRenameCooldown
	}
	p, err = scanProfile(tx.QueryRow(ctx, `UPDATE identity.master_profiles SET display_name=$2,normalized_name=$3,profile_version=profile_version+1,nickname_changed_at=$4,updated_at=$4 WHERE newapi_user_id=$1 RETURNING `+profileColumns, userID, display, normalized, now))
	if err != nil {
		return Profile{}, profileDBError(err)
	}
	if err = appendProfileName(ctx, tx, p, normalized); err != nil {
		return Profile{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Profile{}, profileDBError(err)
	}
	return p, nil
}
