package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// Each component uses bootstrapStore's new database, never an accepted runtime.
func v04OpsFixture(t *testing.T) (*Store, *OpsService, OpsPrincipal) {
	t.Helper()
	s := bootstrapStore(t)
	if _, err := s.Bootstrap(context.Background(), bootstrapInput()); err != nil {
		t.Fatal(err)
	}
	service, err := NewOpsService(s, "STAGING")
	if err != nil {
		t.Fatal(err)
	}
	boot, err := service.Bootstrap(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	return s, service, boot.Principal
}

func v04PreparedDisable(t *testing.T, s *Store, service *OpsService, actor OpsPrincipal) (OpsPrepared, OpsExecuteRequest) {
	t.Helper()
	ctx := context.Background()
	target := announcementID(t)
	if _, err := s.pool.Exec(ctx, `INSERT INTO ops.admin_principals(admin_principal_id,newapi_user_id,base_role)
	 VALUES($1,43,'AUDITOR')`, target); err != nil {
		t.Fatal(err)
	}
	prepared, err := service.Prepare(ctx, actor.UserID, OpsPrepareRequest{
		OperationID: announcementID(t), OperationType: "ACCESS_CONTROL_CHANGE", AuthzEpoch: actor.Epoch,
		InputSchemaVersion: "access-control.v1", Target: OpsTarget{Type: "ADMIN_PRINCIPAL", ID: target, ExpectedVersion: "1"},
		Input: json.RawMessage(`{"action":"DISABLE"}`), Reason: "V04 isolated component acceptance",
	})
	if err != nil {
		t.Fatal(err)
	}
	challenge := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("v", 32)))
	if err = service.BindFreshChallenge(ctx, actor.UserID, actor.Epoch, prepared.Operation.OperationID, challenge); err != nil {
		t.Fatal(err)
	}
	return prepared, OpsExecuteRequest{AuthzEpoch: actor.Epoch, ImpactHash: prepared.Operation.ImpactHash,
		Confirmed: true, TypedConfirmation: prepared.ConfirmationPhrase,
		// This is server-side evidence at the OpsService boundary; HTTP proof verification is covered separately.
		Fresh: &OpsFreshEvidence{OperationID: prepared.Operation.OperationID, ChallengeID: challenge,
			ContextHash: strings.Repeat("0", 64), Method: "password", VerifiedAt: time.Now().UTC()},
	}
}

