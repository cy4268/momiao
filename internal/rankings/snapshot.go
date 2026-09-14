package rankings

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/big"
	"sort"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

type assetRow struct {
	user                int64
	name, avatar, value string
}

type snapshotMeta struct {
	id, domain, period, buildKind, operationID string
	start, checked, built                    time.Time
	end                                      *time.Time
	activation                               time.Time
	hash                                     []byte
}

type rankingQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) enabled(now time.Time) bool {
	return s != nil && s.store != nil && !s.activation.IsZero() && !s.activation.After(now)
}

func metricsForDomain(domain string) ([]string, error) {
	var metrics []string
	switch domain {
	case "ASSETS":
		metrics = []string{"TOTAL_ASSETS"}
	case "GAMES":
		metrics = []string{"BIGGEST_WIN", "GAME_PROFIT", "POKER_PROFIT", "TOTAL_WAGERED"}
	case "RP":
		metrics = []string{"RP_CALLS", "RP_CREDITS", "RP_ERRORS"}
	default:
		return nil, ErrQuery
	}
	return metrics, nil
}

func (s *Service) collectAssets(ctx context.Context, now time.Time) ([]assetRow, time.Time, error) {
	if s.assets == nil {
		return nil, time.Time{}, ErrUnavailable
	}
	rows := []assetRow{}
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		read, err := tx.Query(ctx, `SELECT newapi_user_id,display_name,avatar_id
			FROM identity.master_profiles ORDER BY newapi_user_id`)
		if err != nil {
			return err
		}
		defer read.Close()
		for read.Next() {
			var row assetRow
			if err = read.Scan(&row.user, &row.name, &row.avatar); err != nil {
				return err
			}
			rows = append(rows, row)
		}
		return read.Err()
	})
	if err != nil {
		return nil, time.Time{}, err
	}
	checked := now
	for i := range rows {
		value, observed, readErr := s.assets.ReadRankingAssets(ctx, rows[i].user)
		number, valid := new(big.Int).SetString(value, 10)
		if readErr != nil || !valid || number.Sign() < 0 || len(value) > 38 || observed.IsZero() ||
			now.Sub(observed) > 5*time.Minute || observed.After(now.Add(time.Minute)) {
			return nil, time.Time{}, ErrUnavailable
		}
		if observed.Before(checked) {
			checked = observed
		}
		rows[i].value = value
	}
	return rows, checked.UTC(), nil
}

func (s *Service) buildInputs(ctx context.Context, domain string, now time.Time) ([]assetRow, time.Time, error) {
	checked := now
	if domain == "RP" {
		if s.attributionHealth == nil || s.sourceID == "" {
			return nil, time.Time{}, ErrUnavailable
		}
		var err error
		checked, err = s.attributionHealth(ctx)
		if err != nil || checked.IsZero() || now.Sub(checked) > 5*time.Minute || checked.After(now.Add(time.Minute)) {
			return nil, time.Time{}, ErrUnavailable
		}
	}
	if domain == "ASSETS" {
		return s.collectAssets(ctx, now)
	}
	return nil, checked.UTC(), nil
}

func readSnapshotMeta(ctx context.Context, q rankingQuerier, id string) (snapshotMeta, error) {
	var result snapshotMeta
	var operationID *string
	err := q.QueryRow(ctx, `SELECT snapshot_id::text,domain,period,period_start,period_end,
		activation_at,source_checked_at,built_at,build_kind,operation_id::text,aggregate_hash
		FROM rankings.snapshots WHERE snapshot_id=$1::uuid`, id).Scan(&result.id, &result.domain,
		&result.period, &result.start, &result.end, &result.activation, &result.checked, &result.built,
		&result.buildKind, &operationID, &result.hash)
	if operationID != nil {
		result.operationID = *operationID
	}
	return result, err
}

