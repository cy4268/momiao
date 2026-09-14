package rankings

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

type OpsAggregationStatus struct {
	SliceTargetID          string     `json:"slice_target_id"`
	Domain                 string     `json:"domain"`
	Metric                 string     `json:"metric"`
	Period                 string     `json:"period"`
	Date                   *string    `json:"date"`
	PublishedPointerVersion int64     `json:"published_pointer_version,string"`
	PublishedSnapshotID    *string    `json:"published_snapshot_id"`
	LastSourceCheckedAt    *time.Time `json:"last_source_checked_at"`
	LastBuiltAt            *time.Time `json:"last_built_at"`
	LagSeconds             *int64     `json:"lag_seconds,string"`
	State                   string     `json:"state"`
	ModelScope              string     `json:"model_scope,omitempty"`
	ModelID                 string     `json:"model_id,omitempty"`
}

type OpsSnapshot struct {
	SnapshotID             string     `json:"snapshot_id"`
	SliceTargetID          string     `json:"slice_target_id"`
	Domain                 string     `json:"domain"`
	Period                 string     `json:"period"`
	PeriodStart            time.Time  `json:"period_start"`
	PeriodEnd              *time.Time `json:"period_end"`
	BuildKind              string     `json:"build_kind"`
	SourceCheckedAt        time.Time  `json:"source_checked_at"`
	BuiltAt                time.Time  `json:"built_at"`
	EntryCount             int64      `json:"entry_count,string"`
	AggregateHash          string     `json:"aggregate_hash"`
	Published              bool       `json:"published"`
	OperationID            *string    `json:"operation_id"`
	PointerVersion         int64      `json:"pointer_version,string"`
	CurrentPointerVersion  int64      `json:"current_pointer_version,string"`
}

type OpsDiff struct {
	OldCount int64   `json:"old_count,string"`
	NewCount int64   `json:"new_count,string"`
	Added    int64   `json:"added,string"`
	Removed  int64   `json:"removed,string"`
	Changed  int64   `json:"changed,string"`
	OldHash  *string `json:"old_hash"`
	NewHash  string  `json:"new_hash"`
}

type OpsRebuildJob struct {
	OperationID            string     `json:"operation_id"`
	SliceTargetID          string     `json:"slice_target_id"`
	Domain                 string     `json:"domain"`
	Metric                 string     `json:"metric"`
	Period                 string     `json:"period"`
	Date                   string     `json:"date,omitempty"`
	ModelScope             string     `json:"model_scope"`
	ModelID                string     `json:"model_id,omitempty"`
	ExpectedPointerVersion int64      `json:"expected_pointer_version,string"`
	Status                  string     `json:"status"`
	ShadowSnapshotID       *string    `json:"shadow_snapshot_id,omitempty"`
	Attempts                int64      `json:"attempts,string"`
	FailureCode             *string    `json:"failure_code,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

type OpsOverview struct {
	GeneratedAt      time.Time              `json:"generated_at"`
	AggregationStatus []OpsAggregationStatus `json:"aggregation_status"`
	Published        []OpsSnapshot           `json:"published"`
	ShadowRebuilds   []OpsSnapshot           `json:"shadow_rebuilds"`
	RebuildJobs      []OpsRebuildJob         `json:"rebuild_jobs"`
}

type OpsSnapshotDetail struct {
	Snapshot OpsSnapshot  `json:"snapshot"`
	Current  *OpsSnapshot `json:"current"`
	Diff     OpsDiff      `json:"diff"`
}

type opsSliceFocus struct {
	metric, scope, model string
}

func rankingDate(period string, start time.Time) *string {
	if period != "DAY" && period != "WEEK" {
		return nil
	}
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	value := start.In(zone).Format("2006-01-02")
	return &value
}

func snapshotAggregateHash(ctx context.Context, q rankingQuerier, meta snapshotMeta) (string, error) {
	hash := meta.hash
	if len(hash) == 0 {
		var err error
		hash, err = computeSnapshotHash(ctx, q, meta.id)
		if err != nil {
			return "", err
		}
	}
	if len(hash) != sha256.Size {
		return "", platform.ErrOpsUnavailable
	}
	return hex.EncodeToString(hash), nil
}

func opsSnapshot(ctx context.Context, tx pgx.Tx, id string, focus opsSliceFocus) (OpsSnapshot, error) {
	var result OpsSnapshot
	meta, err := readSnapshotMeta(ctx, tx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, platform.ErrOpsNotFound
	}
	if err != nil {
		return result, err
	}
	if focus.scope == "" {
		focus.scope = "ALL"
	}
	if focus.metric == "" || metricDomain(focus.metric) != meta.domain || focus.scope != "ALL" || focus.model != "" {
		return result, platform.ErrOpsInvalid
	}
	hash, err := snapshotAggregateHash(ctx, tx, meta)
	if err != nil {
		return result, err
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FROM rankings.entries WHERE snapshot_id=$1::uuid AND metric=$2`,
		id, focus.metric).Scan(&result.EntryCount)
	if err != nil {
		return result, err
	}
	currentID, currentVersion, err := readPublishedPointer(ctx, tx, meta.domain, focus.metric, meta.period,
		meta.start, meta.activation, false)
	if err != nil {
		return result, err
	}
	var pointerVersion int64
	err = tx.QueryRow(ctx, `SELECT coalesce(max(pointer_version),0) FROM rankings.publication_events
		WHERE snapshot_id=$1::uuid AND metric=$2`, id, focus.metric).Scan(&pointerVersion)
	if err != nil {
		return result, err
	}
	var operationID *string
	if meta.operationID != "" {
		value := meta.operationID
		operationID = &value
	}
	result.SnapshotID = meta.id
	result.SliceTargetID = sliceTargetID(meta.domain, focus.metric, meta.period, meta.start)
	result.Domain = meta.domain
	result.Period = meta.period
	result.PeriodStart = meta.start.UTC()
	result.PeriodEnd = meta.end
	result.BuildKind = meta.buildKind
	result.SourceCheckedAt = meta.checked.UTC()
	result.BuiltAt = meta.built.UTC()
	result.AggregateHash = hash
	result.Published = currentID == meta.id
	result.OperationID = operationID
	result.PointerVersion = pointerVersion
	result.CurrentPointerVersion = currentVersion
	return result, nil
}

