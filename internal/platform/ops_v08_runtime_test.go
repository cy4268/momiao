package platform

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

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
