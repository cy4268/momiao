package platform

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/cy4268/momiao/internal/identity"
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
	page, err := listLocalOpsUsers(ctx, tx, OpsUserFilter{Page: 1, PageSize: 1, Search: strconv.FormatInt(user, 10), Role: 1, Status: 1, Group: original.Group})
	if err != nil || page.Total < 1 || len(page.Items) != 1 {
		t.Fatal("local paged user search", err)
	}
	for _, search := range []string{original.Username, original.DisplayName, original.DiscordID} {
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