func rankingDiff(ctx context.Context, tx pgx.Tx, oldID, newID, metric string) (OpsDiff, error) {
	var result OpsDiff
	err := tx.QueryRow(ctx, `WITH old_rows AS (
		 SELECT model_scope,model_id,newapi_user_id,value,calls,errors,credits_units,models,
		  rank() OVER(PARTITION BY model_scope,model_id ORDER BY value DESC) rank_no FROM rankings.entries
		 WHERE snapshot_id=NULLIF($1,'')::uuid AND metric=$3
		),new_rows AS (
		 SELECT model_scope,model_id,newapi_user_id,value,calls,errors,credits_units,models,
		  rank() OVER(PARTITION BY model_scope,model_id ORDER BY value DESC) rank_no FROM rankings.entries
		 WHERE snapshot_id=$2::uuid AND metric=$3
		),joined AS (
		 SELECT o.newapi_user_id old_user,n.newapi_user_id new_user,
		  (o.value,o.calls,o.errors,o.credits_units,o.models,o.rank_no)
		   IS DISTINCT FROM (n.value,n.calls,n.errors,n.credits_units,n.models,n.rank_no) changed
		 FROM old_rows o FULL OUTER JOIN new_rows n USING(model_scope,model_id,newapi_user_id)
		)
		SELECT (SELECT count(*) FROM old_rows),(SELECT count(*) FROM new_rows),
		 count(*) FILTER(WHERE old_user IS NULL),count(*) FILTER(WHERE new_user IS NULL),
		 count(*) FILTER(WHERE old_user IS NOT NULL AND new_user IS NOT NULL AND changed) FROM joined`,
		oldID, newID, metric).Scan(&result.OldCount, &result.NewCount, &result.Added,
		&result.Removed, &result.Changed)
	if err != nil {
		return result, err
	}
	newMeta, err := readSnapshotMeta(ctx, tx, newID)
	if err != nil {
		return result, err
	}
	result.NewHash, err = snapshotAggregateHash(ctx, tx, newMeta)
	if err != nil {
		return result, err
	}
	if oldID != "" {
		oldMeta, readErr := readSnapshotMeta(ctx, tx, oldID)
		if readErr != nil {
			return result, readErr
		}
		value, readErr := snapshotAggregateHash(ctx, tx, oldMeta)
		if readErr != nil {
			return result, readErr
		}
		result.OldHash = &value
	}
	return result, nil
}

