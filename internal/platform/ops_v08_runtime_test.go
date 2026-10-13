package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = readOpsHealthSnapshot(ctx, tx, "MISSING"); !errors.Is(err, ErrOpsUnavailable) {
		t.Fatal("missing health", err)
	}
	var beforeHealth int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM ops.health_snapshots WHERE environment='STAGING'`).Scan(&beforeHealth); err != nil {
		t.Fatal(err)
	}
	snapshotInput := json.RawMessage(`{"ready":false,"checked_at":"2026-10-13T00:00:00Z","components":[{"name":"platform_database","state":"UNAVAILABLE"}]}`)
	observed := time.Now().UTC().Add(-time.Minute)
	if _, err = tx.Exec(ctx, `INSERT INTO ops.health_snapshots(environment,snapshot,observed_at,stale_after) VALUES('STAGING',$1,$2,$3) ON CONFLICT(environment) DO UPDATE SET snapshot=excluded.snapshot,observed_at=excluded.observed_at,stale_after=excluded.stale_after`, snapshotInput, observed, observed.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	view, err := readOpsHealthSnapshot(ctx, tx, "STAGING")
	if err != nil || !view.Stale {
		t.Fatal("stale health", err)
	}
	var value struct {
		Ready bool `json:"ready"`
	}
	if err = json.Unmarshal(view.Snapshot, &value); err != nil || value.Ready {
		t.Fatal("failure masked", err)
	}
	now := time.Now().UTC()
	if _, err = tx.Exec(ctx, `UPDATE ops.health_snapshots SET snapshot='{"ready":true,"components":[]}'::jsonb,observed_at=$1,stale_after=$2 WHERE environment='STAGING'`, now, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	view, err = readOpsHealthSnapshot(ctx, tx, "STAGING")
	if err != nil || view.Stale {
		t.Fatal("fresh health", err)
	}
	if err = json.Unmarshal(view.Snapshot, &value); err != nil || !value.Ready {
		t.Fatal("recovery missing", err)
	}
	// Reuse a real announcement and the existing job table in this rolled-back
	// transaction. No worker can observe or execute this uncommitted row.
	id := "ops-runtime-" + announcementID(t)
	tag, err := tx.Exec(ctx, `INSERT INTO content.announcement_jobs(job_key,announcement_id,kind,content_version,notification_revision,due_at)
 SELECT $1,announcement_id,'PUBLISH',current_content_version,notification_revision,clock_timestamp()-interval '1 minute' FROM content.announcements ORDER BY announcement_id LIMIT 1`, id)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatal("existing announcement job", err)
	}
	filter := OpsJobsFilter{Page: 1, Kind: "ANNOUNCEMENTS", State: "PENDING", From: time.Now().Add(-time.Minute), To: time.Now().Add(time.Minute)}
	page, err := readOpsJobs(ctx, tx, filter)
	if err != nil || page.Total != 1 || len(page.Items) != 1 || len(page.Summary) != 8 {
		t.Fatal("queue filter", err, page.Total, len(page.Items))
	}
	if page.Items[0].ID != id || page.Items[0].Legacy || page.Items[0].Attention || !page.Items[0].Pending || page.Items[0].RunAt == nil {
		t.Fatal("job metadata", page.Items)
	}
	for _, summary := range page.Summary {
		if summary.Kind == "ANNOUNCEMENTS" && (summary.Pending != 1 || summary.Due != 1 || summary.Attention != 0) {
			t.Fatal("queue summary", summary)
		}
	}
	filter.Page = 2
	page, err = readOpsJobs(ctx, tx, filter)
	if err != nil || page.Total != 1 || len(page.Items) != 0 {
		t.Fatal("page offset", err)
	}
	filter.Page = 1
	filter.AttentionOnly = true
	page, err = readOpsJobs(ctx, tx, filter)
	if err != nil || page.Total != 0 {
		t.Fatal("attention subset", err)
	}
	filter.AttentionOnly = false
	filter.To = time.Now().Add(-time.Hour)
	filter.From = time.Now().Add(-2 * time.Hour)
	page, err = readOpsJobs(ctx, tx, filter)
	if err != nil || page.Total != 0 {
		t.Fatal("time filter", err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var remaining, afterHealth int
	if err = s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM content.announcement_jobs WHERE job_key=$1),(SELECT count(*) FROM ops.health_snapshots WHERE environment='STAGING')`, id).Scan(&remaining, &afterHealth); err != nil || remaining != 0 || beforeHealth != afterHealth {
		t.Fatal("rollback", err, remaining)
	}
	t.Log("real local runtime role: missing/stale/recovered health; queue metadata + summary/filters/page/attention; no payloads or worker execution; temporary job and STAGING snapshot rolled back")
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
