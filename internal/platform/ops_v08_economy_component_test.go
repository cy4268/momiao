package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"
)

// Every test below uses bootstrapStore's one-shot PostgreSQL database. It never
// connects to the accepted runtime, Native, Redis, or Poker.
func v08EconomyOpsFixture(t *testing.T) (*Store, *OpsService, OpsPrincipal) {
	t.Helper()
	s := bootstrapStore(t)
	if _, err := s.Bootstrap(context.Background(), bootstrapInput()); err != nil {
		t.Fatal(err)
	}
	economy, err := NewOpsEconomyService(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewOpsService(s, "STAGING", economy.OpsBindings()...)
	if err != nil {
		t.Fatal(err)
	}
	boot, err := service.Bootstrap(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	return s, service, boot.Principal
}

func v08EconomyRequest(actor OpsPrincipal, operationID, operationType, targetType, targetID, version, input string) OpsPrepareRequest {
	return OpsPrepareRequest{
		OperationID: operationID, OperationType: operationType, AuthzEpoch: actor.Epoch,
		InputSchemaVersion: map[string]string{
			"ECONOMY_ADJUSTMENT":         "economy-adjustment.v1",
			"ECONOMY_TRANSFER_RECONCILE": "economy-transfer-reconcile.v1",
			"REWARD_CLAIM_RETRY":         "reward-claim-retry.v1",
		}[operationType],
		Target: OpsTarget{Type: targetType, ID: targetID, ExpectedVersion: version},
		Input:  json.RawMessage(input), Reason: "V08 isolated economy component acceptance",
	}
}

func v08EconomyPrepare(t *testing.T, service *OpsService, actor OpsPrincipal, operationType, targetType, targetID, version, input string) OpsPrepared {
	t.Helper()
	prepared, err := service.Prepare(context.Background(), actor.UserID,
		v08EconomyRequest(actor, announcementID(t), operationType, targetType, targetID, version, input))
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func v08EconomyAssertAbsent(t *testing.T, service *OpsService, actor OpsPrincipal, operationID string) {
	t.Helper()
	if _, err := service.Operation(context.Background(), actor.UserID, actor.Epoch, operationID); !errors.Is(err, ErrOpsNotFound) {
		t.Fatalf("rejected operation %s persisted: %v", operationID, err)
	}
}

func v08EconomyAssertTimeline(t *testing.T, view OpsOperationView, states ...string) {
	t.Helper()
	if view.Operation.State != "SUCCEEDED" || view.Operation.StateVersion != int64(len(states)) ||
		view.AuditID == "" || len(view.Timeline) != len(states) {
		t.Fatalf("terminal receipt state=%s/%d timeline=%d audit=%q", view.Operation.State,
			view.Operation.StateVersion, len(view.Timeline), view.AuditID)
	}
	for i, state := range states {
		if view.Timeline[i].State != state || view.Timeline[i].StateVersion != int64(i+1) {
			t.Fatalf("timeline[%d]=%s/%d, want %s/%d", i, view.Timeline[i].State,
				view.Timeline[i].StateVersion, state, i+1)
		}
	}
}

func TestV08OpsEconomyAdjustment(t *testing.T) {
	s, service, actor := v08EconomyOpsFixture(t)
	ctx := context.Background()
	const user = int64(51)
	if err := s.EnsureAccount(ctx, user); err != nil {
		t.Fatal(err)
	}

	negativeID := announcementID(t)
	_, err := service.Prepare(ctx, actor.UserID, v08EconomyRequest(actor, negativeID,
		"ECONOMY_ADJUSTMENT", "WALLET", "51", "1",
		`{"asset":"RESERVE_API_CREDIT","delta_units":"-1","reference":"reject negative balance"}`))
	if !errors.Is(err, ErrOpsConflict) {
		t.Fatalf("negative balance prepare=%v, want ErrOpsConflict", err)
	}
	v08EconomyAssertAbsent(t, service, actor, negativeID)

	issue := v08EconomyPrepare(t, service, actor, "ECONOMY_ADJUSTMENT", "WALLET", "51", "1",
		`{"asset":"RESERVE_API_CREDIT","delta_units":"1","reference":"V08 one-unit issue"}`)
	issueRequest := v08Fresh(t, service, actor, issue)
	issued, err := service.Execute(ctx, actor.UserID, issue.Operation.OperationID, issueRequest)
	if err != nil {
		t.Fatal(err)
	}
	v08EconomyAssertTimeline(t, issued, "PREPARED", "PREPARED", "AUTHORIZED", "EXECUTING", "SUCCEEDED")
	wallet, err := s.ReadWallet(ctx, user, ReserveAPICredit)
	if err != nil || wallet.BalanceUnits != 1 || wallet.LedgerSeq != 1 || wallet.Version != 2 {
		t.Fatalf("issued wallet=%+v error=%v", wallet, err)
	}

	staleID := announcementID(t)
	_, err = service.Prepare(ctx, actor.UserID, v08EconomyRequest(actor, staleID,
		"ECONOMY_ADJUSTMENT", "WALLET", "51", "1",
		`{"asset":"RESERVE_API_CREDIT","delta_units":"-1","reference":"reject stale version"}`))
	if !errors.Is(err, ErrOpsPreviewStale) {
		t.Fatalf("stale prepare=%v, want ErrOpsPreviewStale", err)
	}
	v08EconomyAssertAbsent(t, service, actor, staleID)

	burn := v08EconomyPrepare(t, service, actor, "ECONOMY_ADJUSTMENT", "WALLET", "51", "2",
		`{"asset":"RESERVE_API_CREDIT","delta_units":"-1","reference":"V08 compensate one-unit issue"}`)
	burnRequest := v08Fresh(t, service, actor, burn)
	burned, err := service.Execute(ctx, actor.UserID, burn.Operation.OperationID, burnRequest)
	if err != nil {
		t.Fatal(err)
	}
	v08EconomyAssertTimeline(t, burned, "PREPARED", "PREPARED", "AUTHORIZED", "EXECUTING", "SUCCEEDED")
	wallet, err = s.ReadWallet(ctx, user, ReserveAPICredit)
	if err != nil || wallet.BalanceUnits != 0 || wallet.LedgerSeq != 2 || wallet.Version != 3 {
		t.Fatalf("compensated wallet=%+v error=%v", wallet, err)
	}
	entries, err := s.Ledger(ctx, user, ReserveAPICredit, 0, 10)
	if err != nil || len(entries) != 2 {
		t.Fatalf("ledger rows=%d error=%v", len(entries), err)
	}
	want := []struct {
		entryType, operationID string
		delta, before, after   int64
	}{
		{"ADMIN_ADJUSTMENT_ISSUE", issue.Operation.OperationID, 1, 0, 1},
		{"ADMIN_ADJUSTMENT_BURN", burn.Operation.OperationID, -1, 1, 0},
	}
	for i := range want {
		got := entries[i]
		if got.EntryType != want[i].entryType || got.BizType != "ADMIN_ADJUSTMENT_V1" ||
			got.BizID != want[i].operationID || got.DeltaUnits != want[i].delta ||
			got.BalanceBeforeUnits != want[i].before || got.BalanceAfterUnits != want[i].after ||
			got.LedgerSeq != int64(i+1) || got.WalletVersion != int64(i+2) {
			t.Fatalf("ledger[%d]=%+v", i, got)
		}
	}
	var transactions, idempotency, audits, events int
	if err = s.pool.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM economy.asset_transactions WHERE newapi_user_id=$1),
	 (SELECT count(*) FROM platform_meta.mutation_idempotency_records WHERE newapi_user_id=$1),
	 (SELECT count(*) FROM audit.audit_events WHERE operation_id IN ($2,$3)),
	 (SELECT count(*) FROM ops.admin_operation_events WHERE operation_id IN ($2,$3))`,
		user, issue.Operation.OperationID, burn.Operation.OperationID).
		Scan(&transactions, &idempotency, &audits, &events); err != nil {
		t.Fatal(err)
	}
	if transactions != 2 || idempotency != 2 || audits != 2 || events != 10 {
		t.Fatalf("effects transactions=%d idempotency=%d audits=%d events=%d",
			transactions, idempotency, audits, events)
	}
	if _, err = service.Execute(ctx, actor.UserID, issue.Operation.OperationID, issueRequest); !errors.Is(err, ErrOpsConflict) {
		t.Fatalf("terminal issue replay=%v, want ErrOpsConflict", err)
	}

	if _, err = s.pool.Exec(ctx, `UPDATE economy.wallet_balances SET balance_units=$2
	 WHERE newapi_user_id=$1 AND asset_type='RESERVE_API_CREDIT'`, user, int64(math.MaxInt64)); err != nil {
		t.Fatal(err)
	}
	overflowID := announcementID(t)
	_, err = service.Prepare(ctx, actor.UserID, v08EconomyRequest(actor, overflowID,
		"ECONOMY_ADJUSTMENT", "WALLET", "51", "3",
		`{"asset":"RESERVE_API_CREDIT","delta_units":"1","reference":"reject overflow"}`))
	if !errors.Is(err, ErrOpsConflict) {
		t.Fatalf("overflow prepare=%v, want ErrOpsConflict", err)
	}
	v08EconomyAssertAbsent(t, service, actor, overflowID)
}

type v08TransferState struct {
	fingerprint, status, reason, nativeBefore, nativeAfter string
	updatedAt                                              time.Time
	version                                                string
}

func v08ReadTransfer(t *testing.T, s *Store, id string) v08TransferState {
	t.Helper()
	var out v08TransferState
	err := s.pool.QueryRow(context.Background(), `SELECT jsonb_build_object(
	 'id',transfer_id::text,'user',newapi_user_id::text,'request_key_hash',encode(request_key_hash,'hex'),
	 'amount_units',amount_units::text,'created_at',created_at)::text,
	 status,reason,coalesce(native_before::text,''),coalesce(native_after::text,''),updated_at
	 FROM economy.quota_transfers WHERE transfer_id=$1`, id).Scan(&out.fingerprint, &out.status,
		&out.reason, &out.nativeBefore, &out.nativeAfter, &out.updatedAt)
	if err != nil {
		t.Fatal(err)
	}
	out.updatedAt = out.updatedAt.UTC()
	out.version = opsTimeVersion(out.updatedAt)
	return out
}

func v08SeedTransfer(t *testing.T, s *Store, user int64, status string, at time.Time) string {
	t.Helper()
	ctx := context.Background()
	if err := s.EnsureAccount(ctx, user); err != nil {
		t.Fatal(err)
	}
	id := announcementID(t)
	_, err := s.pool.Exec(ctx, `INSERT INTO economy.quota_transfers(
	 transfer_id,newapi_user_id,request_key_hash,amount_units,status,reason,native_before,native_after,created_at,updated_at)
	 VALUES($1,$2,decode(repeat('ab',32),'hex'),7,$3,$3,
	 CASE WHEN $3='CONFIRMED' THEN 100::bigint ELSE NULL END,
	 CASE WHEN $3='CONFIRMED' THEN 107::bigint ELSE NULL END,$4,$4)`, id, user, status, at)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestV08OpsEconomyTransferReconcile(t *testing.T) {
	s, service, actor := v08EconomyOpsFixture(t)
	ctx := context.Background()
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	pendingID := v08SeedTransfer(t, s, 52, "PENDING", at)
	confirmedID := v08SeedTransfer(t, s, 53, "CONFIRMED", at.Add(time.Second))
	refundedID := v08SeedTransfer(t, s, 54, "REFUNDED", at.Add(2*time.Second))
	initial := v08ReadTransfer(t, s, pendingID)

	mark := v08EconomyPrepare(t, service, actor, "ECONOMY_TRANSFER_RECONCILE", "QUOTA_TRANSFER",
		pendingID, initial.version, `{"action":"MARK_FOR_REVIEW"}`)
	markRequest := v08Fresh(t, service, actor, mark)
	marked, err := service.Execute(ctx, actor.UserID, mark.Operation.OperationID, markRequest)
	if err != nil {
		t.Fatal(err)
	}
	v08EconomyAssertTimeline(t, marked, "PREPARED", "PREPARED", "AUTHORIZED", "EXECUTING", "SUCCEEDED")
	afterMark := v08ReadTransfer(t, s, pendingID)
	if afterMark.fingerprint != initial.fingerprint || afterMark.status != "NEEDS_REVIEW" ||
		afterMark.reason != "ADMIN_MARKED_FOR_REVIEW" || afterMark.version == initial.version {
		t.Fatalf("mark result=%+v initial=%+v", afterMark, initial)
	}

	staleID := announcementID(t)
	_, err = service.Prepare(ctx, actor.UserID, v08EconomyRequest(actor, staleID,
		"ECONOMY_TRANSFER_RECONCILE", "QUOTA_TRANSFER", pendingID, initial.version, `{"action":"RESUME"}`))
	if !errors.Is(err, ErrOpsPreviewStale) {
		t.Fatalf("stale reconcile=%v, want ErrOpsPreviewStale", err)
	}
	v08EconomyAssertAbsent(t, service, actor, staleID)

	resume := v08EconomyPrepare(t, service, actor, "ECONOMY_TRANSFER_RECONCILE", "QUOTA_TRANSFER",
		pendingID, afterMark.version, `{"action":"RESUME"}`)
	resumeRequest := v08Fresh(t, service, actor, resume)
	resumed, err := service.Execute(ctx, actor.UserID, resume.Operation.OperationID, resumeRequest)
	if err != nil {
		t.Fatal(err)
	}
	v08EconomyAssertTimeline(t, resumed, "PREPARED", "PREPARED", "AUTHORIZED", "EXECUTING", "SUCCEEDED")
	afterResume := v08ReadTransfer(t, s, pendingID)
	if afterResume.fingerprint != initial.fingerprint || afterResume.status != "PENDING" ||
		afterResume.reason != "ADMIN_RESUME" || afterResume.nativeBefore != "" || afterResume.nativeAfter != "" ||
		afterResume.version == afterMark.version {
		t.Fatalf("resume result=%+v initial=%+v", afterResume, initial)
	}
	if _, err = service.Execute(ctx, actor.UserID, mark.Operation.OperationID, markRequest); !errors.Is(err, ErrOpsConflict) {
		t.Fatalf("terminal mark replay=%v, want ErrOpsConflict", err)
	}

	for _, target := range []string{confirmedID, refundedID} {
		state := v08ReadTransfer(t, s, target)
		rejectedID := announcementID(t)
		_, err = service.Prepare(ctx, actor.UserID, v08EconomyRequest(actor, rejectedID,
			"ECONOMY_TRANSFER_RECONCILE", "QUOTA_TRANSFER", target, state.version, `{"action":"MARK_FOR_REVIEW"}`))
		if !errors.Is(err, ErrOpsConflict) {
			t.Fatalf("terminal transfer %s reconcile=%v, want ErrOpsConflict", target, err)
		}
		v08EconomyAssertAbsent(t, service, actor, rejectedID)
	}
	var operations, audits, events int
	if err = s.pool.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM ops.admin_operations WHERE operation_id IN ($1,$2)),
	 (SELECT count(*) FROM audit.audit_events WHERE operation_id IN ($1,$2)),
	 (SELECT count(*) FROM ops.admin_operation_events WHERE operation_id IN ($1,$2))`,
		mark.Operation.OperationID, resume.Operation.OperationID).Scan(&operations, &audits, &events); err != nil {
		t.Fatal(err)
	}
	if operations != 2 || audits != 2 || events != 10 {
		t.Fatalf("reconcile effects operations=%d audits=%d events=%d", operations, audits, events)
	}
}

type v08RewardState struct {
	claimID, fingerprint, version string
	nextAttempt, updatedAt        time.Time
}

func v08ReadReward(t *testing.T, s *Store, user int64) v08RewardState {
	t.Helper()
	var out v08RewardState
	err := s.pool.QueryRow(context.Background(), `SELECT g.claim_id::text,jsonb_build_object(
	 'claim_id',g.claim_id::text,'user',g.newapi_user_id::text,'claim_kind',g.claim_kind,
	 'source_ordinal',g.source_ordinal::text,'business_id',g.biz_id,'claim_status',g.status,
	 'transaction_id',g.transaction_id::text,'created_at',g.created_at,'confirmed_at',g.confirmed_at,
	 'policy_version',n.policy_version,'job_status',j.status,'attempts',j.attempts::text,
	 'last_error_code',coalesce(j.last_error_code,''),'issuances',
	 (SELECT count(*)::text FROM rewards.registration_issuances i WHERE i.claim_id=g.claim_id))::text,
	 j.next_attempt_at,j.updated_at
	 FROM rewards.registration_grants g
	 JOIN identity.native_registration_inbox n ON n.ordinal=g.source_ordinal
	 JOIN platform_meta.registration_grant_jobs j USING(claim_id)
	 WHERE g.newapi_user_id=$1`, user).Scan(&out.claimID, &out.fingerprint, &out.nextAttempt, &out.updatedAt)
	if err != nil {
		t.Fatal(err)
	}
	out.nextAttempt, out.updatedAt = out.nextAttempt.UTC(), out.updatedAt.UTC()
	out.version = opsTimeVersion(out.updatedAt)
	return out
}

func v08SeedRegistrationClaims(t *testing.T, s *Store) {
	t.Helper()
	created := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	receipts := make([]RegistrationReceipt, 0, 4)
	for i := int64(1); i <= 4; i++ {
		receipts = append(receipts, RegistrationReceipt{
			Ordinal: i, OperationID: fmt.Sprintf("8208000%d-0000-4000-8000-%012d", i, i),
			NativeUserID: 60 + i, DiscordSubject: fmt.Sprintf("9000000000000000%d", i),
			Source: "NEW_DISCORD_REGISTRATION", PolicyVersion: "fixture-v1", CreatedAt: created.Add(time.Duration(i) * time.Second),
		})
	}
	if err := s.IngestRegistrationPage(context.Background(), 0, RegistrationPage{Receipts: receipts, NextCursor: 4}); err != nil {
		t.Fatal(err)
	}
}

func TestV08OpsRewardRetry(t *testing.T) {
	s, service, actor := v08EconomyOpsFixture(t)
	ctx := context.Background()
	v08SeedRegistrationClaims(t, s)
	old := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)
	future := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	if _, err := s.pool.Exec(ctx, `UPDATE platform_meta.registration_grant_jobs j
	 SET attempts=2,last_error_code='GRANT_RETRY_REQUIRED',next_attempt_at=$2,updated_at=$3
	 FROM rewards.registration_grants g WHERE j.claim_id=g.claim_id AND g.newapi_user_id=$1`, int64(61), future, old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE rewards.registration_grants SET status='RECOVERING' WHERE newapi_user_id=62`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE platform_meta.registration_grant_jobs j
	 SET status='RECOVERING',attempts=3,last_error_code='GRANT_RETRY_REQUIRED',next_attempt_at=$2,updated_at=$3
	 FROM rewards.registration_grants g WHERE j.claim_id=g.claim_id AND g.newapi_user_id=$1`, int64(62), future, old.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE platform_meta.registration_grant_jobs j SET status='DONE'
	 FROM rewards.registration_grants g WHERE j.claim_id=g.claim_id AND g.newapi_user_id=63`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE platform_meta.registration_grant_jobs j
	 SET next_attempt_at=clock_timestamp()-interval '1 minute'
	 FROM rewards.registration_grants g WHERE j.claim_id=g.claim_id AND g.newapi_user_id=64`); err != nil {
		t.Fatal(err)
	}
	if found, err := s.RecoverRegistrationGrant(ctx); err != nil || !found {
		t.Fatalf("confirmed control fixture found=%v error=%v", found, err)
	}
	confirmed := v08ReadReward(t, s, 64)
	done := v08ReadReward(t, s, 63)

	successful := make([]string, 0, 2)
	for _, user := range []int64{61, 62} {
		before := v08ReadReward(t, s, user)
		prepared := v08EconomyPrepare(t, service, actor, "REWARD_CLAIM_RETRY", "REWARD_CLAIM",
			before.claimID, before.version, `{"action":"RETRY"}`)
		view, err := service.Execute(ctx, actor.UserID, prepared.Operation.OperationID, OpsExecuteRequest{
			AuthzEpoch: actor.Epoch, ImpactHash: prepared.Operation.ImpactHash, Confirmed: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		v08EconomyAssertTimeline(t, view, "PREPARED", "AUTHORIZED", "EXECUTING", "SUCCEEDED")
		after := v08ReadReward(t, s, user)
		if after.fingerprint != before.fingerprint || !after.updatedAt.After(before.updatedAt) ||
			!after.nextAttempt.After(before.updatedAt) || after.version == before.version {
			t.Fatalf("retry changed immutable claim/job facts user=%d before=%+v after=%+v", user, before, after)
		}
		successful = append(successful, prepared.Operation.OperationID)
		if user == 61 {
			staleID := announcementID(t)
			_, err = service.Prepare(ctx, actor.UserID, v08EconomyRequest(actor, staleID,
				"REWARD_CLAIM_RETRY", "REWARD_CLAIM", before.claimID, before.version, `{"action":"RETRY"}`))
			if !errors.Is(err, ErrOpsPreviewStale) {
				t.Fatalf("stale reward retry=%v, want ErrOpsPreviewStale", err)
			}
			v08EconomyAssertAbsent(t, service, actor, staleID)
		}
	}

	for label, state := range map[string]v08RewardState{"confirmed": confirmed, "done": done} {
		rejectedID := announcementID(t)
		_, err := service.Prepare(ctx, actor.UserID, v08EconomyRequest(actor, rejectedID,
			"REWARD_CLAIM_RETRY", "REWARD_CLAIM", state.claimID, state.version, `{"action":"RETRY"}`))
		if !errors.Is(err, ErrOpsConflict) {
			t.Fatalf("%s reward retry=%v, want ErrOpsConflict", label, err)
		}
		v08EconomyAssertAbsent(t, service, actor, rejectedID)
	}

	if err := s.EnsureAccount(ctx, 65); err != nil {
		t.Fatal(err)
	}
	daily, err := s.Apply(ctx, Mutation{UserID: 65, Asset: ReserveAPICredit, DeltaUnits: DailyAmount,
		BizType: "DAILY_CHECKIN_V1", BizID: "v08-daily-non-registration", EntryType: "DAILY_CHECKIN",
		IdempotencyKey: "v08-daily-non-registration"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `INSERT INTO rewards.daily_checkins(
	 newapi_user_id,checkin_date,policy_version,amount_units,asset_type,transaction_id)
	 VALUES(65,current_date,1,250000000,'RESERVE_API_CREDIT',$1)`, daily.TransactionID); err != nil {
		t.Fatal(err)
	}
	nonRegistrationID := announcementID(t)
	_, err = service.Prepare(ctx, actor.UserID, v08EconomyRequest(actor, nonRegistrationID,
		"REWARD_CLAIM_RETRY", "REWARD_CLAIM", daily.TransactionID, "1", `{"action":"RETRY"}`))
	if !errors.Is(err, ErrOpsNotFound) {
		t.Fatalf("daily reward retry=%v, want ErrOpsNotFound", err)
	}
	v08EconomyAssertAbsent(t, service, actor, nonRegistrationID)

	var operations, audits, events int
	if err = s.pool.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM ops.admin_operations WHERE operation_id IN ($1,$2)),
	 (SELECT count(*) FROM audit.audit_events WHERE operation_id IN ($1,$2)),
	 (SELECT count(*) FROM ops.admin_operation_events WHERE operation_id IN ($1,$2))`,
		successful[0], successful[1]).Scan(&operations, &audits, &events); err != nil {
		t.Fatal(err)
	}
	if operations != 2 || audits != 2 || events != 8 {
		t.Fatalf("reward retry effects operations=%d audits=%d events=%d", operations, audits, events)
	}
}
