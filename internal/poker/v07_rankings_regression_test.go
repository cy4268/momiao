package poker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/rankings"
	"github.com/jackc/pgx/v5/pgxpool"
)

type v07RankingAssets struct {
	values   map[int64]string
	observed time.Time
	failure  error
	block    bool
	entered  chan struct{}
}

func (a *v07RankingAssets) ReadRankingAssets(ctx context.Context, user int64) (string, time.Time, error) {
	if a.block {
		select {
		case a.entered <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return "", time.Time{}, ctx.Err()
	}
	if a.failure != nil {
		return "", time.Time{}, a.failure
	}
	value, ok := a.values[user]
	if !ok {
		return "", time.Time{}, errors.New("synthetic ranking asset is missing")
	}
	return value, a.observed, nil
}

func v07RankingPointer(t *testing.T, ctx context.Context, owner *pgxpool.Pool, metric, period string, activation time.Time) (string, int64) {
	t.Helper()
	var id string
	var version int64
	v07Must(t, owner.QueryRow(ctx, `SELECT snapshot_id::text,version FROM rankings.published_pointers
		WHERE metric=$1 AND period=$2 AND activation_at=$3`, metric, period, activation).Scan(&id, &version))
	return id, version
}

// built_at and snapshot_id are deliberately outside the canonical aggregate
// digest. Copying every hashed field and entry therefore makes a valid aged
// fixture without weakening the production immutability triggers.
func v07AgedRankingSnapshot(t *testing.T, ctx context.Context, owner *pgxpool.Pool, source string) string {
	t.Helper()
	var id string
	v07Must(t, owner.QueryRow(ctx, `INSERT INTO rankings.snapshots(snapshot_id,domain,period,
		period_start,period_end,activation_at,built_at,source_checked_at,status,build_kind,operation_id,aggregate_hash)
		SELECT gen_random_uuid(),domain,period,period_start,period_end,activation_at,
		 clock_timestamp()-interval '6 minutes',source_checked_at,status,build_kind,NULL,aggregate_hash
		FROM rankings.snapshots WHERE snapshot_id=$1::uuid RETURNING snapshot_id::text`, source).Scan(&id))
	_, err := owner.Exec(ctx, `INSERT INTO rankings.entries(snapshot_id,metric,model_id,model_scope,
		newapi_user_id,display_name,avatar_id,value,calls,errors,credits_units,models)
		SELECT $2::uuid,metric,model_id,model_scope,newapi_user_id,display_name,avatar_id,
		 value,calls,errors,credits_units,models FROM rankings.entries WHERE snapshot_id=$1::uuid`, source, id)
	v07Must(t, err)
	var hashBytes int
	v07Must(t, owner.QueryRow(ctx, `SELECT octet_length(aggregate_hash) FROM rankings.snapshots
		WHERE snapshot_id=$1::uuid`, id).Scan(&hashBytes))
	if hashBytes != 32 {
		t.Fatal("aged ranking fixture lost its canonical aggregate hash")
	}
	return id
}

func TestV07RankingsPublicationAndRecovery(t *testing.T) {
	owner, _ := localPokerDB(t)
	store := v07OwnerStore(t, owner)
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()

	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	localNow := time.Now().In(zone)
	historicalStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, zone).AddDate(0, 0, -1)
	activation := historicalStart.Add(-time.Hour).UTC()
	assets := &v07RankingAssets{
		values: map[int64]string{
			910001: "9007199254740993",
			910002: "7",
			910003: "0",
		},
		observed: time.Now().UTC(),
	}
	service := rankings.NewService(store, rankings.Options{ActivationTime: activation, Assets: assets})
	query := rankings.Query{Metric: "TOTAL_ASSETS", Period: "CURRENT", Page: 1}

	page, err := service.Read(ctx, query, 0)
	v07Must(t, err)
	if page.State != "UNAVAILABLE" || len(page.Items) != 0 || page.Total != 0 {
		t.Fatal("missing publication pointer did not fail closed")
	}

	firstID, err := service.Build(ctx, "ASSETS", "CURRENT", "")
	v07Must(t, err)
	page, err = service.Read(ctx, query, 0)
	v07Must(t, err)
	if firstID == "" || page.State != "READY" || page.Total != 2 || len(page.Items) != 2 ||
		page.Items[0].Value != "9007199254740993" || page.MyRank != nil {
		t.Fatal("nonempty public asset ranking was not published exactly")
	}
	pointerID, pointerVersion := v07RankingPointer(t, ctx, owner, "TOTAL_ASSETS", "CURRENT", activation)
	if pointerID != firstID || pointerVersion != 1 {
		t.Fatal("first routine build did not install version-one pointer")
	}
	var snapshots, entries int64
	v07Must(t, owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM rankings.snapshots WHERE domain='ASSETS'),
		(SELECT count(*) FROM rankings.entries WHERE snapshot_id=$1::uuid)`, firstID).Scan(&snapshots, &entries))

	assets.failure = errors.New("bounded synthetic asset source failure")
	if failedID, buildErr := service.Build(ctx, "ASSETS", "CURRENT", ""); failedID != "" || !errors.Is(buildErr, rankings.ErrUnavailable) {
		t.Fatal("asset source failure was not bounded", failedID, buildErr)
	}
	var afterSnapshots, afterEntries int64
	v07Must(t, owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM rankings.snapshots WHERE domain='ASSETS'),
		(SELECT count(*) FROM rankings.entries WHERE snapshot_id=$1::uuid)`, firstID).Scan(&afterSnapshots, &afterEntries))
	afterID, afterVersion := v07RankingPointer(t, ctx, owner, "TOTAL_ASSETS", "CURRENT", activation)
	if afterSnapshots != snapshots || afterEntries != entries || afterID != pointerID || afterVersion != pointerVersion {
		t.Fatal("failed asset build changed snapshots, entries, or published pointer")
	}

	assets.failure = nil
	assets.values[910002] = "17"
	assets.observed = time.Now().UTC()
	recoveredID, err := service.Build(ctx, "ASSETS", "CURRENT", "")
	v07Must(t, err)
	recoveredPointer, recoveredVersion := v07RankingPointer(t, ctx, owner, "TOTAL_ASSETS", "CURRENT", activation)
	page, err = service.Read(ctx, query, 910002)
	v07Must(t, err)
	if recoveredID == firstID || recoveredPointer != recoveredID || recoveredVersion != pointerVersion+1 ||
		page.State != "READY" || page.MyRank == nil || page.MyRank.Value != "17" {
		t.Fatal("healthy source did not recover publication and private own-rank lookup")
	}

	agedCurrent := v07AgedRankingSnapshot(t, ctx, owner, recoveredID)
	tag, err := owner.Exec(ctx, `UPDATE rankings.published_pointers SET snapshot_id=$1::uuid,
		version=version+1,published_at=clock_timestamp() WHERE domain='ASSETS' AND metric='TOTAL_ASSETS'
		AND period='CURRENT' AND activation_at=$2`, agedCurrent, activation)
	v07Must(t, err)
	if tag.RowsAffected() != 1 {
		t.Fatal("aged current fixture did not replace exactly one pointer")
	}
	page, err = service.Read(ctx, query, 0)
	v07Must(t, err)
	if page.State != "STALE" || page.Total != 2 || page.Items[0].Value != "9007199254740993" {
		t.Fatal("aged active-period snapshot was not served as STALE")
	}
	agedPointer, agedVersion := v07RankingPointer(t, ctx, owner, "TOTAL_ASSETS", "CURRENT", activation)

	historicalDate := historicalStart.Format("2006-01-02")
	historicalID, err := service.Build(ctx, "GAMES", "DAY", historicalDate)
	v07Must(t, err)
	agedHistorical := v07AgedRankingSnapshot(t, ctx, owner, historicalID)
	tag, err = owner.Exec(ctx, `UPDATE rankings.published_pointers SET snapshot_id=$1::uuid,
		version=version+1,published_at=clock_timestamp() WHERE domain='GAMES' AND metric='GAME_PROFIT'
		AND period='DAY' AND activation_at=$2`, agedHistorical, activation)
	v07Must(t, err)
	if tag.RowsAffected() != 1 {
		t.Fatal("aged historical fixture did not replace exactly one pointer")
	}
	historical, err := service.Read(ctx, rankings.Query{Metric: "GAME_PROFIT", Period: "DAY", Date: historicalDate, Page: 1}, 0)
	v07Must(t, err)
	if historical.State != "READY" || historical.PeriodEnd == nil || historical.PeriodEnd.After(time.Now()) {
		t.Fatal("expired historical period was incorrectly marked STALE")
	}

	insertJob := func(operation string) {
		t.Helper()
		_, insertErr := owner.Exec(ctx, `INSERT INTO rankings.rebuild_jobs(operation_id,actor_user_id,
			domain,metric,period,requested_date,model_scope,model_id,expected_pointer_version)
			VALUES($1::uuid,910001,'ASSETS','TOTAL_ASSETS','CURRENT','','ALL','',$2)`, operation, agedVersion)
		v07Must(t, insertErr)
	}
	readJob := func(operation string) (string, string, int64, *string) {
		t.Helper()
		var status, failure string
		var attempts int64
		var shadow *string
		v07Must(t, owner.QueryRow(ctx, `SELECT status,coalesce(failure_code,''),attempts,
			shadow_snapshot_id::text FROM rankings.rebuild_jobs WHERE operation_id=$1::uuid`, operation).
			Scan(&status, &failure, &attempts, &shadow))
		return status, failure, attempts, shadow
	}

	failingJob := uuid()
	insertJob(failingJob)
	assets.failure = errors.New("synthetic rebuild source failure")
	claimed, rebuildErr := service.RunRebuildStep(ctx)
	if !claimed || !errors.Is(rebuildErr, rankings.ErrUnavailable) {
		t.Fatal("rebuild source failure was not returned", claimed, rebuildErr)
	}
	status, failure, attempts, shadow := readJob(failingJob)
	if status != "PENDING" || failure != "RANKING_SOURCE_UNAVAILABLE" || attempts != 1 || shadow != nil {
		t.Fatal("failed rebuild was not retained for bounded retry")
	}
	if id, version := v07RankingPointer(t, ctx, owner, "TOTAL_ASSETS", "CURRENT", activation); id != agedPointer || version != agedVersion {
		t.Fatal("failed rebuild changed the public pointer")
	}
	_, err = owner.Exec(ctx, `UPDATE rankings.rebuild_jobs SET updated_at=clock_timestamp()-interval '6 seconds'
		WHERE operation_id=$1::uuid`, failingJob)
	v07Must(t, err)
	assets.failure = nil
	assets.observed = time.Now().UTC()
	claimed, rebuildErr = service.RunRebuildStep(ctx)
	if !claimed || rebuildErr != nil {
		t.Fatal("retryable rebuild did not recover", claimed, rebuildErr)
	}
	status, failure, attempts, shadow = readJob(failingJob)
	if status != "SHADOW" || failure != "" || attempts != 2 || shadow == nil {
		t.Fatal("recovered rebuild did not retain a shadow snapshot")
	}
	var kind string
	var hashBytes int
	v07Must(t, owner.QueryRow(ctx, `SELECT build_kind,octet_length(aggregate_hash)
		FROM rankings.snapshots WHERE snapshot_id=$1::uuid`, *shadow).Scan(&kind, &hashBytes))
	if kind != "REPAIR" || hashBytes != 32 {
		t.Fatal("rebuild shadow is not a finalized repair snapshot")
	}
	if id, version := v07RankingPointer(t, ctx, owner, "TOTAL_ASSETS", "CURRENT", activation); id != agedPointer || version != agedVersion {
		t.Fatal("shadow rebuild changed the public pointer")
	}

	leasedJob := uuid()
	insertJob(leasedJob)
	assets.block = true
	assets.entered = make(chan struct{}, 1)
	work, stop := context.WithCancel(ctx)
	type rebuildResult struct {
		claimed bool
		err     error
	}
	done := make(chan rebuildResult, 1)
	go func() {
		value, runErr := service.RunRebuildStep(work)
		done <- rebuildResult{claimed: value, err: runErr}
	}()
	select {
	case <-assets.entered:
		stop()
	case early := <-done:
		stop()
		t.Fatal("rebuild ended before its source lease could be cancelled", early.claimed, early.err)
	case <-ctx.Done():
		stop()
		t.Fatal("rebuild did not reach its bounded source read")
	}
	cancelled := <-done
	if !cancelled.claimed || !errors.Is(cancelled.err, context.Canceled) {
		t.Fatal("cancelled rebuild did not retain its claimed lease", cancelled.claimed, cancelled.err)
	}
	status, _, attempts, shadow = readJob(leasedJob)
	if status != "RUNNING" || attempts != 1 || shadow != nil {
		t.Fatal("cancelled worker did not leave a recoverable RUNNING lease")
	}
	_, err = owner.Exec(ctx, `UPDATE rankings.rebuild_jobs SET updated_at=clock_timestamp()-interval '3 minutes'
		WHERE operation_id=$1::uuid`, leasedJob)
	v07Must(t, err)
	assets.block = false
	assets.observed = time.Now().UTC()
	claimed, rebuildErr = service.RunRebuildStep(ctx)
	if !claimed || rebuildErr != nil {
		t.Fatal("expired rebuild lease was not reclaimed", claimed, rebuildErr)
	}
	status, failure, attempts, shadow = readJob(leasedJob)
	if status != "SHADOW" || failure != "" || attempts != 2 || shadow == nil {
		t.Fatal("reclaimed rebuild did not finish as an unpublished shadow")
	}
	if id, version := v07RankingPointer(t, ctx, owner, "TOTAL_ASSETS", "CURRENT", activation); id != agedPointer || version != agedVersion {
		t.Fatal("lease recovery changed the public pointer")
	}

	t.Log("V07_RELEASED_RANKINGS: local PG fixture verified fail-closed reads, atomic publication, exact integers, stale semantics, retry and lease recovery; repair builds remained unpublished")
}
