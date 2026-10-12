package platform

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestLocalMaintenanceSave(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires the existing dev database")
	}
	dsn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read dev DSN")
	}
	s, err := Open(t.Context(), strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("open dev database")
	}
	defer s.Close()
	ctx := t.Context()
	original, err := s.ReadMaintenance(ctx, "DEVELOPMENT")
	if err != nil {
		t.Fatal(err)
	}
	var auditBefore int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM ops.audit_log`).Scan(&auditBefore); err != nil {
		t.Fatal(err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var user int64
	if err = tx.QueryRow(ctx, `SELECT u.id FROM identity.users u JOIN identity.account_refs a ON a.newapi_user_id=u.id WHERE role>=10 AND status=1 AND deleted_at IS NULL ORDER BY id LIMIT 1`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	value := MaintenanceSettings{Enabled: true, Message: "维护主流程验证"}
	if err = ValidateMaintenance(&value); err != nil {
		t.Fatal(err)
	}
	on, err := saveMaintenance(ctx, tx, user, "DEVELOPMENT", value)
	if err != nil || !on.Enabled || len(on.Scopes) != 7 {
		t.Fatal("enable maintenance", err)
	}
	if err = RequireNoMaintenance(ctx, tx, "REWARDS"); !errors.Is(err, ErrMaintenanceActive) {
		t.Fatal("new work not blocked", err)
	}
	current, err := readMaintenance(ctx, tx, "DEVELOPMENT")
	if err != nil || !reflect.DeepEqual(current, on) {
		t.Fatal("maintenance read differs", err)
	}
	var auditCount int
	var auditMatches bool
	err = tx.QueryRow(ctx, `SELECT count(*),bool_and(actor_user_id=$1 AND action='MAINTENANCE_SAVE' AND target='DEVELOPMENT' AND (after_state->>'enabled')::boolean AND after_state->>'message'=$2) FROM ops.audit_log WHERE after_state->>'maintenance_id'=$3`, user, value.Message, on.ID).Scan(&auditCount, &auditMatches)
	if err != nil || auditCount != 1 || !auditMatches {
		t.Fatal("atomic audit missing", err)
	}
	// A narrower save replaces the current scopes, rather than adding another authority.
	value.Scopes = []string{"REWARDS"}
	value.Message = "仅奖励维护"
	edit, err := saveMaintenance(ctx, tx, user, "DEVELOPMENT", value)
	if err != nil || edit.ID == on.ID {
		t.Fatal("edit maintenance", err)
	}
	if err = RequireNoMaintenance(ctx, tx, "POKER_NEW_TABLES_NEW_HANDS"); err != nil {
		t.Fatal("removed scope still blocked", err)
	}
	off, err := saveMaintenance(ctx, tx, user, "DEVELOPMENT", MaintenanceSettings{Enabled: false})
	if err != nil || off.Enabled {
		t.Fatal("end maintenance", err)
	}
	if err = RequireNoMaintenance(ctx, tx, "REWARDS"); err != nil {
		t.Fatal("maintenance did not end", err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM ops.audit_log`).Scan(&auditCount); err != nil || auditCount != auditBefore+3 {
		t.Fatal("save must append exactly one audit", err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := s.ReadMaintenance(ctx, "DEVELOPMENT")
	if err != nil || !reflect.DeepEqual(restored, original) {
		t.Fatal("maintenance rollback mismatch", err)
	}
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM ops.audit_log`).Scan(&auditCount); err != nil || auditCount != auditBefore {
		t.Fatal("audit escaped transaction", err)
	}
	t.Log("on/edit/off + admission guards + one audit per save; transaction rollback restored settings/audit")
}
