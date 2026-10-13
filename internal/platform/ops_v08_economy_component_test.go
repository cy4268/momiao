package platform

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// Reuse this reward regression on the isolated dev database. The original
// registration inbox/claim/job setup now uses one existing, unclaimed account
// inside a rolled-back transaction instead of a new fixture database/users.
func TestV08OpsRewardRetry(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires existing isolated dev database")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read dev DSN")
	}
	s, err := Open(t.Context(), strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal("open dev database")
	}
	defer s.Close()
	ctx := t.Context()
	service, err := NewOpsEconomyService(s)
	if err != nil {
		t.Fatal(err)
	}
	var admin, regular int64
	if err = s.pool.QueryRow(ctx, `SELECT min(id) FILTER(WHERE role>=10),min(id) FILTER(WHERE role<10) FROM identity.users WHERE status=1 AND deleted_at IS NULL`).Scan(&admin, &regular); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Overview(ctx, regular); !errors.Is(err, ErrCatalogForbidden) {
		t.Fatal("regular read", err)
	}
	overview, err := service.Overview(ctx, admin)
	if err != nil || overview.WalletAccounts == "0" {
		t.Fatal("local overview", err)
	}
	first, err := service.Ledger(ctx, admin, OpsLedgerQuery{Limit: 2})
	if err != nil || len(first.Items) != 2 || first.NextBefore == nil {
		t.Fatal("global ledger", err)
	}
	second, err := service.Ledger(ctx, admin, OpsLedgerQuery{Limit: 2, Before: *first.NextBefore, BeforeID: first.NextBeforeID})
	if err != nil || len(second.Items) != 2 || second.Items[0].ID == first.Items[1].ID {
		t.Fatal("ledger cursor", err)
	}
	sample := first.Items[0]
	filtered, err := service.Ledger(ctx, admin, OpsLedgerQuery{UserID: sample.UserID, Asset: sample.Asset, BizType: sample.BizType, Limit: 2})
	if err != nil || len(filtered.Items) == 0 {
		t.Fatal("ledger filters", err)
	}
	for _, v := range filtered.Items {
		if v.UserID != sample.UserID || v.Asset != sample.Asset || v.BizType != sample.BizType {
			t.Fatal("ledger filter leak")
		}
	}
	claims, err := service.RewardClaims(ctx, admin, OpsRewardClaimQuery{Program: "REGISTRATION", Status: "SUCCEEDED", Limit: 2})
	if err != nil || len(claims.Items) != 2 {
		t.Fatal("global claims", err)
	}
	var issued int64
	if err = s.pool.QueryRow(ctx, `SELECT amount_units FROM rewards.registration_issuances WHERE claim_id=$1`, claims.Items[0].ID).Scan(&issued); err != nil || issued != claims.Items[0].AmountUnits {
		t.Fatal("actual reward amount", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var subject, ordinal int64
	if err = tx.QueryRow(ctx, `SELECT min(u.id) FROM identity.users u WHERE NOT EXISTS(SELECT 1 FROM rewards.registration_grants g WHERE g.newapi_user_id=u.id) AND NOT EXISTS(SELECT 1 FROM identity.native_registration_inbox n WHERE n.native_user_id=u.id)`).Scan(&subject); err != nil {
		t.Fatal("existing unclaimed dev account", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO identity.account_refs(newapi_user_id) VALUES($1) ON CONFLICT DO NOTHING`, subject); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT coalesce(max(ordinal),0)+1 FROM identity.native_registration_inbox`).Scan(&ordinal); err != nil {
		t.Fatal(err)
	}
	id := announcementID(t)
	if _, err = tx.Exec(ctx, `INSERT INTO identity.native_registration_inbox(ordinal,operation_id,native_user_id,discord_subject,source,policy_version,native_created_at) VALUES($1,$2,$3,'900000000000000061','NEW_DISCORD_REGISTRATION','v1',clock_timestamp())`, ordinal, id, subject); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO rewards.registration_grants(claim_id,newapi_user_id,claim_kind,source_ordinal,biz_id) VALUES($1,$2,'INITIAL_GRANT_REGISTRATION',$3,'initial_grant:registration:'||$2::bigint::text)`, id, subject, ordinal); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO platform_meta.registration_grant_jobs(claim_id) VALUES($1)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE platform_meta.registration_grant_jobs SET next_attempt_at=clock_timestamp()+interval '1 hour' WHERE claim_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	var ledger, audit int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM economy.wallet_ledger),(SELECT count(*) FROM ops.audit_log)`).Scan(&ledger, &audit); err != nil {
		t.Fatal(err)
	}
	if _, err = retryRegistrationReward(ctx, tx, regular, id); !errors.Is(err, ErrCatalogForbidden) {
		t.Fatal("regular retry", err)
	}
	before, err := loadRegistrationRetry(ctx, tx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	after, err := retryRegistrationReward(ctx, tx, admin, id)
	if err != nil || after.ClaimID != id || after.UserID != subject || after.Attempts != before.Attempts || !after.NextAttempt.Before(before.NextAttempt) || after.ClaimStatus != "PENDING" {
		t.Fatal("pending retry", err, after)
	}
	if _, err = tx.Exec(ctx, `UPDATE rewards.registration_grants SET status='RECOVERING' WHERE claim_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE platform_meta.registration_grant_jobs SET status='RECOVERING',attempts=3,last_error_code='GRANT_RETRY_REQUIRED',next_attempt_at=clock_timestamp()+interval '1 hour' WHERE claim_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	// A local role-10 account needs no legacy principal or grant.
	if _, err = tx.Exec(ctx, `UPDATE identity.users SET role=10 WHERE id=$1`, regular); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		after, err = retryRegistrationReward(ctx, tx, regular, id)
		if err != nil || after.ClaimStatus != "RECOVERING" || after.JobStatus != "RECOVERING" || after.Attempts != 3 || after.LastError != "GRANT_RETRY_REQUIRED" || after.NextAttempt.After(time.Now().Add(time.Second)) {
			t.Fatal("recovering/repeated retry", err, after)
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE platform_meta.registration_grant_jobs SET status='DONE' WHERE claim_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = retryRegistrationReward(ctx, tx, admin, id); !errors.Is(err, ErrOpsConflict) {
		t.Fatal("done retry", err)
	}
	if _, err = retryRegistrationReward(ctx, tx, admin, claims.Items[0].ID); !errors.Is(err, ErrOpsConflict) {
		t.Fatal("confirmed retry", err)
	}
	if _, err = retryRegistrationReward(ctx, tx, admin, announcementID(t)); !errors.Is(err, ErrOpsNotFound) {
		t.Fatal("missing retry", err)
	}
	var newLedger, newAudit, issuances, goodAudit int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM economy.wallet_ledger),(SELECT count(*) FROM ops.audit_log),(SELECT count(*) FROM rewards.registration_issuances WHERE claim_id=$1),(SELECT count(*) FROM ops.audit_log WHERE action='REWARD_CLAIM_RETRY' AND target=$1::text AND before_state->>'claim_id'=$1::text AND after_state->>'claim_id'=$1::text AND actor_user_id IN ($2,$3))`, id, admin, regular).Scan(&newLedger, &newAudit, &issuances, &goodAudit); err != nil {
		t.Fatal(err)
	}
	if newLedger != ledger || issuances != 0 || newAudit != audit+3 || goodAudit != 3 {
		t.Fatal("retry must only schedule and audit", newLedger-ledger, issuances, newAudit-audit, goodAudit)
	}
	if _, err = tx.Exec(ctx, "SET CONSTRAINTS ALL IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM rewards.registration_grants WHERE claim_id=$1`, id).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("rolled-back claim", err, remaining)
	}
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM ops.audit_log`).Scan(&remaining); err != nil || remaining != audit {
		t.Fatal("rolled-back audit", err, remaining)
	}
	t.Log("local global ledger + filters/cursor; actual issuance amounts; retry pending/recovering/repeat = 3 audits, zero credits; roles, DONE/CONFIRMED/missing rejected; all edits rolled back")
}
