package platform

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func announcementTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("MOMIAO_ANNOUNCEMENTS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated announcements PostgreSQL database required")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil || cfg.Host != "127.0.0.1" || !strings.HasPrefix(cfg.Database, "m3_announcements_") {
		t.Fatal("refusing a nonlocal or non-announcements test database")
	}
	s, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatal("local announcement database unavailable")
	}
	t.Cleanup(s.Close)
	if err = s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func announcementFixture(t *testing.T) (*Store, context.Context, AnnouncementPrincipal) {
	t.Helper()
	s := announcementTestStore(t)
	ctx := context.Background()
	id := time.Now().UnixMicro()
	if err := s.seedAnnouncementPrincipal(ctx, id, "SUPER_ADMIN", false); err != nil {
		t.Fatal(err)
	}
	p, err := s.AnnouncementAuthority(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return s, ctx, p
}
func announcementID(t *testing.T) string {
	t.Helper()
	id, err := uuidV7()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func announcementDraft(t *testing.T, s *Store, ctx context.Context, p AnnouncementPrincipal) AnnouncementResult {
	t.Helper()
	c := AnnouncementCommand{OperationID: announcementID(t), Epoch: p.Epoch, Action: "SAVE", Content: &AnnouncementContent{Title: "Synthetic announcement", Type: "SYSTEM", Visibility: "PUBLIC", Markdown: "## Test\n\nA real stored test body."}}
	r, err := s.ExecuteAnnouncement(ctx, p.UserID, c, "", false)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func announcementPublishCommand(t *testing.T, p AnnouncementPrincipal, r AnnouncementResult) AnnouncementCommand {
	t.Helper()
	now := time.Now().UTC().Add(-time.Minute)
	return AnnouncementCommand{OperationID: announcementID(t), Epoch: p.Epoch, ID: r.ID, ExpectedVersion: r.Version, Action: "PUBLISH", PublishAt: &now, VisibleFrom: &now, Reason: "Synthetic acceptance"}
}
func announcementConfirm(t *testing.T, s *Store, ctx context.Context, p AnnouncementPrincipal, c AnnouncementCommand) AnnouncementResult {
	t.Helper()
	preview, err := s.PrepareAnnouncement(ctx, p.UserID, c)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.ExecuteAnnouncement(ctx, p.UserID, c, preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestAnnouncementLifecycleAndReads(t *testing.T) {
	s, ctx, p := announcementFixture(t)
	r := announcementDraft(t, s, ctx, p)
	if _, err := s.PublicAnnouncement(ctx, 0, r.ID, false); !errors.Is(err, ErrAnnouncementNotFound) {
		t.Fatal("draft leaked", err)
	}
	c := announcementPublishCommand(t, p, r)
	if _, err := s.ExecuteAnnouncement(ctx, p.UserID, c, "", true); !errors.Is(err, ErrAnnouncementConfirmation) {
		t.Fatal("confirmation bypass", err)
	}
	r = announcementConfirm(t, s, ctx, p, c)
	a, err := s.PublicAnnouncement(ctx, p.UserID, r.ID, false)
	if err != nil || a.Read {
		t.Fatal("GET should not mark read", err)
	}
	readAt, err := s.ReadAnnouncement(ctx, p.UserID, r.ID, a.NotificationRevision)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.ReadAnnouncement(ctx, p.UserID, r.ID, a.NotificationRevision)
	if err != nil || !readAt.Equal(again) {
		t.Fatal("read not idempotent", err)
	}
	if other, err := s.PublicAnnouncement(ctx, p.UserID+1, r.ID, false); err != nil || other.Read {
		t.Fatal("cross-account read leak", err)
	}
	c = AnnouncementCommand{OperationID: announcementID(t), Epoch: p.Epoch, ID: r.ID, ExpectedVersion: r.Version, Action: "UPDATE_CONTENT_ONLY", Content: &AnnouncementContent{Title: "Edited synthetic notice", Type: "IMPORTANT", Visibility: "PUBLIC", Markdown: "**Still read**"}}
	r = announcementConfirm(t, s, ctx, p, c)
	a, err = s.PublicAnnouncement(ctx, p.UserID, r.ID, false)
	if err != nil || !a.Read || a.ContentVersion != 2 || a.NotificationRevision != 1 {
		t.Fatal("content update reset read", a, err)
	}
	c = AnnouncementCommand{OperationID: announcementID(t), Epoch: p.Epoch, ID: r.ID, ExpectedVersion: r.Version, Action: "RE_NOTIFY", Reason: "Notify again in synthetic test"}
	r = announcementConfirm(t, s, ctx, p, c)
	if _, err = s.ReadAnnouncement(ctx, p.UserID, r.ID, 1); err != nil {
		t.Fatal(err)
	}
	a, err = s.PublicAnnouncement(ctx, p.UserID, r.ID, false)
	if err != nil || a.Read || a.NotificationRevision != 2 {
		t.Fatal("old read marked new revision", a, err)
	}
	if _, err = s.ReadAnnouncement(ctx, 0, r.ID, 2); !errors.Is(err, ErrAnnouncementForbidden) {
		t.Fatal("guest read accepted", err)
	}
	c = AnnouncementCommand{OperationID: announcementID(t), Epoch: p.Epoch, ID: r.ID, ExpectedVersion: r.Version, Action: "WITHDRAW", Reason: "Synthetic cleanup"}
	announcementConfirm(t, s, ctx, p, c)
	if _, err = s.PublicAnnouncement(ctx, 0, r.ID, true); !errors.Is(err, ErrAnnouncementNotFound) {
		t.Fatal("withdrawn archive leak", err)
	}
}

func TestAnnouncementAuthorityAndAtomicity(t *testing.T) {
	s, ctx, p := announcementFixture(t)
	r := announcementDraft(t, s, ctx, p)
	if _, err := s.AnnouncementAuthority(ctx, p.UserID+42); !errors.Is(err, ErrAnnouncementForbidden) {
		t.Fatal("unassigned identity authorized", err)
	}
	auditID := p.UserID + 1
	if err := s.seedAnnouncementPrincipal(ctx, auditID, "AUDITOR", false); err != nil {
		t.Fatal(err)
	}
	auditor, err := s.AnnouncementAuthority(ctx, auditID)
	if err != nil {
		t.Fatal(err)
	}
	c := announcementPublishCommand(t, auditor, r)
	if _, err = s.PrepareAnnouncement(ctx, auditID, c); !errors.Is(err, ErrAnnouncementForbidden) {
		t.Fatal("auditor may write", err)
	}
	c = announcementPublishCommand(t, p, r)
	preview, err := s.PrepareAnnouncement(ctx, p.UserID, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, "UPDATE ops.admin_principals SET updated_at=now() WHERE newapi_user_id=$1", p.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteAnnouncement(ctx, p.UserID, c, preview.ID, true); !errors.Is(err, ErrAnnouncementStale) {
		t.Fatal("old epoch accepted", err)
	}
	p, err = s.AnnouncementAuthority(ctx, p.UserID)
	if err != nil {
		t.Fatal(err)
	}
	c = announcementPublishCommand(t, p, r)
	preview, err = s.PrepareAnnouncement(ctx, p.UserID, c)
	if err != nil {
		t.Fatal(err)
	}
	var results [2]AnnouncementResult
	var errs [2]error
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() { results[i], errs[i] = s.ExecuteAnnouncement(ctx, p.UserID, c, preview.ID, true) })
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || results[0] != results[1] {
		t.Fatal("duplicate outcome", errs, results)
	}
	var n int
	if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM ops.admin_operations WHERE operation_id=$1", c.OperationID).Scan(&n); err != nil || n != 1 {
		t.Fatal("duplicate audit", n, err)
	}
	c.Reason = "different semantics"
	if _, err = s.ExecuteAnnouncement(ctx, p.UserID, c, preview.ID, true); !errors.Is(err, ErrAnnouncementOperation) {
		t.Fatal("idempotency conflict lost", err)
	}
	if _, err = s.pool.Exec(ctx, "UPDATE content.announcement_revisions SET title='tampered' WHERE announcement_id=$1", r.ID); err == nil {
		t.Fatal("mutable revision")
	}
	if _, err = s.pool.Exec(ctx, "DELETE FROM ops.admin_operations WHERE operation_id=$1", c.OperationID); err == nil {
		t.Fatal("mutable audit")
	}
}

func TestAnnouncementWindowConcurrencyAndRestart(t *testing.T) {
	s, ctx, p := announcementFixture(t)
	if err := s.seedAnnouncementPrincipal(ctx, p.UserID+9, "SUPER_ADMIN", false); err != nil {
		t.Fatal(err)
	}
	p2, err := s.AnnouncementAuthority(ctx, p.UserID+9)
	if err != nil {
		t.Fatal(err)
	}
	actors := []AnnouncementPrincipal{p, p2}
	// An isolated future half-open interval prevents other synthetic test runs sharing placements.
	from := time.Now().UTC().AddDate(60, 0, 0)
	until := from.Add(time.Hour)
	first := announcementDraft(t, s, ctx, p)
	second := announcementDraft(t, s, ctx, p2)
	commands := []AnnouncementCommand{announcementPublishCommand(t, p, first), announcementPublishCommand(t, p2, second)}
	var previews [2]AnnouncementPreview
	for i := range commands {
		commands[i].Action = "SCHEDULE"
		commands[i].PublishAt = &from
		commands[i].VisibleFrom = &from
		commands[i].VisibleUntil = &until
		commands[i].Placements = []AnnouncementPlacement{{Placement: "ENTRY_POPUP"}, {Placement: "PUBLIC_HOME_BANNER"}}
		var err error
		previews[i], err = s.PrepareAnnouncement(ctx, actors[i].UserID, commands[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	var errs [2]error
	var results [2]AnnouncementResult
	for i := range commands {
		wg.Go(func() {
			results[i], errs[i] = s.ExecuteAnnouncement(ctx, actors[i].UserID, commands[i], previews[i].ID, true)
		})
	}
	wg.Wait()
	winner, loser := 0, 1
	if errs[0] != nil {
		winner, loser = 1, 0
	}
	if errs[winner] != nil || !errors.Is(errs[loser], ErrAnnouncementWindow) {
		t.Fatal("competing windows", errs)
	}
	var jobs, audits int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM content.announcement_jobs WHERE announcement_id=$1", commands[loser].ID).Scan(&jobs); err != nil || jobs != 0 {
		t.Fatal("loser got durable jobs", jobs, err)
	}
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM ops.admin_operations WHERE operation_id=$1", commands[loser].OperationID).Scan(&audits); err != nil || audits != 0 {
		t.Fatal("loser got success audit", audits, err)
	}
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM content.announcement_jobs WHERE announcement_id=$1", commands[winner].ID).Scan(&jobs); err != nil || jobs != 2 {
		t.Fatal("scheduled without durable pair", jobs, err)
	}
	// Adjacent windows are permitted, including a second independent principal.
	nextUntil := until.Add(time.Hour)
	c := commands[loser]
	c.OperationID = announcementID(t)
	c.PublishAt = &until
	c.VisibleFrom = &until
	c.VisibleUntil = &nextUntil
	r2 := announcementConfirm(t, s, ctx, actors[loser], c)
	for _, r := range []AnnouncementResult{results[winner], r2} {
		c = AnnouncementCommand{OperationID: announcementID(t), Epoch: p.Epoch, ID: r.ID, ExpectedVersion: r.Version, Action: "WITHDRAW", Reason: "Synthetic window cleanup"}
		announcementConfirm(t, s, ctx, p, c)
	}
	// Emulate downtime only in this dedicated DB: due facts remain PENDING after a dropped connection.
	due := announcementDraft(t, s, ctx, p)
	c = announcementPublishCommand(t, p, due)
	c.Action = "SCHEDULE"
	c.PublishAt = &from
	c.VisibleFrom = &from
	c.VisibleUntil = &until
	due = announcementConfirm(t, s, ctx, p, c)
	if _, err := s.pool.Exec(ctx, `UPDATE content.announcements SET publish_at=now()-interval '2 hours',visible_from=now()-interval '2 hours',visible_until=now()-interval '1 hour' WHERE announcement_id=$1`, due.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE content.announcement_jobs SET due_at=now()-interval '1 hour' WHERE announcement_id=$1`, due.ID); err != nil {
		t.Fatal(err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "SELECT 1 FROM content.announcement_jobs WHERE announcement_id=$1 FOR UPDATE", due.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "UPDATE content.announcements SET state='PUBLISHED' WHERE announcement_id=$1", due.ID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Conn().Close(ctx); err != nil {
		t.Fatal(err)
	} // crash before COMMIT, so both changes roll back
	_ = tx.Rollback(ctx) // release the now-closed pooled connection after the synthetic crash
	reopened, err := Open(ctx, os.Getenv("MOMIAO_ANNOUNCEMENTS_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err = reopened.RunAnnouncementJobs(ctx); err != nil {
		t.Fatal(err)
	}
	_, stored, err := reopened.OpsAnnouncement(ctx, p.UserID, due.ID)
	if err != nil || stored.State != "EXPIRED" || stored.ExpiredReason != "MISSED_PUBLISH_WINDOW" || stored.FirstPublishedAt != nil {
		t.Fatal("missed window recovered incorrectly", stored, err)
	}
	if _, err = reopened.PublicAnnouncement(ctx, 0, due.ID, true); !errors.Is(err, ErrAnnouncementNotFound) {
		t.Fatal("missed window leaked into archive", err)
	}
	if count, err := reopened.RunAnnouncementJobs(ctx); err != nil || count != 0 {
		t.Fatal("job repeated", count, err)
	}
}

func TestAnnouncementTimeVisibilityAndPermissionScopes(t *testing.T) {
	s, ctx, p := announcementFixture(t)
	r := announcementDraft(t, s, ctx, p)
	c := announcementPublishCommand(t, p, r)
	r = announcementConfirm(t, s, ctx, p, c)
	// Live-time filtering is authoritative even if the expiry worker has not executed.
	if _, err := s.pool.Exec(ctx, `UPDATE content.announcements SET publish_at=now()-interval '2 hours',visible_from=now()-interval '2 hours',visible_until=now()-interval '1 second' WHERE announcement_id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PublicAnnouncement(ctx, 0, r.ID, false); !errors.Is(err, ErrAnnouncementNotFound) {
		t.Fatal("expired content in active detail", err)
	}
	a, err := s.PublicAnnouncement(ctx, 0, r.ID, true)
	if err != nil || a.State != "EXPIRED" {
		t.Fatal("archive missing", err)
	}
	c = AnnouncementCommand{OperationID: announcementID(t), Epoch: p.Epoch, ID: r.ID, ExpectedVersion: r.Version, Action: "RE_NOTIFY", Reason: "must not notify expired window"}
	if _, err = s.PrepareAnnouncement(ctx, p.UserID, c); !errors.Is(err, ErrAnnouncementInvalid) {
		t.Fatal("expired re-notify accepted", err)
	}
	operatorID := p.UserID + 10
	if err = s.seedAnnouncementPrincipal(ctx, operatorID, "OPERATOR", false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AnnouncementAuthority(ctx, operatorID); !errors.Is(err, ErrAnnouncementForbidden) {
		t.Fatal("unscoped operator accepted", err)
	}
	if _, err = s.pool.Exec(ctx, `INSERT INTO ops.admin_principal_scopes SELECT admin_principal_id,'ANNOUNCEMENTS' FROM ops.admin_principals WHERE newapi_user_id=$1`, operatorID); err != nil {
		t.Fatal(err)
	}
	op, err := s.AnnouncementAuthority(ctx, operatorID)
	if err != nil || op.Epoch != 2 {
		t.Fatal("scope epoch", op, err)
	}
	draft := announcementDraft(t, s, ctx, op)
	if _, err = s.pool.Exec(ctx, `DELETE FROM ops.admin_principal_scopes WHERE admin_principal_id=(SELECT admin_principal_id FROM ops.admin_principals WHERE newapi_user_id=$1)`, operatorID); err != nil {
		t.Fatal(err)
	}
	c = announcementPublishCommand(t, op, draft)
	if _, err = s.PrepareAnnouncement(ctx, operatorID, c); !errors.Is(err, ErrAnnouncementForbidden) {
		t.Fatal("revoked scope accepted", err)
	}
}

func TestAnnouncementSameVersionAndAuditRollback(t *testing.T) {
	s, ctx, p := announcementFixture(t)
	r := announcementDraft(t, s, ctx, p)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		c := AnnouncementCommand{OperationID: announcementID(t), Epoch: p.Epoch, ID: r.ID, ExpectedVersion: r.Version, Action: "SAVE", Content: &AnnouncementContent{Title: "Concurrent edit", Type: "SYSTEM", Visibility: "PUBLIC", Markdown: "A new version"}}
		wg.Go(func() { _, errs[i] = s.ExecuteAnnouncement(ctx, p.UserID, c, "", false) })
	}
	wg.Wait()
	if !(errs[0] == nil && errors.Is(errs[1], ErrAnnouncementConflict) || errs[1] == nil && errors.Is(errs[0], ErrAnnouncementConflict)) {
		t.Fatal("two edits accepted same version", errs)
	}
	// Force the audit insert to fail. The draft insert and revision must roll back with it.
	if _, err := s.pool.Exec(ctx, `CREATE FUNCTION ops.m3_test_reject_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='SAVE' THEN RAISE EXCEPTION 'synthetic audit failure'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER m3_test_audit_failure BEFORE INSERT ON ops.admin_operations FOR EACH ROW EXECUTE FUNCTION ops.m3_test_reject_audit()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(ctx, `DROP TRIGGER IF EXISTS m3_test_audit_failure ON ops.admin_operations; DROP FUNCTION IF EXISTS ops.m3_test_reject_audit()`)
	})
	var before, after int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM content.announcements").Scan(&before); err != nil {
		t.Fatal(err)
	}
	c := AnnouncementCommand{OperationID: announcementID(t), Epoch: p.Epoch, Action: "SAVE", Content: &AnnouncementContent{Title: "Audit rollback", Type: "SYSTEM", Visibility: "PUBLIC", Markdown: "Must not survive"}}
	if _, err := s.ExecuteAnnouncement(ctx, p.UserID, c, "", false); err == nil {
		t.Fatal("audit failure ignored")
	}
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM content.announcements").Scan(&after); err != nil || before != after {
		t.Fatal("mutation survived failed audit", before, after, err)
	}
}

func TestAnnouncementRevocationWaiterCannotGuessNewEpoch(t *testing.T) {
	for _, action := range []string{"PREPARE", "EXECUTE"} {
		t.Run(action, func(t *testing.T) {
			s := announcementTestStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			user := time.Now().UnixMicro()
			if err := s.seedAnnouncementPrincipal(ctx, user, "OPERATOR", true); err != nil {
				t.Fatal(err)
			}
			p, err := s.AnnouncementAuthority(ctx, user)
			if err != nil {
				t.Fatal(err)
			}
			draft := announcementDraft(t, s, ctx, p)
			c := announcementPublishCommand(t, p, draft)
			c.Epoch = p.Epoch + 1
			if action == "EXECUTE" {
				c.Action = "SAVE"
				c.PublishAt = nil
				c.VisibleFrom = nil
				c.Content = &AnnouncementContent{Title: "Must be denied after revoke", Type: "SYSTEM", Visibility: "PUBLIC", Markdown: "No longer authorized"}
			}
			revoke, err := s.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer rollback(revoke)
			pid := int(revoke.Conn().PgConn().PID())
			if _, err = revoke.Exec(ctx, `DELETE FROM ops.admin_principal_scopes WHERE admin_principal_id=(SELECT admin_principal_id FROM ops.admin_principals WHERE newapi_user_id=$1)`, user); err != nil {
				t.Fatal(err)
			}
			outcome := make(chan error, 1)
			go func() {
				if action == "PREPARE" {
					_, err := s.PrepareAnnouncement(ctx, user, c)
					outcome <- err
				} else {
					_, err := s.ExecuteAnnouncement(ctx, user, c, "", false)
					outcome <- err
				}
			}()
			// Observe the actual database lock waiter before allowing the revocation to commit.
			waiting := false
			for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
				if err = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)) AND query LIKE '%FROM ops.admin_principals p%')`, pid).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !waiting {
				t.Fatal("did not observe authority request waiting on revocation row lock")
			}
			if err = revoke.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-outcome; !errors.Is(err, ErrAnnouncementForbidden) {
				t.Errorf("revoked scope authorized with guessed new epoch: %v", err)
			}
			var facts int
			if err = s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM ops.admin_operations WHERE operation_id=$1)+(SELECT count(*) FROM ops.announcement_previews WHERE command_hash=$2)`, c.OperationID, announcementHash(c)).Scan(&facts); err != nil || facts != 0 {
				t.Errorf("unauthorized success facts=%d error=%v", facts, err)
			}
		})
	}
}

func TestAnnouncementSchema(t *testing.T) {
	s := announcementTestStore(t)
	for _, table := range []string{"content.announcements", "content.announcement_revisions", "content.notification_revisions", "content.announcement_placements", "content.placement_guards", "content.announcement_reads", "content.announcement_jobs", "ops.admin_principals", "ops.admin_principal_scopes", "ops.announcement_previews", "ops.admin_operations"} {
		var exists bool
		if err := s.pool.QueryRow(context.Background(), "SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists); err != nil || !exists {
			t.Errorf("missing %s", table)
		}
	}
}

func TestAnnouncementTargetWaiterUsesLockedContent(t *testing.T) {
	s, _, p := announcementFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	r := announcementDraft(t, s, ctx, p)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(tx)
	pid := int(tx.Conn().PgConn().PID())
	content := AnnouncementContent{Title: "Now private", Type: "SYSTEM", Visibility: "AUTHENTICATED", Markdown: "Private synthetic content"}
	rendered, err := validAnnouncementContent(&content)
	if err != nil {
		t.Fatal(err)
	}
	if err = insertAnnouncementContent(ctx, tx, r.ID, 2, p.UserID, content, rendered); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "UPDATE content.announcements SET current_content_version=2,version=2 WHERE announcement_id=$1", r.ID); err != nil {
		t.Fatal(err)
	}
	c := announcementPublishCommand(t, p, r)
	c.ExpectedVersion = 2
	c.Placements = []AnnouncementPlacement{{Placement: "ENTRY_POPUP"}}
	outcome := make(chan error, 1)
	go func() { _, err := s.PrepareAnnouncement(ctx, p.UserID, c); outcome <- err }()
	waiting := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if err = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, pid).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("target lock waiter not observed")
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-outcome; !errors.Is(err, ErrAnnouncementInvalid) {
		t.Fatal("stale public content authorized a private entry placement", err)
	}
}

func TestAnnouncementLivePlacementsPreserveVersions(t *testing.T) {
	s, ctx, p := announcementFixture(t)
	r := announcementConfirm(t, s, ctx, p, announcementPublishCommand(t, p, announcementDraft(t, s, ctx, p)))
	if _, err := s.ReadAnnouncement(ctx, p.UserID, r.ID, r.NotificationRevision); err != nil {
		t.Fatal(err)
	}
	c := AnnouncementCommand{OperationID: announcementID(t), Epoch: p.Epoch, ID: r.ID, ExpectedVersion: r.Version, Action: "UPDATE_PLACEMENTS", Placements: []AnnouncementPlacement{{Placement: "PINNED_LIST", ManualOrder: 4}, {Placement: "DASHBOARD_SUMMARY"}}}
	next := announcementConfirm(t, s, ctx, p, c)
	a, err := s.PublicAnnouncement(ctx, p.UserID, r.ID, false)
	if err != nil || !a.Pinned || !a.Read || next.ContentVersion != r.ContentVersion || next.NotificationRevision != r.NotificationRevision || next.Version != r.Version+1 {
		t.Fatal("placement edit changed content/read state", next, a, err)
	}
	c.OperationID = announcementID(t)
	c.ExpectedVersion = next.Version
	c.Placements = []AnnouncementPlacement{}
	announcementConfirm(t, s, ctx, p, c)
	a, err = s.PublicAnnouncement(ctx, p.UserID, r.ID, false)
	if err != nil || a.Pinned || !a.Read {
		t.Fatal("independent placement removal failed", a, err)
	}
}

func TestAnnouncementPublishedProjectionAndConfirmation(t *testing.T) {
	s, ctx, p := announcementFixture(t)
	tag := announcementID(t)
	ids := make([]string, 4)
	for i := range ids {
		content := &AnnouncementContent{Title: tag, Type: "SYSTEM", Visibility: "PUBLIC", Markdown: "Projection fixture"}
		if i == 3 {
			content.Visibility = "AUTHENTICATED"
		}
		r, err := s.ExecuteAnnouncement(ctx, p.UserID, AnnouncementCommand{OperationID: announcementID(t), Epoch: p.Epoch, Action: "SAVE", Content: content}, "", false)
		if err != nil {
			t.Fatal(err)
		}
		c := announcementPublishCommand(t, p, r)
		if i < 2 {
			c.Placements = []AnnouncementPlacement{{Placement: "PINNED_LIST", ManualOrder: 10 - i}}
		}
		c.Placements = append(c.Placements, AnnouncementPlacement{Placement: "POST_LOGIN_POPUP"})
		r = announcementConfirm(t, s, ctx, p, c)
		ids[i] = r.ID
	}
	page, err := s.PublicAnnouncements(ctx, 0, AnnouncementFilter{Search: tag, Limit: 2})
	if err != nil || len(page.Items) != 2 || !page.HasMore || page.Items[0].ID != ids[1] || page.Items[1].ID != ids[0] {
		t.Fatal("public pinned order/page", page, err)
	}
	page, err = s.PublicAnnouncements(ctx, 0, AnnouncementFilter{Search: tag, Limit: 2, Offset: 2})
	if err != nil || len(page.Items) != 1 || page.HasMore || page.Items[0].ID != ids[2] {
		t.Fatal("public pagination/visibility", page, err)
	}
	if _, err = s.PublicAnnouncement(ctx, 0, ids[3], false); !errors.Is(err, ErrAnnouncementNotFound) {
		t.Fatal("private detail leaked", err)
	}
	page, err = s.PublicAnnouncements(ctx, p.UserID, AnnouncementFilter{Search: tag, Placement: "POST_LOGIN_POPUP"})
	if err != nil || len(page.Items) != 4 {
		t.Fatal("authenticated candidates", page, err)
	}
	if _, err = s.ReadAnnouncement(ctx, p.UserID, ids[3], 1); err != nil {
		t.Fatal(err)
	}
	page, err = s.PublicAnnouncements(ctx, p.UserID, AnnouncementFilter{Search: tag, Placement: "POST_LOGIN_POPUP"})
	if err != nil || len(page.Items) != 3 {
		t.Fatal("read candidate not removed", page, err)
	}
	page, err = s.PublicAnnouncements(ctx, 0, AnnouncementFilter{Search: tag, Type: "IMPORTANT"})
	if err != nil || len(page.Items) != 0 {
		t.Fatal("type filter", page, err)
	}
	future := time.Now().Add(time.Hour)
	page, err = s.PublicAnnouncements(ctx, 0, AnnouncementFilter{Search: tag, DateFrom: &future})
	if err != nil || len(page.Items) != 0 {
		t.Fatal("date filter", page, err)
	}
	_, a, err := s.OpsAnnouncement(ctx, p.UserID, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	c := AnnouncementCommand{OperationID: announcementID(t), Epoch: p.Epoch, ID: a.ID, ExpectedVersion: a.Version, Action: "RE_NOTIFY", Reason: "Preview binding"}
	preview, err := s.PrepareAnnouncement(ctx, p.UserID, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteAnnouncement(ctx, p.UserID, c, preview.ID, false); !errors.Is(err, ErrAnnouncementConfirmation) {
		t.Fatal("explicit confirmation bypass", err)
	}
	c.Reason = "Changed after preview"
	if _, err = s.ExecuteAnnouncement(ctx, p.UserID, c, preview.ID, true); !errors.Is(err, ErrAnnouncementConfirmation) {
		t.Fatal("preview body binding bypass", err)
	}
	expiredID := announcementID(t)
	if _, err = s.pool.Exec(ctx, `INSERT INTO ops.announcement_previews(preview_id,newapi_user_id,authz_epoch,command_hash,impact,expires_at) VALUES($1,$2,$3,$4,'{}',now()-interval '1 second')`, expiredID, p.UserID, p.Epoch, announcementHash(c)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteAnnouncement(ctx, p.UserID, c, expiredID, true); !errors.Is(err, ErrAnnouncementConfirmation) {
		t.Fatal("expired preview accepted", err)
	}
	var facts int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM ops.admin_operations WHERE operation_id=$1`, c.OperationID).Scan(&facts); err != nil || facts != 0 {
		t.Fatal("failed confirmation committed", facts, err)
	}
}

func TestAnnouncementCanonicalAcknowledgements(t *testing.T) {
	s, ctx, p := announcementFixture(t)
	content := AnnouncementContent{Title: "Synthetic acknowledgements", Type: "ACKNOWLEDGEMENTS", Visibility: "AUTHENTICATED", Markdown: "Test-only acknowledgements; no actual contributors."}
	c := AnnouncementCommand{OperationID: announcementID(t), Epoch: p.Epoch, Action: "SAVE", Content: &content}
	if _, err := s.ExecuteAnnouncement(ctx, p.UserID, c, "", false); !errors.Is(err, ErrAnnouncementInvalid) {
		t.Fatal("private canonical created with public default placements", err)
	}
	content.Visibility = "PUBLIC"
	var existing int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM content.announcements WHERE canonical_key='ACKNOWLEDGEMENTS'").Scan(&existing); err != nil {
		t.Fatal(err)
	}
	if existing == 0 {
		c.OperationID = announcementID(t)
		if _, err := s.ExecuteAnnouncement(ctx, p.UserID, c, "", false); err != nil {
			t.Fatal(err)
		}
	}
	c.OperationID = announcementID(t)
	if _, err := s.ExecuteAnnouncement(ctx, p.UserID, c, "", false); !errors.Is(err, ErrAnnouncementConflict) {
		t.Fatal("second canonical accepted", err)
	}
	var count int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM ops.admin_operations WHERE operation_id=$1", c.OperationID).Scan(&count); err != nil || count != 0 {
		t.Fatal("rejected canonical audit committed", count, err)
	}
}

func TestAnnouncementScheduledRecoveryPublishesAndExpires(t *testing.T) {
	s, ctx, p := announcementFixture(t)
	r := announcementDraft(t, s, ctx, p)
	from := time.Now().UTC().Add(time.Hour)
	until := from.Add(time.Hour)
	c := announcementPublishCommand(t, p, r)
	c.Action = "SCHEDULE"
	c.PublishAt = &from
	c.VisibleFrom = &from
	c.VisibleUntil = &until
	r = announcementConfirm(t, s, ctx, p, c)
	if _, err := s.pool.Exec(ctx, `UPDATE content.announcements SET publish_at=now()-interval '1 minute',visible_from=now()-interval '1 minute',visible_until=now()+interval '1 hour' WHERE announcement_id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE content.announcement_jobs SET due_at=now()-interval '1 minute' WHERE announcement_id=$1 AND kind='PUBLISH'`, r.ID); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, os.Getenv("MOMIAO_ANNOUNCEMENTS_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err = reopened.RunAnnouncementJobs(ctx); err != nil {
		t.Fatal(err)
	}
	a, err := reopened.PublicAnnouncement(ctx, 0, r.ID, false)
	if err != nil || a.State != "PUBLISHED" {
		t.Fatal("durable publish failed after reopening", a, err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE content.announcements SET visible_until=now()-interval '1 second' WHERE announcement_id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE content.announcement_jobs SET due_at=now()-interval '1 second' WHERE announcement_id=$1 AND kind='EXPIRE'`, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.RunAnnouncementJobs(ctx); err != nil {
		t.Fatal(err)
	}
	_, stored, err := reopened.OpsAnnouncement(ctx, p.UserID, r.ID)
	if err != nil || stored.State != "EXPIRED" || stored.FirstPublishedAt == nil || stored.ExpiredReason != "VISIBLE_WINDOW_ENDED" {
		t.Fatal("durable expiry failed", stored, err)
	}
	if _, err = reopened.PublicAnnouncement(ctx, 0, r.ID, true); err != nil {
		t.Fatal("published history missing", err)
	}
	var facts int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM ops.admin_operations WHERE announcement_id=$1 AND actor_kind='SYSTEM'`, r.ID).Scan(&facts); err != nil || facts != 2 {
		t.Fatal("job audit count", facts, err)
	}
}
