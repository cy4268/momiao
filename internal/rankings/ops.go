package rankings

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"

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

type opsPublishInput struct {
	Metric string `json:"metric"`
}

type pointerState struct {
	SnapshotID string `json:"snapshot_id,omitempty"`
	Version    int64  `json:"version,string"`
}

type rebuildPlan struct {
	SliceTargetID string          `json:"slice_target_id"`
	Input         OpsRebuildInput `json:"input"`
	State         string          `json:"state"`
}

type publishPlan struct {
	SnapshotID             string `json:"snapshot_id"`
	Metric                 string `json:"metric"`
	PreviousSnapshotID     string `json:"previous_snapshot_id,omitempty"`
	CurrentPointerVersion  int64  `json:"current_pointer_version,string"`
	ProposedPointerVersion int64  `json:"proposed_pointer_version,string"`
}

func decodeRankingInput(raw json.RawMessage, target any) error {
	if len(raw) == 0 || len(raw) > 4096 || !utf8.Valid(raw) {
		return platform.ErrOpsInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return platform.ErrOpsInvalid
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return platform.ErrOpsInvalid
	}
	return nil
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

func strictPointerVersion(value string) (int64, bool) {
	version, err := strconv.ParseInt(value, 10, 64)
	return version, err == nil && version >= 0 && strconv.FormatInt(version, 10) == value
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

type rankingRebuildOpsHandler struct{ service *Service }

func (handler rankingRebuildOpsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var input OpsRebuildInput
	if handler.service == nil || handler.service.activation.IsZero() {
		return nil, platform.ErrOpsUnavailable
	}
	if err := decodeRankingInput(raw, &input); err != nil {
		return nil, err
	}
	var err error
	input, _, err = normalizeRebuildInput(input, time.Now().UTC(), handler.service.activation)
	if err != nil {
		return nil, err
	}
	return json.Marshal(input)
}

func (handler rankingRebuildOpsHandler) Prepare(ctx context.Context, tx pgx.Tx, _ platform.OpsPrincipal,
	request platform.OpsPrepareRequest, lock bool) (platform.OpsPreparedMaterial, error) {
	if handler.service == nil || handler.service.store == nil || tx == nil || !handler.service.enabled(time.Now().UTC()) {
		return platform.OpsPreparedMaterial{}, platform.ErrOpsUnavailable
	}
	var input OpsRebuildInput
	if err := decodeRankingInput(request.Input, &input); err != nil {
		return platform.OpsPreparedMaterial{}, err
	}
	now := time.Now().UTC()
	input, start, err := normalizeRebuildInput(input, now, handler.service.activation)
	if err != nil {
		return platform.OpsPreparedMaterial{}, err
	}
	targetID := sliceTargetID(input.Domain, input.Metric, input.Period, start)
	if request.Target.ID != targetID {
		return platform.OpsPreparedMaterial{}, platform.ErrOpsInvalid
	}
	expected, valid := strictPointerVersion(request.Target.ExpectedVersion)
	if !valid {
		return platform.OpsPreparedMaterial{}, platform.ErrOpsInvalid
	}
	currentID, currentVersion, err := readPublishedPointer(ctx, tx, input.Domain, input.Metric,
		input.Period, start, handler.service.activation, lock)
	if err != nil {
		return platform.OpsPreparedMaterial{}, err
	}
	if currentVersion != expected {
		return platform.OpsPreparedMaterial{}, platform.ErrOpsPreviewStale
	}
	var active bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rankings.rebuild_jobs WHERE domain=$1 AND metric=$2
		AND period=$3 AND requested_date=$4 AND model_scope=$5 AND model_id=$6 AND status IN('PENDING','RUNNING'))`,
		input.Domain, input.Metric, input.Period, input.Date, input.ModelScope, input.ModelID).Scan(&active)
	if err != nil {
		return platform.OpsPreparedMaterial{}, err
	}
	if active {
		return platform.OpsPreparedMaterial{}, platform.ErrOpsConflict
	}
	current, _ := json.Marshal(pointerState{SnapshotID: currentID, Version: currentVersion})
	plan := rebuildPlan{SliceTargetID: targetID, Input: input, State: "PENDING"}
	proposed, _ := json.Marshal(plan)
	one := int64(1)
	impact := platform.OpsImpact{CurrentState: current, ProposedChange: proposed, AffectedItems: &one,
		BlockingFacts: []string{}, ContinuingAcceptedWork: []string{"ranking_ingestion", "routine_ranking_publication"},
		RelatedIDs: []string{targetID}, UnavailableMeasurements: []string{"affected_users", "changed_entries"}}
	return platform.OpsPreparedMaterial{Impact: impact, TargetVersion: strconv.FormatInt(currentVersion, 10),
		TargetLocator: targetID}, nil
}

func (handler rankingRebuildOpsHandler) Execute(ctx context.Context, tx pgx.Tx, actor platform.OpsPrincipal,
	operation platform.OpsOperation, material platform.OpsPreparedMaterial) (platform.OpsExecutionResult, error) {
	var plan rebuildPlan
	if err := decodeRankingInput(material.Impact.ProposedChange, &plan); err != nil || plan.State != "PENDING" ||
		plan.SliceTargetID != operation.Target.ID {
		return platform.OpsExecutionResult{}, platform.ErrOpsInvalid
	}
	expected, valid := strictPointerVersion(operation.Target.ExpectedVersion)
	if !valid {
		return platform.OpsExecutionResult{}, platform.ErrOpsInvalid
	}
	_, err := tx.Exec(ctx, `INSERT INTO rankings.rebuild_jobs(operation_id,actor_user_id,domain,metric,
		period,requested_date,model_scope,model_id,expected_pointer_version,status)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'PENDING')`, operation.OperationID, actor.UserID, plan.Input.Domain,
		plan.Input.Metric, plan.Input.Period, plan.Input.Date, plan.Input.ModelScope, plan.Input.ModelID,
		expected)
	if err != nil {
		return platform.OpsExecutionResult{}, err
	}
	after, _ := json.Marshal(plan)
	result, _ := json.Marshal(map[string]any{"operation_id": operation.OperationID, "state": "PENDING",
		"slice_target_id": plan.SliceTargetID})
	return platform.OpsExecutionResult{BeforeSnapshot: material.Impact.CurrentState, AfterSnapshot: after,
		Result: result, RelatedBusinessID: operation.OperationID}, nil
}

type rankingPublishOpsHandler struct{ service *Service }

func (rankingPublishOpsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var input opsPublishInput
	if err := decodeRankingInput(raw, &input); err != nil {
		return nil, err
	}
	metrics, err := metricsForDomain(metricDomain(input.Metric))
	if err != nil || !rankingContains(metrics, input.Metric) {
		return nil, platform.ErrOpsInvalid
	}
	return json.Marshal(input)
}

type repairCandidate struct {
	meta                   snapshotMeta
	metric, modelScope     string
	modelID                 string
	currentID               string
	currentPointerVersion   int64
	expectedPointerVersion  int64
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
	err = tx.QueryRow(ctx, `SELECT metric,model_scope,model_id,expected_pointer_version
		FROM rankings.rebuild_jobs j WHERE shadow_snapshot_id=$1::uuid AND status='SHADOW'
		AND NOT EXISTS(SELECT 1 FROM rankings.publication_events e
		 WHERE e.snapshot_id=j.shadow_snapshot_id AND e.metric=j.metric AND e.source='REPAIR')`, snapshotID).
		Scan(&result.metric, &result.modelScope, &result.modelID, &result.expectedPointerVersion)
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
	result.meta = meta
	return result, nil
}

func (handler rankingPublishOpsHandler) Prepare(ctx context.Context, tx pgx.Tx, _ platform.OpsPrincipal,
	request platform.OpsPrepareRequest, lock bool) (platform.OpsPreparedMaterial, error) {
	if handler.service == nil || handler.service.store == nil || tx == nil || !handler.service.enabled(time.Now().UTC()) {
		return platform.OpsPreparedMaterial{}, platform.ErrOpsUnavailable
	}
	var input opsPublishInput
	if err := decodeRankingInput(request.Input, &input); err != nil {
		return platform.OpsPreparedMaterial{}, err
	}
	candidate, err := loadRepairCandidate(ctx, tx, request.Target.ID)
	if err != nil {
		return platform.OpsPreparedMaterial{}, err
	}
	if input.Metric != candidate.metric {
		return platform.OpsPreparedMaterial{}, platform.ErrOpsInvalid
	}
	expected, valid := strictPointerVersion(request.Target.ExpectedVersion)
	if !valid {
		return platform.OpsPreparedMaterial{}, platform.ErrOpsInvalid
	}
	if lock {
		if err = platform.RequireNoMaintenance(ctx, tx, "RANKINGS_PUBLISHING"); err != nil {
			if errors.Is(err, platform.ErrMaintenanceActive) {
				return platform.OpsPreparedMaterial{}, platform.ErrOpsConflict
			}
			return platform.OpsPreparedMaterial{}, err
		}
	}
	candidate.currentID, candidate.currentPointerVersion, err = readPublishedPointer(ctx, tx,
		candidate.meta.domain, candidate.metric, candidate.meta.period, candidate.meta.start,
		candidate.meta.activation, lock)
	if err != nil {
		return platform.OpsPreparedMaterial{}, err
	}
	if candidate.currentPointerVersion != expected || candidate.currentID == candidate.meta.id {
		return platform.OpsPreparedMaterial{}, platform.ErrOpsPreviewStale
	}
	diff, err := rankingDiff(ctx, tx, candidate.currentID, candidate.meta.id, candidate.metric)
	if err != nil {
		return platform.OpsPreparedMaterial{}, err
	}
	current, _ := json.Marshal(pointerState{SnapshotID: candidate.currentID, Version: candidate.currentPointerVersion})
	plan := publishPlan{SnapshotID: candidate.meta.id, Metric: candidate.metric,
		PreviousSnapshotID: candidate.currentID, CurrentPointerVersion: candidate.currentPointerVersion,
		ProposedPointerVersion: candidate.currentPointerVersion + 1}
	proposed, _ := json.Marshal(plan)
	delta, _ := json.Marshal(diff)
	affected := diff.Added + diff.Removed + diff.Changed
	impact := platform.OpsImpact{CurrentState: current, ProposedChange: proposed, AffectedItems: &affected,
		Delta: delta, BlockingFacts: []string{}, ContinuingAcceptedWork: []string{"ranking_ingestion"},
		RelatedIDs: []string{candidate.meta.id, candidate.currentID}, UnavailableMeasurements: []string{"affected_users"}}
	if candidate.currentID == "" {
		impact.RelatedIDs = []string{candidate.meta.id}
	}
	return platform.OpsPreparedMaterial{Impact: impact, TargetVersion: strconv.FormatInt(candidate.currentPointerVersion, 10),
		TargetLocator: candidate.meta.id}, nil
}

func (handler rankingPublishOpsHandler) Execute(ctx context.Context, tx pgx.Tx, actor platform.OpsPrincipal,
	operation platform.OpsOperation, material platform.OpsPreparedMaterial) (platform.OpsExecutionResult, error) {
	var plan publishPlan
	if err := decodeRankingInput(material.Impact.ProposedChange, &plan); err != nil || plan.SnapshotID != operation.Target.ID {
		return platform.OpsExecutionResult{}, platform.ErrOpsInvalid
	}
	candidate, err := loadRepairCandidate(ctx, tx, operation.Target.ID)
	if err != nil || candidate.metric != plan.Metric {
		if err != nil {
			return platform.OpsExecutionResult{}, err
		}
		return platform.OpsExecutionResult{}, platform.ErrOpsPreviewStale
	}
	expected, valid := strictPointerVersion(operation.Target.ExpectedVersion)
	if !valid {
		return platform.OpsExecutionResult{}, platform.ErrOpsInvalid
	}
	version, previous, err := publishPointer(ctx, tx, candidate.meta, candidate.metric, &expected,
		operation.OperationID, &actor.UserID, "REPAIR")
	if err != nil {
		return platform.OpsExecutionResult{}, err
	}
	before, _ := json.Marshal(pointerState{SnapshotID: previous, Version: version - 1})
	after, _ := json.Marshal(pointerState{SnapshotID: candidate.meta.id, Version: version})
	result, _ := json.Marshal(map[string]any{"snapshot_id": candidate.meta.id, "metric": candidate.metric,
		"pointer_version": strconv.FormatInt(version, 10), "state": "PUBLISHED"})
	return platform.OpsExecutionResult{BeforeSnapshot: before, AfterSnapshot: after, Result: result,
		RelatedBusinessID: candidate.meta.id}, nil
}

// OpsBindings is the only Rankings write surface registered with the shared
// operation engine. Rebuild is Level 2; pointer publication is fresh Level 3.
func OpsBindings(service *Service) []platform.OpsOperationBinding {
	if service == nil || service.store == nil {
		return []platform.OpsOperationBinding{}
	}
	return []platform.OpsOperationBinding{
		{Descriptor: platform.OpsOperationDescriptor{OperationType: "RANKING_REBUILD_CREATE",
			Risk: platform.OpsRiskImpactful, RequiredPermission: "rankings.rebuild",
			AllowedRoles: []string{"SUPER_ADMIN", "OPERATOR"}, TargetType: "ranking_slice",
			InputSchemaVersion: "ranking-rebuild-create.v1", ImpactSchemaVersion: "ranking-rebuild-create-impact.v1",
			RequiresReason: true, ConfirmationMode: platform.OpsConfirmExplicit, ExecutionMode: platform.OpsSameDatabase},
			Handler: rankingRebuildOpsHandler{service: service}},
		{Descriptor: platform.OpsOperationDescriptor{OperationType: "RANKING_REPAIR_PUBLISH",
			Risk: platform.OpsRiskCritical, RequiredPermission: "rankings.repair.publish",
			AllowedRoles: []string{"SUPER_ADMIN"}, TargetType: "ranking_snapshot",
			InputSchemaVersion: "ranking-repair-publish.v1", ImpactSchemaVersion: "ranking-repair-publish-impact.v1",
			RequiresReason: true, RequiresFreshAuth: true, ConfirmationMode: platform.OpsConfirmTyped,
			ExecutionMode: platform.OpsSameDatabase}, Handler: rankingPublishOpsHandler{service: service}},
	}
}

type rebuildJob struct {
	operationID, domain, metric, period, date, modelScope, modelID string
	actor, expected, attempts                                      int64
	runnable                                                       bool
}

func (s *Service) claimRebuildJob(ctx context.Context) (rebuildJob, bool, error) {
	var job rebuildJob
	claimed := false
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT operation_id::text,actor_user_id,domain,metric,period,requested_date,
			model_scope,model_id,expected_pointer_version,attempts FROM rankings.rebuild_jobs
			WHERE (status='PENDING' AND (failure_code IS NULL OR updated_at<=clock_timestamp()-interval '5 seconds'))
			 OR (status='RUNNING' AND updated_at<=clock_timestamp()-interval '2 minutes')
			ORDER BY created_at,operation_id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&job.operationID, &job.actor,
			&job.domain, &job.metric, &job.period, &job.date, &job.modelScope, &job.modelID,
			&job.expected, &job.attempts)
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
