package poker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrTableAccessRequired  = errors.New("TABLE_ACCESS_REQUIRED")
	ErrTablePasswordInvalid = errors.New("TABLE_PASSWORD_INVALID")
	ErrRateLimited          = errors.New("RATE_LIMITED")
)

type TableAccessResult struct {
	TableID string `json:"table_id"`
}

type PasswordLimits struct {
	OwnerAttempts      uint32
	OwnerWindow        time.Duration
	OwnerTableAttempts uint32
	OwnerTableWindow   time.Duration
}

func (l PasswordLimits) valid() bool {
	validWindow := func(v time.Duration) bool {
		return v >= time.Millisecond && v <= 24*time.Hour && v%time.Millisecond == 0
	}
	return l.OwnerAttempts > 0 && l.OwnerTableAttempts > 0 && validWindow(l.OwnerWindow) && validWindow(l.OwnerTableWindow)
}

type TableAccessStore interface {
	AccessPut(context.Context, string, string, string, time.Time) (bool, error)
	AccessValid(context.Context, string, string, string, time.Time) (bool, error)
	AccessAttempt(context.Context, int64, string, uint32, time.Duration, uint32, time.Duration) (bool, error)
}

type credentialQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func validAccessAuth(auth AuthSession) bool {
	return auth.UserID > 0 && isHex(auth.SessionIDHash, 64) && auth.SessionVersion > 0 && auth.SessionVersion <= 9007199254740991 && auth.SecurityEpoch <= 9007199254740991
}

func accessBinding(auth AuthSession, phc string) string {
	tag := sha256.Sum256([]byte(phc))
	return "v1:" + strconv.FormatInt(auth.UserID, 10) + ":" + strconv.FormatUint(auth.SessionVersion, 10) + ":" + strconv.FormatUint(auth.SecurityEpoch, 10) + ":" + hex.EncodeToString(tag[:])
}

func validPasswordRuntime(runtime *PasswordRuntime) bool {
	if runtime == nil || runtime.slots == nil || cap(runtime.slots) < 1 || cap(runtime.slots) > 16 || !runtime.current.valid() || len(runtime.profiles) < 1 || len(runtime.profiles) > 4 {
		return false
	}
	current := false
	for _, profile := range runtime.profiles {
		if !profile.valid() {
			return false
		}
		current = current || profile == runtime.current
	}
	return current
}

func (s *Service) passwordEnabled() bool {
	return validPasswordRuntime(s.opts.Password) && s.opts.TableAccess != nil && s.opts.SessionDeadline != nil && s.opts.PasswordLimits.valid()
}

func (s *Service) passwordDeadline(ctx context.Context, auth AuthSession) (time.Time, error) {
	if !s.passwordEnabled() || !validAccessAuth(auth) {
		return time.Time{}, ErrPasswordUnavailable
	}
	until, err := s.opts.SessionDeadline(ctx, auth)
	if err != nil || until.IsZero() || !until.After(time.Now()) {
		if err != nil {
			return time.Time{}, errors.Join(ErrPasswordUnavailable, err)
		}
		return time.Time{}, ErrPasswordUnavailable
	}
	return until.UTC(), nil
}

func readAccessCredential(ctx context.Context, q credentialQuerier, tableID string) (string, string, error) {
	if !canonicalSessionID(tableID) {
		return "", "", ErrInvalid
	}
	var mode, phc string
	err := q.QueryRow(ctx, `SELECT t.access_mode,coalesce(c.password_phc,'') FROM poker.tables t
		LEFT JOIN poker.table_access_credentials c USING(table_id) WHERE t.table_id=$1`, tableID).Scan(&mode, &phc)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrDenied
	}
	if err != nil {
		return "", "", err
	}
	if mode == "PUBLIC" && phc == "" {
		return mode, phc, nil
	}
	if mode != "PASSWORD" || phc == "" || len(phc) > 512 {
		return "", "", ErrPasswordUnavailable
	}
	return mode, phc, nil
}

