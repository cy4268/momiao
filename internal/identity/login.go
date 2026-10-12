// Copyright QuantumNous and contributors. AGPL-3.0-or-later.
// Adapted from official new-api@f1164142 controller/user.go and twofa.go:
// password -> optional five-minute factor challenge -> server session.
package identity

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

type Challenge struct {
	ID        string    `json:"challenge_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type loginFlow struct {
	UserID     int64  `json:"user_id"`
	BrowserKey string `json:"browser_key"`
}

// A different Redis key prevents a factor token from ever being a login cookie.
func loginFlowKey(token string) string { return sessionKey(token) + ":2fa" }

func (s *Store) PasswordLogin(ctx context.Context, sessions Sessions, browser Session, username, password string) (Session, *Challenge, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Session{}, nil, err
	}
	defer tx.Rollback(ctx)
	user, err := authenticatePassword(ctx, tx, username, password)
	if err != nil {
		return Session{}, nil, err
	}
	return finishLogin(ctx, tx, sessions, browser, user.ID)
}

// The official OAuth flow looks up the provider ID after code exchange. Login
// never auto-registers here; momiao's admission path owns new accounts.
func (s *Store) DiscordLogin(ctx context.Context, sessions Sessions, browser Session, discordID string) (Session, *Challenge, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Session{}, nil, err
	}
	defer tx.Rollback(ctx)
	user, err := scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM identity.users WHERE discord_id=$1 AND deleted_at IS NULL FOR UPDATE`, discordID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, nil, Fault{403, "MOMIAO_DISCORD_UNBOUND"}
	}
	if err != nil {
		return Session{}, nil, err
	}
	if user.Status != 1 {
		return Session{}, nil, ErrCredentials
	}
	// Keep the same existing per-user 2FA gate for both password and Discord.
	return finishLogin(ctx, tx, sessions, browser, user.ID)
}

func finishLogin(ctx context.Context, tx pgx.Tx, sessions Sessions, browser Session, userID int64) (Session, *Challenge, error) {
	var twoFA bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM identity.two_fas WHERE user_id=$1 AND is_enabled AND deleted_at IS NULL)`, userID).Scan(&twoFA); err != nil {
		return Session{}, nil, err
	}
	if twoFA {
		now := time.Now().UTC()
		challenge := &Challenge{ID: sessionToken(), ExpiresAt: now.Add(5 * time.Minute)}
		raw, err := json.Marshal(loginFlow{UserID: userID, BrowserKey: sessionKey(browser.Token)})
		if err != nil {
			return Session{}, nil, err
		}
		err = sessions.Redis.Eval(ctx, createSession, []string{loginFlowKey(challenge.ID), userSessionsKey(userID)},
			string(raw), (5 * time.Minute).Milliseconds(), challenge.ExpiresAt.UnixMilli(), now.UnixMilli(), sessionLifetime.Milliseconds()).Err()
		if err != nil {
			return Session{}, nil, err
		}
		return Session{}, challenge, tx.Commit(ctx)
	}
	value, err := issueLogin(ctx, tx, sessions, userID)
	return value, nil, err
}

func (s *Store) SecondFactorLogin(ctx context.Context, sessions Sessions, browser Session, token, code string) (Session, error) {
	key := loginFlowKey(token)
	readFlow := func() (loginFlow, error) {
		raw, err := sessions.Redis.Get(ctx, key).Bytes()
		if errors.Is(err, redis.Nil) {
			return loginFlow{}, ErrCredentials
		}
		if err != nil {
			return loginFlow{}, err
		}
		var flow loginFlow
		if err := json.Unmarshal(raw, &flow); err != nil {
			return loginFlow{}, err
		}
		if flow.BrowserKey != sessionKey(browser.Token) {
			return loginFlow{}, ErrCredentials
		}
		return flow, nil
	}
	flow, err := readFlow()
	if err != nil {
		return Session{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback(ctx)
	user, err := scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM identity.users WHERE id=$1 AND status=1 AND deleted_at IS NULL FOR UPDATE`, flow.UserID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrCredentials
	}
	if err != nil {
		return Session{}, err
	}
	// Password/status changes revoke indexed flows under this same user lock.
	// Re-read after locking, so a consumed or revoked flow cannot issue a session.
	if _, err := readFlow(); err != nil {
		return Session{}, err
	}
	if err := authenticateSecondFactor(ctx, tx, user.ID, code); err != nil {
		if errors.Is(err, ErrCredentials) {
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return Session{}, commitErr
			}
		}
		return Session{}, err
	}
	if err := sessions.Redis.Eval(ctx, revokeSession, []string{key, userSessionsKey(user.ID)}).Err(); err != nil {
		return Session{}, err
	}
	return issueLogin(ctx, tx, sessions, user.ID)
}

// Caller holds the user's row lock from credential validation through issuance.
func issueLogin(ctx context.Context, tx pgx.Tx, sessions Sessions, userID int64) (Session, error) {
	if _, err := tx.Exec(ctx, `UPDATE identity.users SET last_login_at=now() WHERE id=$1`, userID); err != nil {
		return Session{}, err
	}
	value, err := sessions.Create(ctx, userID)
	if err != nil {
		return Session{}, err
	}
	return value, tx.Commit(ctx)
}
