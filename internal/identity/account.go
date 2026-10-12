// Copyright QuantumNous and contributors. AGPL-3.0-or-later.
// Adapted from official new-api@f1164142 controller/user.go (checkUpdatePassword)
// and controller/twofa.go (setup/enable/disable). Local pgx transactions and
// Redis revocation replace GORM auth versions and token rotation bundles.
package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type TwoFASetup struct {
	Secret      string   `json:"secret"`
	QRCodeData  string   `json:"qr_code_data"`
	BackupCodes []string `json:"backup_codes"`
}

// Account mutations and login issuance use the same user lock. Re-read the
// cookie after acquiring it so a revoked, already-queued request cannot write.
func (s *Store) accountTx(ctx context.Context, sessions Sessions, browser Session) (pgx.Tx, User, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, User{}, err
	}
	user, err := scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM identity.users WHERE id=$1 AND status=1 AND deleted_at IS NULL FOR UPDATE`, browser.UserID))
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrSessionExpired
	}
	if err == nil {
		_, err = sessions.Read(ctx, browser.Token)
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, User{}, err
	}
	return tx, user, nil
}

func rotateAccountSession(ctx context.Context, tx pgx.Tx, sessions Sessions, userID int64) (Session, error) {
	if err := sessions.RevokeUser(ctx, userID); err != nil {
		return Session{}, err
	}
	value, err := sessions.Create(ctx, userID)
	if err != nil {
		return Session{}, err
	}
	return value, tx.Commit(ctx)
}

func (s *Store) ChangePassword(ctx context.Context, sessions Sessions, browser Session, oldPassword, password string) (Session, error) {
	tx, user, err := s.accountTx(ctx, sessions, browser)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback(ctx)
	if user.PasswordHash == "" || !ValidatePasswordAndHash(oldPassword, user.PasswordHash) {
		return Session{}, Fault{400, "MOMIAO_INVALID_REQUEST"}
	}
	hash, err := Password2Hash(password)
	if err != nil {
		return Session{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE identity.users SET password_hash=$2 WHERE id=$1`, user.ID, hash); err != nil {
		return Session{}, err
	}
	return rotateAccountSession(ctx, tx, sessions, user.ID)
}

func (s *Store) SetupTwoFA(ctx context.Context, sessions Sessions, browser Session) (TwoFASetup, error) {
	tx, user, err := s.accountTx(ctx, sessions, browser)
	if err != nil {
		return TwoFASetup{}, err
	}
	defer tx.Rollback(ctx)
	var enabled bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM identity.two_fas WHERE user_id=$1 AND is_enabled AND deleted_at IS NULL)`, user.ID).Scan(&enabled); err != nil {
		return TwoFASetup{}, err
	}
	if enabled {
		return TwoFASetup{}, Fault{409, "AUTH_CONFLICT"}
	}
	key, err := GenerateTOTPSecret(user.Username)
	if err != nil {
		return TwoFASetup{}, err
	}
	codes, err := GenerateBackupCodes()
	if err != nil {
		return TwoFASetup{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity.two_fas(user_id,secret) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET secret=$2,is_enabled=false,failed_attempts=0,locked_until=NULL,last_used_at=NULL,deleted_at=NULL,updated_at=now()`, user.ID, key.Secret()); err != nil {
		return TwoFASetup{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM identity.two_fa_backup_codes WHERE user_id=$1`, user.ID); err != nil {
		return TwoFASetup{}, err
	}
	for _, code := range codes {
		hash, err := HashBackupCode(code)
		if err != nil {
			return TwoFASetup{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO identity.two_fa_backup_codes(user_id,code_hash) VALUES($1,$2)`, user.ID, hash); err != nil {
			return TwoFASetup{}, err
		}
	}
	return TwoFASetup{Secret: key.Secret(), QRCodeData: key.URL(), BackupCodes: codes}, tx.Commit(ctx)
}

func (s *Store) SetTwoFA(ctx context.Context, sessions Sessions, browser Session, code string, enable bool) (Session, error) {
	tx, user, err := s.accountTx(ctx, sessions, browser)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback(ctx)
	if enable {
		var secret string
		err := tx.QueryRow(ctx, `SELECT secret FROM identity.two_fas WHERE user_id=$1 AND NOT is_enabled AND deleted_at IS NULL`, user.ID).Scan(&secret)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && !ValidateTOTPCode(secret, code) {
			return Session{}, Fault{400, "MOMIAO_INVALID_REQUEST"}
		}
		if err != nil {
			return Session{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE identity.two_fas SET is_enabled=true,updated_at=now() WHERE user_id=$1`, user.ID); err != nil {
			return Session{}, err
		}
	} else {
		if err := authenticateSecondFactor(ctx, tx, user.ID, code); err != nil {
			if errors.Is(err, ErrCredentials) || errors.Is(err, ErrTwoFALocked) {
				if commitErr := tx.Commit(ctx); commitErr != nil {
					return Session{}, commitErr
				}
				return Session{}, Fault{400, "MOMIAO_INVALID_REQUEST"}
			}
			return Session{}, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM identity.two_fa_backup_codes WHERE user_id=$1`, user.ID); err != nil {
			return Session{}, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM identity.two_fas WHERE user_id=$1`, user.ID); err != nil {
			return Session{}, err
		}
	}
	return rotateAccountSession(ctx, tx, sessions, user.ID)
}

func (h *HTTP) accountWrite(w http.ResponseWriter, r *http.Request, browser Session) error {
	if browser.UserID == 0 {
		return ErrSessionExpired
	}
	if r.URL.Path == "/api/v1/account/2fa/setup" {
		setup, err := h.Store.SetupTwoFA(r.Context(), h.Sessions, browser)
		if err != nil {
			return err
		}
		writeSuccess(w, setup)
		return nil
	}
	var body struct {
		Password    string `json:"password"`
		OldPassword string `json:"old_password"`
		Code        string `json:"code"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		return Fault{400, "AUTH_INPUT_INVALID"}
	}
	var value Session
	var err error
	if r.URL.Path == "/api/v1/account/password/change" {
		length := utf8.RuneCountInString(body.Password)
		if length < 8 || length > 20 || len(body.Password) > 72 {
			return Fault{400, "MOMIAO_INVALID_REQUEST"}
		}
		value, err = h.Store.ChangePassword(r.Context(), h.Sessions, browser, body.OldPassword, body.Password)
	} else {
		value, err = h.Store.SetTwoFA(r.Context(), h.Sessions, browser, body.Code, r.URL.Path == "/api/v1/account/2fa/enable")
	}
	if err != nil {
		return err
	}
	WriteSessionCookie(w, value)
	writeSuccess(w, bootstrap(value))
	return nil
}
