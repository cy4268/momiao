package poker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/rivo/uniseg"
)

func validPasswordCreate(c CreateTableCommand) bool {
	return c.AccessMode == "PASSWORD" && validTablePassword(c.Password) && c.Auth.UserID == c.UserID &&
		validAccessAuth(c.Auth) && validIdentityKey(c.UserID, c.Key) && utf8.ValidString(c.Name) && c.Name != "" &&
		strings.TrimSpace(c.Name) == c.Name && strings.IndexFunc(c.Name, unicode.IsControl) < 0 &&
		uniseg.GraphemeClusterCount(c.Name) <= 40 && c.MaxSeats >= 2 && c.MaxSeats <= 9
}

func (s *Service) createPasswordTable(ctx context.Context, c CreateTableCommand) (Receipt, error) {
	if !validPasswordCreate(c) {
		return Receipt{}, ErrInvalid
	}
	if !s.passwordEnabled() {
		return Receipt{}, ErrPasswordUnavailable
	}
	if _, err := s.passwordDeadline(ctx, c.Auth); err != nil {
		return Receipt{}, err
	}
	l := s.opts.PasswordLimits
	allowed, err := s.opts.TableAccess.AccessAttempt(ctx, c.UserID, "", l.OwnerAttempts, l.OwnerWindow, l.OwnerTableAttempts, l.OwnerTableWindow)
	if err != nil {
		return Receipt{}, ErrPasswordUnavailable
	}
	if !allowed {
		return Receipt{}, ErrRateLimited
	}
	prior, stored, found, err := s.readPasswordCreate(ctx, c)
	if err != nil {
		return Receipt{}, err
	}
	if found {
		return s.verifyPasswordCreateReplay(ctx, c, prior, stored)
	}
	phc, err := s.opts.Password.Hash(ctx, c.Password)
	if err != nil {
		return Receipt{}, err
	}
	if _, err = s.passwordDeadline(ctx, c.Auth); err != nil {
		return Receipt{}, err
	}
	r, raced, err := s.insertPasswordCreate(ctx, c, phc)
	if err != nil || !raced {
		return r, err
	}
	prior, stored, found, err = s.readPasswordCreate(ctx, c)
	if err != nil || !found {
		if err != nil {
			return Receipt{}, err
		}
		return Receipt{}, ErrPasswordUnavailable
	}
	return s.verifyPasswordCreateReplay(ctx, c, prior, stored)
}

func (s *Service) readPasswordCreate(ctx context.Context, c CreateTableCommand) (Receipt, string, bool, error) {
	tx, err := s.opts.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return Receipt{}, "", false, err
	}
	defer rollback(tx)
	r, found, err := findReceipt(ctx, tx, c.UserID, "poker.create.v1", c.Key, c)
	if err != nil || !found {
		return r, "", found, err
	}
	var mode, phc string
	if err = tx.QueryRow(ctx, `SELECT t.access_mode,c.password_phc FROM poker.tables t
		JOIN poker.table_access_credentials c USING(table_id) WHERE t.table_id=$1`, r.TableID).Scan(&mode, &phc); err != nil {
		return Receipt{}, "", false, err
	}
	if mode != "PASSWORD" || phc == "" || len(phc) > 512 {
		return Receipt{}, "", false, ErrPasswordUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return Receipt{}, "", false, err
	}
	return r, phc, true, nil
}

func (s *Service) verifyPasswordCreateReplay(ctx context.Context, c CreateTableCommand, prior Receipt, phc string) (Receipt, error) {
	matched, err := s.opts.Password.Verify(ctx, phc, c.Password)
	if err != nil {
		return Receipt{}, err
	}
	if !matched {
		return Receipt{}, ErrConflict
	}
	if _, err = s.passwordDeadline(ctx, c.Auth); err != nil {
		return Receipt{}, err
	}
	tx, err := s.opts.Pool.Begin(ctx)
	if err != nil {
		return Receipt{}, err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", c.UserID); err != nil {
		return Receipt{}, err
	}
	current, found, err := findReceipt(ctx, tx, c.UserID, "poker.create.v1", c.Key, c)
	if err != nil || !found || current != prior {
		if err != nil {
			return Receipt{}, err
		}
		return Receipt{}, ErrPasswordUnavailable
	}
	var mode, currentPHC string
	if err = tx.QueryRow(ctx, `SELECT t.access_mode,c.password_phc FROM poker.tables t
		JOIN poker.table_access_credentials c USING(table_id) WHERE t.table_id=$1`, current.TableID).Scan(&mode, &currentPHC); err != nil {
		return Receipt{}, err
	}
	if mode != "PASSWORD" || currentPHC != phc {
		return Receipt{}, ErrPasswordUnavailable
	}
	if _, err = s.passwordDeadline(ctx, c.Auth); err != nil {
		return Receipt{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Receipt{}, err
	}
	return current, nil
}

func (s *Service) insertPasswordCreate(ctx context.Context, c CreateTableCommand, phc string) (Receipt, bool, error) {
	tx, err := s.opts.Pool.Begin(ctx)
	if err != nil {
		return Receipt{}, false, err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", c.UserID); err != nil {
		return Receipt{}, false, err
	}
	if r, found, err := findReceipt(ctx, tx, c.UserID, "poker.create.v1", c.Key, c); err != nil || found {
		return r, found, err
	}
	if _, err = s.passwordDeadline(ctx, c.Auth); err != nil {
		return Receipt{}, false, err
	}
	if err = requireAdmission(ctx, tx, nil, c.UserID, c.BlindPreset); err != nil {
		return Receipt{}, false, err
	}
	id := uuid()
	_, err = tx.Exec(ctx, `INSERT INTO poker.tables(table_id,owner_newapi_user_id,name,max_seats,blind_preset_version,ruleset_version,allow_spectators,access_mode,chat_enabled) VALUES($1,$2,$3,$4,$5,$6,$7,'PASSWORD',$8)`, id, c.UserID, c.Name, c.MaxSeats, c.BlindPreset, approvedRulesetVersion, c.AllowSpectators, c.ChatEnabled)
	if err != nil {
		return Receipt{}, false, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO poker.table_access_credentials(table_id,password_phc) VALUES($1,$2)", id, phc); err != nil {
		return Receipt{}, false, err
	}
	for seat := 1; seat <= c.MaxSeats; seat++ {
		var seed [32]byte
		if _, err = io.ReadFull(rand.Reader, seed[:]); err != nil {
			return Receipt{}, false, err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO poker.seats(table_id,seat_no,next_client_seed_contribution) VALUES($1,$2,$3)", id, seat, "pcs1-"+hex.EncodeToString(seed[:])); err != nil {
			return Receipt{}, false, err
		}
	}
	r := Receipt{TableID: id, Status: "WAITING", Version: 1}
	if err = saveReceipt(ctx, tx, c.UserID, "poker.create.v1", c.Key, c, r); err != nil {
		return Receipt{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Receipt{}, false, err
	}
	s.publish(id, 1)
	return r, false, nil
}
