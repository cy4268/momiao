package rankings

import (
	"context"
	"crypto/sha256"
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

var (
	rankingDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	rankingUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

type OpsRebuildInput struct {
	Domain     string `json:"domain"`
	Metric     string `json:"metric"`
	Period     string `json:"period"`
	Date       string `json:"date,omitempty"`
	ModelScope string `json:"model_scope"`
	ModelID    string `json:"model_id,omitempty"`
}

type pointerState struct {
	SnapshotID string `json:"snapshot_id,omitempty"`
	Version    int64  `json:"version,string"`
}

func rankingContains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func normalizeRebuildInput(input OpsRebuildInput, now, activation time.Time) (OpsRebuildInput, time.Time, error) {
	metrics, err := metricsForDomain(input.Domain)
	if err != nil || !rankingContains(metrics, input.Metric) {
		return input, time.Time{}, platform.ErrOpsInvalid
	}
	if input.ModelScope == "" {
		input.ModelScope = "ALL"
	}
	// Publication pointers are metric/period aggregate-set pointers. Repair
	// rebuilds therefore cover the full metric slice; a MODEL-only preview
	// would understate what the atomic pointer swap publishes.
	if input.ModelScope != "ALL" || input.ModelID != "" {
		return input, time.Time{}, platform.ErrOpsInvalid
	}
	if input.Period == "CURRENT" {
		if input.Domain != "ASSETS" || input.Date != "" {
			return input, time.Time{}, platform.ErrOpsInvalid
		}
	} else if input.Period == "ALL_TIME" {
		if input.Domain == "ASSETS" || input.Date != "" {
			return input, time.Time{}, platform.ErrOpsInvalid
		}
	} else if input.Period == "DAY" || input.Period == "WEEK" {
		if input.Domain == "ASSETS" || input.Date != "" && !rankingDatePattern.MatchString(input.Date) {
			return input, time.Time{}, platform.ErrOpsInvalid
		}
	} else {
		return input, time.Time{}, platform.ErrOpsInvalid
	}
	start, _, err := periodBounds(input.Period, input.Date, now, activation)
	if err != nil {
		return input, time.Time{}, platform.ErrOpsInvalid
	}
	if input.Period == "DAY" || input.Period == "WEEK" {
		zone := time.FixedZone("Asia/Shanghai", 8*60*60)
		input.Date = start.In(zone).Format("2006-01-02")
	}
	return input, start, nil
}

func sliceTargetID(domain, metric, period string, start time.Time) string {
	return domain + ":" + metric + ":" + period + ":" + strconv.FormatInt(start.UTC().Unix(), 10)
}

func readPublishedPointer(ctx context.Context, tx pgx.Tx, domain, metric, period string,
	start, activation time.Time, lock bool) (string, int64, error) {
	if lock {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
			pointerLockKey(domain, metric, period, start, activation)); err != nil {
			return "", 0, err
		}
	}
	query := `SELECT snapshot_id::text,version FROM rankings.published_pointers
		WHERE domain=$1 AND metric=$2 AND period=$3 AND period_start=$4 AND activation_at=$5`
	if lock {
		query += ` FOR UPDATE`
	}
	var snapshotID string
	var version int64
	err := tx.QueryRow(ctx, query, domain, metric, period, start, activation).Scan(&snapshotID, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, nil
	}
	return snapshotID, version, err
}

type RebuildResult struct {
	JobID string `json:"job_id"`
}

func (s *Service) CreateRebuild(ctx context.Context, actor int64, input OpsRebuildInput) (RebuildResult, error) {
	var result RebuildResult
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		result, err = s.createRebuild(ctx, tx, actor, input)
		return err
	})
	return result, err
}

