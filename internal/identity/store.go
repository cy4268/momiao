// Copyright QuantumNous and contributors. AGPL-3.0-or-later.
// Adapted from official new-api@f1164142 model/user.go and model/twofa.go:
// preserve credential validation and factor lockout, replace GORM with pgx.
package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrCredentials = errors.New("用户名、密码或验证码错误")
	ErrTwoFALocked = errors.New("验证失败次数过多，请五分钟后重试")
)

type User struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	DisplayName  string     `json:"display_name"`
	Email        string     `json:"email"`
	Role         int        `json:"role"`
	Status       int        `json:"status"`
	DiscordID    string     `json:"discord_id"`
	Group        string     `json:"group"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at"`
}

type Store struct{ Pool *pgxpool.Pool }

const userColumns = `id,username,password_hash,display_name,email,role,status,coalesce(discord_id,''),group_name,created_at,last_login_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Email, &u.Role, &u.Status, &u.DiscordID, &u.Group, &u.CreatedAt, &u.LastLoginAt)
	return u, err
}

func (s *Store) User(ctx context.Context, id int64) (User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `SELECT `+userColumns+` FROM identity.users WHERE id=$1 AND deleted_at IS NULL`, id))
}

func (s *Store) DiscordUser(ctx context.Context, discordID string) (User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `SELECT `+userColumns+` FROM identity.users WHERE discord_id=$1 AND deleted_at IS NULL`, discordID))
}

// authenticatePassword follows official User.ValidateAndFill: trim the login
// name, accept username/email, verify the existing bcrypt hash, reject bans.
// The caller holds this transaction through challenge/session creation.
func authenticatePassword(ctx context.Context, tx pgx.Tx, username, password string) (User, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return User{}, ErrCredentials
	}
	u, err := scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM identity.users WHERE (username=$1 OR email=$1) AND deleted_at IS NULL ORDER BY id LIMIT 1 FOR UPDATE`, username))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrCredentials
	}
	if err != nil {
		return User{}, err
	}
	if u.PasswordHash == "" || !ValidatePasswordAndHash(password, u.PasswordHash) || u.Status != 1 {
		return User{}, ErrCredentials
	}
	return u, nil
}

// authenticateSecondFactor retains official TOTP/backup-code normalization,
// bcrypt backup hashes, one-use backup codes and the five-attempt lockout.
func authenticateSecondFactor(ctx context.Context, tx pgx.Tx, userID int64, code string) error {
	var secret string
	var failed int
	var lockedUntil *time.Time
	err := tx.QueryRow(ctx, `SELECT secret,failed_attempts,locked_until FROM identity.two_fas WHERE user_id=$1 AND is_enabled AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&secret, &failed, &lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCredentials
	}
	if err != nil {
		return err
	}
	if lockedUntil != nil && time.Now().Before(*lockedUntil) {
		return ErrTwoFALocked
	}
	valid := ValidateTOTPCode(secret, code)
	if !valid && ValidateBackupCode(code) {
		rows, err := tx.Query(ctx, `SELECT id,code_hash FROM identity.two_fa_backup_codes WHERE user_id=$1 AND NOT is_used AND deleted_at IS NULL`, userID)
		if err != nil {
			return err
		}
		type backup struct {
			id   int64
			hash string
		}
		codes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (backup, error) {
			var b backup
			err := row.Scan(&b.id, &b.hash)
			return b, err
		})
		if err != nil {
			return err
		}
		for _, b := range codes {
			if ValidatePasswordAndHash(NormalizeBackupCode(code), b.hash) {
				_, err = tx.Exec(ctx, `UPDATE identity.two_fa_backup_codes SET is_used=true,used_at=now() WHERE id=$1`, b.id)
				if err != nil {
					return err
				}
				valid = true
				break
			}
		}
	}
	if !valid {
		failed++
		if failed >= MaxFailAttempts {
			until := time.Now().Add(LockoutDuration * time.Second)
			lockedUntil = &until
		}
		_, err = tx.Exec(ctx, `UPDATE identity.two_fas SET failed_attempts=$2,locked_until=$3,updated_at=now() WHERE user_id=$1`, userID, failed, lockedUntil)
	} else {
		_, err = tx.Exec(ctx, `UPDATE identity.two_fas SET failed_attempts=0,locked_until=NULL,last_used_at=now(),updated_at=now() WHERE user_id=$1`, userID)
	}
	if err != nil {
		return err
	}
	if !valid {
		return ErrCredentials
	}
	return nil
}

// AccountStatus exposes only account flags, not hashes, factor secrets or provider tokens.
func (s *Store) AccountStatus(ctx context.Context, user User) (map[string]any, error) {
	var twoFA bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM identity.two_fas WHERE user_id=$1 AND is_enabled AND deleted_at IS NULL)`, user.ID).Scan(&twoFA)
	return map[string]any{"id": user.ID, "username": user.Username, "has_password": user.PasswordHash != "", "discord_linked": user.DiscordID != "", "two_fa_enabled": twoFA}, err
}
