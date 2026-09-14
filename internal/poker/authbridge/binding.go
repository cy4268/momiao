// Package authbridge binds an online-verified native authentication chain to an
// immutable platform security epoch. It stores no bearer/refresh credentials.
package authbridge

import (
	"context"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/poker/connectticket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxSafe = uint64(9007199254740991)

type NativeRef struct {
	UserID         string
	SessionIDHash  string
	SessionVersion uint64
}
type NativeSession struct {
	Ref             NativeRef
	UserAuthVersion uint64
	CreatedAt       time.Time
	ExpiresAt       time.Time
}
type NativeCheck func(context.Context, NativeRef) (NativeSession, error)

type Authority struct {
	pool   *pgxpool.Pool
	native NativeCheck
}

func New(pool *pgxpool.Pool, native NativeCheck) (*Authority, error) {
	if pool == nil || native == nil {
		return nil, connectticket.ErrConfig
	}
	return &Authority{pool, native}, nil
}

func validRef(r NativeRef) bool {
	id, e := strconv.ParseInt(r.UserID, 10, 64)
	if e != nil || id <= 0 || strconv.FormatInt(id, 10) != r.UserID || r.SessionVersion == 0 || r.SessionVersion > maxSafe {
		return false
	}
	h, e := hex.DecodeString(r.SessionIDHash)
	return e == nil && len(h) == 32 && hex.EncodeToString(h) == r.SessionIDHash
}

func (a *Authority) live(ctx context.Context, r NativeRef) (NativeSession, error) {
	if a == nil || ctx == nil || !validRef(r) {
		return NativeSession{}, connectticket.ErrInvalid
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	n, e := a.native(c, r)
	if e != nil {
		if errors.Is(e, connectticket.ErrRevoked) {
			return NativeSession{}, connectticket.ErrRevoked
		}
		return NativeSession{}, connectticket.ErrUnavailable
	}
	now := time.Now()
	if c.Err() != nil {
		return NativeSession{}, connectticket.ErrUnavailable
	}
	if n.Ref != r || n.UserAuthVersion == 0 || n.UserAuthVersion > maxSafe || n.CreatedAt.IsZero() || n.CreatedAt.After(now) || !n.ExpiresAt.After(now) || !n.ExpiresAt.After(n.CreatedAt) {
		return NativeSession{}, connectticket.ErrRevoked
	}
	return n, nil
}

// Bind is called only after the HTTP adapter verified the original native
// credential. A decoded JWT/reference alone is not proof of possession.
func (a *Authority) Bind(ctx context.Context, r NativeRef) (connectticket.Session, error) {
	n, e := a.live(ctx, r)
	if e != nil {
		return connectticket.Session{}, e
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, e := a.pool.Begin(c)
	if e != nil {
		return connectticket.Session{}, connectticket.ErrUnavailable
	}
	defer rollback(tx)
	var epoch int64
	var changed *time.Time
	e = tx.QueryRow(c, `SELECT security_epoch,security_epoch_changed_at FROM identity.account_refs WHERE newapi_user_id=$1`, r.UserID).Scan(&epoch, &changed)
	if e != nil {
		return connectticket.Session{}, dbError(e)
	}
	if epoch < 0 || uint64(epoch) > maxSafe {
		return connectticket.Session{}, connectticket.ErrRevoked
	}
	// ON CONFLICT serializes first-bind races on this exact SID; no account
	// UPDATE privilege or long-lived native-token cache is required.
	_, e = tx.Exec(c, `INSERT INTO identity.native_session_bindings(session_id_hash,newapi_user_id,session_version,native_auth_version,security_epoch_snapshot,native_created_at)
		VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(session_id_hash) DO NOTHING`, r.SessionIDHash, r.UserID, int64(r.SessionVersion), int64(n.UserAuthVersion), epoch, n.CreatedAt)
	if e != nil {
		return connectticket.Session{}, dbError(e)
	}
	var user string
	var sv, uv, snapshot int64
	var created time.Time
	e = tx.QueryRow(c, `SELECT newapi_user_id::text,session_version,native_auth_version,security_epoch_snapshot,native_created_at
		FROM identity.native_session_bindings WHERE session_id_hash=$1 FOR UPDATE`, r.SessionIDHash).Scan(&user, &sv, &uv, &snapshot, &created)
	if e != nil {
		return connectticket.Session{}, dbError(e)
	}
	if user != r.UserID || snapshot != epoch || sv > int64(r.SessionVersion) || uv > int64(n.UserAuthVersion) || !created.Equal(n.CreatedAt) {
		return connectticket.Session{}, connectticket.ErrRevoked
	}
	if sv != int64(r.SessionVersion) || uv != int64(n.UserAuthVersion) {
		_, e = tx.Exec(c, `UPDATE identity.native_session_bindings SET session_version=$2,native_auth_version=$3 WHERE session_id_hash=$1`, r.SessionIDHash, int64(r.SessionVersion), int64(n.UserAuthVersion))
		if e != nil {
			return connectticket.Session{}, dbError(e)
		}
	}
	if e = tx.Commit(c); e != nil {
		return connectticket.Session{}, connectticket.ErrUnavailable
	}
	if !n.ExpiresAt.After(time.Now()) {
		return connectticket.Session{}, connectticket.ErrRevoked
	}
	return connectticket.Session{UserID: r.UserID, SessionIDHash: r.SessionIDHash, SessionVersion: r.SessionVersion, SecurityEpoch: uint64(epoch)}, nil
}

// Check is directly usable as connectticket.SessionCheck, for mint, consume,
// socket commands and controller renewal. Native and PG are both mandatory.
func (a *Authority) Check(ctx context.Context, s connectticket.Session) error {
	_, e := a.CheckUntil(ctx, s)
	return e
}

// CheckUntil returns the online native absolute expiry only after the same
// mandatory native and PG binding checks as Check. Every error returns zero time.
func (a *Authority) CheckUntil(ctx context.Context, s connectticket.Session) (time.Time, error) {
	if s.SecurityEpoch > maxSafe {
		return time.Time{}, connectticket.ErrInvalid
	}
	n, e := a.live(ctx, NativeRef{s.UserID, s.SessionIDHash, s.SessionVersion})
	if e != nil {
		return time.Time{}, e
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var exists bool
	e = a.pool.QueryRow(c, `SELECT EXISTS(SELECT 1 FROM identity.native_session_bindings b
		JOIN identity.account_refs a USING(newapi_user_id)
		WHERE b.session_id_hash=$1 AND b.newapi_user_id=$2 AND b.session_version=$3 AND b.native_auth_version=$4
		AND b.security_epoch_snapshot=$5 AND a.security_epoch=b.security_epoch_snapshot AND b.native_created_at=$6)`, s.SessionIDHash, s.UserID, int64(s.SessionVersion), int64(n.UserAuthVersion), int64(s.SecurityEpoch), n.CreatedAt).Scan(&exists)
	if e != nil || c.Err() != nil {
		return time.Time{}, connectticket.ErrUnavailable
	}
	if !exists || !n.ExpiresAt.After(time.Now()) {
		return time.Time{}, connectticket.ErrRevoked
	}
	return n.ExpiresAt.UTC(), nil
}
func rollback(tx pgx.Tx) {
	c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = tx.Rollback(c)
}
func dbError(e error) error {
	var pg *pgconn.PgError
	if errors.Is(e, pgx.ErrNoRows) || errors.As(e, &pg) && (pg.Code == "23514" || pg.Code == "23503") {
		return connectticket.ErrRevoked
	}
	return connectticket.ErrUnavailable
}