func (s *Service) createRebuild(ctx context.Context, tx pgx.Tx, actor int64, input OpsRebuildInput) (RebuildResult, error) {
	var result RebuildResult
	if err := platform.RequireLocalOpsAdmin(ctx, tx, actor); err != nil {
		return result, err
	}
	if !s.enabled(time.Now().UTC()) {
		return result, platform.ErrOpsUnavailable
	}
	input, start, err := normalizeRebuildInput(input, time.Now().UTC(), s.activation)
	if err != nil {
		return result, err
	}
	_, version, err := readPublishedPointer(ctx, tx, input.Domain, input.Metric, input.Period, start, s.activation, true)
	if err != nil {
		return result, err
	}
	var active bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rankings.rebuild_jobs WHERE domain=$1 AND metric=$2
  AND period=$3 AND requested_date=$4 AND model_scope=$5 AND model_id=$6 AND status IN('PENDING','RUNNING'))`,
		input.Domain, input.Metric, input.Period, input.Date, input.ModelScope, input.ModelID).Scan(&active)
	if err != nil {
		return result, err
	}
	if active {
		return result, platform.ErrOpsConflict
	}
	err = tx.QueryRow(ctx, `INSERT INTO rankings.rebuild_jobs(operation_id,actor_user_id,domain,metric,
  period,requested_date,model_scope,model_id,expected_pointer_version,status)
  VALUES(gen_random_uuid(),$1,$2,$3,$4,$5,$6,$7,$8,'PENDING') RETURNING operation_id::text`,
		actor, input.Domain, input.Metric, input.Period, input.Date, input.ModelScope, input.ModelID, version).Scan(&result.JobID)
	if err != nil {
		return result, err
	}
	return result, platform.RecordOpsChange(ctx, tx, actor, "RANKING_REBUILD_CREATE", "ranking_job", result.JobID, nil, input)
}

type repairCandidate struct {
	meta                        snapshotMeta
	metric, modelScope, modelID string
}

func loadRepairCandidate(ctx context.Context, tx pgx.Tx, snapshotID string) (repairCandidate, error) {
	var result repairCandidate
	if !rankingUUIDPattern.MatchString(snapshotID) {
		return result, platform.ErrOpsInvalid
	}
	meta, err := readSnapshotMeta(ctx, tx, snapshotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, platform.ErrOpsNotFound
	}
	if err != nil {
		return result, err
	}
	err = tx.QueryRow(ctx, `SELECT metric,model_scope,model_id
		FROM rankings.rebuild_jobs j WHERE shadow_snapshot_id=$1::uuid AND status='SHADOW' FOR UPDATE`, snapshotID).
		Scan(&result.metric, &result.modelScope, &result.modelID)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, platform.ErrOpsNotFound
	}
	if err != nil {
		return result, err
	}
	if meta.buildKind != "REPAIR" || len(meta.hash) != sha256.Size || metricDomain(result.metric) != meta.domain ||
		result.modelScope != "ALL" || result.modelID != "" {
		return result, platform.ErrOpsConflict
	}
	var published bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rankings.publication_events WHERE snapshot_id=$1::uuid AND metric=$2 AND source='REPAIR')`, snapshotID, result.metric).Scan(&published); err != nil {
		return result, err
	}
	if published {
		return result, platform.ErrOpsConflict
	}
	result.meta = meta
	return result, nil
}

func (s *Service) PublishRepair(ctx context.Context, actor int64, snapshotID string) error {
	return s.store.WithTx(ctx, func(tx pgx.Tx) error { return s.publishRepair(ctx, tx, actor, snapshotID) })
}

func (s *Service) publishRepair(ctx context.Context, tx pgx.Tx, actor int64, snapshotID string) error {
	if err := platform.RequireLocalOpsAdmin(ctx, tx, actor); err != nil {
		return err
	}
	if !s.enabled(time.Now().UTC()) {
		return platform.ErrOpsUnavailable
	}
	candidate, err := loadRepairCandidate(ctx, tx, snapshotID)
	if err != nil {
		return err
	}
	if err = platform.RequireNoMaintenance(ctx, tx, "RANKINGS_PUBLISHING"); err != nil {
		return err
	}
	version, previous, err := publishPointer(ctx, tx, candidate.meta, candidate.metric, candidate.meta.operationID, &actor, "REPAIR")
	if err != nil {
		return err
	}
	return platform.RecordOpsChange(ctx, tx, actor, "RANKING_REPAIR_PUBLISH", "ranking_snapshot", snapshotID,
		pointerState{SnapshotID: previous, Version: version - 1}, pointerState{SnapshotID: snapshotID, Version: version})
}

type rebuildJob struct {
	operationID, domain, metric, period, date, modelScope, modelID string
	actor, attempts                                                int64
	runnable                                                       bool
}

