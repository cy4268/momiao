package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/rankings"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The one campaign regression grows with the snapshot/reset implementation;
// reuse the existing quota fixture and real-PG harness, not a second framework.
func TestGamblerCampaignSnapshotResetLifecycle(t *testing.T) {
	var err error
	owner, runtime := gameBrowserStores(t)
	ctx := context.Background()
	for _, u := range []int64{701, 702, 703} {
		if err := owner.EnsureAccount(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := owner.Bootstrap(ctx, platform.BootstrapInput{Environment: "STAGING", UserID: 701, Username: "SyntheticAdmin", ReleaseBuild: "gambler-test", ExpectedEmpty: true}); err != nil {
		t.Fatal(err)
	}
	if err := owner.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `CREATE TABLE public.users(id bigint PRIMARY KEY,quota bigint NOT NULL DEFAULT 0,status int NOT NULL DEFAULT 1,deleted_at timestamptz);`+platform.NativeQuotaMigration+`; UPDATE momiao_quota.settings SET enabled=true; INSERT INTO public.users(id,quota) VALUES(701,17),(702,19),(703,0);`, pgx.QueryExecModeSimpleProtocol)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(os.Getenv("MOMIAO_GAMES_TEST_CONNECTION_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	var connection struct{ OwnerURL, RuntimeURL, RuntimeRole string }
	if json.Unmarshal(raw, &connection) != nil {
		t.Fatal("local connection")
	}
	if err = owner.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "GRANT SELECT ON identity.users TO "+pgx.Identifier{connection.RuntimeRole}.Sanitize(), pgx.QueryExecModeSimpleProtocol)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	// Exercise the production startup boundary, not only domain operations:
	// Poker receives the cap tables, never direct wallet authority.
	authPool, err := pgxpool.New(ctx, connection.RuntimeURL)
	if err != nil {
		t.Fatal(err)
	}
	defer authPool.Close()
	pokerRole := connection.RuntimeRole + "_poker"
	pokerPassword := rand.Text()
	if err = owner.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "CREATE ROLE "+pgx.Identifier{pokerRole}.Sanitize()+" LOGIN PASSWORD '"+pokerPassword+"' NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS; GRANT USAGE ON SCHEMA economy TO "+pgx.Identifier{pokerRole}.Sanitize()+"; GRANT SELECT ON economy.policy_versions,economy.policy_runtime TO "+pgx.Identifier{pokerRole}.Sanitize()+"; GRANT SELECT,INSERT ON economy.cap_settlements TO "+pgx.Identifier{pokerRole}.Sanitize()+"; GRANT EXECUTE ON FUNCTION economy.poker_buy_in_apply(uuid,bigint,uuid),economy.poker_top_up_apply(uuid,bigint,uuid),economy.poker_cash_out_apply(uuid,bigint,uuid) TO "+pgx.Identifier{pokerRole}.Sanitize()+";", pgx.QueryExecModeSimpleProtocol)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = owner.WithTx(context.Background(), func(tx pgx.Tx) error {
			_, e := tx.Exec(context.Background(), "DROP OWNED BY "+pgx.Identifier{pokerRole}.Sanitize()+"; DROP ROLE "+pgx.Identifier{pokerRole}.Sanitize(), pgx.QueryExecModeSimpleProtocol)
			return e
		})
	})
	pokerConfig, err := pgxpool.ParseConfig(connection.RuntimeURL)
	if err != nil {
		t.Fatal(err)
	}
	pokerConfig.ConnConfig.User, pokerConfig.ConnConfig.Password = pokerRole, pokerPassword
	pokerPool, err := pgxpool.NewWithConfig(ctx, pokerConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer pokerPool.Close()
	if err = validatePokerPools(ctx, authPool, pokerPool); err != nil {
		t.Fatal("production cap grants rejected by startup", err)
	}
	for _, grant := range []string{"SELECT ON economy.wallet_balances", "UPDATE ON economy.cap_settlements", "INSERT ON economy.policy_runtime", "REFERENCES ON economy.policy_versions"} {
		if err = owner.WithTx(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, "GRANT "+grant+" TO "+pgx.Identifier{pokerRole}.Sanitize())
			return e
		}); err != nil {
			t.Fatal(err)
		}
		if validatePokerPools(ctx, authPool, pokerPool) == nil {
			t.Fatal("excess economy privilege accepted", grant)
		}
		if err = owner.WithTx(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, "REVOKE "+grant+" FROM "+pgx.Identifier{pokerRole}.Sanitize())
			return e
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err = validatePokerPools(ctx, authPool, pokerPool); err != nil {
		t.Fatal("narrow cap grants not restored", err)
	}
	native, err := platform.OpenNativeQuota(ctx, connection.OwnerURL)
	if err != nil {
		t.Fatal(err)
	}
	defer native.Close()
	for _, m := range []platform.Mutation{
		{UserID: 701, Asset: platform.ReserveAPICredit, DeltaUnits: 500000000000000, BizType: "CAMPAIGN_TEST", BizID: "threshold", EntryType: "GRANT", IdempotencyKey: "campaign-threshold"},
		{UserID: 702, Asset: platform.AvailableChips, DeltaUnits: 500000000000001, BizType: "CAMPAIGN_TEST", BizID: "eligible", EntryType: "GRANT", IdempotencyKey: "campaign-eligible"},
	} {
		if _, err = owner.Apply(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	// Apply the real, column-limited Ops/campaign GRANT statements to the existing
	// separate login role; owner remains only the fixture seeder/Native observer.
	for _, file := range []string{"runtime-grants-0027-rankings.psql", "runtime-grants-0031-games-poker-rankings-ops.psql", "runtime-grants-0028-ops-authorization.psql", "runtime-grants-0029-ops-runtime.psql", "runtime-grants-0044-gambler-campaign.psql"} {
		body, e := os.ReadFile(filepath.Join("..", "..", "deploy", "sql", file))
		if e != nil {
			t.Fatal(e)
		}
		for _, grant := range regexp.MustCompile(`(?ms)^GRANT [^;]+;`).FindAllString(string(body), -1) {
			if strings.Contains(grant, `:"poker_runtime_role"`) {
				continue
			}
			grant = strings.ReplaceAll(strings.ReplaceAll(grant, `:"platform_runtime_role"`, pgx.Identifier{connection.RuntimeRole}.Sanitize()), `:"runtime_role"`, pgx.Identifier{connection.RuntimeRole}.Sanitize())
			if e = owner.WithTx(ctx, func(tx pgx.Tx) error { _, e := tx.Exec(ctx, grant); return e }); e != nil {
				t.Fatal(e)
			}
		}
	}
	if err = runtime.WithTx(ctx, func(tx pgx.Tx) error {
		var broad bool
		if e := tx.QueryRow(ctx, `SELECT has_table_privilege(current_user,(SELECT c.oid FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='poker' AND c.relname='hands'),'SELECT') OR has_table_privilege(current_user,'public.users','UPDATE')`).Scan(&broad); e != nil {
			return e
		}
		if broad {
			return fmt.Errorf("runtime received broad privilege")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assets := platform.NewUnifiedAssetReader(runtime)
	campaign, err := platform.NewGamblerCampaignService(runtime, assets)
	if err != nil {
		t.Fatal(err)
	}
	bindings := campaign.OpsBindings()
	ops, err := platform.NewOpsService(runtime, "STAGING", bindings...)
	if err != nil {
		t.Fatal(err)
	}
	boot, err := ops.Bootstrap(ctx, 701)
	if err != nil {
		t.Fatal(err)
	}
	seq := 100
	idFor := func() string { seq++; return fmt.Sprintf("10000000-0000-4000-8000-%012d", seq) }
	fresh := func(p platform.OpsPrepared) platform.OpsExecuteRequest {
		challenge := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("g", 32)))
		if err := ops.BindFreshChallenge(ctx, 701, boot.Principal.Epoch, p.Operation.OperationID, challenge); err != nil {
			t.Fatal(err)
		}
		return platform.OpsExecuteRequest{AuthzEpoch: boot.Principal.Epoch, ImpactHash: p.Operation.ImpactHash, Confirmed: true, TypedConfirmation: p.ConfirmationPhrase, Fresh: &platform.OpsFreshEvidence{OperationID: p.Operation.OperationID, ChallengeID: challenge, ContextHash: strings.Repeat("0", 64), Method: "password", VerifiedAt: time.Now().UTC()}}
	}
	perform := func(action, targetType, target, version, schema, input string) platform.OpsOperationView {
		p, err := ops.Prepare(ctx, 701, platform.OpsPrepareRequest{OperationID: idFor(), OperationType: action, AuthzEpoch: boot.Principal.Epoch, InputSchemaVersion: schema, Target: platform.OpsTarget{Type: targetType, ID: target, ExpectedVersion: version}, Input: json.RawMessage(input), Reason: "Isolated campaign lifecycle verification"})
		if err != nil {
			t.Fatalf("prepare %s: %v", action, err)
		}
		out, err := ops.Execute(ctx, 701, p.Operation.OperationID, fresh(p))
		if err != nil || out.Operation.State != "SUCCEEDED" {
			t.Fatalf("execute %s: %s %v", action, out.Operation.State, err)
		}
		return out
	}
	setMaintenance := func(enabled bool) string {
		settings := platform.MaintenanceSettings{Enabled: enabled}
		if err := platform.ValidateMaintenance(&settings); err != nil {
			t.Fatal(err)
		}
		saved, err := owner.SaveMaintenance(ctx, 701, "STAGING", settings)
		if err != nil {
			t.Fatal(err)
		}
		return saved.ID
	}
	maint := setMaintenance(true)
	cid := platform.GamblerCampaignID
	input := fmt.Sprintf(`{"maintenance_id":%q,"native_pause_evidence":"isolated-native-admission-drained","backup_reference":"isolated-local-before-reset"}`, maint)
	if err = runtime.EnsureAccount(ctx, 704); !errors.Is(err, platform.ErrMaintenanceActive) {
		t.Fatalf("wallet/bootstrap admitted new account during freeze: %v", err)
	}
	if err = runtime.EnsureAccount(ctx, 701); err != nil {
		t.Fatal("existing account recovery blocked", err)
	}
	if _, err = owner.EnsureProvisionalProfile(ctx, 704); !errors.Is(err, platform.ErrMaintenanceActive) {
		t.Fatalf("new account admitted during freeze: %v", err)
	}
	if err = owner.IngestRegistrationPage(ctx, 0, platform.RegistrationPage{NextCursor: 1, Receipts: []platform.RegistrationReceipt{{Ordinal: 1, OperationID: idFor(), NativeUserID: 704, DiscordSubject: "123456789012345678", Source: "NEW_DISCORD_REGISTRATION", PolicyVersion: "v1", CreatedAt: time.Now().UTC()}}}); !errors.Is(err, platform.ErrMaintenanceActive) {
		t.Fatalf("new grant admitted during freeze: %v", err)
	}
	if _, err = owner.PlanActiveQuotaRefill(ctx, native, platform.ActiveQuotaRefillRequest{RequestID: "frozen-refill", UserID: 701, RequiredRawQuota: 100}, platform.ActiveQuotaRefillPolicy{Enabled: true, LowWatermark: 50, TargetWatermark: 200, MaxActiveBuffer: 400}); !errors.Is(err, platform.ErrMaintenanceActive) {
		t.Fatalf("new refill admitted during freeze: %v", err)
	}
	perform("GAMBLER_SNAPSHOT_PREPARE", "GAMBLER_CAMPAIGN", cid, "1", "gambler.v1", input)
	for i := 0; i < 20; i++ {
		more, err := campaign.RunBatch(ctx, cid, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			break
		}
	}
	v, err := campaign.Read(ctx, 701, cid)
	if err != nil || v.Phase != "SNAPSHOT_READY" || v.EligibleCount != "1" || v.TargetCount != "3" {
		t.Fatal("snapshot boundary", v, err)
	}
	perform("GAMBLER_MEDALS_GRANT", "GAMBLER_CAMPAIGN", cid, v.Version, "gambler.v1", `{}`)
	v, err = campaign.Read(ctx, 701, cid)
	if err != nil {
		t.Fatal(err)
	}
	perform("GAMBLER_MEDALS_GRANT", "GAMBLER_CAMPAIGN", cid, v.Version, "gambler.v1", `{}`)
	var badges int
	if err = owner.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM identity.account_badges WHERE newapi_user_id=702`).Scan(&badges)
	}); err != nil || badges != 1 {
		t.Fatal("permanent unique badge", badges, err)
	}
	v, _ = campaign.Read(ctx, 701, cid)
	perform("GAMBLER_RESET_PREPARE", "GAMBLER_CAMPAIGN", cid, v.Version, "gambler.v1", input)
	for i := 0; i < 20; i++ {
		more, err := campaign.RunBatch(ctx, cid, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			break
		}
	}
	v, err = campaign.Read(ctx, 701, cid)
	if err != nil || v.Phase != "RESET_READY" {
		t.Fatal("reset preview", v, err)
	}
	for _, stamp := range []*time.Time{v.CutoffAt, v.ResetCutoffAt, v.PolicyActivatedAt, v.RankingsPublishedAt} {
		if stamp != nil && !strings.HasSuffix(stamp.Format(time.RFC3339Nano), "Z") {
			t.Fatalf("campaign timestamp must use UTC for Ops UI: %s", stamp.Format(time.RFC3339Nano))
		}
	}
	// Real pending-transfer / Poker funding blockers in
	// rolled-back local transactions. The same typed handler refuses execution.
	for _, mode := range []string{"quota", "poker"} {
		sentinel := errors.New("local blocker rollback")
		e := owner.WithTx(ctx, func(tx pgx.Tx) error {
			switch mode {
			case "quota":
				_, err = tx.Exec(ctx, `INSERT INTO economy.quota_transfers(transfer_id,newapi_user_id,request_key_hash,amount_units,status) VALUES($1,701,decode(repeat('cf',32),'hex'),2,'PENDING')`, idFor())
			case "poker":
				tid, sid := idFor(), idFor()
				_, err = tx.Exec(ctx, `INSERT INTO poker.tables(table_id,owner_newapi_user_id,name,max_seats,blind_preset_version,ruleset_version) SELECT $1,701,'Synthetic drain',6,b.version,r.version FROM poker.blind_preset_versions b CROSS JOIN poker.ruleset_versions r LIMIT 1`, tid)
				if err == nil {
					_, err = tx.Exec(ctx, `INSERT INTO poker.seats(table_id,seat_no) VALUES($1,1)`, tid)
				}
				if err == nil {
					_, err = tx.Exec(ctx, `INSERT INTO poker.sessions(session_id,newapi_user_id,table_id,seat_no,display_name_snapshot,initial_buyin_units,current_stack_units) VALUES($1,701,$2,1,'Synthetic',500000,500000)`, sid, tid)
				}
			}
			if err != nil {
				return err
			}
			for _, binding := range campaign.OpsBindings() {
				if binding.Descriptor.OperationType != "GAMBLER_RESET_START" {
					continue
				}
				m, e := binding.Handler.(platform.OpsSameDatabaseHandler).Prepare(ctx, tx, platform.OpsPrincipal{}, platform.OpsPrepareRequest{Target: platform.OpsTarget{ID: cid, ExpectedVersion: v.Version}, Input: json.RawMessage(`{}`)}, true)
				if e != nil {
					return e
				}
				if len(m.Impact.BlockingFacts) == 0 {
					return fmt.Errorf("%s did not block reset", mode)
				}
				if _, e = binding.Handler.(platform.OpsSameDatabaseHandler).Execute(ctx, tx, platform.OpsPrincipal{}, platform.OpsOperation{}, m); !errors.Is(e, platform.ErrGamblerBlocked) {
					return fmt.Errorf("%s reset executed: %v", mode, e)
				}
			}
			return sentinel
		})
		if !errors.Is(e, sentinel) {
			t.Fatal(mode, e)
		}
	}
	// Stale preview, wrong epoch and ordinary user are all rejected before effects.
	request := platform.OpsPrepareRequest{OperationID: idFor(), OperationType: "GAMBLER_RESET_START", AuthzEpoch: boot.Principal.Epoch, InputSchemaVersion: "gambler.v1", Target: platform.OpsTarget{Type: "GAMBLER_CAMPAIGN", ID: cid, ExpectedVersion: "1"}, Input: json.RawMessage(`{}`), Reason: "Reject stale reset"}
	if _, err = ops.Prepare(ctx, 701, request); err == nil {
		t.Fatal("stale reset prepared")
	}
	request.OperationID = idFor()
	request.Target.ExpectedVersion = v.Version
	request.AuthzEpoch++
	if _, err = ops.Prepare(ctx, 701, request); err == nil {
		t.Fatal("stale authority accepted")
	}
	request.OperationID = idFor()
	request.AuthzEpoch = boot.Principal.Epoch
	if _, err = ops.Prepare(ctx, 702, request); err == nil {
		t.Fatal("ordinary user reset accepted")
	}
	preparedSummary, e := ops.Prepare(ctx, 701, platform.OpsPrepareRequest{OperationID: idFor(), OperationType: "GAMBLER_RESET_START", AuthzEpoch: boot.Principal.Epoch, InputSchemaVersion: "gambler.v1", Target: platform.OpsTarget{Type: "GAMBLER_CAMPAIGN", ID: cid, ExpectedVersion: v.Version}, Input: json.RawMessage(`{}`), Reason: "Verify exact frozen adjustment totals"})
	if e != nil {
		t.Fatal(e)
	}
	summary := string(preparedSummary.Operation.Impact.Delta)
	for _, value := range []string{`"affected_users":"3"`, `"reserve_increase_units":"10000000000"`, `"reserve_decrease_units":"499995000000000"`, `"chips_decrease_units":"500000000000001"`} {
		if !strings.Contains(summary, value) {
			t.Fatalf("missing exact reset impact %s: %s", value, summary)
		}
	}
	perform("GAMBLER_RESET_START", "GAMBLER_CAMPAIGN", cid, v.Version, "gambler.v1", `{}`)

	if more, err := campaign.RunBatch(ctx, cid, 1); err != nil || !more {
		t.Fatal("first durable batch", more, err)
	}
	first, _ := owner.ReadWallet(ctx, 701, platform.ReserveAPICredit)
	// Rotate accepted authority and close the old maintenance window mid-reset.
	if err = owner.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE ops.admin_principals SET authz_epoch=authz_epoch+1,version=version+1 WHERE newapi_user_id=701`)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if more, e := campaign.RunBatch(ctx, cid, 1); e != nil || more {
		t.Fatal("stale batch authority advanced", more, e)
	}
	boot, err = ops.Bootstrap(ctx, 701)
	if err != nil {
		t.Fatal(err)
	}
	setMaintenance(false)
	maint = setMaintenance(true)
	input = fmt.Sprintf(`{"maintenance_id":%q,"native_pause_evidence":"isolated-native-admission-drained","backup_reference":"isolated-local-before-reset"}`, maint)
	v, _ = campaign.Read(ctx, 701, cid)
	perform("GAMBLER_REAUTHORIZE", "GAMBLER_CAMPAIGN", cid, v.Version, "gambler.v1", input)

	// A new service instance continues the same accepted authority/checkpoint.
	campaign, err = platform.NewGamblerCampaignService(runtime, assets)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		more, err := campaign.RunBatch(ctx, cid, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			break
		}
	}
	v, err = campaign.Read(ctx, 701, cid)
	if err != nil || v.Phase != "COMPLETED" || v.CompletedCount != "3" || !v.NativeUnchanged {
		t.Fatal("resumed reset", v, err)
	}
	after, _ := owner.ReadWallet(ctx, 701, platform.ReserveAPICredit)
	if first.LedgerSeq != after.LedgerSeq {
		t.Fatal("completed user reset twice")
	}
	for _, u := range []int64{701, 702, 703} {
		reserve, e := owner.ReadWallet(ctx, u, platform.ReserveAPICredit)
		chips, e2 := owner.ReadWallet(ctx, u, platform.AvailableChips)
		n, e3 := native.ReadNativeQuota(ctx, u)
		wantNative := map[int64]int64{701: 17, 702: 19, 703: 0}[u]
		if e != nil || e2 != nil || e3 != nil || reserve.BalanceUnits != 5000000000 || chips.BalanceUnits != 0 || n.RawQuota != wantNative {
			t.Fatal("reset exact delta/native", u, reserve, chips, n, e, e2, e3)
		}
	}
	request.OperationID = idFor()
	request.Target.ExpectedVersion = v.Version
	if _, err = ops.Prepare(ctx, 701, request); err == nil {
		t.Fatal("completed campaign reset twice")
	}
	perform("ECONOMY_POLICY_ACTIVATE", "GAMBLER_CAMPAIGN", cid, v.Version, "gambler.v1", `{}`)
	// The publication gate remains in force until the operator ends maintenance.
	ranker := rankings.NewService(runtime, rankings.Options{Assets: assets, ActivationTime: time.Now().UTC().Add(-time.Hour)})
	builds := 0
	build := func(c context.Context) (string, error) { builds++; return ranker.Build(c, "ASSETS", "CURRENT", "") }
	if changed, e := campaign.RefreshRankings(ctx, build); e != nil || changed || builds != 0 {
		t.Fatal("publication ignored maintenance", changed, e)
	}
	setMaintenance(false)
	if changed, e := campaign.RefreshRankings(ctx, build); e != nil || !changed || builds != 1 {
		t.Fatal("campaign ranking publication", changed, builds, e)
	}
	if changed, e := campaign.RefreshRankings(ctx, build); e != nil || changed || builds != 1 {
		t.Fatal("duplicate campaign publication", changed, builds, e)
	}
	if _, e := ranker.Build(ctx, "GAMES", "DAY", ""); e != nil {
		t.Fatal(e)
	}
	profit, e := ranker.Read(ctx, rankings.Query{Metric: "GAME_PROFIT", Period: "DAY", Page: 1}, 701)
	if e != nil {
		t.Fatal(e)
	}
	if len(profit.Items) != 0 {
		t.Fatal("reset counted as game profit", profit)
	}
	if err = owner.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM identity.account_badges WHERE newapi_user_id=702`).Scan(&badges)
	}); err != nil || badges != 1 {
		t.Fatal("reset removed badge")
	}
	t.Log("strict qualification, separate grant/reset, durable batch resume, unchanged Native, stale authority and permanent badge verified")

}