func v04AssertEffects(t *testing.T, s *Store, operationID, targetID, status string, version int64, effects, terminals int) {
	t.Helper()
	ctx := context.Background()
	var gotStatus string
	var gotVersion, gotEpoch int64
	if err := s.pool.QueryRow(ctx, `SELECT status,version,authz_epoch FROM ops.admin_principals WHERE admin_principal_id=$1`, targetID).
		Scan(&gotStatus, &gotVersion, &gotEpoch); err != nil {
		t.Fatal(err)
	}
	if gotStatus != status || gotVersion != version || gotEpoch != version {
		t.Fatalf("principal status/version/epoch=%s/%d/%d, want %s/%d/%d", gotStatus, gotVersion, gotEpoch, status, version, version)
	}
	for _, table := range []string{"ops.admin_role_history", "audit.audit_events"} {
		var count int
		if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE operation_id=$1", operationID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != effects {
			t.Fatalf("%s effects=%d, want %d", table, count, effects)
		}
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM ops.admin_operation_events WHERE operation_id=$1
	 AND state IN ('SUCCEEDED','CANCELLED','FAILED_NO_EFFECT','NEEDS_REVIEW')`, operationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != terminals {
		t.Fatalf("terminal events=%d, want %d", count, terminals)
	}
}

// Removing the Fresh age guard must fail before any principal or durable history mutation.
func TestV04OpsExpiredFreshPreservesPrepared(t *testing.T) {
	s, service, actor := v04OpsFixture(t)
	prepared, request := v04PreparedDisable(t, s, service, actor)
	ctx := context.Background()
	id := prepared.Operation.OperationID
	before, err := service.Operation(ctx, actor.UserID, actor.Epoch, id)
	if err != nil {
		t.Fatal(err)
	}
	request.Fresh.VerifiedAt = time.Now().UTC().Add(-11 * time.Minute)
	if _, err = service.Execute(ctx, actor.UserID, id, request); !errors.Is(err, ErrOpsFreshRequired) {
		t.Fatalf("expired Fresh execute=%v, want ErrOpsFreshRequired", err)
	}
	after, err := service.Operation(ctx, actor.UserID, actor.Epoch, id)
	if err != nil {
		t.Fatal(err)
	}
	if after.Operation.State != "PREPARED" || after.Operation.StateVersion != before.Operation.StateVersion ||
		len(after.Timeline) != len(before.Timeline) || after.AuditID != "" || after.Operation.AuthorizedAt != nil || after.Operation.CompletedAt != nil {
		t.Fatal("expired Fresh changed the prepared operation or its durable timeline")
	}
	v04AssertEffects(t, s, id, prepared.Operation.Target.ID, "ACTIVE", 1, 0, 0)
	request.Fresh.VerifiedAt = time.Now().UTC()
	result, err := service.Execute(ctx, actor.UserID, id, request)
	if err != nil || result.Operation.State != "SUCCEEDED" || result.AuditID == "" {
		t.Fatalf("valid Fresh positive control state=%s audit=%s error=%v", result.Operation.State, result.AuditID, err)
	}
	v04AssertEffects(t, s, id, prepared.Operation.Target.ID, "DISABLED", 2, 1, 1)
}

// Losing the operation-row serialization must not produce two terminal effects.
func TestV04OpsConcurrentExecuteCancelSingleEffect(t *testing.T) {
	s, service, actor := v04OpsFixture(t)
	prepared, request := v04PreparedDisable(t, s, service, actor)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	id := prepared.Operation.OperationID
	type outcome struct {
		execute bool
		view    OpsOperationView
		err     error
	}
	start := make(chan struct{})
	ready := make(chan struct{}, 2)
	results := make(chan outcome, 2)
	go func() {
		ready <- struct{}{}
		<-start
		view, err := service.Execute(ctx, actor.UserID, id, request)
		results <- outcome{execute: true, view: view, err: err}
	}()
	go func() {
		ready <- struct{}{}
		<-start
		view, err := service.CancelOperation(ctx, actor.UserID, actor.Epoch, id)
		results <- outcome{view: view, err: err}
	}()
	<-ready
	<-ready
	close(start)
	first, second := <-results, <-results
	final, err := service.Operation(ctx, actor.UserID, actor.Epoch, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range []outcome{first, second} {
		if result.execute && final.Operation.State == "CANCELLED" {
			if !errors.Is(result.err, ErrOpsConflict) {
				t.Fatalf("execute losing to cancel=%v, want ErrOpsConflict", result.err)
			}
		} else if result.err != nil || result.view.Operation.State != final.Operation.State {
			t.Fatalf("concurrent receipt state=%s final=%s error=%v", result.view.Operation.State, final.Operation.State, result.err)
		}
	}
	status, version, effects := "ACTIVE", int64(1), 0
	switch final.Operation.State {
	case "SUCCEEDED":
		status, version, effects = "DISABLED", 2, 1
	case "CANCELLED":
	default:
		t.Fatalf("nonterminal race outcome %s", final.Operation.State)
	}
	if final.Operation.CompletedAt == nil {
		t.Fatal("terminal operation lacks completion receipt")
	}
	v04AssertEffects(t, s, id, prepared.Operation.Target.ID, status, version, effects, 1)
	if _, err = service.Execute(ctx, actor.UserID, id, request); !errors.Is(err, ErrOpsConflict) {
		t.Fatalf("terminal execute replay=%v, want ErrOpsConflict", err)
	}
	replayed, err := service.CancelOperation(ctx, actor.UserID, actor.Epoch, id)
	if err != nil || replayed.Operation.State != final.Operation.State || replayed.Operation.StateVersion != final.Operation.StateVersion {
		t.Fatalf("terminal cancel replay changed receipt: %v", err)
	}
	v04AssertEffects(t, s, id, prepared.Operation.Target.ID, status, version, effects, 1)
}

// Removing last-SUPER_ADMIN protection must fail without creating an operation.
func TestV04OpsLastSuperAdminDisablePreservesAuthority(t *testing.T) {
	s, service, actor := v04OpsFixture(t)
	ctx := context.Background()
	id := announcementID(t)
	var before, after string
	if err := s.pool.QueryRow(ctx, `SELECT row_to_json(p)::text FROM ops.admin_principals p WHERE admin_principal_id=$1`, actor.PrincipalID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	_, err := service.Prepare(ctx, actor.UserID, OpsPrepareRequest{
		OperationID: id, OperationType: "ACCESS_CONTROL_CHANGE", AuthzEpoch: actor.Epoch,
		InputSchemaVersion: "access-control.v1", Target: OpsTarget{Type: "ADMIN_PRINCIPAL", ID: actor.PrincipalID, ExpectedVersion: "1"},
		Input: json.RawMessage(`{"action":"DISABLE"}`), Reason: "V04 isolated last administrator protection",
	})
	if !errors.Is(err, ErrOpsLastSuperAdmin) {
		t.Fatalf("last SUPER_ADMIN disable=%v, want ErrOpsLastSuperAdmin", err)
	}
	if err = s.pool.QueryRow(ctx, `SELECT row_to_json(p)::text FROM ops.admin_principals p WHERE admin_principal_id=$1`, actor.PrincipalID).Scan(&after); err != nil || before != after {
		t.Fatalf("rejected disable changed principal: %v", err)
	}
	if _, err = service.Operation(ctx, actor.UserID, actor.Epoch, id); !errors.Is(err, ErrOpsNotFound) {
		t.Fatalf("rejected prepare persisted an operation: %v", err)
	}
	v04AssertEffects(t, s, id, actor.PrincipalID, "ACTIVE", 1, 0, 0)
	if _, err = s.Bootstrap(ctx, bootstrapInput()); !errors.Is(err, ErrBootstrapClosed) {
		t.Fatalf("rejected disable reopened bootstrap: %v", err)
	}
	bootstrapCounts(t, s, 1)
}
