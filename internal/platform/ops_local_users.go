package platform

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/jackc/pgx/v5"
)

type LocalOpsUser struct {
	ID                int64      `json:"id"`
	Username          string     `json:"username"`
	DisplayName       string     `json:"display_name"`
	DiscordID         string     `json:"discord_id"`
	Group             string     `json:"group"`
	Role              int        `json:"role"`
	Status            int        `json:"status"`
	TwoFA             bool       `json:"two_fa_enabled"`
	Reserve           string     `json:"reserve"`
	WalletInitialized bool       `json:"wallet_initialized"`
	CreatedAt         time.Time  `json:"created_at"`
	LastLoginAt       *time.Time `json:"last_login_at"`
}
type OpsUserFilter struct {
	Page, PageSize int64
	Search, Group  string
	Role, Status   int
}
type LocalOpsUserPage struct {
	Items    []LocalOpsUser `json:"items"`
	Total    int64          `json:"total"`
	Page     int64          `json:"page"`
	PageSize int64          `json:"page_size"`
}

const localOpsUserTables = ` FROM identity.users u LEFT JOIN economy.wallet_balances w ON w.newapi_user_id=u.id AND w.asset_type='RESERVE_API_CREDIT'`
const localOpsUserColumns = `u.id,u.username,u.display_name,coalesce(u.discord_id,''),u.group_name,u.role,u.status,
 EXISTS(SELECT 1 FROM identity.two_fas WHERE user_id=u.id AND is_enabled AND deleted_at IS NULL),
 coalesce(w.balance_units,0),w.newapi_user_id IS NOT NULL,u.created_at,u.last_login_at`