func (s *Service) ReadOpsOverview(ctx context.Context) (OpsOverview, error) {
	result := OpsOverview{AggregationStatus: []OpsAggregationStatus{}, Published: []OpsSnapshot{},
		ShadowRebuilds: []OpsSnapshot{}, RebuildJobs: []OpsRebuildJob{}}
	if !s.enabled(time.Now().UTC()) {
		return result, platform.ErrOpsUnavailable
	}
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY`); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&result.GeneratedAt); err != nil {
			return err
		}
		published := map[string]opsSliceFocus{}
		for _, domain := range []string{"ASSETS", "GAMES", "RP"} {
			metrics, _ := metricsForDomain(domain)
			periods := []string{"DAY", "WEEK", "ALL_TIME"}
			if domain == "ASSETS" {
				periods = []string{"CURRENT"}
			}
			for _, metric := range metrics {
				for _, period := range periods {
					start, end, boundErr := periodBounds(period, "", result.GeneratedAt, s.activation)
					if boundErr != nil {
						return boundErr
					}
					status := OpsAggregationStatus{SliceTargetID: sliceTargetID(domain, metric, period, start),
						Domain: domain, Metric: metric, Period: period, Date: rankingDate(period, start), State: "UNAVAILABLE"}
					pointerID, version, readErr := readPublishedPointer(ctx, tx, domain, metric, period,
						start, s.activation, false)
					if readErr != nil {
						return readErr
					}
					status.PublishedPointerVersion = version
					if pointerID != "" {
						value := pointerID
						status.PublishedSnapshotID = &value
						meta, metaErr := readSnapshotMeta(ctx, tx, pointerID)
						if metaErr != nil {
							return metaErr
						}
						checked, built := meta.checked.UTC(), meta.built.UTC()
						status.LastSourceCheckedAt, status.LastBuiltAt = &checked, &built
						lag := int64(result.GeneratedAt.Sub(meta.checked).Seconds())
						if lag < 0 {
							lag = 0
						}
						status.LagSeconds = &lag
						status.State = "READY"
						if (end == nil || end.After(result.GeneratedAt)) &&
							(result.GeneratedAt.Sub(meta.built) > 5*time.Minute || result.GeneratedAt.Sub(meta.checked) > 5*time.Minute) {
							status.State = "STALE"
						}
						focus, exists := published[pointerID]
						if !exists || metric < focus.metric {
							published[pointerID] = opsSliceFocus{metric: metric, scope: "ALL"}
						}
					} else {
						var built, checked time.Time
						readErr = tx.QueryRow(ctx, `SELECT built_at,source_checked_at FROM rankings.snapshots
							WHERE domain=$1 AND period=$2 AND period_start=$3 AND activation_at=$4
							ORDER BY built_at DESC,snapshot_id DESC LIMIT 1`, domain, period, start, s.activation).
							Scan(&built, &checked)
						if readErr == nil {
							built, checked = built.UTC(), checked.UTC()
							status.LastBuiltAt, status.LastSourceCheckedAt = &built, &checked
							lag := int64(result.GeneratedAt.Sub(checked).Seconds())
							if lag < 0 {
								lag = 0
							}
							status.LagSeconds = &lag
							status.State = "UNPUBLISHED"
						} else if !errors.Is(readErr, pgx.ErrNoRows) {
							return readErr
						}
					}
					result.AggregationStatus = append(result.AggregationStatus, status)
				}
			}
		}
		for id, focus := range published {
			snapshot, readErr := opsSnapshot(ctx, tx, id, focus)
			if readErr != nil {
				return readErr
			}
			result.Published = append(result.Published, snapshot)
		}
		sort.Slice(result.Published, func(i, j int) bool {
			if result.Published[i].BuiltAt.Equal(result.Published[j].BuiltAt) {
				return result.Published[i].SnapshotID < result.Published[j].SnapshotID
			}
			return result.Published[i].BuiltAt.After(result.Published[j].BuiltAt)
		})
		shadowRows, err := tx.Query(ctx, `SELECT shadow_snapshot_id::text,metric,model_scope,model_id
			FROM rankings.rebuild_jobs j WHERE status='SHADOW' AND NOT EXISTS(
			 SELECT 1 FROM rankings.publication_events e WHERE e.snapshot_id=j.shadow_snapshot_id
			  AND e.metric=j.metric AND e.source='REPAIR')
			ORDER BY updated_at DESC,operation_id DESC LIMIT 100`)
		if err != nil {
			return err
		}
		type shadowRef struct {
			id    string
			focus opsSliceFocus
		}
		shadowRefs := []shadowRef{}
		for shadowRows.Next() {
			var ref shadowRef
			if err = shadowRows.Scan(&ref.id, &ref.focus.metric, &ref.focus.scope, &ref.focus.model); err != nil {
				shadowRows.Close()
				return err
			}
			shadowRefs = append(shadowRefs, ref)
		}
		if err = shadowRows.Err(); err != nil {
			shadowRows.Close()
			return err
		}
		shadowRows.Close()
		for _, ref := range shadowRefs {
			snapshot, readErr := opsSnapshot(ctx, tx, ref.id, ref.focus)
			if readErr != nil {
				return readErr
			}
			result.ShadowRebuilds = append(result.ShadowRebuilds, snapshot)
		}
		jobRows, err := tx.Query(ctx, `SELECT operation_id::text,domain,metric,period,requested_date,
			model_scope,model_id,expected_pointer_version,
			CASE WHEN status='SHADOW' AND EXISTS(SELECT 1 FROM rankings.publication_events e
			 WHERE e.snapshot_id=rebuild_jobs.shadow_snapshot_id AND e.metric=rebuild_jobs.metric
			  AND e.source='REPAIR') THEN 'PUBLISHED' ELSE status END,
			shadow_snapshot_id::text,attempts,
			failure_code,created_at,updated_at FROM rankings.rebuild_jobs
			ORDER BY created_at DESC,operation_id DESC LIMIT 100`)
		if err != nil {
			return err
		}
		for jobRows.Next() {
			var job OpsRebuildJob
			if err = jobRows.Scan(&job.OperationID, &job.Domain, &job.Metric, &job.Period, &job.Date,
				&job.ModelScope, &job.ModelID, &job.ExpectedPointerVersion, &job.Status, &job.ShadowSnapshotID,
				&job.Attempts, &job.FailureCode, &job.CreatedAt, &job.UpdatedAt); err != nil {
				jobRows.Close()
				return err
			}
			start, _, boundErr := periodBounds(job.Period, job.Date, result.GeneratedAt, s.activation)
			if boundErr != nil {
				jobRows.Close()
				return platform.ErrOpsUnavailable
			}
			job.SliceTargetID = sliceTargetID(job.Domain, job.Metric, job.Period, start)
			job.CreatedAt, job.UpdatedAt = job.CreatedAt.UTC(), job.UpdatedAt.UTC()
			result.RebuildJobs = append(result.RebuildJobs, job)
		}
		if err = jobRows.Err(); err != nil {
			jobRows.Close()
			return err
		}
		jobRows.Close()
		return nil
	})
	return result, err
}

func snapshotFocus(ctx context.Context, tx pgx.Tx, id string) (opsSliceFocus, error) {
	var focus opsSliceFocus
	err := tx.QueryRow(ctx, `SELECT metric,model_scope,model_id FROM rankings.rebuild_jobs
		WHERE shadow_snapshot_id=$1::uuid ORDER BY created_at DESC LIMIT 1`, id).
		Scan(&focus.metric, &focus.scope, &focus.model)
	if err == nil {
		return focus, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return focus, err
	}
	err = tx.QueryRow(ctx, `SELECT metric,'ALL','' FROM rankings.published_pointers
		WHERE snapshot_id=$1::uuid ORDER BY metric COLLATE "C" LIMIT 1`, id).
		Scan(&focus.metric, &focus.scope, &focus.model)
	if errors.Is(err, pgx.ErrNoRows) {
		return focus, platform.ErrOpsNotFound
	}
	return focus, err
}

func (s *Service) ReadOpsSnapshot(ctx context.Context, id string) (OpsSnapshotDetail, error) {
	var result OpsSnapshotDetail
	if !s.enabled(time.Now().UTC()) || !rankingUUIDPattern.MatchString(id) {
		return result, platform.ErrOpsNotFound
	}
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY`); err != nil {
			return err
		}
		focus, err := snapshotFocus(ctx, tx, id)
		if err != nil {
			return err
		}
		result.Snapshot, err = opsSnapshot(ctx, tx, id, focus)
		if err != nil {
			return err
		}
		meta, err := readSnapshotMeta(ctx, tx, id)
		if err != nil {
			return err
		}
		currentID, _, err := readPublishedPointer(ctx, tx, meta.domain, focus.metric, meta.period,
			meta.start, meta.activation, false)
		if err != nil {
			return err
		}
		if currentID != "" {
			current, readErr := opsSnapshot(ctx, tx, currentID, focus)
			if readErr != nil {
				return readErr
			}
			result.Current = &current
		}
		result.Diff, err = rankingDiff(ctx, tx, currentID, id, focus.metric)
		return err
	})
	return result, err
}
