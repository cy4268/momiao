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
	"time"
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

type PasswordProof struct {
	Token     string `json:"proof"`
	Purpose   string `json:"purpose"`
	ExpiresAt int64  `json:"expires_at"`
}

func passwordPurpose(user User) string {
	if user.PasswordHash == "" {
		return "PASSWORD_SET"
	}
	return "PASSWORD_RESET"
}

func passwordProofKey(token string) string { return sessionKey(token) + ":password" }
func passwordProofBinding(browser Session, discordID, purpose string) string {
	return sessionKey(browser.Token) + ":" + discordID + ":" + purpose
}

// momiao's password recovery uses the official OAuth exchange, then requires
// the already-bound Discord account. A proof cannot become a login cookie.
func (s *Store) DiscordPasswordProof(ctx context.Context, sessions Sessions, browser Session, discordID, purpose string) (PasswordProof, error) {
	tx, user, err := s.accountTx(ctx, sessions, browser)
	if err != nil {
		return PasswordProof{}, err
	}
	defer tx.Rollback(ctx)
	if discordID == "" || user.DiscordID != discordID || purpose != passwordPurpose(user) {
		return PasswordProof{}, Fault{403, "MOMIAO_AUTH_RESTART_REQUIRED"}
	}
	now := time.Now()
	expires := now.Add(5 * time.Minute)
	proof := PasswordProof{Token: sessionToken(), Purpose: purpose, ExpiresAt: expires.Unix()}
	err = sessions.Redis.Eval(ctx, createSession, []string{passwordProofKey(proof.Token), userSessionsKey(user.ID)},
		passwordProofBinding(browser, discordID, purpose), (5 * time.Minute).Milliseconds(), expires.UnixMilli(), now.UnixMilli(), sessionLifetime.Milliseconds()).Err()
	if err != nil {
		return PasswordProof{}, err
	}
	return proof, tx.Commit(ctx)
}

func (s *Store) UpdatePassword(ctx context.Context, sessions Sessions, browser Session, oldPassword, password, proof, purpose string) (Session, error) {
	tx, user, err := s.accountTx(ctx, sessions, browser)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback(ctx)
	if purpose == "" {
		if user.PasswordHash == "" || !ValidatePasswordAndHash(oldPassword, user.PasswordHash) {
			return Session{}, Fault{400, "MOMIAO_INVALID_REQUEST"}
		}
	} else {
		if user.DiscordID == "" || purpose != passwordPurpose(user) {
			return Session{}, Fault{409, "AUTH_CONFLICT"}
		}
		consumed, err := sessions.Redis.Eval(ctx, consumeBoundToken, []string{passwordProofKey(proof)}, passwordProofBinding(browser, user.DiscordID, purpose)).Int()
		if err != nil {
			return Session{}, err
		}
		if consumed != 1 {
			return Session{}, Fault{403, "MOMIAO_AUTH_RESTART_REQUIRED"}
		}
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
		Proof       string `json:"proof"`
		Code        string `json:"code"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		return Fault{400, "AUTH_INPUT_INVALID"}
	}
	var value Session
	var err error
	switch r.URL.Path {
	case "/api/v1/account/password/change", "/api/v1/account/password/set", "/api/v1/account/password/reset":
		length := utf8.RuneCountInString(body.Password)
		if length < 8 || length > 20 || len(body.Password) > 72 {
			return Fault{400, "MOMIAO_INVALID_REQUEST"}
		}
		purpose := ""
		if r.URL.Path == "/api/v1/account/password/set" {
			purpose = "PASSWORD_SET"
		} else if r.URL.Path == "/api/v1/account/password/reset" {
			purpose = "PASSWORD_RESET"
		}
		value, err = h.Store.UpdatePassword(r.Context(), h.Sessions, browser, body.OldPassword, body.Password, body.Proof, purpose)
	default:
		value, err = h.Store.SetTwoFA(r.Context(), h.Sessions, browser, body.Code, r.URL.Path == "/api/v1/account/2fa/enable")
	}
	if err != nil {
		return err
	}
	WriteSessionCookie(w, value)
	writeSuccess(w, bootstrap(value))
	return nil
}
