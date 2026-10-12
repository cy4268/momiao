package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func v08Fresh(t *testing.T, service *OpsService, actor OpsPrincipal, prepared OpsPrepared) OpsExecuteRequest {
	t.Helper()
	challenge := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("8", 32)))
	if err := service.BindFreshChallenge(context.Background(), actor.UserID, actor.Epoch, prepared.Operation.OperationID, challenge); err != nil {
		t.Fatal(err)
	}
	return OpsExecuteRequest{AuthzEpoch: actor.Epoch, ImpactHash: prepared.Operation.ImpactHash, Confirmed: true,
		TypedConfirmation: prepared.ConfirmationPhrase,
		Fresh: &OpsFreshEvidence{OperationID: prepared.Operation.OperationID, ChallengeID: challenge,
			ContextHash: strings.Repeat("0", 64), Method: "password", VerifiedAt: time.Now().UTC()}}
}

func TestV08HealthUnavailableStaleAndRecovery(t *testing.T) {
	s, _, _ := v04OpsFixture(t)
	ctx := context.Background()
	if _, err := s.ReadOpsHealthSnapshot(ctx, 42, "STAGING"); !errors.Is(err, ErrOpsUnavailable) {
		t.Fatalf("missing health: %v", err)
	}
	if _, err := s.ReadOpsHealthSnapshot(ctx, 77, "STAGING"); !errors.Is(err, ErrOpsForbidden) {
		t.Fatalf("unauthorized health: %v", err)
	}
	raw := json.RawMessage(`{"ready":false,"checks":{"fixture_dependency":{"ready":false,"code":"FIXTURE_UNAVAILABLE"}}}`)
	old := time.Now().UTC().Add(-time.Minute)
	if err := s.WriteOpsHealthSnapshot(ctx, "STAGING", raw, old, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.ReadOpsHealthSnapshot(ctx, 42, "STAGING")
	if err != nil || !snapshot.Stale {
		t.Fatalf("stale failure missing: %+v %v", snapshot, err)
	}
	var value map[string]any
	if err := json.Unmarshal(snapshot.Snapshot, &value); err != nil || value["ready"] != false {
		t.Fatal("dependency failure was masked", err)
	}
	good := json.RawMessage(`{"ready":true,"checks":{}}`)
	now := time.Now().UTC()
	if err := s.WriteOpsHealthSnapshot(ctx, "STAGING", good, now, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteOpsHealthSnapshot(ctx, "STAGING", raw, old, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteOpsHealthSnapshot(ctx, "STAGING", raw, now.Add(time.Second), time.Second); !errors.Is(err, ErrOpsInvalid) {
		t.Fatalf("invalid TTL: %v", err)
	}
	snapshot, err = s.ReadOpsHealthSnapshot(ctx, 42, "STAGING")
	if err != nil || snapshot.Stale || snapshot.ObservedAt.Sub(now).Abs() > time.Microsecond {
		t.Fatalf("fresh recovery replaced by old writer: %+v %v", snapshot, err)
	}
	if err := json.Unmarshal(snapshot.Snapshot, &value); err != nil || value["ready"] != true {
		t.Fatal("fresh recovery missing", err)
	}
}

type v08RemoteReceiptFixture struct {
	executions int
	queries    []string
	receipt    json.RawMessage
}

func (*v08RemoteReceiptFixture) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	return canonicalOpsObject(raw, 32768)
}
func (*v08RemoteReceiptFixture) PrepareRemote(_ context.Context, _ OpsPrincipal, request OpsPrepareRequest) (OpsPreparedMaterial, error) {
	return OpsPreparedMaterial{TargetVersion: request.Target.ExpectedVersion, TargetLocator: request.Target.ID,
		Impact: OpsImpact{CurrentState: json.RawMessage(`{"accepting":true}`), ProposedChange: request.Input, Before: json.RawMessage(`{"accepting":true}`)}}, nil
}
func (*v08RemoteReceiptFixture) BuildRemoteCommand(_ OpsPrincipal, operation OpsOperation, _ json.RawMessage, _ OpsPreparedMaterial) (json.RawMessage, error) {
	return json.Marshal(map[string]string{"operation_id": operation.OperationID})
}
func (f *v08RemoteReceiptFixture) QueryOperation(_ context.Context, _ string, id string) (json.RawMessage, bool, error) {
	f.queries = append(f.queries, id)
	return f.receipt, len(f.receipt) > 0, nil
}
func (f *v08RemoteReceiptFixture) ExecuteOperation(_ context.Context, _ string, command json.RawMessage) (json.RawMessage, error) {
	f.executions++
	f.receipt = append(json.RawMessage{}, command...)
	// The remote effect and receipt exist; only the transport reply is lost.
	return nil, errors.New("V08 fixture committed remote reply lost")
}
func (*v08RemoteReceiptFixture) AcceptRemoteReceipt(operation OpsOperation, command, receipt json.RawMessage) (OpsRemoteCompletion, error) {
	if string(command) != string(receipt) {
		return OpsRemoteCompletion{}, ErrOpsConflict
	}
	return OpsRemoteCompletion{State: "SUCCEEDED", Execution: OpsExecutionResult{
		BeforeSnapshot: json.RawMessage(`{"accepting":true}`), AfterSnapshot: json.RawMessage(`{"accepting":false}`),
		Result: json.RawMessage(`{"applied":true}`), RelatedBusinessID: operation.Target.ID}}, nil
}

func TestV08OpsRemoteLostReplyUsesOriginalOperation(t *testing.T) {
	s, _, actor := v04OpsFixture(t)
	fake := &v08RemoteReceiptFixture{}
	binding := OpsOperationBinding{Descriptor: OpsOperationDescriptor{OperationType: "POKER_ACCEPTING_PLAYERS_SET",
		Risk: OpsRiskImpactful, RequiredPermission: "poker.accepting_players.write", AllowedRoles: []string{"SUPER_ADMIN"},
		TargetType: "poker_table", InputSchemaVersion: "poker-accepting-players.v1", ImpactSchemaVersion: "poker-operation-impact.v1",
		RequiresReason: true, ConfirmationMode: OpsConfirmExplicit, ExecutionMode: OpsDurableRemote}, Handler: fake}
	service, err := NewOpsService(s, "STAGING", binding)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	id := announcementID(t)
	prepared, err := service.Prepare(ctx, actor.UserID, OpsPrepareRequest{OperationID: id, OperationType: binding.Descriptor.OperationType,
		AuthzEpoch: actor.Epoch, InputSchemaVersion: binding.Descriptor.InputSchemaVersion,
		Target: OpsTarget{Type: "poker_table", ID: announcementID(t), ExpectedVersion: "1"},
		Input:  json.RawMessage(`{"accepting":false}`), Reason: "V08 isolated remote receipt"})
	if err != nil {
		t.Fatal(err)
	}
	execute := OpsExecuteRequest{AuthzEpoch: actor.Epoch, ImpactHash: prepared.Operation.ImpactHash, Confirmed: true}
	accepted, err := service.Execute(ctx, actor.UserID, id, execute)
	if err != nil || accepted.Operation.State != "EXECUTING" {
		t.Fatalf("outbox acceptance=%s error=%v", accepted.Operation.State, err)
	}
	if advanced, err := service.RunRemoteDispatchStep(ctx); err == nil || !advanced {
		t.Fatalf("lost reply=%v error=%v", advanced, err)
	}
	uncertain, err := service.Operation(ctx, actor.UserID, actor.Epoch, id)
	if err != nil || uncertain.Operation.State != "RECOVERING" || uncertain.AuditID != "" {
		t.Fatalf("unknown state=%s audit=%s error=%v", uncertain.Operation.State, uncertain.AuditID, err)
	}
	if fake.executions != 1 || !reflect.DeepEqual(fake.queries, []string{id}) {
		t.Fatal("first dispatch differs")
	}
	if advanced, err := service.RunRemoteDispatchStep(ctx); err != nil || advanced {
		t.Fatalf("retry before its deadline=%v error=%v", advanced, err)
	}
	if fake.executions != 1 || !reflect.DeepEqual(fake.queries, []string{id}) {
		t.Fatal("premature retry contacted the remote handler")
	}
	// The database deliberately forbids rewriting RETRY in place. Honor the
	// first five-second backoff instead of bypassing the real transition guard.
	time.Sleep(6 * time.Second)
	// A newly constructed service must recover the original durable command.
	service, err = NewOpsService(s, "STAGING", binding)
	if err != nil {
		t.Fatal(err)
	}
	if advanced, err := service.RunRemoteDispatchStep(ctx); err != nil || !advanced {
		t.Fatalf("same-ID receipt recovery=%v error=%v", advanced, err)
	}
	complete, err := service.Operation(ctx, actor.UserID, actor.Epoch, id)
	if err != nil || complete.Operation.State != "SUCCEEDED" || complete.AuditID == "" {
		t.Fatalf("completion=%s audit=%s error=%v", complete.Operation.State, complete.AuditID, err)
	}
	if fake.executions != 1 || !reflect.DeepEqual(fake.queries, []string{id, id}) {
		t.Fatal("recovery repeated remote effect or changed ID")
	}
	if _, err = service.Execute(ctx, actor.UserID, id, execute); !errors.Is(err, ErrOpsConflict) {
		t.Fatalf("duplicate execution=%v", err)
	}
	if advanced, err := service.RunRemoteDispatchStep(ctx); err != nil || advanced {
		t.Fatalf("terminal dispatch repeated=%v error=%v", advanced, err)
	}
	var audits, attempts int
	var state string
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM audit.audit_events WHERE operation_id=$1`, id).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit count=%d error=%v", audits, err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT state,attempt_count FROM ops.remote_dispatches WHERE operation_id=$1`, id).Scan(&state, &attempts); err != nil || state != "SUCCEEDED" || attempts != 2 {
		t.Fatalf("outbox=%s/%d error=%v", state, attempts, err)
	}
}
