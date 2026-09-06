package platform

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const RegistrationGrantAmount int64 = 500000000

type AdmissionStatus struct {
	UserID          int64   `json:"user_id,string"`
	Source          string  `json:"source"`
	GrantStatus     string  `json:"grant_status"`
	AmountUnits     int64   `json:"amount_units,string"`
	TransactionID   *string `json:"transaction_id"`
	SourceAvailable bool    `json:"source_available"`
}

func (s *Store) EnsureProvisionalProfile(ctx context.Context, user int64) (Profile, error) {
	if user <= 0 {
		return Profile{}, ErrInvalidProfile
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Profile{}, err
	}
	defer rollback(tx)
	if err = lockIdentity(ctx, tx, "master-profile.initialize.v1", user); err != nil {
		return Profile{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO identity.account_refs(newapi_user_id) VALUES($1) ON CONFLICT DO NOTHING", user); err != nil {
		return Profile{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO identity.master_profiles(newapi_user_id,display_name,normalized_name,profile_version) VALUES($1,'',NULL,0) ON CONFLICT(newapi_user_id) DO NOTHING`, user); err != nil {
		return Profile{}, err
	}
	p, err := scanProfile(tx.QueryRow(ctx, "SELECT "+profileColumns+" FROM identity.master_profiles WHERE newapi_user_id=$1", user))
	if err != nil {
		return Profile{}, err
	}
	return p, tx.Commit(ctx)
}

func (s *Store) ReadAdmission(ctx context.Context, user int64) (AdmissionStatus, error) {
	out := AdmissionStatus{UserID: user}
	if user <= 0 {
		return out, ErrInvalidProfile
	}
	err := s.pool.QueryRow(ctx, `SELECT CASE WHEN g.claim_id IS NULL THEN 'UNVERIFIED' ELSE 'NEW_DISCORD_REGISTRATION' END,
 coalesce(g.status,'PENDING_SOURCE'),CASE WHEN g.claim_id IS NULL THEN 0 ELSE 500000000 END,g.transaction_id::text,
 coalesce(c.source_available AND c.last_success_at>clock_timestamp()-interval '1 minute',false)
 FROM platform_meta.registration_cursor c LEFT JOIN rewards.registration_grants g ON g.newapi_user_id=$1
 WHERE c.singleton`, user).Scan(&out.Source, &out.GrantStatus, &out.AmountUnits, &out.TransactionID, &out.SourceAvailable)
	return out, err
}
func (s *Store) RegistrationCursor(ctx context.Context) (int64, error) {
	var cursor int64
	err := s.pool.QueryRow(ctx, "SELECT ordinal FROM platform_meta.registration_cursor WHERE singleton").Scan(&cursor)
	return cursor, err
}
func (s *Store) MarkRegistrationSourceUnavailable(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, "UPDATE platform_meta.registration_cursor SET source_available=false,last_attempt_at=clock_timestamp() WHERE singleton")
	return err
}

// Receipt, account source, pending claim, job and cursor commit as one local unit.
// An old response can replay only its identical immutable facts, never skip work.
func (s *Store) IngestRegistrationPage(ctx context.Context, after int64, page RegistrationPage) error {
	if err := ValidateRegistrationPage(page, after, 100); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	var cursor int64
	if err = tx.QueryRow(ctx, "SELECT ordinal FROM platform_meta.registration_cursor WHERE singleton FOR UPDATE").Scan(&cursor); err != nil {
		return err
	}
	if after > cursor {
		return ErrRegistrationReceipt
	}
	for _, r := range page.Receipts {
		if r.Ordinal <= cursor {
			var old RegistrationReceipt
			err = tx.QueryRow(ctx, `SELECT ordinal,operation_id::text,native_user_id,discord_subject,source,policy_version,native_created_at FROM identity.native_registration_inbox WHERE ordinal=$1`, r.Ordinal).Scan(&old.Ordinal, &old.OperationID, &old.NativeUserID, &old.DiscordSubject, &old.Source, &old.PolicyVersion, &old.CreatedAt)
			if err != nil {
				return err
			}
			if old.Ordinal != r.Ordinal || old.OperationID != r.OperationID || old.NativeUserID != r.NativeUserID || old.DiscordSubject != r.DiscordSubject || old.Source != r.Source || old.PolicyVersion != r.PolicyVersion || !old.CreatedAt.Equal(r.CreatedAt) {
				return ErrRegistrationReceipt
			}
			continue
		}
		if r.Ordinal != cursor+1 {
			return ErrRegistrationReceipt
		}
		if _, err = tx.Exec(ctx, "INSERT INTO identity.account_refs(newapi_user_id) VALUES($1) ON CONFLICT DO NOTHING", r.NativeUserID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO identity.native_registration_inbox(ordinal,operation_id,native_user_id,discord_subject,source,policy_version,native_created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, r.Ordinal, r.OperationID, r.NativeUserID, r.DiscordSubject, r.Source, r.PolicyVersion, r.CreatedAt); err != nil {
			return err
		}
		id, e := uuidV7()
		if e != nil {
			return e
		}
		if _, err = tx.Exec(ctx, `INSERT INTO rewards.registration_grants(claim_id,newapi_user_id,claim_kind,source_ordinal,biz_id) VALUES($1,$2,'INITIAL_GRANT_REGISTRATION',$3,$4)`, id, r.NativeUserID, r.Ordinal, fmt.Sprintf("initial_grant:registration:%d", r.NativeUserID)); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO platform_meta.registration_grant_jobs(claim_id) VALUES($1)", id); err != nil {
			return err
		}
		cursor = r.Ordinal
	}
	if _, err = tx.Exec(ctx, "UPDATE platform_meta.registration_cursor SET ordinal=$1,source_available=true,last_attempt_at=clock_timestamp(),last_success_at=clock_timestamp() WHERE singleton", cursor); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Recovery uses a PostgreSQL row lock instead of an expiring external lease:
// the entire job is local SQL, so process/connection death releases the lock
// and rolls back every uncommitted effect. No network call occurs while locked.
func (s *Store) RecoverRegistrationGrant(ctx context.Context) (found bool, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer rollback(tx)
	var claim, biz, policy string
	var user int64
	err = tx.QueryRow(ctx, `SELECT g.claim_id::text,g.newapi_user_id,g.biz_id,n.policy_version
 FROM platform_meta.registration_grant_jobs j JOIN rewards.registration_grants g USING(claim_id)
 JOIN identity.native_registration_inbox n ON n.ordinal=g.source_ordinal
 WHERE j.status<>'DONE' AND g.status<>'CONFIRMED' AND j.next_attempt_at<=clock_timestamp()
 ORDER BY j.next_attempt_at,g.source_ordinal LIMIT 1 FOR UPDATE OF j,g SKIP LOCKED`).Scan(&claim, &user, &biz, &policy)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	found = true
	defer func() {
		if err == nil {
			return
		}
		rollback(tx)
		retryCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		retry, e := s.pool.Begin(retryCtx)
		if e != nil {
			return
		}
		defer rollback(retry)
		// A COMMIT response may be lost. Lock/read the original claim first; confirmed
		// jobs are left untouched, and will not be issued a second time.
		var state string
		if retry.QueryRow(retryCtx, "SELECT g.status FROM platform_meta.registration_grant_jobs j JOIN rewards.registration_grants g USING(claim_id) WHERE g.claim_id=$1 FOR UPDATE OF j,g", claim).Scan(&state) != nil || state == "CONFIRMED" {
			return
		}
		if _, e = retry.Exec(retryCtx, "UPDATE rewards.registration_grants SET status='RECOVERING' WHERE claim_id=$1", claim); e != nil {
			return
		}
		if _, e = retry.Exec(retryCtx, `UPDATE platform_meta.registration_grant_jobs SET status='RECOVERING',attempts=attempts+1,next_attempt_at=clock_timestamp()+interval '10 seconds',last_error_code='GRANT_RETRY_REQUIRED',updated_at=clock_timestamp() WHERE claim_id=$1`, claim); e != nil {
			return
		}
		_ = retry.Commit(retryCtx)
	}()
	if _, err = tx.Exec(ctx, `INSERT INTO economy.wallet_balances(newapi_user_id,asset_type) VALUES($1,'RESERVE_API_CREDIT'),($1,'AVAILABLE_CHIPS') ON CONFLICT DO NOTHING`, user); err != nil {
		return found, err
	}
	var entry LedgerEntry
	entry, err = applyInTx(ctx, tx, Mutation{UserID: user, Asset: ReserveAPICredit, DeltaUnits: RegistrationGrantAmount, BizType: "INITIAL_GRANT_REGISTRATION", BizID: biz, EntryType: "INITIAL_GRANT_REGISTRATION", IdempotencyKey: biz})
	if err != nil {
		return found, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO rewards.registration_issuances(claim_id,newapi_user_id,biz_id,direction,amount_units,asset_type,policy_version,transaction_id,ledger_entry_id) VALUES($1,$2,$3,'ISSUE',500000000,'RESERVE_API_CREDIT',$4,$5,$6)`, claim, user, biz, policy, entry.TransactionID, entry.ID); err != nil {
		return found, err
	}
	if _, err = tx.Exec(ctx, "UPDATE rewards.registration_grants SET status='CONFIRMED',transaction_id=$2,confirmed_at=clock_timestamp() WHERE claim_id=$1", claim, entry.TransactionID); err != nil {
		return found, err
	}
	if _, err = tx.Exec(ctx, "UPDATE platform_meta.registration_grant_jobs SET status='DONE',attempts=attempts+1,last_error_code=NULL,updated_at=clock_timestamp() WHERE claim_id=$1", claim); err != nil {
		return found, err
	}
	return found, tx.Commit(ctx)
}
