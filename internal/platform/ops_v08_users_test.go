package platform

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

// Replaces the legacy short-ID/permission-epoch reader test with the same user
// workflow on imported dev accounts. No account or wallet fixture is created.
func TestV08OpsUsersShortAccountSearch(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires existing isolated dev PG and Redis")
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
	raw, err := os.ReadFile(os.Getenv("MOMIAO_SESSION_REDIS_CONFIG_FILE"))
	if err != nil {
		t.Fatal("read dev Redis config")
	}
	var cfg struct {
		Addr, Username, Password string
		CAFile                   string `json:"ca_file"`
		ServerName               string `json:"server_name"`
	}
	if json.Unmarshal(raw, &cfg) != nil {
		t.Fatal("decode dev Redis config")
	}
	pem, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		t.Fatal("read dev Redis CA")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		t.Fatal("dev Redis CA")
	}
	client := redis.NewClient(&redis.Options{Addr: cfg.Addr, Username: cfg.Username, Password: cfg.Password, Protocol: 2, DisableIdentity: true, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: cfg.ServerName}})
	defer client.Close()
	sessions := identity.Sessions{Redis: client}
	ctx := t.Context()
	var root int64
	if err = s.pool.QueryRow(ctx, `SELECT min(id) FROM identity.users WHERE role=100 AND status=1 AND deleted_at IS NULL`).Scan(&root); err != nil {
		t.Fatal(err)
	}
	candidates, err := s.pool.Query(ctx, `SELECT u.id FROM identity.users u WHERE u.role=1 AND u.status=1 AND u.deleted_at IS NULL AND EXISTS(SELECT 1 FROM economy.wallet_balances w WHERE w.newapi_user_id=u.id AND w.asset_type='RESERVE_API_CREDIT') AND EXISTS(SELECT 1 FROM relay.api_keys k WHERE k.user_id=u.id) ORDER BY u.id`)
	if err != nil {
		t.Fatal(err)
	}
	defer candidates.Close()
	var user int64
	for candidates.Next() {
		var id int64
		if err = candidates.Scan(&id); err != nil {
			t.Fatal(err)
		}
		values, e := client.ZRangeByScore(ctx, "chaldea:session:local:user:"+strconv.FormatInt(id, 10), &redis.ZRangeBy{Min: "-inf", Max: "+inf"}).Result()
		if e != nil {
			t.Fatal("read dev session index", e)
		}
		if len(values) == 0 {
			user = id
			break
		}
	}
	candidates.Close()
	if user == 0 {
		t.Fatal("no idle existing dev account")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var audits int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM ops.audit_log`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	original, err := readLocalOpsUser(ctx, tx, user)
	if err != nil {
		t.Fatal(err)
	}
	master, err := readLocalOpsMaster(ctx, tx, user)
	if err != nil || original.MasterName != master.DisplayName {
		t.Fatal("Master name in user DTO", err)
	}
	page, err := listLocalOpsUsers(ctx, tx, OpsUserFilter{Page: 1, PageSize: 1, Search: strconv.FormatInt(user, 10), Role: 1, Status: 1, Group: original.Group})
	if err != nil || page.Total < 1 || len(page.Items) != 1 {
		t.Fatal("local paged user search", err)
	}
	for _, search := range []string{original.Username, original.DisplayName, original.DiscordID, original.MasterName} {
		if search == "" {
			continue
		}
		v, e := listLocalOpsUsers(ctx, tx, OpsUserFilter{Page: 1, PageSize: 100, Search: search})
		if e != nil || v.Total < 1 {
			t.Fatal("local name/Discord search", e)
		}
	}
	session, err := sessions.Create(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	defer sessions.Revoke(context.Background(), session)
	promoted := 10
	if err = changeLocalOpsUser(ctx, tx, sessions, root, user, "profile", OpsUserChange{Group: original.Group, Status: 1, Role: &promoted}); err != nil {
		t.Fatal("promote local user", err)
	}
	if _, err = sessions.Read(ctx, session.Token); !errors.Is(err, identity.ErrSessionExpired) {
		t.Fatal("role change did not revoke", err)
	}
	if err = changeLocalOpsUser(ctx, tx, sessions, user, root, "logout", OpsUserChange{}); !errors.Is(err, ErrOpsForbidden) {
		t.Fatal("admin managed root", err)
	}
	common := 1
	if err = changeLocalOpsUser(ctx, tx, sessions, user, user, "profile", OpsUserChange{Group: original.Group, Status: 1, Role: &common}); !errors.Is(err, ErrOpsForbidden) {
		t.Fatal("admin changed own role", err)
	}
	var group string
	if err = tx.QueryRow(ctx, `SELECT name FROM relay.groups WHERE name<>$1 ORDER BY name LIMIT 1`, original.Group).Scan(&group); err != nil {
		t.Fatal(err)
	}
	if err = changeLocalOpsUser(ctx, tx, sessions, root, user, "profile", OpsUserChange{Group: group, Status: 1, Role: &common}); err != nil {
		t.Fatal("change group and demote", err)
	}
	for _, status := range []int{2, 1} {
		if err = changeLocalOpsUser(ctx, tx, sessions, root, user, "profile", OpsUserChange{Group: group, Status: status}); err != nil {
			t.Fatal("ban/unban", err)
		}
	}
	// Existing account's factor rows live only inside this rolled-back workflow.
	if _, err = tx.Exec(ctx, `INSERT INTO identity.two_fas(user_id,secret,is_enabled) VALUES($1,'JBSWY3DPEHPK3PXP',true)`, user); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO identity.two_fa_backup_codes(user_id,code_hash) VALUES($1,'unused-test-hash')`, user); err != nil {
		t.Fatal(err)
	}
	second, err := sessions.Create(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	defer sessions.Revoke(context.Background(), second)
	if err = changeLocalOpsUser(ctx, tx, sessions, root, user, "reset-2fa", OpsUserChange{}); err != nil {
		t.Fatal("reset 2FA", err)
	}
	var factors int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM identity.two_fas WHERE user_id=$1)+(SELECT count(*) FROM identity.two_fa_backup_codes WHERE user_id=$1)`, user).Scan(&factors); err != nil || factors != 0 {
		t.Fatal("factor rows remain", err)
	}
	if _, err = sessions.Read(ctx, second.Token); !errors.Is(err, identity.ErrSessionExpired) {
		t.Fatal("2FA reset session remains", err)
	}
	third, err := sessions.Create(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	defer sessions.Revoke(context.Background(), third)
	if err = changeLocalOpsUser(ctx, tx, sessions, root, user, "logout", OpsUserChange{}); err != nil {
		t.Fatal("force logout", err)
	}
	if _, err = sessions.Read(ctx, third.Token); !errors.Is(err, identity.ErrSessionExpired) {
		t.Fatal("force logout session remains", err)
	}
	plus, _ := uuidV7()
	adjust := OpsUserChange{Amount: "0.000002", Note: "minimal dev adjustment", RequestID: plus}
	for i := 0; i < 2; i++ {
		if err = changeLocalOpsUser(ctx, tx, sessions, root, user, "reserve", adjust); err != nil {
			t.Fatal("Reserve/replay", err)
		}
	}
	changed, err := readLocalOpsUser(ctx, tx, user)
	if err != nil {
		t.Fatal(err)
	}
	var beforeUnits int64
	if err = tx.QueryRow(ctx, `SELECT balance_before_units FROM economy.wallet_ledger WHERE biz_type='OPS_ADJUST' AND biz_id=$1`, plus).Scan(&beforeUnits); err != nil || changed.Reserve != FormatAmount(beforeUnits+1) {
		t.Fatal("exact one-unit adjustment", err)
	}
	entries, err := localOpsLedger(ctx, tx, user, 1, 20)
	if err != nil || len(entries.Items) == 0 || entries.Items[0].BizType != "OPS_ADJUST" || entries.Items[0].Note != adjust.Note || entries.Items[0].DeltaAmount != "0.000002" {
		t.Fatal("adjustment ledger/note", err)
	}
	adjust.Note = "changed"
	if err = changeLocalOpsUser(ctx, tx, sessions, root, user, "reserve", adjust); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal("changed replay", err)
	}
	minus, _ := uuidV7()
	if err = changeLocalOpsUser(ctx, tx, sessions, root, user, "reserve", OpsUserChange{Amount: "-" + FormatAmount(beforeUnits+2), Note: "overdraft rejected", RequestID: minus}); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatal("Reserve overdraft", err)
	}
	if err = changeLocalOpsUser(ctx, tx, sessions, root, user, "reserve", OpsUserChange{Amount: "-0.000002", Note: "minimal reversal", RequestID: minus}); err != nil {
		t.Fatal("Reserve debit", err)
	}
	var key, logs int64
	if err = tx.QueryRow(ctx, `SELECT id FROM relay.api_keys WHERE user_id=$1 ORDER BY id LIMIT 1`, user).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM relay.logs WHERE key_id=$1`, key).Scan(&logs); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"disable-key", "delete-key"} {
		if err = changeLocalOpsUser(ctx, tx, sessions, root, user, action, OpsUserChange{KeyID: key}); err != nil {
			t.Fatal(action, err)
		}
	}
	var retained int64
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM relay.logs WHERE key_id=$1`, key).Scan(&retained); err != nil || retained != logs {
		t.Fatal("key history removed", err)
	}
	rootUser, err := readLocalOpsUser(ctx, tx, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE identity.users SET role=10 WHERE role=100 AND id<>$1`, root); err != nil {
		t.Fatal(err)
	}
	if err = changeLocalOpsUser(ctx, tx, sessions, root, root, "profile", OpsUserChange{Group: rootUser.Group, Status: 2}); !errors.Is(err, ErrOpsLastSuperAdmin) {
		t.Fatal("last root ban", err)
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM ops.audit_log`).Scan(&count); err != nil || count != audits+10 {
		t.Fatal("one audit per committed mutation", count-audits, err)
	}
	for _, dimension := range []string{"model", "channel", "user"} {
		page, err := localOpsLogs(ctx, tx, OpsLogFilter{Page: 1, PageSize: 2, User: user, Dimension: dimension})
		if err != nil {
			t.Fatal("filtered logs/top", dimension, err)
		}
		var requests, attempts int64
		var cost string
		if err = tx.QueryRow(ctx, `SELECT count(DISTINCT coalesce(request_id,'native:'||id::text)),count(*),trim_scale(coalesce(sum(cost_units),0)::numeric/500000)::text FROM relay.logs WHERE user_id=$1 AND kind IN(2,5)`, user).Scan(&requests, &attempts, &cost); err != nil || page.Summary.Requests != requests || page.Total != attempts || page.Summary.Cost != cost || len(page.Top) > 20 {
			t.Fatal("filtered exact summary", err)
		}
	}
	var sample OpsLogFilter
	var occurred int64
	if err = tx.QueryRow(ctx, `SELECT user_id,key_id,model,channel_id,group_name,extract(epoch FROM created_at)::bigint FROM relay.logs WHERE kind=2 ORDER BY created_at DESC,id DESC LIMIT 1`).Scan(&sample.User, &sample.Key, &sample.Model, &sample.Channel, &sample.Group, &occurred); err != nil {
		t.Fatal(err)
	}
	sample.Page, sample.PageSize, sample.Start, sample.End, sample.Dimension = 1, 20, occurred-1, occurred+1, "model"
	selected, err := localOpsLogs(ctx, tx, sample)
	if err != nil || selected.Total < 1 {
		t.Fatal("all log filters", err)
	}
	log := selected.Items[0]
	detail, err := localOpsLogDetail(ctx, tx, log.ID)
	if err != nil || len(detail.Items) == 0 {
		t.Fatal("log detail attempts", err)
	}
	var expectedName string
	var attemptCount int
	if err = tx.QueryRow(ctx, `SELECT coalesce(nullif(p.display_name,''),u.username,'') FROM identity.users u LEFT JOIN identity.master_profiles p ON p.newapi_user_id=u.id WHERE u.id=$1`, log.UserID).Scan(&expectedName); err != nil || log.DisplayName != expectedName {
		t.Fatal("Master name in logs", err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM relay.logs WHERE kind IN(2,5) AND user_id=$1 AND (id=$2 OR ($3<>'' AND request_id=$3))`, log.UserID, log.ID, log.RequestID).Scan(&attemptCount); err != nil || len(detail.Items) != attemptCount {
		t.Fatal("complete request attempts", err)
	}
	for _, attempt := range detail.Items {
		if attempt.UserID != log.UserID || attempt.RequestID != log.RequestID || attempt.CostUnits == "" || attempt.PromptTokens == "" {
			t.Fatal("log detail fields or request boundary")
		}
	}
	if _, err = localOpsLogDetail(ctx, tx, 9223372036854775807); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("missing log detail", err)
	}
	sample.ErrorsOnly = true
	failed, err := localOpsLogs(ctx, tx, sample)
	if err != nil {
		t.Fatal("error-only logs", err)
	}
	for _, v := range failed.Items {
		if v.Kind != 5 && v.Status < 400 {
			t.Fatal("success in error-only logs")
		}
	}
	t.Logf("existing log filter: attempts=%d errors_only=%d; exact Credit costs", selected.Total, failed.Total)
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), "password_hash") || strings.Contains(string(encoded), "secret") {
		t.Fatal("secret in Ops user DTO")
	}
	t.Log("existing dev user: role/group/ban/2FA/logout; +1/replay/-1 Reserve units; masked keys/history; 10 audits; filtered SQL Top20; all PG writes rolled back")
}