func computeSnapshotHash(ctx context.Context, q rankingQuerier, id string) ([]byte, error) {
	meta, err := readSnapshotMeta(ctx, q, id)
	if err != nil {
		return nil, err
	}
	digest := sha256.New()
	write := func(value string) {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(value)))
		_, _ = digest.Write(size[:])
		_, _ = digest.Write([]byte(value))
	}
	write("CHALDEA-RANKING-AGGREGATE-V1")
	write(meta.domain)
	write(meta.period)
	write(meta.start.UTC().Format(time.RFC3339Nano))
	if meta.end == nil {
		write("")
	} else {
		write(meta.end.UTC().Format(time.RFC3339Nano))
	}
	write(meta.activation.UTC().Format(time.RFC3339Nano))
	write(meta.checked.UTC().Format(time.RFC3339Nano))
	rows, err := q.Query(ctx, `SELECT metric,model_scope,model_id,newapi_user_id::text,value::text,
		calls::text,errors::text,credits_units::text,models::text FROM rankings.entries
		WHERE snapshot_id=$1::uuid ORDER BY metric COLLATE "C",model_scope COLLATE "C",
		model_id COLLATE "C",newapi_user_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var metric, scope, model, user, value, calls, failures, credits, models string
		if err = rows.Scan(&metric, &scope, &model, &user, &value, &calls, &failures, &credits, &models); err != nil {
			return nil, err
		}
		for _, field := range []string{metric, scope, model, user, value, calls, failures, credits, models} {
			write(field)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return digest.Sum(nil), nil
}

func (s *Service) buildSnapshotTx(ctx context.Context, tx pgx.Tx, domain, period string,
	start time.Time, end *time.Time, checked time.Time, assets []assetRow, buildKind, operationID string) (string, error) {
	if buildKind != "ROUTINE" && buildKind != "REPAIR" {
		return "", ErrQuery
	}
	if operationID != "" {
		var existing string
		err := tx.QueryRow(ctx, `SELECT snapshot_id::text FROM rankings.snapshots
			WHERE operation_id=$1::uuid`, operationID).Scan(&existing)
		if err == nil {
			meta, readErr := readSnapshotMeta(ctx, tx, existing)
			if readErr != nil || meta.domain != domain || meta.period != period || !meta.start.Equal(start) ||
				!meta.activation.Equal(s.activation) || meta.buildKind != buildKind || len(meta.hash) != sha256.Size {
				return "", platform.ErrOpsConflict
			}
			return existing, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}
	lockKey := "rankings:build:" + domain + ":" + period + ":" + start.UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return "", err
	}
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO rankings.snapshots(domain,period,period_start,period_end,
		activation_at,source_checked_at,status,build_kind,operation_id)
		VALUES($1,$2,$3,$4,$5,$6,'READY',$7,NULLIF($8,'')::uuid) RETURNING snapshot_id::text`,
		domain, period, start, end, s.activation, checked, buildKind, operationID).Scan(&id)
	if err != nil {
		return "", err
	}
	switch domain {
	case "GAMES":
		_, err = tx.Exec(ctx, gameAggregateSQL, id)
	case "RP":
		_, err = tx.Exec(ctx, rpAggregateSQL, id, s.sourceID)
	case "ASSETS":
		for _, row := range assets {
			if row.value == "0" {
				continue
			}
			_, err = tx.Exec(ctx, `INSERT INTO rankings.entries(snapshot_id,metric,newapi_user_id,
				display_name,avatar_id,value) VALUES($1,'TOTAL_ASSETS',$2,$3,$4,$5::numeric)`,
				id, row.user, row.name, row.avatar, row.value)
			if err != nil {
				return "", err
			}
		}
	}
	if err != nil {
		return "", err
	}
	hash, err := computeSnapshotHash(ctx, tx, id)
	if err != nil {
		return "", err
	}
	if len(hash) != sha256.Size {
		return "", platform.ErrOpsConflict
	}
	tag, err := tx.Exec(ctx, `UPDATE rankings.snapshots SET aggregate_hash=$2
		WHERE snapshot_id=$1::uuid AND aggregate_hash IS NULL`, id, hash)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() != 1 {
		return "", platform.ErrOpsConflict
	}
	return id, nil
}