func (s *Service) claimRebuildJob(ctx context.Context) (rebuildJob, bool, error) {
	var job rebuildJob
	claimed := false
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT operation_id::text,actor_user_id,domain,metric,period,requested_date,
			model_scope,model_id,attempts FROM rankings.rebuild_jobs
			WHERE (status='PENDING' AND (failure_code IS NULL OR updated_at<=clock_timestamp()-interval '5 seconds'))
			 OR (status='RUNNING' AND updated_at<=clock_timestamp()-interval '2 minutes')
			ORDER BY created_at,operation_id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&job.operationID, &job.actor,
			&job.domain, &job.metric, &job.period, &job.date, &job.modelScope, &job.modelID,
			&job.attempts)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		claimed = true
		if job.attempts >= 10 {
			_, err = tx.Exec(ctx, `UPDATE rankings.rebuild_jobs SET status='FAILED',
				failure_code='RANKING_REBUILD_ATTEMPTS_EXHAUSTED',updated_at=clock_timestamp()
				WHERE operation_id=$1::uuid`, job.operationID)
			return err
		}
		job.attempts++
		_, err = tx.Exec(ctx, `UPDATE rankings.rebuild_jobs SET status='RUNNING',attempts=$2,
			failure_code=NULL,updated_at=clock_timestamp() WHERE operation_id=$1::uuid`, job.operationID, job.attempts)
		job.runnable = err == nil
		return err
	})
	return job, claimed, err
}

func rebuildFailureCode(err error) string {
	switch {
	case errors.Is(err, ErrUnavailable):
		return "RANKING_SOURCE_UNAVAILABLE"
	case errors.Is(err, ErrQuery), errors.Is(err, platform.ErrOpsInvalid):
		return "RANKING_REBUILD_INPUT_INVALID"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "RANKING_REBUILD_TIMEOUT"
	default:
		return "RANKING_REBUILD_FAILED"
	}
}

func (s *Service) recordRebuildFailure(ctx context.Context, job rebuildJob, cause error) error {
	status := "PENDING"
	if job.attempts >= 10 || errors.Is(cause, ErrQuery) || errors.Is(cause, platform.ErrOpsInvalid) {
		status = "FAILED"
	}
	return s.store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE rankings.rebuild_jobs SET status=$2,failure_code=$3,
			updated_at=clock_timestamp() WHERE operation_id=$1::uuid AND status='RUNNING' AND attempts=$4`,
			job.operationID, status, rebuildFailureCode(cause), job.attempts)
		return err
	})
}

// RunRebuildStep claims at most one durable request. It performs the full
// authoritative build and records SHADOW without touching a published pointer.
func (s *Service) RunRebuildStep(ctx context.Context) (bool, error) {
	if !s.enabled(time.Now().UTC()) {
		return false, ErrUnavailable
	}
	job, claimed, err := s.claimRebuildJob(ctx)
	if err != nil || !claimed || !job.runnable {
		return claimed, err
	}
	input := OpsRebuildInput{Domain: job.domain, Metric: job.metric, Period: job.period, Date: job.date,
		ModelScope: job.modelScope, ModelID: job.modelID}
	now := time.Now().UTC()
	input, start, err := normalizeRebuildInput(input, now, s.activation)
	if err != nil {
		if recordErr := s.recordRebuildFailure(ctx, job, err); recordErr != nil {
			return true, recordErr
		}
		return true, err
	}
	assets, checked, err := s.buildInputs(ctx, input.Domain, now)
	if err != nil {
		if recordErr := s.recordRebuildFailure(ctx, job, err); recordErr != nil {
			return true, recordErr
		}
		return true, err
	}
	_, end, err := periodBounds(input.Period, input.Date, now, s.activation)
	if err != nil {
		if recordErr := s.recordRebuildFailure(ctx, job, err); recordErr != nil {
			return true, recordErr
		}
		return true, err
	}
	err = s.store.WithTx(ctx, func(tx pgx.Tx) error {
		var status string
		var attempts int64
		if readErr := tx.QueryRow(ctx, `SELECT status,attempts FROM rankings.rebuild_jobs
			WHERE operation_id=$1::uuid FOR UPDATE`, job.operationID).Scan(&status, &attempts); readErr != nil {
			return readErr
		}
		if status != "RUNNING" || attempts != job.attempts {
			return platform.ErrOpsConflict
		}
		id, buildErr := s.buildSnapshotTx(ctx, tx, input.Domain, input.Period, start, end, checked,
			assets, "REPAIR", job.operationID)
		if buildErr != nil {
			return buildErr
		}
		tag, updateErr := tx.Exec(ctx, `UPDATE rankings.rebuild_jobs SET status='SHADOW',shadow_snapshot_id=$2,
			failure_code=NULL,updated_at=clock_timestamp() WHERE operation_id=$1::uuid AND status='RUNNING'`,
			job.operationID, id)
		if updateErr != nil {
			return updateErr
		}
		if tag.RowsAffected() != 1 {
			return platform.ErrOpsConflict
		}
		return nil
	})
	if err != nil {
		if recordErr := s.recordRebuildFailure(ctx, job, err); recordErr != nil {
			return true, recordErr
		}
		return true, err
	}
	return true, nil
}
