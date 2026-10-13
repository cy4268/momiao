package platform

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// One real-dev transaction exercises pricing, wallet/log/key/attribution settlement
// and its readers. Every mutation (including leaderboard creation) is rolled back.
func TestLocalRelayBilling(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires the existing dev database")
	}
	dsn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read dev DSN")
	}
	s, err := Open(t.Context(), strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("open dev database")
	}
	defer s.Close()
	ctx := t.Context()
	usage := RelayUsage{Prompt: 1000, Completion: 100, CacheRead: 200, CacheWrite: 100, CacheWrite1h: 40}
	cost, err := s.RelayCost(ctx, "gpt-5.6-sol", "default", usage, nil)
	if err != nil || cost != 2750 {
		t.Fatal("final token/cache prices", cost, err)
	}
	image, err := s.RelayCost(ctx, "grok-imagine-image-2.0", "default", RelayUsage{}, nil)
	if err != nil || image != 20000 {
		t.Fatal("per-request price", image, err)
	}
	video, err := s.RelayCost(ctx, "grok-imagine-video", "default", RelayUsage{}, nil)
	if err != nil || video != 280000 {
		t.Fatal("duration price/default", video, err)
	}
	zero, err := s.RelayCost(ctx, "gpt-5.6-sol", "default", RelayUsage{}, nil)
	if err != nil || zero != 0 {
		t.Fatal("missing usage estimated tokens", zero, err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var input RelaySettlement
	var before, usedBefore int64
	err = tx.QueryRow(ctx, `SELECT k.user_id,k.id,k.name,k.purpose,u.group_name,b.balance_units,k.used_quota
 FROM relay.api_keys k JOIN identity.users u ON u.id=k.user_id JOIN identity.master_profiles m ON m.newapi_user_id=u.id
 JOIN economy.wallet_balances b ON b.newapi_user_id=u.id AND b.asset_type='RESERVE_API_CREDIT'
 WHERE k.status=1 AND k.purpose='ROLEPLAY' AND u.status=1 AND u.deleted_at IS NULL AND u.group_name='default' AND b.balance_units>10000 ORDER BY k.id LIMIT 1`).Scan(&input.UserID, &input.KeyID, &input.KeyName, &input.Purpose, &input.Group, &before, &usedBefore)
	if err != nil {
		t.Fatal("existing dev RP account/key", err)
	}
	input.RequestID, _ = uuidV7()
	input.Model = "gpt-5.6-sol"
	input.RequestKind = "OPENAI_CHAT"
	input.Usage = usage
	input.CostUnits = cost
	input.RequestedAt = time.Now().UTC()
	input.CompletedAt = input.RequestedAt.Add(time.Second)
	input.DurationMS = 1000
	input.FinalStatus = "SUCCESS"
	input.StatusCode = 200
	input.ProviderAttempts = 1
	paid, err := settleRelay(ctx, tx, input)
	if err != nil || paid.CostUnits != cost || paid.ChargedUnits != cost {
		t.Fatal("Reserve settlement", paid, err)
	}
	repeated, err := settleRelay(ctx, tx, input)
	if err != nil || repeated != paid {
		t.Fatal("request ID replay", repeated, err)
	}
	var balance, used, entries, logged, attributed int64
	err = tx.QueryRow(ctx, `SELECT b.balance_units,k.used_quota,
 (SELECT count(*) FROM economy.wallet_ledger WHERE biz_type='API_USAGE' AND biz_id=$3),
 (SELECT cost_units FROM relay.logs WHERE id=$4),
 (SELECT charged_raw_quota FROM economy.request_attributions WHERE source_instance_id=$5 AND logical_request_id=$3)
 FROM economy.wallet_balances b CROSS JOIN relay.api_keys k WHERE b.newapi_user_id=$1 AND b.asset_type='RESERVE_API_CREDIT' AND k.id=$2`, input.UserID, input.KeyID, input.RequestID, paid.LogID, LocalRelaySourceID).Scan(&balance, &used, &entries, &logged, &attributed)
	if err != nil || balance != before-cost || used != usedBefore+cost || entries != 1 || logged != cost || attributed != cost {
		t.Fatal("atomic debit/key/log/attribution", err)
	}
	// A late attribution constraint failure must undo the debit and key/log writes.
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bad := input
	bad.RequestID, _ = uuidV7()
	bad.FinalStatus = "INVALID"
	if _, err = settleRelay(ctx, savepoint, bad); err == nil {
		t.Fatal("invalid final status committed")
	}
	if err = savepoint.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var afterFailure int64
	if err = tx.QueryRow(ctx, `SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type='RESERVE_API_CREDIT'`, input.UserID).Scan(&afterFailure); err != nil || afterFailure != balance {
		t.Fatal("partial settlement remained", err)
	}
	drained := input
	drained.RequestID, _ = uuidV7()
	drained.CostUnits = balance + 7
	capped, err := settleRelay(ctx, tx, drained)
	if err != nil || capped.ChargedUnits != balance || capped.CostUnits != balance+7 {
		t.Fatal("cap debit at available Reserve", capped, err)
	}
	failed := input
	failed.RequestID, _ = uuidV7()
	failed.CostUnits = 0
	failed.Usage = RelayUsage{}
	failed.FinalStatus = "ERROR"
	failed.Error = "UPSTREAM_ERROR"
	failed.StatusCode = 502
	failed.ProviderAttempts = 2
	failure, err := settleRelay(ctx, tx, failed)
	if err != nil || failure.ChargedUnits != 0 {
		t.Fatal("zero-cost error at empty Reserve", failure, err)
	}
	if err = tx.QueryRow(ctx, `SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type='RESERVE_API_CREDIT'`, input.UserID).Scan(&balance); err != nil || balance != 0 {
		t.Fatal("Reserve became negative", err)
	}
	var source string
	if err = tx.QueryRow(ctx, `SELECT source_instance_id::text FROM platform_meta.attribution_checkpoints ORDER BY source_instance_id LIMIT 1`).Scan(&source); err != nil {
		t.Fatal(err)
	}
	page, err := readRPUsage(ctx, tx, input.UserID, RPUsageFilter{SourceInstanceID: source, ActivationTime: input.RequestedAt, ModelID: input.Model, Page: 1, PageSize: 10})
	if err != nil || page.Total != 3 {
		t.Fatal("RP reader missed local source", page.Total, err)
	}
	var snapshot string
	err = tx.QueryRow(ctx, `INSERT INTO rankings.snapshots(domain,period,period_start,activation_at,source_checked_at,status)
 VALUES('RP','ALL_TIME',$1,$1,$1,'READY') RETURNING snapshot_id::text`, input.RequestedAt).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SELECT rankings.build_rp_entries($1,$2)`, snapshot, source); err != nil {
		t.Fatal("rank local attribution", err)
	}
	var calls, errorsCount, credits int64
	err = tx.QueryRow(ctx, `SELECT calls::bigint,errors::bigint,credits_units::bigint FROM rankings.entries WHERE snapshot_id=$1 AND metric='RP_CREDITS' AND model_scope='ALL' AND newapi_user_id=$2`, snapshot, input.UserID).Scan(&calls, &errorsCount, &credits)
	if err != nil || calls != 2 || errorsCount != 1 || credits != before {
		t.Fatal("rankings omitted local costs", calls, errorsCount, credits, err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	original, err := s.ReadWallet(ctx, input.UserID, ReserveAPICredit)
	if err != nil || original.BalanceUnits != before {
		t.Fatal("dev transaction rollback", err)
	}
	var remaining int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM economy.request_attributions WHERE source_instance_id=$1 AND logical_request_id=ANY($2::text[])`, LocalRelaySourceID, []string{input.RequestID, bad.RequestID, drained.RequestID, failed.RequestID}).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("test left local attributions", remaining, err)
	}
}