func (s *Service) buildSnapshot(ctx context.Context, domain, period, date, buildKind, operationID string) (string, error) {
	now := time.Now().UTC()
	if !s.enabled(now) {
		return "", ErrUnavailable
	}
	if _, err := metricsForDomain(domain); err != nil || domain == "ASSETS" && period != "CURRENT" ||
		domain != "ASSETS" && period == "CURRENT" {
		return "", ErrQuery
	}
	start, end, err := periodBounds(period, date, now, s.activation)
	if err != nil {
		return "", err
	}
	assets, checked, err := s.buildInputs(ctx, domain, now)
	if err != nil {
		return "", err
	}
	var id string
	err = s.store.WithTx(ctx, func(tx pgx.Tx) error {
		var buildErr error
		id, buildErr = s.buildSnapshotTx(ctx, tx, domain, period, start, end, checked, assets, buildKind, operationID)
		return buildErr
	})
	return id, err
}

func pointerLockKey(domain, metric, period string, start, activation time.Time) string {
	return "rankings:pointer:" + domain + ":" + metric + ":" + period + ":" +
		start.UTC().Format(time.RFC3339Nano) + ":" + activation.UTC().Format(time.RFC3339Nano)
}

func publishPointer(ctx context.Context, tx pgx.Tx, meta snapshotMeta, metric string,
	expectedVersion *int64, operationID string, actor *int64, source string) (int64, string, error) {
	if len(meta.hash) != sha256.Size || metricDomain(metric) != meta.domain {
		return 0, "", platform.ErrOpsConflict
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		pointerLockKey(meta.domain, metric, meta.period, meta.start, meta.activation)); err != nil {
		return 0, "", err
	}
	var previous string
	var currentVersion int64
	err := tx.QueryRow(ctx, `SELECT snapshot_id::text,version FROM rankings.published_pointers
		WHERE domain=$1 AND metric=$2 AND period=$3 AND period_start=$4 AND activation_at=$5 FOR UPDATE`,
		meta.domain, metric, meta.period, meta.start, meta.activation).Scan(&previous, &currentVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		if expectedVersion != nil && *expectedVersion != 0 {
			return 0, "", platform.ErrOpsPreviewStale
		}
		currentVersion = 0
		previous = ""
	} else if err != nil {
		return 0, "", err
	} else if expectedVersion != nil && *expectedVersion != currentVersion {
		return 0, "", platform.ErrOpsPreviewStale
	}
	if previous == meta.id {
		if source == "REPAIR" {
			return 0, "", platform.ErrOpsConflict
		}
		return currentVersion, previous, nil
	}
	version := currentVersion + 1
	if currentVersion == 0 {
		_, err = tx.Exec(ctx, `INSERT INTO rankings.published_pointers(domain,metric,period,period_start,
			activation_at,snapshot_id,version) VALUES($1,$2,$3,$4,$5,$6,$7)`, meta.domain, metric,
			meta.period, meta.start, meta.activation, meta.id, version)
	} else {
		_, err = tx.Exec(ctx, `UPDATE rankings.published_pointers SET snapshot_id=$6,version=$7,
			published_at=clock_timestamp() WHERE domain=$1 AND metric=$2 AND period=$3 AND period_start=$4
			AND activation_at=$5`, meta.domain, metric, meta.period, meta.start, meta.activation, meta.id, version)
	}
	if err != nil {
		return 0, "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO rankings.publication_events(operation_id,domain,metric,period,
		period_start,activation_at,previous_snapshot_id,snapshot_id,pointer_version,actor_user_id,source)
		VALUES(NULLIF($1,'')::uuid,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid,$8,$9,$10,$11)`, operationID,
		meta.domain, metric, meta.period, meta.start, meta.activation, previous, meta.id, version, actor, source)
	return version, previous, err
}

func (s *Service) publishRoutine(ctx context.Context, id string) error {
	return s.store.WithTx(ctx, func(tx pgx.Tx) error {
		meta, err := readSnapshotMeta(ctx, tx, id)
		if err != nil || meta.buildKind != "ROUTINE" || len(meta.hash) != sha256.Size {
			if err != nil {
				return err
			}
			return platform.ErrOpsConflict
		}
		if err = platform.RequireNoMaintenance(ctx, tx, "RANKINGS_PUBLISHING"); err != nil {
			return err
		}
		metrics, err := metricsForDomain(meta.domain)
		if err != nil {
			return err
		}
		sort.Strings(metrics)
		for _, metric := range metrics {
			if _, _, err = publishPointer(ctx, tx, meta, metric, nil, "", nil, "ROUTINE"); err != nil {
				return err
			}
		}
		return nil
	})
}

// Build appends and finalizes an immutable routine snapshot. Publication uses a
// separate transaction so maintenance can retain the verified build while the
// public reader continues serving the last good pointer.
func (s *Service) Build(ctx context.Context, domain, period, date string) (string, error) {
	id, err := s.buildSnapshot(ctx, domain, period, date, "ROUTINE", "")
	if err != nil {
		return "", err
	}
	if err = s.publishRoutine(ctx, id); errors.Is(err, platform.ErrMaintenanceActive) {
		return id, nil
	}
	return id, err
}

// Run retains the existing minute worker and gives one durable repair job
// priority on every pass. A successful repair build remains a shadow.
func (s *Service) Run(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		work, cancel := context.WithTimeout(ctx, 45*time.Second)
		_, _ = s.RunRebuildStep(work)
		cancel()
		for _, domain := range []string{"GAMES", "RP", "ASSETS"} {
			periods := []string{"DAY", "WEEK", "ALL_TIME"}
			if domain == "ASSETS" {
				periods = []string{"CURRENT"}
			}
			for _, period := range periods {
				if ctx.Err() != nil {
					return
				}
				work, cancel = context.WithTimeout(ctx, 45*time.Second)
				_, _ = s.Build(work, domain, period, "")
				cancel()
			}
			if domain != "ASSETS" {
				s.repairMissingPeriod(ctx, domain)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Service) repairMissingPeriod(ctx context.Context, domain string) {
	if !s.enabled(time.Now()) {
		return
	}
	var period, date string
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `WITH candidates AS (
		 SELECT 'DAY' period,d start,d+interval '1 day' finish FROM generate_series(date_trunc('day',$1::timestamptz AT TIME ZONE 'Asia/Shanghai'),date_trunc('day',now() AT TIME ZONE 'Asia/Shanghai')-interval '1 day',interval '1 day') d
		 UNION ALL SELECT 'WEEK',d,d+interval '7 days' FROM generate_series(date_trunc('week',$1::timestamptz AT TIME ZONE 'Asia/Shanghai'),date_trunc('week',now() AT TIME ZONE 'Asia/Shanghai')-interval '7 days',interval '7 days') d
		) SELECT c.period,to_char(c.start,'YYYY-MM-DD') FROM candidates c WHERE NOT EXISTS(
		 SELECT 1 FROM rankings.published_pointers p WHERE p.domain=$2 AND p.period=c.period AND p.period_start=c.start AT TIME ZONE 'Asia/Shanghai'
		  AND p.activation_at=$1)
		 OR ($2='RP' AND EXISTS(SELECT 1 FROM economy.request_attributions a WHERE a.source_instance_id=$3::uuid
		  AND a.key_purpose_snapshot='ROLEPLAY' AND a.entered_model_flow
		  AND a.requested_at>=greatest($1::timestamptz,c.start AT TIME ZONE 'Asia/Shanghai') AND a.requested_at<c.finish AT TIME ZONE 'Asia/Shanghai'
		  AND a.imported_at>coalesce((SELECT min(s.built_at) FROM rankings.published_pointers p
		   JOIN rankings.snapshots s USING(snapshot_id) WHERE p.domain=$2 AND p.period=c.period
		   AND p.period_start=c.start AT TIME ZONE 'Asia/Shanghai' AND p.activation_at=$1),'-infinity'::timestamptz)))
		 ORDER BY c.start,c.period LIMIT 1`, s.activation, domain, nullableSourceID(s.sourceID)).Scan(&period, &date)
	})
	if err != nil {
		return
	}
	work, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	_, _ = s.Build(work, domain, period, date)
}

func nullableSourceID(value string) any {
	if value == "" {
		return nil
	}
	return value
}