func (s *Service) accessValid(ctx context.Context, tableID string, auth AuthSession, phc string, until time.Time) (bool, error) {
	if !validAccessAuth(auth) || !canonicalSessionID(tableID) || phc == "" || until.IsZero() {
		return false, ErrPasswordUnavailable
	}
	ok, err := s.opts.TableAccess.AccessValid(ctx, auth.SessionIDHash, tableID, accessBinding(auth, phc), until)
	if err != nil {
		return false, errors.Join(ErrPasswordUnavailable, err)
	}
	return ok, nil
}

func (s *Service) authorizeCredential(ctx context.Context, tableID string, auth AuthSession, mode, phc string) error {
	if mode == "PUBLIC" {
		return nil
	}
	if mode != "PASSWORD" || !s.passwordEnabled() {
		return ErrPasswordUnavailable
	}
	until, err := s.passwordDeadline(ctx, auth)
	if err != nil {
		return err
	}
	ok, err := s.accessValid(ctx, tableID, auth, phc, until)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTableAccessRequired
	}
	return nil
}

func (s *Service) authorizeTableRow(ctx context.Context, q credentialQuerier, t *tableRow, auth AuthSession) error {
	if t.AccessMode == "PUBLIC" {
		return nil
	}
	mode, phc, err := readAccessCredential(ctx, q, t.ID)
	if err != nil {
		return err
	}
	if mode != t.AccessMode {
		return ErrTableAccessRequired
	}
	return s.authorizeCredential(ctx, t.ID, auth, mode, phc)
}

// AuthorizeTable is a read-only, non-renewing check. It is repeated at every
// protected domain boundary; neither a client flag nor a Poker session is a grant.
func (s *Service) AuthorizeTable(ctx context.Context, tableID string, auth AuthSession) error {
	mode, phc, err := readAccessCredential(ctx, s.opts.Pool, tableID)
	if err != nil {
		return err
	}
	return s.authorizeCredential(ctx, tableID, auth, mode, phc)
}

func (s *Service) VerifyTableAccess(ctx context.Context, tableID string, auth AuthSession, password string) (TableAccessResult, error) {
	result := TableAccessResult{}
	if !validAccessAuth(auth) || !canonicalSessionID(tableID) || !validTablePassword(password) {
		return result, ErrInvalid
	}
	if !s.passwordEnabled() {
		return result, ErrPasswordUnavailable
	}
	if _, err := s.passwordDeadline(ctx, auth); err != nil {
		return result, err
	}
	mode, phc, err := readAccessCredential(ctx, s.opts.Pool, tableID)
	if err != nil {
		return result, err
	}
	if mode != "PASSWORD" {
		return result, ErrInvalid
	}
	l := s.opts.PasswordLimits
	allowed, err := s.opts.TableAccess.AccessAttempt(ctx, auth.UserID, tableID, l.OwnerAttempts, l.OwnerWindow, l.OwnerTableAttempts, l.OwnerTableWindow)
	if err != nil {
		return result, errors.Join(ErrPasswordUnavailable, err)
	}
	if !allowed {
		return result, ErrRateLimited
	}
	matched, err := s.opts.Password.Verify(ctx, phc, password)
	if err != nil {
		return result, err
	}
	if !matched {
		return result, ErrTablePasswordInvalid
	}
	until, err := s.passwordDeadline(ctx, auth)
	if err != nil {
		return result, err
	}
	modeAfter, phcAfter, err := readAccessCredential(ctx, s.opts.Pool, tableID)
	if err != nil || modeAfter != mode || accessBinding(auth, phcAfter) != accessBinding(auth, phc) {
		if err != nil {
			return result, err
		}
		return result, ErrPasswordUnavailable
	}
	put, err := s.opts.TableAccess.AccessPut(ctx, auth.SessionIDHash, tableID, accessBinding(auth, phcAfter), until)
	if err != nil || !put {
		if err != nil {
			return result, errors.Join(ErrPasswordUnavailable, err)
		}
		return result, ErrPasswordUnavailable
	}
	result.TableID = tableID
	return result, nil
}
