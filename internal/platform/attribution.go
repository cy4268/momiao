package platform

import (
	"context"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type RPUsageFilter struct {
	ActivationTime time.Time
	ModelID        string
	Status         string
	From           time.Time
	To             time.Time
	Page           int
	PageSize       int
}

type RPUsageItem struct {
	LogicalRequestID     string    `json:"logical_request_id"`
	TokenID              int64     `json:"token_id,string"`
	ModelID              string    `json:"model_id"`
	ModelName            string    `json:"model_name"`
	RequestKind          string    `json:"request_kind"`
	ProviderAttemptCount int       `json:"provider_attempt_count"`
	FinalStatus          string    `json:"final_status"`
	ErrorCategory        string    `json:"error_category"`
	ChargedRawQuota      int64     `json:"charged_raw_quota,string"`
	ChargedAmount        string    `json:"charged_amount"`
	RequestedAt          time.Time `json:"requested_at"`
	CompletedAt          time.Time `json:"completed_at"`
}

type RPUsagePage struct {
	Items    []RPUsageItem `json:"items"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	Total    int64         `json:"total,string"`
	HasMore  bool          `json:"has_more"`
}

func validRPUsageFilter(filter RPUsageFilter) bool {
	if filter.ActivationTime.IsZero() || filter.Page < 1 || filter.Page > 10000 || filter.PageSize < 1 || filter.PageSize > 100 {
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
	if user <= 0 || !validRPUsageFilter(filter) {
		return result, ErrInvalidPage
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer rollback(tx)
	result, err = readRPUsage(ctx, tx, user, filter)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func readRPUsage(ctx context.Context, tx pgx.Tx, user int64, filter RPUsageFilter) (RPUsagePage, error) {
	result := RPUsagePage{Items: []RPUsageItem{}, Page: filter.Page, PageSize: filter.PageSize}
	var err error
	// A dev restore may retain the same historical request under two source IDs.
	where := ` FROM (SELECT DISTINCT ON (logical_request_id) * FROM economy.request_attributions
	 WHERE newapi_user_id=$1 ORDER BY logical_request_id,imported_at DESC,source_instance_id DESC) a
	 WHERE key_purpose_snapshot='ROLEPLAY'
	 AND entered_model_flow AND requested_at>=$2`
	args := []any{user, filter.ActivationTime}
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
		return result, nil
	}
	query := `SELECT logical_request_id,token_id,request_model_id_snapshot,request_model_name_snapshot,request_kind,
	 provider_attempt_count,final_status,error_category,charged_raw_quota,requested_at,completed_at` + where
	args = append(args, filter.PageSize, offset)
	query += " ORDER BY requested_at DESC,source_event_id DESC,source_instance_id DESC LIMIT $" + strconv.Itoa(len(args)-1) + " OFFSET $" + strconv.Itoa(len(args))
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
	return result, nil
}
