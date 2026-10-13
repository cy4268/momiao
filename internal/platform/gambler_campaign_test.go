package platform

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Reuse the imported, completed dev campaign. All admin/worker changes roll
// back; this test does not rerun the real snapshot, medal grant or wallet reset.
func TestGamblerCampaignLocalOps(t *testing.T) {
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	campaign := NewGamblerCampaignService(s, "DEVELOPMENT")
	before, err := readGambler(ctx, tx, GamblerCampaignID, false)
	if err != nil || before.Phase != "COMPLETED" || before.PolicyActivatedAt == nil {
		t.Fatal("requires completed imported campaign", err)
	}
	var actor int64
	if err = tx.QueryRow(ctx, `SELECT max(id) FROM identity.users WHERE role=1 AND status=1 AND deleted_at IS NULL`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	var audits, operations int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM ops.audit_log),(SELECT count(*) FROM ops.admin_operations)`).Scan(&audits, &operations); err != nil {
		t.Fatal(err)
	}
	if _, err = campaign.saveGambler(ctx, tx, actor, GamblerCommand{Action: "activate"}); !errors.Is(err, ErrCatalogForbidden) {
		t.Fatal("regular role", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE identity.users SET role=10 WHERE id=$1`, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = campaign.saveGambler(ctx, tx, actor, GamblerCommand{}); !errors.Is(err, ErrOpsInvalid) {
		t.Fatal("missing action", err)
	}
	if _, err = campaign.saveGambler(ctx, tx, actor, GamblerCommand{Action: "snapshot", RequestsDrained: true}); !errors.Is(err, ErrOpsConflict) {
		t.Fatal("completed snapshot restarted", err)
	}
	if _, err = campaign.saveGambler(ctx, tx, actor, GamblerCommand{Action: "reset", RequestsDrained: true}); !errors.Is(err, ErrOpsConflict) {
		t.Fatal("completed reset restarted", err)
	}
	active, err := campaign.saveGambler(ctx, tx, actor, GamblerCommand{Action: "activate"})
	if err != nil || !reflect.DeepEqual(active.PolicyActivatedAt, before.PolicyActivatedAt) || active.Version != before.Version {
		t.Fatal("same-value activation", err)
	}
	if _, err = campaign.saveGambler(ctx, tx, actor, GamblerCommand{Action: "resume", RequestsDrained: true}); !errors.Is(err, ErrGamblerBlocked) {
		t.Fatal("missing maintenance", err)
	}
	if _, err = saveMaintenance(ctx, tx, actor, "DEVELOPMENT", MaintenanceSettings{Enabled: true, Scopes: gamblerScopes}); err != nil {
		t.Fatal(err)
	}
	if _, err = campaign.saveGambler(ctx, tx, actor, GamblerCommand{Action: "resume"}); !errors.Is(err, ErrGamblerBlocked) {
		t.Fatal("accepted relay requests not confirmed drained", err)
	}
	if _, err = campaign.saveGambler(ctx, tx, actor, GamblerCommand{Action: "resume", RequestsDrained: true}); err != nil {
		t.Fatal("local role 10 resume", err)
	}
	resumed, err := readGambler(ctx, tx, GamblerCampaignID, false)
	if err != nil || resumed.actor != actor || !resumed.reverifyPending {
		t.Fatal("local accepted authority", err)
	}
	var legacyNull bool
	if err = tx.QueryRow(ctx, `SELECT accepted_epoch IS NULL AND accepted_operation_id IS NULL FROM economy.gambler_campaigns WHERE campaign_id=$1`, GamblerCampaignID).Scan(&legacyNull); err != nil || !legacyNull {
		t.Fatal("old operation dependency", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE identity.users SET role=1 WHERE id=$1`, actor); err != nil {
		t.Fatal(err)
	}
	if progressed, err := campaign.runBatchInTx(ctx, tx, GamblerCampaignID, 1); err != nil || progressed {
		t.Fatal("demoted worker actor", err, progressed)
	}
	blocked, err := readGambler(ctx, tx, GamblerCampaignID, false)
	if err != nil || !slices.Contains(blocked.BlockingFacts, "ACCEPTED_AUTHORITY_CHANGED") {
		t.Fatal("missing local worker block", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE identity.users SET role=10 WHERE id=$1`, actor); err != nil {
		t.Fatal(err)
	}
	// Imported balances have changed since the historical reset. Do not replace
	// that immutable checkpoint or allow a stale reset to pass after migration.
	if _, err = campaign.runBatchInTx(ctx, tx, GamblerCampaignID, 50); err != nil {
		t.Fatal("local worker", err)
	}
	blocked, err = readGambler(ctx, tx, GamblerCampaignID, false)
	if err != nil || !slices.Contains(blocked.BlockingFacts, "RESUME_CHECKPOINT_CHANGED_OR_UNAVAILABLE") {
		t.Fatal("changed local checkpoint accepted", err, blocked.BlockingFacts)
	}
	page, err := readGamblerPage(ctx, tx, GamblerQuery{Page: 1})
	if err != nil || page.PageSize != 20 || len(page.Accounts) != 20 || page.Total != 40 {
		t.Fatal("global campaign page", err)
	}
	id := page.Accounts[0].UserID
	filtered, err := readGamblerPage(ctx, tx, GamblerQuery{Page: 1, UserID: id})
	if err != nil || filtered.Total != 1 || filtered.Accounts[0].UserID != id {
		t.Fatal("user filter", err)
	}
	var historical UnifiedAssets
	if err = json.Unmarshal(filtered.Accounts[0].After, &historical); err != nil {
		t.Fatal(err)
	}
	changed := historical
	changed.TotalUnits++
	if !sameGamblerAssets(historical, changed) {
		t.Fatal("legacy Native total blocked identical local buckets")
	}
	changed.ReserveUnits++
	if sameGamblerAssets(historical, changed) {
		t.Fatal("local wallet change ignored")
	}
	var afterAudits, afterOperations int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM ops.audit_log),(SELECT count(*) FROM ops.admin_operations)`).Scan(&afterAudits, &afterOperations); err != nil {
		t.Fatal(err)
	}
	if afterAudits != audits+3 || afterOperations != operations {
		t.Fatal("direct saves must audit without old operations", afterAudits-audits, afterOperations-operations)
	}
	var changedAccounts int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM economy.gambler_campaign_accounts WHERE campaign_id=$1 AND reset_state<>'COMPLETED'`, GamblerCampaignID).Scan(&changedAccounts); err != nil || changedAccounts != 0 {
		t.Fatal("history changed", err)
	}
	if _, err = tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	t.Log("local role 10; direct audit 2 + maintenance 1; old operations 0; worker demotion/checkpoint blocks; immutable accounts " + strconv.FormatInt(page.Total, 10) + "; transaction rolled back")
}
