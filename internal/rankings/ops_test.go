package rankings

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/platform"
)

func TestLocalOpsRankings(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires existing isolated dev database")
	}
	dsn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store, err := platform.Open(t.Context(), strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("open dev database")
	}
	defer store.Close()
	ctx := t.Context()
	tx, err := store.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var admin, regular int64
	if err = tx.QueryRow(ctx, `SELECT min(id) FILTER(WHERE role>=10),min(id) FILTER(WHERE role<10) FROM identity.users WHERE status=1 AND deleted_at IS NULL`).Scan(&admin, &regular); err != nil {
		t.Fatal(err)
	}
	var activation, start time.Time
	var end *time.Time
	if err = tx.QueryRow(ctx, `SELECT activation_at,period_start,period_end FROM rankings.snapshots s WHERE domain='GAMES' AND period='DAY' AND EXISTS(SELECT 1 FROM rankings.entries e WHERE e.snapshot_id=s.snapshot_id AND metric='GAME_PROFIT') ORDER BY built_at DESC LIMIT 1`).Scan(&activation, &start, &end); err != nil {
		t.Fatal(err)
	}
	// Exercise an ordinary local operator, not root-only publication rights.
	if _, err = tx.Exec(ctx, `UPDATE identity.users SET role=10 WHERE id=$1`, admin); err != nil {
		t.Fatal(err)
	}
	service := NewService(store, Options{ActivationTime: activation})
	input := OpsRebuildInput{Domain: "GAMES", Metric: "GAME_PROFIT", Period: "DAY", Date: start.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")}
	if _, err = service.createRebuild(ctx, tx, regular, input); !errors.Is(err, platform.ErrCatalogForbidden) {
		t.Fatal("regular write", err)
	}
	oldID, oldVersion, err := readPublishedPointer(ctx, tx, "GAMES", "GAME_PROFIT", "DAY", start, activation, false)
	if err != nil {
		t.Fatal(err)
	}
	job, err := service.createRebuild(ctx, tx, admin, input)
	if err != nil {
		t.Fatal("direct rebuild", err)
	}
	if _, err = service.createRebuild(ctx, tx, admin, input); !errors.Is(err, platform.ErrOpsConflict) {
		t.Fatal("duplicate pending rebuild", err)
	}
	shadow, err := service.buildSnapshotTx(ctx, tx, "GAMES", "DAY", start, end, time.Now().UTC(), nil, "REPAIR", job.JobID)
	if err != nil {
		t.Fatal("existing game aggregation", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE rankings.rebuild_jobs SET status='SHADOW',shadow_snapshot_id=$2,attempts=1 WHERE operation_id=$1::uuid`, job.JobID, shadow); err != nil {
		t.Fatal(err)
	}
	id, version, err := readPublishedPointer(ctx, tx, "GAMES", "GAME_PROFIT", "DAY", start, activation, false)
	if err != nil || id != oldID || version != oldVersion {
		t.Fatal("shadow must not publish", id, version, err)
	}
	diff, err := rankingDiff(ctx, tx, oldID, shadow, "GAME_PROFIT")
	if err != nil || len(diff.NewHash) != 64 || diff.NewCount == 0 {
		t.Fatal("real snapshot diff", diff, err)
	}
	if err = service.publishRepair(ctx, tx, admin, shadow); err != nil {
		t.Fatal("direct publish", err)
	}
	id, version, err = readPublishedPointer(ctx, tx, "GAMES", "GAME_PROFIT", "DAY", start, activation, false)
	if err != nil || id != shadow || version != oldVersion+1 {
		t.Fatal("published pointer", id, version, err)
	}
	if err = service.publishRepair(ctx, tx, admin, shadow); !errors.Is(err, platform.ErrOpsConflict) {
		t.Fatal("already published", err)
	}
	var audits, events int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM ops.audit_log WHERE actor_user_id=$1 AND ((action='RANKING_REBUILD_CREATE' AND target=$2) OR (action='RANKING_REPAIR_PUBLISH' AND target=$3))`, admin, job.JobID, shadow).Scan(&audits); err != nil || audits != 2 {
		t.Fatal("transaction audit", audits, err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM rankings.publication_events WHERE snapshot_id=$1::uuid AND actor_user_id=$2 AND source='REPAIR'`, shadow, admin).Scan(&events); err != nil || events != 1 {
		t.Fatal("publication history", events, err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var remains int
	if err = store.Pool().QueryRow(ctx, `SELECT (SELECT count(*) FROM rankings.rebuild_jobs WHERE operation_id=$1::uuid)+(SELECT count(*) FROM rankings.snapshots WHERE snapshot_id=$2::uuid)+(SELECT count(*) FROM ops.audit_log WHERE target IN($1::text,$2::text))`, job.JobID, shadow).Scan(&remains); err != nil || remains != 0 {
		t.Fatal("transaction rollback", remains, err)
	}
	t.Logf("LOCAL_RANKINGS_TX: actor_role=10; shadow=%d entries; pointer +1; audits=2; publication=1; rollback remaining=0", diff.NewCount)
}
