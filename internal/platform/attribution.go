package platform

import (
	"context"
	"errors"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type AttributionHealth struct {
	SourceInstanceID string    `json:"source_instance_id"`
	LastCursor       int64     `json:"last_cursor,string"`
	LastSuccessAt    time.Time `json:"last_success_at"`
	SourceObservedAt time.Time `json:"source_observed_at"`
	FullyCaughtUp    bool      `json:"fully_caught_up"`
	LastErrorCode    string    `json:"last_error_code"`
}

func (s *Store) AttributionCursor(ctx context.Context, expectedSourceID string) (int64, error) {
	if !ValidOperationKey(expectedSourceID) {
		return 0, ErrInvalidMutation
	}
	var cursor int64
	err := s.pool.QueryRow(ctx, `SELECT last_cursor FROM platform_meta.attribution_checkpoints WHERE source_instance_id=$1`, expectedSourceID).Scan(&cursor)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return cursor, err
}

func (s *Store) ImportAttributionPage(ctx context.Context, expectedSourceID string, page AttributionPage) error {
	if !ValidOperationKey(expectedSourceID) || page.SourceInstanceID != expectedSourceID || page.NextCursor < 0 || page.ObservedAt.IsZero() {
		return ErrInvalidMutation
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = lockIdentity(ctx, tx, "attribution-source", expectedSourceID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO platform_meta.attribution_checkpoints(source_instance_id) VALUES($1) ON CONFLICT DO NOTHING`, expectedSourceID)
	if err != nil {
		return err
	}
	var cursor int64
	if err = tx.QueryRow(ctx, `SELECT last_cursor FROM platform_meta.attribution_checkpoints WHERE source_instance_id=$1 FOR UPDATE`, expectedSourceID).Scan(&cursor); err != nil {
		return err
	}
	if len(page.Items) == 0 {
		if page.NextCursor != cursor || page.HasMore {
			return ErrNativeAttributionDependency
		}
	} else if page.Items[0].SourceEventID != cursor+1 || page.Items[len(page.Items)-1].SourceEventID != page.NextCursor {
		return ErrNativeAttributionDependency
	}
	for i, row := range page.Items {
		if row.SourceInstanceID != expectedSourceID || row.SourceEventID != cursor+int64(i)+1 {
			return ErrNativeAttributionDependency
		}
		_, err = tx.Exec(ctx, `INSERT INTO economy.request_attributions(
 source_instance_id,source_event_id,logical_request_id,newapi_user_id,token_id,key_purpose_snapshot,purpose_version_snapshot,
 request_model_id_snapshot,request_model_name_snapshot,request_kind,entered_model_flow,provider_attempt_count,
 final_status,error_category,charged_raw_quota,requested_at,completed_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
			expectedSourceID, row.SourceEventID, row.LogicalRequestID, row.UserID, row.TokenID, row.KeyPurposeSnapshot, row.PurposeVersionSnapshot,
			row.RequestModelIDSnapshot, row.RequestModelNameSnapshot, row.RequestKind, row.EnteredModelFlow, row.ProviderAttemptCount,
			row.FinalStatus, row.ErrorCategory, row.ChargedRawQuota, row.RequestedAt, row.CompletedAt)
		if err != nil {
			return err
		}
	}
	var updated int64
	err = tx.QueryRow(ctx, `UPDATE platform_meta.attribution_checkpoints SET last_cursor=$2,last_success_at=clock_timestamp(),
 source_observed_at=$3,fully_caught_up=$4,last_error_code='',updated_at=clock_timestamp()
 WHERE source_instance_id=$1 AND last_cursor=$5 RETURNING last_cursor`, expectedSourceID, page.NextCursor, page.ObservedAt.UTC(), !page.HasMore, cursor).Scan(&updated)
	if err != nil || updated != page.NextCursor {
		if err != nil {
			return err
		}
		return ErrNativeAttributionDependency
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordAttributionFailure(ctx context.Context, expectedSourceID, code string) error {
	if !ValidOperationKey(expectedSourceID) || !validAttributionLabel(code, 64) {
		return ErrInvalidMutation
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO platform_meta.attribution_checkpoints(source_instance_id,last_error_code,fully_caught_up)
 VALUES($1,$2,false) ON CONFLICT(source_instance_id) DO UPDATE SET last_error_code=excluded.last_error_code,
 fully_caught_up=false,updated_at=clock_timestamp()`, expectedSourceID, code)
	return err
}

func (s *Store) ReadAttributionHealth(ctx context.Context, expectedSourceID string) (AttributionHealth, error) {
	health := AttributionHealth{SourceInstanceID: expectedSourceID}
	if !ValidOperationKey(expectedSourceID) {
		return health, ErrInvalidMutation
	}
	var success, observed *time.Time
	err := s.pool.QueryRow(ctx, `SELECT last_cursor,last_success_at,source_observed_at,fully_caught_up,last_error_code
 FROM platform_meta.attribution_checkpoints WHERE source_instance_id=$1`, expectedSourceID).Scan(
		&health.LastCursor, &success, &observed, &health.FullyCaughtUp, &health.LastErrorCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return health, nil
	}
	if err != nil {
		return health, err
	}
	if success != nil {
		health.LastSuccessAt = success.UTC()
	}
	if observed != nil {
		health.SourceObservedAt = observed.UTC()
	}
	return health, nil
}

func RunAttributionWorker(ctx context.Context, store *Store, reader NativeAttributionReader, expectedSourceID string) {
	if store == nil || reader == nil || !ValidOperationKey(expectedSourceID) {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		work, cancel := context.WithTimeout(ctx, 8*time.Second)
		cursor, err := store.AttributionCursor(work, expectedSourceID)
		var page AttributionPage
		if err == nil {
			page, err = reader.ReadAttributions(work, cursor, 500)
		}
		if err == nil {
			err = store.ImportAttributionPage(work, expectedSourceID, page)
		}
		cancel()
		if err != nil {
			failure, stop := context.WithTimeout(ctx, 3*time.Second)
			_ = store.RecordAttributionFailure(failure, expectedSourceID, "ATTRIBUTION_SYNC_FAILED")
			stop()
		}
		if err == nil && page.HasMore {
			continue
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

type RPUsageFilter struct {
	SourceInstanceID string
	ActivationTime   time.Time
	ModelID          string
	Status           string
	From             time.Time
	To               time.Time
	Page             int
	PageSize         int
}

type RPUsageItem struct {
	LogicalRequestID         string    `json:"logical_request_id"`
	TokenID                  int64     `json:"token_id,string"`
	ModelID                  string    `json:"model_id"`
	ModelName                string    `json:"model_name"`
	RequestKind              string    `json:"request_kind"`
	ProviderAttemptCount     int       `json:"provider_attempt_count"`
	FinalStatus              string    `json:"final_status"`
	ErrorCategory            string    `json:"error_category"`
	ChargedRawQuota          int64     `json:"charged_raw_quota,string"`
	ChargedAmount            string    `json:"charged_amount"`
	RequestedAt              time.Time `json:"requested_at"`
	CompletedAt              time.Time `json:"completed_at"`
}

type RPUsagePage struct {
	Items    []RPUsageItem `json:"items"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	Total    int64         `json:"total,string"`
	HasMore  bool          `json:"has_more"`
}

func validRPUsageFilter(filter RPUsageFilter) bool {
	if !ValidOperationKey(filter.SourceInstanceID) || filter.ActivationTime.IsZero() || filter.Page < 1 || filter.Page > 10000 || filter.PageSize < 1 || filter.PageSize > 100 {
		return false
	}
	if filter.ModelID != "" && (!utf8.ValidString(filter.ModelID) || len(filter.ModelID) > 128) {
		return false
	}
	if filter.Status != "" && filter.Status != "SUCCESS" && filter.Status != "ERROR" && filter.Status != "CANCELLED_POST_UPSTREAM" {
		return false
	}
	return (filter.From.IsZero() || filter.To.IsZero() || filter.From.Before(filter.To)) && filter.ActivationTime.UTC().Equal(filter.ActivationTime)
}

func (s *Store) ReadRPUsage(ctx context.Context, user int64, filter RPUsageFilter) (RPUsagePage, error) {
	result := RPUsagePage{Items: []RPUsageItem{}, Page: filter.Page, PageSize: filter.PageSize}
	if !validateQuotaUser(user) || !validRPUsageFilter(filter) {
		return result, ErrInvalidPage
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer rollback(tx)
	where := ` FROM economy.request_attributions WHERE source_instance_id=$1 AND newapi_user_id=$2 AND key_purpose_snapshot='ROLEPLAY'
	 AND entered_model_flow AND requested_at>=$3`
	args := []any{filter.SourceInstanceID, user, filter.ActivationTime}
	add := func(value any, clause string) {
		args = append(args, value)
		where += clause + "$" + strconv.Itoa(len(args))
	}
	if filter.ModelID != "" {
		add(filter.ModelID, " AND request_model_id_snapshot=")
	}
	if filter.Status != "" {
		add(filter.Status, " AND final_status=")
	}
	if !filter.From.IsZero() {
		add(filter.From.UTC(), " AND requested_at>=")
	}
	if !filter.To.IsZero() {
		add(filter.To.UTC(), " AND requested_at<")
	}
	if err = tx.QueryRow(ctx, "SELECT count(*)"+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	offset := (filter.Page - 1) * filter.PageSize
	if int64(offset) >= result.Total {
		return result, tx.Commit(ctx)
	}
	query := `SELECT logical_request_id,token_id,request_model_id_snapshot,request_model_name_snapshot,request_kind,
	 provider_attempt_count,final_status,error_category,charged_raw_quota,requested_at,completed_at` + where
	args = append(args, filter.PageSize, offset)
	query += " ORDER BY requested_at DESC,source_event_id DESC LIMIT $" + strconv.Itoa(len(args)-1) + " OFFSET $" + strconv.Itoa(len(args))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item RPUsageItem
		if err = rows.Scan(&item.LogicalRequestID, &item.TokenID, &item.ModelID, &item.ModelName, &item.RequestKind,
			&item.ProviderAttemptCount, &item.FinalStatus, &item.ErrorCategory, &item.ChargedRawQuota, &item.RequestedAt, &item.CompletedAt); err != nil {
			return result, err
		}
		item.ChargedAmount = FormatAmount(item.ChargedRawQuota)
		item.RequestedAt = item.RequestedAt.UTC()
		item.CompletedAt = item.CompletedAt.UTC()
		result.Items = append(result.Items, item)
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	result.HasMore = int64(filter.Page*filter.PageSize) < result.Total
	return result, tx.Commit(ctx)
}
