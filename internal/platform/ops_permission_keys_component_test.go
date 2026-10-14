package platform

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// The old support permission/prepare regression now exercises direct local
// writes on the existing dev database. All business writes roll back together.
func TestLocalOpsSupportIncidents(t *testing.T) {
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
	var admin, regular int64
	if err = tx.QueryRow(ctx, `SELECT min(id) FILTER(WHERE role>=10),max(id) FILTER(WHERE role<10) FROM identity.users WHERE status=1 AND deleted_at IS NULL`).Scan(&admin, &regular); err != nil {
		t.Fatal(err)
	}
	var audits, operations int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM ops.audit_log),(SELECT count(*) FROM ops.admin_operations)`).Scan(&audits, &operations); err != nil {
		t.Fatal(err)
	}
	summary := "support-local-" + announcementID(t)
	input := OpsSupportInput{SubjectID: regular, Category: "OTHER", SafeSummary: summary}
	if _, err = saveOpsSupportCase(ctx, tx, regular, "", "create", input); !errors.Is(err, ErrCatalogForbidden) {
		t.Fatal("regular support write", err)
	}
	// An ordinary role 10 admin is sufficient, including a local user without Master.
	if _, err = tx.Exec(ctx, `UPDATE identity.users SET role=10 WHERE id=$1`, regular); err != nil {
		t.Fatal(err)
	}
	actor := regular
	c, err := saveOpsSupportCase(ctx, tx, actor, "", "create", input)
	if err != nil || c.State != "OPEN" {
		t.Fatal("create case", err, c.State)
	}
	if _, err = saveOpsSupportCase(ctx, tx, actor, c.CaseID, "state", OpsSupportInput{NextState: "EXECUTED"}); !errors.Is(err, ErrOpsConflict) {
		t.Fatal("case skipped state", err)
	}
	if _, err = saveOpsSupportCase(ctx, tx, actor, c.CaseID, "facts", OpsSupportInput{FactType: "IDENTITY_MATCH", Result: "VERIFIED", SafeReference: "dev:metadata"}); err != nil {
		t.Fatal("fact", err)
	}
	for _, state := range []string{"VERIFYING", "VERIFYING", "APPROVED", "EXECUTED", "CLOSED"} {
		c, err = saveOpsSupportCase(ctx, tx, actor, c.CaseID, "state", OpsSupportInput{NextState: state})
		if err != nil || c.State != state {
			t.Fatal("case transition", state, err)
		}
	}
	if c.ClosedAt == nil {
		t.Fatal("case closed timestamp")
	}
	if _, err = saveOpsSupportCase(ctx, tx, actor, c.CaseID, "facts", OpsSupportInput{FactType: "CONTACT_VERIFIED", Result: "VERIFIED"}); !errors.Is(err, ErrOpsConflict) {
		t.Fatal("closed fact", err)
	}
	d, err := readOpsSupportCase(ctx, tx, c.CaseID)
	if err != nil || len(d.VerificationFacts) != 1 || d.VerificationFacts[0].ActorUserID != actor || d.VerificationFacts[0].OperationID != "" {
		t.Fatal("case detail", err)
	}
	filter := OpsSupportCaseQuery{Page: 1, SubjectID: regular, State: "CLOSED", Category: "OTHER", Search: summary, From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour)}
	page, err := readOpsSupportCases(ctx, tx, filter)
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatal("case filters", err, page.Total)
	}
	filter.Page = 2
	page, err = readOpsSupportCases(ctx, tx, filter)
	if err != nil || page.Total != 1 || len(page.Items) != 0 {
		t.Fatal("case second page", err)
	}
	filter.Page = 1
	filter.SubjectID = admin
	page, err = readOpsSupportCases(ctx, tx, filter)
	if err != nil || page.Total != 0 {
		t.Fatal("case user filter", err)
	}
	incidentInput := OpsIncidentInput{Title: summary, Severity: "WARNING", SafeSummary: "Local development incident"}
	i, err := saveOpsIncident(ctx, tx, actor, "", "create", incidentInput)
	if err != nil || i.State != "OPEN" {
		t.Fatal("create incident", err)
	}
	if _, err = saveOpsIncident(ctx, tx, actor, i.IncidentID, "state", OpsIncidentInput{NextState: "RESOLVED"}); !errors.Is(err, ErrOpsConflict) {
		t.Fatal("incident skipped state", err)
	}
	if _, err = saveOpsIncident(ctx, tx, actor, i.IncidentID, "events", OpsIncidentInput{EventType: "COMMENT", SafeSummary: "Investigated in dev"}); err != nil {
		t.Fatal("incident comment", err)
	}
	if _, err = saveOpsIncident(ctx, tx, actor, i.IncidentID, "events", OpsIncidentInput{EventType: "BUSINESS_FACT_LINKED", RelatedType: "BUSINESS_FACT", RelatedID: c.CaseID}); err != nil {
		t.Fatal("incident link", err)
	}
	for _, state := range []string{"TRIAGED", "IN_PROGRESS", "MONITORING", "RESOLVED", "CLOSED"} {
		i, err = saveOpsIncident(ctx, tx, actor, i.IncidentID, "state", OpsIncidentInput{NextState: state})
		if err != nil || i.State != state {
			t.Fatal("incident transition", state, err)
		}
	}
	if i.ResolvedAt == nil || i.ClosedAt == nil {
		t.Fatal("incident timestamps")
	}
	if _, err = saveOpsIncident(ctx, tx, actor, i.IncidentID, "events", OpsIncidentInput{EventType: "COMMENT", SafeSummary: "closed"}); !errors.Is(err, ErrOpsConflict) {
		t.Fatal("closed incident comment", err)
	}
	detail, err := readOpsIncident(ctx, tx, i.IncidentID)
	if err != nil || len(detail.Events) != 8 {
		t.Fatal("incident timeline", err, len(detail.Events))
	}
	for _, event := range detail.Events {
		if event.ActorUserID != actor || event.OperationID != "" {
			t.Fatal("incident actor/history")
		}
	}
	inf := OpsIncidentQuery{Page: 1, Search: summary, State: "CLOSED", Severity: "WARNING", From: filter.From, To: filter.To}
	ip, err := readOpsIncidents(ctx, tx, inf)
	if err != nil || ip.Total != 1 || len(ip.Items) != 1 {
		t.Fatal("incident filters", err)
	}
	inf.Page = 2
	ip, err = readOpsIncidents(ctx, tx, inf)
	if err != nil || ip.Total != 1 || len(ip.Items) != 0 {
		t.Fatal("incident page", err)
	}
	var added, legacy, facts, eventAudit int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM ops.audit_log),(SELECT count(*) FROM ops.admin_operations),(SELECT count(*) FROM ops.audit_log WHERE target=$1::text AND after_state->'verification_fact'->>'result'='VERIFIED'),(SELECT count(*) FROM ops.audit_log WHERE target=$2::text AND after_state->'event'->>'event_type'='COMMENT')`, c.CaseID, i.IncidentID).Scan(&added, &legacy, &facts, &eventAudit); err != nil {
		t.Fatal(err)
	}
	if added-audits != 15 || legacy != operations || facts != 1 || eventAudit != 1 {
		t.Fatal("atomic audit", added-audits, legacy-operations, facts, eventAudit)
	}
	if _, err = tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err = s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM ops.support_cases WHERE case_id=$1)+(SELECT count(*) FROM ops.incidents WHERE incident_id=$2)+(SELECT count(*) FROM ops.audit_log WHERE target IN ($1::text,$2::text))`, c.CaseID, i.IncidentID).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("rollback", err, remaining)
	}
	t.Log("role 10 local writes; support create/fact/transition and incident timeline; filters + page; closed/skipped state denied; 15 atomic audits / 0 legacy operations; all rows and actor role rolled back")
}