// Reuse imported dev identities and Master history; every write stays in one
// transaction that rolls back, including the temporary local administrator role.
func TestLocalOpsMasterModeration(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires existing isolated dev database")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read dev DSN")
	}
	s, err := Open(t.Context(), strings.TrimSpace(string(raw)))
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
	var actor, subject int64
	if err = tx.QueryRow(ctx, `SELECT max(id) FROM identity.users WHERE role=1 AND status=1 AND deleted_at IS NULL`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT min(u.id) FROM identity.users u JOIN identity.master_profiles p ON p.newapi_user_id=u.id WHERE u.role=1 AND u.status=1 AND u.deleted_at IS NULL AND p.profile_version>0 AND u.id<>$1`, actor).Scan(&subject); err != nil {
		t.Fatal(err)
	}
	before, err := readLocalOpsMaster(ctx, tx, subject)
	if err != nil {
		t.Fatal(err)
	}
	var audits, operations int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM ops.audit_log),(SELECT count(*) FROM ops.admin_operations)`).Scan(&audits, &operations); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE identity.users SET role=10 WHERE id=$1`, actor); err != nil {
		t.Fatal(err)
	}
	if err = changeLocalOpsUser(ctx, tx, identity.Sessions{}, subject, actor, "master-rename-required", OpsUserChange{}); !errors.Is(err, ErrOpsForbidden) {
		t.Fatal("regular role", err)
	}
	if err = changeLocalOpsUser(ctx, tx, identity.Sessions{}, actor, actor, "master-rename-required", OpsUserChange{}); !errors.Is(err, ErrOpsForbidden) {
		t.Fatal("peer role", err)
	}
	required := true
	for i := 0; i < 2; i++ {
		if err = changeLocalOpsUser(ctx, tx, identity.Sessions{}, actor, subject, "master-rename-required", OpsUserChange{Required: &required}); err != nil {
			t.Fatal("require rename and same-value save", err)
		}
	}
	marked, err := readLocalOpsMaster(ctx, tx, subject)
	if err != nil || !marked.RenameRequired || marked.ProfileVersion != before.ProfileVersion+2 {
		t.Fatal("rename marker", err, marked)
	}
	if err = changeLocalOpsUser(ctx, tx, identity.Sessions{}, actor, subject, "master-rename-required", OpsUserChange{}); !errors.Is(err, ErrOpsInvalid) {
		t.Fatal("missing required", err)
	}
	if err = changeLocalOpsUser(ctx, tx, identity.Sessions{}, actor, subject, "master-force-rename", OpsUserChange{DisplayName: "Admin"}); !errors.Is(err, ErrNicknameReserved) {
		t.Fatal("reserved name", err)
	}
	name := "Dev " + announcementID(t)[:8]
	if err = changeLocalOpsUser(ctx, tx, identity.Sessions{}, actor, subject, "master-force-rename", OpsUserChange{DisplayName: " Ｄｅｖ " + strings.TrimPrefix(name, "Dev ") + " "}); err != nil {
		t.Fatal("normalized forced rename", err)
	}
	if err = changeLocalOpsUser(ctx, tx, identity.Sessions{}, actor, subject, "master-force-rename", OpsUserChange{DisplayName: name}); err != nil {
		t.Fatal("same name save", err)
	}
	required = false
	if err = changeLocalOpsUser(ctx, tx, identity.Sessions{}, actor, subject, "master-rename-required", OpsUserChange{Required: &required}); err != nil {
		t.Fatal("clear rename required", err)
	}
	after, err := readLocalOpsMaster(ctx, tx, subject)
	if err != nil || after.DisplayName != name || after.RenameRequired || after.ProfileVersion != before.ProfileVersion+5 || !reflect.DeepEqual(after.NicknameChangedAt, before.NicknameChangedAt) {
		t.Fatal("Master state or user cooldown changed", err, after)
	}
	var duplicate string
	if err = tx.QueryRow(ctx, `SELECT display_name FROM identity.master_profiles WHERE newapi_user_id<>$1 AND profile_version>0 ORDER BY newapi_user_id LIMIT 1`, subject).Scan(&duplicate); err != nil {
		t.Fatal(err)
	}
	sub, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = changeLocalOpsUser(ctx, sub, identity.Sessions{}, actor, subject, "master-force-rename", OpsUserChange{DisplayName: duplicate})
	if !errors.Is(err, ErrNicknameTaken) {
		t.Fatal("unique nickname", err)
	}
	if err = sub.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var count, oldOperations, masterAudits int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM ops.audit_log),(SELECT count(*) FROM ops.admin_operations),(SELECT count(*) FROM ops.audit_log WHERE actor_user_id=$1 AND target_type='MASTER_PROFILE' AND target=$2 AND before_state->>'display_name' IS NOT NULL AND after_state->>'display_name' IS NOT NULL)`, actor, strconv.FormatInt(subject, 10)).Scan(&count, &oldOperations, &masterAudits); err != nil || count != audits+5 || oldOperations != operations || masterAudits != 5 {
		t.Fatal("atomic audit without legacy operations", err, count-audits, masterAudits)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := readLocalOpsMaster(ctx, s.pool, subject)
	if err != nil || !reflect.DeepEqual(restored, before) {
		t.Fatal("Master rollback", err)
	}
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM ops.audit_log`).Scan(&count); err != nil || count != audits {
		t.Fatal("audit rollback", err)
	}
	t.Log("local role10: require/clear and normalized force rename; same-value saves; regular/peer/reserved/duplicate denied; 5 atomic audits, 0 legacy operations; user cooldown unchanged; all rows rolled back")
}

func TestLocalAssetsAndChips(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires existing isolated dev database")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read dev DSN")
	}
	s, err := Open(t.Context(), strings.TrimSpace(string(raw)))
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
	var admin, user int64
	if err = tx.QueryRow(ctx, `SELECT min(id) FROM identity.users WHERE role>=10 AND status=1 AND deleted_at IS NULL`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT u.id FROM identity.users u JOIN economy.wallet_balances w ON w.newapi_user_id=u.id AND w.asset_type='RESERVE_API_CREDIT' AND w.balance_units>2 CROSS JOIN LATERAL economy.unified_poker_assets_read(u.id) p CROSS JOIN LATERAL economy.cap_local_assets_read(u.id) g WHERE u.role=1 AND u.status=1 AND u.deleted_at IS NULL AND p.healthy AND g.healthy AND p.stack_units+p.pot_units+g.single_player_units+g.roulette_units=0 AND NOT EXISTS(SELECT 1 FROM rewards.relief_claims r WHERE r.newapi_user_id=u.id AND r.cooldown_until>clock_timestamp()) ORDER BY u.id DESC LIMIT 1`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	before, err := ReadUnifiedAssetsInTx(ctx, tx, user)
	if err != nil {
		t.Fatal(err)
	}
	if before.TotalUnits != before.ReserveUnits+before.AvailableChipsUnits+before.PokerStackUnits+before.PokerPotUnits+before.SinglePlayerInFlightUnits+before.RouletteEscrowUnits || before.TotalAmount != FormatAmount(before.TotalUnits) {
		t.Fatal("local six-bucket total", before)
	}
	wire, _ := json.Marshal(before)
	if strings.Contains(string(wire), "active_quota") || strings.Contains(string(wire), "native_observed") || strings.Contains(string(wire), "quota_transfer") {
		t.Fatal("native fields", string(wire))
	}
	var audits, ledger int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM ops.audit_log),(SELECT count(*) FROM economy.wallet_ledger)`).Scan(&audits, &ledger); err != nil {
		t.Fatal(err)
	}
	add := OpsUserChange{Amount: "0.000004", Note: "dev chip adjustment", RequestID: announcementID(t)}
	if err = changeLocalOpsUser(ctx, tx, identity.Sessions{}, user, admin, "chips", add); !errors.Is(err, ErrOpsForbidden) {
		t.Fatal("regular role", err)
	}
	for range 2 {
		if err = changeLocalOpsUser(ctx, tx, identity.Sessions{}, admin, user, "chips", add); err != nil {
			t.Fatal("chips and retry", err)
		}
	}
	debit := OpsUserChange{Amount: "-0.000002", Note: "dev chip debit", RequestID: announcementID(t)}
	if err = changeLocalOpsUser(ctx, tx, identity.Sessions{}, admin, user, "chips", debit); err != nil {
		t.Fatal(err)
	}
	changed, err := ReadUnifiedAssetsInTx(ctx, tx, user)
	if err != nil || changed.AvailableChipsUnits != before.AvailableChipsUnits+1 || changed.ReserveUnits != before.ReserveUnits || changed.TotalUnits != before.TotalUnits+1 {
		t.Fatal("chip-only delta", changed, err)
	}
	if err = changeLocalOpsUser(ctx, tx, identity.Sessions{}, admin, user, "chips", OpsUserChange{Amount: "-" + FormatAmount(changed.AvailableChipsUnits+1), Note: "reject overdraft", RequestID: announcementID(t)}); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatal("chip overdraft", err)
	}
	var count, good int
	if err = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE before_state->>'asset'='AVAILABLE_CHIPS' AND after_state->>'asset'='AVAILABLE_CHIPS') FROM ops.audit_log WHERE action='OPS_ADJUST' AND target=ANY($1)`, []string{add.RequestID, debit.RequestID}).Scan(&count, &good); err != nil || count != 2 || good != 2 {
		t.Fatal("chip audit", count, good, err)
	}
	exchangeKey := announcementID(t)
	exchanged, err := exchangeInTx(ctx, tx, user, exchangeKey, ReserveAPICredit, 1)
	if err != nil || exchanged.Status != "CONFIRMED" {
		t.Fatal("local exchange", err, exchanged.Status)
	}
	replay, err := exchangeInTx(ctx, tx, user, exchangeKey, ReserveAPICredit, 1)
	if err != nil || replay.ID != exchanged.ID {
		t.Fatal("exchange retry", err)
	}
	afterExchange, err := ReadUnifiedAssetsInTx(ctx, tx, user)
	if err != nil || afterExchange.TotalUnits != changed.TotalUnits || afterExchange.ReserveUnits != changed.ReserveUnits-1 || afterExchange.AvailableChipsUnits != changed.AvailableChipsUnits+1 {
		t.Fatal("exchange conservation", err)
	}
	if _, err = exchangeInTx(ctx, tx, user, announcementID(t), ReserveAPICredit, afterExchange.ReserveUnits+1); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatal("Reserve cannot borrow Native quota", err)
	}
	mutate := func(asset Asset, delta int64) {
		t.Helper()
		if delta == 0 {
			return
		}
		id := announcementID(t)
		if _, e := applyInTx(ctx, tx, Mutation{UserID: user, Asset: asset, DeltaUnits: delta, BizType: "OPS_ADJUST", BizID: id, EntryType: "OPS_ADJUST", IdempotencyKey: id}); e != nil {
			t.Fatal(e)
		}
	}
	mutate(ReserveAPICredit, AssetCapUnits-1-afterExchange.TotalUnits)
	policy, err := ActiveEconomicPolicyInTx(ctx, tx)
	if err != nil || policy.Version == "" {
		t.Fatal("active dev policy", err)
	}
	cap, err := PrepareCapSettlementInTx(ctx, tx, policy, user, 0, 5, 1)
	if err != nil || cap.CreditedPayoutUnits != 1 || cap.WithheldUnits != 4 {
		t.Fatal("local cap", cap, err)
	}
	id := announcementID(t)
	reward, err := s.applyRewardInTx(ctx, tx, Mutation{UserID: user, Asset: ReserveAPICredit, DeltaUnits: 5, BizType: "LOCAL_ASSET_CHECK", BizID: id, EntryType: "REWARD", IdempotencyKey: id})
	if err != nil || reward.DeltaUnits != 1 {
		t.Fatal("local capped reward", reward.DeltaUnits, err)
	}
	capped, err := ReadUnifiedAssetsInTx(ctx, tx, user)
	if err != nil || capped.TotalUnits != AssetCapUnits {
		t.Fatal("asset cap", err, capped.TotalUnits)
	}
	mutate(ReserveAPICredit, -capped.ReserveUnits)
	mutate(AvailableChips, -capped.AvailableChipsUnits)
	service, err := NewEconomyService(s, NewUnifiedAssetReader(s))
	if err != nil {
		t.Fatal(err)
	}
	reliefKey := announcementID(t)
	relief, err := service.claimReliefRewardInTx(ctx, tx, user, reliefKey)
	if err != nil {
		t.Fatal("local relief", err)
	}
	replay, err = service.claimReliefRewardInTx(ctx, tx, user, reliefKey)
	if err != nil || replay.ID != relief.ID {
		t.Fatal("relief retry", err)
	}
	if _, err = service.claimReliefRewardInTx(ctx, tx, user, announcementID(t)); !errors.Is(err, ErrReliefCooldown) {
		t.Fatal("relief cooldown", err)
	}
	var localClaim bool
	if err = tx.QueryRow(ctx, `SELECT total_assets_units=0 AND active_quota_units=0 AND quota_transfer_in_flight_units=0 AND native_observed_at IS NULL AND amount_units=$2 FROM rewards.relief_claims WHERE transaction_id=$1`, relief.ID, ReliefRewardAmount).Scan(&localClaim); err != nil || !localClaim {
		t.Fatal("local relief receipt", err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM ops.audit_log`).Scan(&count); err != nil || count != audits+2 {
		t.Fatal("one audit per chip change", count, err)
	}
	if _, err = tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := NewUnifiedAssetReader(s).ReadUnifiedAssets(ctx, user)
	if err != nil || restored.TotalUnits != before.TotalUnits || restored.ReserveUnits != before.ReserveUnits || restored.AvailableChipsUnits != before.AvailableChipsUnits {
		t.Fatal("wallet rollback", err)
	}
	if err = s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM ops.audit_log),(SELECT count(*) FROM economy.wallet_ledger)`).Scan(&count, &good); err != nil || count != audits || good != ledger {
		t.Fatal("audit/ledger rollback", err)
	}
	t.Log("local six-bucket assets; chips +2/replay/-1, two audits; local exchange conserved; overdrafts rejected; capped reward 1 of 5; relief 300/replay/cooldown with NULL Native observation; all writes rolled back")
}
