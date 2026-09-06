package platform

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestRegistrationAdmissionIntegration(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	base := time.Now().UnixMicro()
	scalar := func(query string, args ...any) int64 {
		t.Helper()
		var v int64
		if err := s.pool.QueryRow(ctx, query, args...).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	execute := func(query string, args ...any) {
		t.Helper()
		if _, err := s.pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("read_does_not_write_and_ensure_preserves_same_row", func(t *testing.T) {
		user := base + 1
		p, err := s.ReadProfile(ctx, user)
		if err != nil || p.Status != "INCOMPLETE" {
			t.Fatal("profile read", err)
		}
		status, err := s.ReadAdmission(ctx, user)
		if err != nil || status.Source != "UNVERIFIED" || status.GrantStatus != "PENDING_SOURCE" {
			t.Fatal("unverified source", err)
		}
		if scalar("SELECT count(*) FROM identity.account_refs WHERE newapi_user_id=$1", user) != 0 {
			t.Fatal("GET created account")
		}
		var wg sync.WaitGroup
		fail := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				p, e := s.EnsureProvisionalProfile(ctx, user)
				if e == nil && (p.ProfileVersion != 0 || p.DisplayName != "") {
					e = fmt.Errorf("not provisional")
				}
				fail <- e
			}()
		}
		wg.Wait()
		close(fail)
		for e := range fail {
			if e != nil {
				t.Fatal(e)
			}
		}
		if scalar("SELECT count(*) FROM identity.master_profiles WHERE newapi_user_id=$1 AND profile_version=0 AND normalized_name IS NULL", user) != 1 {
			t.Fatal("no durable provisional row")
		}
		var created time.Time
		if err := s.pool.QueryRow(ctx, "SELECT created_at FROM identity.master_profiles WHERE newapi_user_id=$1", user).Scan(&created); err != nil {
			t.Fatal(err)
		}
		p, err = s.InitializeProfile(ctx, user, 0, fmt.Sprintf("Master-%d", user), "system-default")
		if err != nil || p.ProfileVersion != 1 {
			t.Fatal("initialize", err)
		}
		p, err = s.EnsureProvisionalProfile(ctx, user)
		if err != nil || p.ProfileVersion != 1 || p.Status != "COMPLETE" {
			t.Fatal("ensure overwrote complete", err)
		}
		var after time.Time
		_ = s.pool.QueryRow(ctx, "SELECT created_at FROM identity.master_profiles WHERE newapi_user_id=$1", user).Scan(&after)
		if !after.Equal(created) || scalar("SELECT count(*) FROM identity.master_profile_name_history WHERE newapi_user_id=$1", user) != 1 {
			t.Fatal("initialization replaced row/history")
		}
		if scalar("SELECT count(*) FROM rewards.registration_grants WHERE newapi_user_id=$1", user) != 0 {
			t.Fatal("profile invented source")
		}
	})
	cursor, err := s.RegistrationCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt := func(ordinal, user int64) RegistrationReceipt {
		return RegistrationReceipt{ordinal, fmt.Sprintf("42684268-0000-4000-8000-%012x", ordinal), user, fmt.Sprint(900000000000000000 + user), "NEW_DISCORD_REGISTRATION", "policy-v1", time.Now().UTC().Truncate(time.Microsecond)}
	}
	ingest := func(r RegistrationReceipt) {
		t.Helper()
		if err := s.IngestRegistrationPage(ctx, r.Ordinal-1, RegistrationPage{[]RegistrationReceipt{r}, r.Ordinal}); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("atomic_source_cursor_and_duplicate_replay", func(t *testing.T) {
		r := receipt(cursor+1, base+2)
		ingest(r)
		ingest(r)
		if scalar("SELECT count(*) FROM identity.native_registration_inbox WHERE native_user_id=$1", r.NativeUserID) != 1 || scalar("SELECT count(*) FROM rewards.registration_grants WHERE newapi_user_id=$1", r.NativeUserID) != 1 || scalar("SELECT count(*) FROM platform_meta.registration_grant_jobs j JOIN rewards.registration_grants g USING(claim_id) WHERE g.newapi_user_id=$1", r.NativeUserID) != 1 {
			t.Fatal("not exactly one inbox/claim/job")
		}
		bad := r
		bad.PolicyVersion = "conflict"
		if s.IngestRegistrationPage(ctx, cursor, RegistrationPage{[]RegistrationReceipt{bad}, bad.Ordinal}) == nil {
			t.Fatal("accepted conflicting replay")
		}
		if _, err := s.pool.Exec(ctx, "UPDATE identity.native_registration_inbox SET policy_version='x' WHERE native_user_id=$1", r.NativeUserID); err == nil {
			t.Fatal("source mutable")
		}
		status, err := s.ReadAdmission(ctx, r.NativeUserID)
		if err != nil || status.GrantStatus != "PENDING" || status.AmountUnits != RegistrationGrantAmount {
			t.Fatal("claim not pending", err)
		}
		cursor++
	})
	t.Run("bad_page_rolls_back_cursor_and_all_rows", func(t *testing.T) {
		a, b := receipt(cursor+1, base+3), receipt(cursor+2, base+4)
		b.DiscordSubject = a.DiscordSubject
		if s.IngestRegistrationPage(ctx, cursor, RegistrationPage{[]RegistrationReceipt{a, b}, b.Ordinal}) == nil {
			t.Fatal("source uniqueness did not abort page")
		}
		got, _ := s.RegistrationCursor(ctx)
		if got != cursor || scalar("SELECT count(*) FROM identity.account_refs WHERE newapi_user_id IN ($1,$2)", a.NativeUserID, b.NativeUserID) != 0 {
			t.Fatal("failed page partially committed")
		}
		gap := receipt(cursor+2, base+4)
		if s.IngestRegistrationPage(ctx, cursor, RegistrationPage{[]RegistrationReceipt{gap}, gap.Ordinal}) == nil {
			t.Fatal("cursor skipped missing receipt")
		}
	})
	t.Run("concurrent_recovery_confirms_full_issuance_once", func(t *testing.T) {
		var wg sync.WaitGroup
		fail := make(chan error, 12)
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, e := s.RecoverRegistrationGrant(ctx); fail <- e }()
		}
		wg.Wait()
		close(fail)
		for e := range fail {
			if e != nil {
				t.Fatal(e)
			}
		}
		user := base + 2
		w, err := s.ReadWallet(ctx, user, ReserveAPICredit)
		if err != nil || w.BalanceUnits != 500000000 || w.LedgerSeq != 1 {
			t.Fatal("wrong one-time grant", err)
		}
		state, err := s.ReadAdmission(ctx, user)
		if err != nil || state.GrantStatus != "CONFIRMED" || state.TransactionID == nil {
			t.Fatal("not confirmed", err)
		}
		if scalar("SELECT count(*) FROM rewards.registration_issuances WHERE newapi_user_id=$1 AND direction='ISSUE' AND amount_units=500000000 AND asset_type='RESERVE_API_CREDIT'", user) != 1 {
			t.Fatal("issuance incomplete")
		}
		if scalar("SELECT count(*) FROM economy.wallet_ledger WHERE newapi_user_id=$1 AND biz_id=$2", user, fmt.Sprintf("initial_grant:registration:%d", user)) != 1 {
			t.Fatal("ledger duplicated")
		}
		for _, q := range []string{"UPDATE rewards.registration_issuances SET amount_units=amount_units WHERE newapi_user_id=$1", "DELETE FROM rewards.registration_issuances WHERE newapi_user_id=$1", "UPDATE rewards.registration_grants SET status='PENDING',transaction_id=NULL WHERE newapi_user_id=$1"} {
			if _, err := s.pool.Exec(ctx, q, user); err == nil {
				t.Fatal("confirmed fact was mutable")
			}
		}
		rows, err := s.Transactions(ctx, user, "")
		if err != nil || len(rows) != 1 || rows[0].Kind != "INITIAL_GRANT_REGISTRATION" {
			t.Fatal("grant missing from history", err)
		}
	})
	t.Run("failed_issuance_has_no_partial_economy_and_restart_recovers", func(t *testing.T) {
		user := base + 5
		r := receipt(cursor+1, user)
		ingest(r)
		cursor++
		execute(`CREATE FUNCTION rewards.m2b_fail_issuance() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic failure'; END $$; CREATE TRIGGER m2b_fail_issuance BEFORE INSERT ON rewards.registration_issuances FOR EACH ROW EXECUTE FUNCTION rewards.m2b_fail_issuance()`)
		found, err := s.RecoverRegistrationGrant(ctx)
		if !found || err == nil {
			t.Fatal("failure not injected")
		}
		execute("DROP TRIGGER m2b_fail_issuance ON rewards.registration_issuances; DROP FUNCTION rewards.m2b_fail_issuance()")
		if scalar("SELECT count(*) FROM economy.wallet_ledger WHERE newapi_user_id=$1", user) != 0 || scalar("SELECT count(*) FROM economy.asset_transactions WHERE newapi_user_id=$1", user) != 0 || scalar("SELECT count(*) FROM rewards.registration_issuances WHERE newapi_user_id=$1", user) != 0 {
			t.Fatal("partial grant escaped rollback")
		}
		status, err := s.ReadAdmission(ctx, user)
		if err != nil || status.GrantStatus != "RECOVERING" {
			t.Fatal("failed job lost recovery state", err)
		}
		execute("UPDATE platform_meta.registration_grant_jobs SET next_attempt_at=clock_timestamp() WHERE claim_id=(SELECT claim_id FROM rewards.registration_grants WHERE newapi_user_id=$1)", user)
		fresh := integrationStore(t)
		found, err = fresh.RecoverRegistrationGrant(ctx)
		if err != nil || !found {
			t.Fatal("restart recovery failed", err)
		}
		found, err = fresh.RecoverRegistrationGrant(ctx)
		if err != nil || found {
			t.Fatal("confirmed job repeated", err)
		}
		w, err := fresh.ReadWallet(ctx, user, ReserveAPICredit)
		if err != nil || w.BalanceUnits != RegistrationGrantAmount {
			t.Fatal("restart amount", err)
		}
	})
	t.Run("source_outage_preserves_cursor_and_legacy_account", func(t *testing.T) {
		if err := s.MarkRegistrationSourceUnavailable(ctx); err != nil {
			t.Fatal(err)
		}
		got, _ := s.RegistrationCursor(ctx)
		if got != cursor {
			t.Fatal("outage moved cursor")
		}
		status, err := s.ReadAdmission(ctx, base+1)
		if err != nil || status.SourceAvailable || status.GrantStatus != "PENDING_SOURCE" {
			t.Fatal("legacy invented grant", err)
		}
	})
}