func scanLocalOpsUser(row pgx.Row) (LocalOpsUser, error) {
	var v LocalOpsUser
	var units int64
	err := row.Scan(&v.ID, &v.Username, &v.DisplayName, &v.DiscordID, &v.Group, &v.Role, &v.Status, &v.TwoFA, &units, &v.WalletInitialized, &v.CreatedAt, &v.LastLoginAt)
	v.Reserve = FormatAmount(units)
	return v, err
}
func (s *Store) ListLocalOpsUsers(ctx context.Context, f OpsUserFilter) (LocalOpsUserPage, error) {
	return listLocalOpsUsers(ctx, s.pool, f)
}
func listLocalOpsUsers(ctx context.Context, q announcementQuerier, f OpsUserFilter) (LocalOpsUserPage, error) {
	out := LocalOpsUserPage{Items: []LocalOpsUser{}, Page: f.Page, PageSize: f.PageSize}
	where := ` WHERE u.deleted_at IS NULL AND ($1='' OR u.id::text=$1 OR strpos(lower(u.username),lower($1))>0 OR strpos(lower(u.display_name),lower($1))>0 OR strpos(coalesce(u.discord_id,''),$1)>0)
 AND ($2='' OR u.group_name=$2) AND ($3::int=0 OR u.role=$3) AND ($4::int=0 OR u.status=$4)`
	args := []any{f.Search, f.Group, f.Role, f.Status}
	if err := q.QueryRow(ctx, `SELECT count(*)`+localOpsUserTables+where, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := q.Query(ctx, `SELECT `+localOpsUserColumns+localOpsUserTables+where+` ORDER BY u.id DESC LIMIT $5 OFFSET $6`, append(args, f.PageSize, (f.Page-1)*f.PageSize)...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scanLocalOpsUser(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}
func (s *Store) LocalOpsUser(ctx context.Context, id int64) (LocalOpsUser, error) {
	return readLocalOpsUser(ctx, s.pool, id)
}
func readLocalOpsUser(ctx context.Context, q announcementQuerier, id int64) (LocalOpsUser, error) {
	return scanLocalOpsUser(q.QueryRow(ctx, `SELECT `+localOpsUserColumns+localOpsUserTables+` WHERE u.id=$1 AND u.deleted_at IS NULL`, id))
}

type opsMasterSnapshot struct {
	UserID            int64      `json:"newapi_user_id,string"`
	DisplayName       string     `json:"display_name"`
	ProfileVersion    int64      `json:"profile_version,string"`
	RenameRequired    bool       `json:"rename_required"`
	NicknameChangedAt *time.Time `json:"nickname_changed_at"`
}

func opsMasterProfileSnapshot(profile Profile) opsMasterSnapshot {
	return opsMasterSnapshot{
		UserID: profile.UserID, DisplayName: profile.DisplayName, ProfileVersion: profile.ProfileVersion,
		RenameRequired: profile.RenameRequired, NicknameChangedAt: profile.NicknameChangedAt,
	}
}

func (s *Store) LocalOpsMaster(ctx context.Context, id int64) (opsMasterSnapshot, error) {
	return readLocalOpsMaster(ctx, s.pool, id)
}
func readLocalOpsMaster(ctx context.Context, q announcementQuerier, id int64) (opsMasterSnapshot, error) {
	var v opsMasterSnapshot
	err := q.QueryRow(ctx, `SELECT u.id,coalesce(p.display_name,''),coalesce(p.profile_version,0),coalesce(p.rename_required,false),p.nickname_changed_at
	 FROM identity.users u LEFT JOIN identity.master_profiles p ON p.newapi_user_id=u.id WHERE u.id=$1 AND u.deleted_at IS NULL`, id).
		Scan(&v.UserID, &v.DisplayName, &v.ProfileVersion, &v.RenameRequired, &v.NicknameChangedAt)
	return v, err
}

type OpsUserChange struct {
	Group       string `json:"group"`
	Status      int    `json:"status"`
	Role        *int   `json:"role"`
	Amount      string `json:"amount"`
	Note        string `json:"note"`
	RequestID   string `json:"request_id"`
	KeyID       int64  `json:"key_id"`
	Required    *bool  `json:"required"`
	DisplayName string `json:"display_name"`
}

func (s *Store) ChangeLocalOpsUser(ctx context.Context, sessions identity.Sessions, actor, subject int64, action string, v OpsUserChange) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = changeLocalOpsUser(ctx, tx, sessions, actor, subject, action, v); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func changeLocalOpsUser(ctx context.Context, tx pgx.Tx, sessions identity.Sessions, actor, subject int64, action string, v OpsUserChange) error {
	// ponytail: serialize low-volume administrator account writes, including the
	// last-root check. Split this lock only if Ops write throughput needs it.
	if err := lockIdentity(ctx, tx, "local-ops-users"); err != nil {
		return err
	}
	if action == "profile" {
		if v.Group == "" || (v.Status != 1 && v.Status != 2) || v.Role != nil && *v.Role != 1 && *v.Role != 10 && *v.Role != 100 {
			return ErrOpsInvalid
		}
		// Same order as group rename: group first, then users.
		var group string
		if err := tx.QueryRow(ctx, `SELECT name FROM relay.groups WHERE name=$1 FOR SHARE`, v.Group).Scan(&group); err != nil {
			return err
		}
	}
	rows, err := tx.Query(ctx, `SELECT id,role,status FROM identity.users WHERE id=ANY($1) AND deleted_at IS NULL ORDER BY id FOR UPDATE`, []int64{actor, subject})
	if err != nil {
		return err
	}
	var actorRole, targetRole int
	var targetFound bool
	for rows.Next() {
		var id int64
		var role, status int
		if err = rows.Scan(&id, &role, &status); err != nil {
			break
		}
		if id == actor && status == 1 {
			actorRole = role
		}
		if id == subject {
			targetRole = role
			targetFound = true
		}
	}
	rows.Close()
	if err != nil {
		return err
	}
	if err = rows.Err(); err != nil {
		return err
	}
	// Official f1164142 controller/user.go canManageTargetRole: root or strictly higher role.
	if actorRole < 10 || actorRole != 100 && actorRole <= targetRole {
		return ErrOpsForbidden
	}
	if !targetFound {
		return pgx.ErrNoRows
	}
	before, err := readLocalOpsUser(ctx, tx, subject)
	if err != nil {
		return err
	}
	var beforeAudit any = before
	var after any
	auditAction := "USER_" + strings.ToUpper(strings.ReplaceAll(action, "-", "_"))
	target := strconv.FormatInt(subject, 10)
	kind := "USER"
	switch action {
	case "master-rename-required", "master-force-rename":
		master, e := scanProfile(tx.QueryRow(ctx, `SELECT `+profileColumns+` FROM identity.master_profiles WHERE newapi_user_id=$1 FOR UPDATE`, subject))
		if errors.Is(e, pgx.ErrNoRows) || e == nil && master.ProfileVersion == 0 {
			return ErrOpsNotFound
		}
		if e != nil {
			return e
		}
		var changed Profile
		if action == "master-rename-required" {
			if v.Required == nil {
				return ErrOpsInvalid
			}
			changed, err = scanProfile(tx.QueryRow(ctx, `UPDATE identity.master_profiles SET rename_required=$2,profile_version=profile_version+1,updated_at=clock_timestamp() WHERE newapi_user_id=$1 RETURNING `+profileColumns, subject, *v.Required))
		} else {
			display, normalized, e := ValidateNickname(v.DisplayName)
			if e != nil {
				return e
			}
			// Administrator moderation does not consume the user's nickname cooldown.
			changed, err = scanProfile(tx.QueryRow(ctx, `UPDATE identity.master_profiles SET display_name=$2,normalized_name=$3,profile_version=profile_version+1,rename_required=false,updated_at=clock_timestamp() WHERE newapi_user_id=$1 RETURNING `+profileColumns, subject, display, normalized))
			if err == nil && master.DisplayName != changed.DisplayName {
				err = appendProfileName(ctx, tx, changed, normalized)
			}
		}
		if err != nil {
			return profileDBError(err)
		}
		beforeAudit, after = opsMasterProfileSnapshot(master), opsMasterProfileSnapshot(changed)
		auditAction, kind = strings.ToUpper(strings.ReplaceAll(action, "-", "_")), "MASTER_PROFILE"
	case "profile":
		role := before.Role
		if v.Role != nil {
			role = *v.Role
			if role != before.Role && actorRole != 100 {
				return ErrOpsForbidden
			}
		}
		if before.Role == 100 && before.Status == 1 && (role != 100 || v.Status != 1) {
			var roots int
			if err = tx.QueryRow(ctx, `SELECT count(*) FROM identity.users WHERE role=100 AND status=1 AND deleted_at IS NULL`).Scan(&roots); err != nil {
				return err
			}
			if roots <= 1 {
				return ErrOpsLastSuperAdmin
			}
		}
		_, err = tx.Exec(ctx, `UPDATE identity.users SET group_name=$2,status=$3,role=$4 WHERE id=$1`, subject, v.Group, v.Status, role)
		if err == nil && (v.Status != before.Status || role != before.Role) {
			err = sessions.RevokeUser(ctx, subject)
		}
	case "logout", "reset-2fa":
		if action == "reset-2fa" {
			// Same deletes as official-derived account.SetTwoFA; the identity row lock
			// also covers login/session creation and backup-code consumption.
			_, err = tx.Exec(ctx, `DELETE FROM identity.two_fa_backup_codes WHERE user_id=$1`, subject)
			if err == nil {
				_, err = tx.Exec(ctx, `DELETE FROM identity.two_fas WHERE user_id=$1`, subject)
			}
		}
		if err == nil {
			err = sessions.RevokeUser(ctx, subject)
		}
	case "reserve":
		units, e := ParseAmount(strings.TrimPrefix(v.Amount, "-"))
		if e != nil || strings.TrimSpace(v.Note) == "" || utf8.RuneCountInString(v.Note) > 1000 || strings.ContainsRune(v.Note, 0) || !ValidOperationKey(v.RequestID) {
			return ErrOpsInvalid
		}
		if strings.HasPrefix(v.Amount, "-") {
			units = -units
		}
		var previousNote string
		e = tx.QueryRow(ctx, `SELECT after_state->>'note' FROM ops.audit_log WHERE action='OPS_ADJUST' AND target=$1`, v.RequestID).Scan(&previousNote)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		replay := e == nil
		if replay && previousNote != v.Note {
			return ErrIdempotencyConflict
		}
		entry, e := applyInTx(ctx, tx, Mutation{UserID: subject, Asset: ReserveAPICredit, DeltaUnits: units, BizType: "OPS_ADJUST", BizID: v.RequestID, EntryType: "OPS_ADJUST", IdempotencyKey: v.RequestID})
		if e != nil {
			return e
		}
		if replay {
			return nil
		}
		beforeAudit = map[string]any{"user_id": subject, "balance": FormatAmount(entry.BalanceBeforeUnits)}
		after = map[string]any{"entry_id": entry.ID, "user_id": subject, "delta": FormatAmount(units), "balance": FormatAmount(entry.BalanceAfterUnits), "note": v.Note}
		auditAction, kind, target = "OPS_ADJUST", "WALLET_ADJUSTMENT", v.RequestID
	case "disable-key", "delete-key":
		var key APIKey
		key, err = scanAPIKey(tx.QueryRow(ctx, `SELECT `+apiKeyColumns+` FROM relay.api_keys WHERE user_id=$1 AND id=$2 FOR UPDATE`, subject, v.KeyID))
		if err != nil {
			return err
		}
		if action == "disable-key" {
			_, err = tx.Exec(ctx, `UPDATE relay.api_keys SET status=2 WHERE user_id=$1 AND id=$2`, subject, v.KeyID)
		} else {
			_, err = tx.Exec(ctx, `DELETE FROM relay.api_keys WHERE user_id=$1 AND id=$2`, subject, v.KeyID)
		}
		beforeAudit = map[string]any{"user_id": subject, "key_id": key.ID, "key_name": key.Name, "status": key.Status}
		after = map[string]any{"user_id": subject, "key_id": key.ID, "key_name": key.Name, "status": 2, "deleted": action == "delete-key"}
		kind, target = "API_KEY", strconv.FormatInt(key.ID, 10)
	default:
		return ErrOpsInvalid
	}
	if err != nil {
		return err
	}
	if after == nil {
		after, err = readLocalOpsUser(ctx, tx, subject)
		if err != nil {
			return err
		}
	}
	return relayOpsAudit(ctx, tx, actor, auditAction, kind, target, beforeAudit, after)
}

type OpsUserLedger struct {
	LedgerEntry
	DeltaAmount        string `json:"delta_amount"`
	BalanceAfterAmount string `json:"balance_after_amount"`
	Note               string `json:"note"`
}
type OpsUserLedgerPage struct {
	Items    []OpsUserLedger `json:"items"`
	Total    int64           `json:"total"`
	Page     int64           `json:"page"`
	PageSize int64           `json:"page_size"`
}

func (s *Store) LocalOpsLedger(ctx context.Context, user, page, size int64) (OpsUserLedgerPage, error) {
	return localOpsLedger(ctx, s.pool, user, page, size)
}
func localOpsLedger(ctx context.Context, q announcementQuerier, user, page, size int64) (OpsUserLedgerPage, error) {
	out := OpsUserLedgerPage{Items: []OpsUserLedger{}, Page: page, PageSize: size}
	if err := q.QueryRow(ctx, `SELECT count(*) FROM economy.wallet_ledger WHERE newapi_user_id=$1 AND asset_type='RESERVE_API_CREDIT'`, user).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := q.Query(ctx, `SELECT `+ledgerColumns+` FROM economy.wallet_ledger WHERE newapi_user_id=$1 AND asset_type='RESERVE_API_CREDIT' ORDER BY ledger_seq DESC LIMIT $2 OFFSET $3`, user, size, (page-1)*size)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		entry, e := scanEntry(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, OpsUserLedger{LedgerEntry: entry, DeltaAmount: FormatAmount(entry.DeltaUnits), BalanceAfterAmount: FormatAmount(entry.BalanceAfterUnits)})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return out, err
	}
	// Only adjustment rows carry an Ops note; the ledger itself stays immutable.
	for i := range out.Items {
		if out.Items[i].BizType == "OPS_ADJUST" {
			err = q.QueryRow(ctx, `SELECT after_state->>'note' FROM ops.audit_log WHERE action='OPS_ADJUST' AND target=$1`, out.Items[i].BizID).Scan(&out.Items[i].Note)
			if err != nil {
				return out, err
			}
		}
	}
	return out, nil
}
