package platform

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// A separate source namespace keeps local log IDs independent of native cursors.
const LocalRelaySourceID = "8be7cf11-88df-46e9-a6ce-e81bf166e1d4"

// Prompt includes cached reads/writes; CacheWrite includes the 1h subset.
// Protocol adaptors normalize official usage semantics before calling billing.
type RelayUsage struct{ Prompt, Completion, CacheRead, CacheWrite, CacheWrite1h int64 }

// Price SQL follows official text_quota.go's disjoint input/cache accounting,
// with final unit prices and PostgreSQL numeric instead of float arithmetic.
const relayCostSQL = `SELECT CASE WHEN amount>0 THEN greatest(1,round(amount))::bigint ELSE 0 END FROM (
 SELECT coalesce(p.per_request,p.per_second*coalesce($8::bigint,p.default_seconds),
 (greatest($3::numeric-$5::numeric-$6::numeric,0)*p.input+$4::numeric*p.output+$5::numeric*p.cache_read+
 greatest($6::numeric-$7::numeric,0)*p.cache_write+$7::numeric*p.cache_write_1h)/1000000)*g.ratio AS amount
 FROM relay.prices p CROSS JOIN relay.groups g WHERE p.model=$1 AND g.name=$2) quote`

func (s *Store) RelayCost(ctx context.Context, model, group string, usage RelayUsage, seconds *int64) (int64, error) {
	var cost int64
	err := s.pool.QueryRow(ctx, relayCostSQL, model, group, usage.Prompt, usage.Completion, usage.CacheRead, usage.CacheWrite, usage.CacheWrite1h, seconds).Scan(&cost)
	return cost, err
}

type RelaySettlement struct {
	RequestID                                   string
	UserID, KeyID, ChannelID                    int64
	KeyName, Purpose, Model, Group, RequestKind string
	Usage                                       RelayUsage
	CostUnits                                   int64
	RequestedAt, CompletedAt                    time.Time
	DurationMS                                  int64
	IsStream                                    bool
	StatusCode                                  int
	FinalStatus, Error                          string
	ProviderAttempts                            int
}
type RelayCharge struct{ LogID, CostUnits, ChargedUnits int64 }

func (s *Store) SettleRelay(ctx context.Context, input RelaySettlement) (RelayCharge, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RelayCharge{}, err
	}
	defer rollback(tx)
	result, err := settleRelay(ctx, tx, input)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func settleRelay(ctx context.Context, tx pgx.Tx, input RelaySettlement) (RelayCharge, error) {
	result := RelayCharge{CostUnits: input.CostUnits}
	if err := LockEconomyUsersInTx(ctx, tx, input.UserID); err != nil {
		return result, err
	}
	err := tx.QueryRow(ctx, `SELECT a.source_event_id,l.cost_units,a.charged_raw_quota FROM economy.request_attributions a
 JOIN relay.logs l ON l.id=a.source_event_id WHERE a.source_instance_id=$1 AND a.logical_request_id=$2 AND a.newapi_user_id=$3`, LocalRelaySourceID, input.RequestID, input.UserID).Scan(&result.LogID, &result.CostUnits, &result.ChargedUnits)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	var balance int64
	if err = tx.QueryRow(ctx, `SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type='RESERVE_API_CREDIT' FOR UPDATE`, input.UserID).Scan(&balance); err != nil {
		return result, err
	}
	result.ChargedUnits = min(balance, input.CostUnits)
	if result.ChargedUnits > 0 {
		_, err = applyInTx(ctx, tx, Mutation{UserID: input.UserID, Asset: ReserveAPICredit, DeltaUnits: -result.ChargedUnits, BizType: "API_USAGE", BizID: input.RequestID, EntryType: "API_USAGE", IdempotencyKey: input.RequestID})
		if err != nil {
			return result, err
		}
	}
	// The request's key/name/purpose are snapshots. A key deleted while an upstream
	// is running must not erase that request's history or prevent Reserve settlement.
	if _, err = tx.Exec(ctx, `UPDATE relay.api_keys SET used_quota=used_quota+$3,accessed_at=$4 WHERE id=$1 AND user_id=$2`, input.KeyID, input.UserID, input.CostUnits, input.CompletedAt); err != nil {
		return result, err
	}
	err = insertRelayLog(ctx, tx, input).Scan(&result.LogID)
	if err != nil {
		return result, err
	}
	attributionError := input.Error
	if input.FinalStatus == "SUCCESS" {
		attributionError = "" // Warnings belong to the log, not the failure category.
	}
	_, err = tx.Exec(ctx, `INSERT INTO economy.request_attributions(source_instance_id,source_event_id,logical_request_id,newapi_user_id,token_id,
 key_purpose_snapshot,purpose_version_snapshot,request_model_id_snapshot,request_model_name_snapshot,request_kind,entered_model_flow,provider_attempt_count,
 final_status,error_category,charged_raw_quota,requested_at,completed_at)
 VALUES($1,$2,$3,$4,$5,$6,CASE WHEN $6='UNCLASSIFIED' THEN 0 ELSE 1 END,$7,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		LocalRelaySourceID, result.LogID, input.RequestID, input.UserID, input.KeyID, input.Purpose, input.Model, input.RequestKind, input.ProviderAttempts > 0,
		input.ProviderAttempts, input.FinalStatus, attributionError, result.ChargedUnits, input.RequestedAt, input.CompletedAt)
	return result, err
}

// Failed attempts have no charge or attribution. The final settlement owns both.
func (s *Store) LogRelayAttempt(ctx context.Context, input RelaySettlement) error {
	input.FinalStatus, input.CostUnits, input.Usage = "ERROR", 0, RelayUsage{}
	var id int64
	return insertRelayLog(ctx, s.pool, input).Scan(&id)
}

func insertRelayLog(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, input RelaySettlement) pgx.Row {
	kind := 2
	if input.FinalStatus != "SUCCESS" {
		kind = 5
	}
	return db.QueryRow(ctx, `INSERT INTO relay.logs(user_id,key_id,key_name,model,channel_id,group_name,kind,prompt_tokens,completion_tokens,
 cache_read_tokens,cache_write_tokens,cache_write_tokens_1h,cost_units,duration_ms,is_stream,status_code,error,created_at,request_id,attempt)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20) RETURNING id`,
		input.UserID, input.KeyID, input.KeyName, input.Model, input.ChannelID, input.Group, kind, input.Usage.Prompt, input.Usage.Completion,
		input.Usage.CacheRead, input.Usage.CacheWrite, input.Usage.CacheWrite1h, input.CostUnits, input.DurationMS, input.IsStream, input.StatusCode, input.Error, input.RequestedAt,
		input.RequestID, max(input.ProviderAttempts, 1))
}
