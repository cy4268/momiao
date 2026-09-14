package platform

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// This component test uses bootstrapStore's fresh disposable database. Before
// migration 0034, the Prepare call fails with admin_operation_typed_shape even
// though support-cases.write is a canonical permission. After 0034 it proves
// the complete local operation and keeps rejecting characters outside the
// bounded permission-key alphabet.
func TestOpsPermissionShapeAcceptsCanonicalHyphenAndRejectsInvalidCharacters(t *testing.T) {
	s, _, actor := v04OpsFixture(t)
	ctx := context.Background()
	service, err := NewOpsService(s, "STAGING", OpsSupportBindings(s)...)
	if err != nil {
		t.Fatal(err)
	}

	operationID, caseID := announcementID(t), announcementID(t)
	prepared, err := service.Prepare(ctx, actor.UserID, OpsPrepareRequest{
		OperationID: operationID, OperationType: "SUPPORT_CASE_CREATE", AuthzEpoch: actor.Epoch,
		InputSchemaVersion: "support-case-create.v1",
		Target:             OpsTarget{Type: "SUPPORT_CASE", ID: caseID, ExpectedVersion: "0"},
		Input:              json.RawMessage(`{"subject_newapi_user_id":"42","category":"OTHER","safe_summary":"Synthetic permission shape regression"}`),
		Reason:             "V08 isolated support permission regression",
	})
	if err != nil {
		t.Fatal("canonical support permission did not prepare", err)
	}
	if prepared.Operation.RequiredPermission != "support-cases.write" || prepared.Operation.State != "PREPARED" {
		t.Fatalf("prepared permission/state=%q/%q", prepared.Operation.RequiredPermission, prepared.Operation.State)
	}

	view, err := service.Execute(ctx, actor.UserID, operationID, OpsExecuteRequest{
		AuthzEpoch: actor.Epoch, ImpactHash: prepared.Operation.ImpactHash,
	})
	if err != nil {
		t.Fatal("canonical support permission did not execute", err)
	}
	if view.Operation.State != "SUCCEEDED" || view.Operation.RelatedBusinessID != caseID || view.AuditID == "" {
		t.Fatalf("support operation state/business/audit=%q/%q/%q", view.Operation.State, view.Operation.RelatedBusinessID, view.AuditID)
	}
	wantTimeline := []string{"PREPARED", "AUTHORIZED", "EXECUTING", "SUCCEEDED"}
	if len(view.Timeline) != len(wantTimeline) {
		t.Fatalf("support timeline length=%d", len(view.Timeline))
	}
	for i, want := range wantTimeline {
		if view.Timeline[i].State != want || view.Timeline[i].StateVersion != int64(i+1) {
			t.Fatalf("support timeline[%d]=%q/%d", i, view.Timeline[i].State, view.Timeline[i].StateVersion)
		}
	}

	var subject, version int64
	var category, state, summary, permission string
	if err = s.pool.QueryRow(ctx, `SELECT c.subject_newapi_user_id,c.category,c.state,c.safe_summary,c.version,o.required_permission
	 FROM ops.support_cases c JOIN ops.admin_operations o ON o.operation_id=c.last_operation_id
	 WHERE c.case_id=$1`, caseID).Scan(&subject, &category, &state, &summary, &version, &permission); err != nil {
		t.Fatal(err)
	}
	if subject != 42 || category != "OTHER" || state != "OPEN" || summary != "Synthetic permission shape regression" ||
		version != 1 || permission != "support-cases.write" {
		t.Fatalf("support durable facts=%d/%q/%q/%q/%d/%q", subject, category, state, summary, version, permission)
	}

	invalidID := announcementID(t)
	_, err = s.pool.Exec(ctx, `INSERT INTO ops.admin_operations(
	 operation_id,actor_kind,newapi_user_id,action,request_hash,details,result,operation_type,
	 actor_role_snapshot,actor_scopes_snapshot,actor_authz_epoch_snapshot,required_permission,risk_level,
	 target_type,target_id,target_version_snapshot,input_schema_version,input_payload,input_hash,environment,
	 impact_schema_version,impact_preview,impact_hash,confirmation_mode,requires_fresh_auth)
	 VALUES($1,'ADMIN',42,'TEST_PERMISSION_SHAPE',repeat('0',64),'{}'::jsonb,'{}'::jsonb,'TEST_PERMISSION_SHAPE',
	 'SUPER_ADMIN',ARRAY['RECORDS'],1,'support/cases.write','LEVEL_1_ROUTINE',
	 'TEST','test','0','test.v1','{}'::jsonb,decode(repeat('00',32),'hex'),'STAGING',
	 'test-impact.v1','{}'::jsonb,decode(repeat('00',32),'hex'),'NONE',false)`, invalidID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != "admin_operation_typed_shape" {
		t.Fatalf("invalid permission bypassed typed shape: %v", err)
	}
	var invalidRows int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM ops.admin_operations WHERE operation_id=$1`, invalidID).Scan(&invalidRows); err != nil || invalidRows != 0 {
		t.Fatalf("invalid permission row count=%d error=%v", invalidRows, err)
	}
}
