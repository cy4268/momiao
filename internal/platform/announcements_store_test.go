package platform

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func announcementID(t *testing.T) string {
	t.Helper()
	id, err := uuidV7()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Reuses the existing lifecycle regression on the isolated dev database. All
// edits, immutable revisions and audit rows stay in one rolled-back transaction.
func TestAnnouncementLifecycleAndReads(t *testing.T) {
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
	if err = tx.QueryRow(ctx, `SELECT min(id) FILTER(WHERE role>=10),min(id) FILTER(WHERE role<10) FROM identity.users WHERE status=1 AND deleted_at IS NULL`).Scan(&admin, &regular); err != nil {
		t.Fatal(err)
	}
	content := AnnouncementContent{Title: "Direct announcement verification", Type: "SYSTEM", Visibility: "PUBLIC", Markdown: "## Direct save\n\n**Stored content**"}
	create := AnnouncementCommand{Action: "SAVE", Content: &content}
	if _, err = saveAnnouncement(ctx, tx, regular, create); !errors.Is(err, ErrCatalogForbidden) {
		t.Fatal("regular write", err)
	}
	var audits, legacy, previews int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM ops.audit_log),(SELECT count(*) FROM ops.admin_operations),(SELECT count(*) FROM ops.announcement_previews)`).Scan(&audits, &legacy, &previews); err != nil {
		t.Fatal(err)
	}
	draft, err := saveAnnouncement(ctx, tx, admin, create)
	if err != nil || draft.State != "DRAFT" || draft.ContentVersion != 1 || !strings.Contains(draft.HTML, "<strong>Stored content</strong>") {
		t.Fatal("local draft", err, draft.State)
	}
	visible := func(id string, user int64, archive bool) bool {
		_, e := scanOpsAnnouncement(tx.QueryRow(ctx, announcementSelect+" WHERE "+announcementPublicWhere+" AND a.announcement_id=$3", user, archive, id))
		if e != nil && !errors.Is(e, ErrAnnouncementNotFound) {
			t.Fatal(e)
		}
		return e == nil
	}
	if visible(draft.ID, 0, false) {
		t.Fatal("draft publicly visible")
	}
	// Temporarily clear the single banner slot inside this transaction only.
	if _, err = tx.Exec(ctx, `DELETE FROM content.announcement_placements WHERE placement='PUBLIC_HOME_BANNER'`); err != nil {
		t.Fatal(err)
	}
	now, until := time.Now().UTC().Add(-time.Minute), time.Now().UTC().Add(time.Hour)
	publish := AnnouncementCommand{ID: draft.ID, Action: "PUBLISH", PublishAt: &now, VisibleFrom: &now, VisibleUntil: &until, Placements: []AnnouncementPlacement{{Placement: "PUBLIC_HOME_BANNER"}, {Placement: "DASHBOARD_SUMMARY"}}}
	published, err := saveAnnouncement(ctx, tx, admin, publish)
	if err != nil || published.State != "PUBLISHED" || !visible(draft.ID, 0, false) {
		t.Fatal("direct publish", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO content.announcement_reads(newapi_user_id,announcement_id,notification_revision) VALUES($1,$2,1)`, regular, draft.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE identity.users SET role=10 WHERE id=$1`, regular); err != nil {
		t.Fatal(err)
	}
	second, err := saveAnnouncement(ctx, tx, regular, create)
	if err != nil {
		t.Fatal("local role 10 without legacy principal", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE identity.users SET role=1 WHERE id=$1`, regular); err != nil {
		t.Fatal(err)
	}
	publish.ID = second.ID
	if _, err = saveAnnouncement(ctx, tx, admin, publish); !errors.Is(err, ErrAnnouncementWindow) {
		t.Fatal("overlapping banner", err)
	}
	later, laterUntil := until.Add(time.Hour), until.Add(2*time.Hour)
	publish.Action = "SCHEDULE"
	publish.PublishAt = &later
	publish.VisibleFrom = &later
	publish.VisibleUntil = &laterUntil
	scheduled, err := saveAnnouncement(ctx, tx, admin, publish)
	if err != nil || scheduled.State != "SCHEDULED" || visible(second.ID, 0, false) {
		t.Fatal("scheduled window", err)
	}
	var jobs int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM content.announcement_jobs WHERE announcement_id=$1 AND status='PENDING'`, second.ID).Scan(&jobs); err != nil || jobs != 2 {
		t.Fatal("durable publish/expiry jobs", jobs, err)
	}
	content.Markdown = "Updated without notifying again"
	updated, err := saveAnnouncement(ctx, tx, admin, AnnouncementCommand{ID: draft.ID, Action: "UPDATE_CONTENT_ONLY", Content: &content})
	if err != nil || updated.ContentVersion != 2 || updated.NotificationRevision != 1 || updated.HTML != "<p>Updated without notifying again</p>\n" {
		t.Fatal("content-only edit", err)
	}
	updated, err = saveAnnouncement(ctx, tx, admin, AnnouncementCommand{ID: draft.ID, Action: "UPDATE_PLACEMENTS", Placements: []AnnouncementPlacement{{Placement: "POST_LOGIN_POPUP"}}})
	if err != nil || updated.ContentVersion != 2 || updated.NotificationRevision != 1 || len(updated.Placements) != 1 {
		t.Fatal("direct placement save", err)
	}
	var read bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM content.announcement_reads WHERE newapi_user_id=$1 AND announcement_id=$2 AND notification_revision=$3)`, regular, draft.ID, updated.NotificationRevision).Scan(&read); err != nil || !read {
		t.Fatal("content/placement edit reset read", err)
	}
	updated, err = saveAnnouncement(ctx, tx, admin, AnnouncementCommand{ID: draft.ID, Action: "RE_NOTIFY"})
	if err != nil || updated.NotificationRevision != 2 || updated.ContentVersion != 2 {
		t.Fatal("direct re-notify without reason", err)
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM content.announcement_reads WHERE newapi_user_id=$1 AND announcement_id=$2 AND notification_revision=$3)`, regular, draft.ID, updated.NotificationRevision).Scan(&read); err != nil || read {
		t.Fatal("re-notify retained old read", err)
	}
	updated, err = saveAnnouncement(ctx, tx, admin, AnnouncementCommand{ID: draft.ID, Action: "ARCHIVE"})
	if err != nil || updated.State != "ARCHIVED" || visible(draft.ID, 0, false) || !visible(draft.ID, 0, true) {
		t.Fatal("archive visibility", err)
	}
	updated, err = saveAnnouncement(ctx, tx, admin, AnnouncementCommand{ID: draft.ID, Action: "WITHDRAW"})
	if err != nil || updated.WithdrawnAt == nil || visible(draft.ID, 0, true) {
		t.Fatal("withdraw archived notice", err)
	}
	if _, err = saveAnnouncement(ctx, tx, admin, AnnouncementCommand{ID: draft.ID, Action: "UPDATE_CONTENT_ONLY", Content: &content}); !errors.Is(err, ErrAnnouncementConflict) {
		t.Fatal("write withdrawn notice", err)
	}
	var count, oldCount, previewCount int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM ops.audit_log),(SELECT count(*) FROM ops.admin_operations),(SELECT count(*) FROM ops.announcement_previews)`).Scan(&count, &oldCount, &previewCount); err != nil || count != audits+9 || oldCount != legacy || previewCount != previews {
		t.Fatal("one audit per direct write; no preview/legacy operations", count-audits, err)
	}
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT before_state->>'content_version'='1' AND after_state->>'content_version'='2' AND actor_user_id=$2 FROM ops.audit_log WHERE target=$1 AND action='ANNOUNCEMENT_UPDATE_CONTENT_ONLY'`, draft.ID, admin).Scan(&valid); err != nil || !valid {
		t.Fatal("actor and before/after snapshots", err)
	}
	if _, err = tx.Exec(ctx, "SET CONSTRAINTS ALL IMMEDIATE"); err != nil {
		t.Fatal("commit constraints", err)
	}
	if err = tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatal(err)
	}
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM content.announcements WHERE announcement_id=ANY($1::uuid[])`, []string{draft.ID, second.ID}).Scan(&count); err != nil || count != 0 {
		t.Fatal("rollback left announcements", err)
	}
	t.Log("9 direct writes and audit rows verified; content/read notification semantics and slot windows preserved; dev transaction rolled back")
}
