package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

// Explicitly opted-in manual acceptance server. Everything native is synthetic;
// the portal handlers, catalog reader, PostgreSQL projection and Ops writes are real.
func TestCatalogBrowserFixture(t *testing.T) {
	if os.Getenv("MOMIAO_CATALOG_BROWSER_FIXTURE") != "1" {
		t.Skip("explicit local catalog browser fixture only")
	}
	pgcfg, err := pgx.ParseConfig(os.Getenv("MOMIAO_CATALOG_TEST_DATABASE_URL"))
	if err != nil || pgcfg.Host != "127.0.0.1" || pgcfg.Port != 55432 || !slices.Contains([]string{"m3_catalog_platform_browser_portal_20260906_01", "m3_catalog_platform_browser_portal_20260906_02"}, pgcfg.Database) {
		t.Fatal("dedicated loopback catalog browser database required")
	}
	ctx := context.Background()
	store, err := platform.Open(ctx, pgcfg.ConnString())
	if err != nil {
		t.Fatal("local fixture database unavailable")
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.ConnectConfig(ctx, pgcfg)
	if err != nil {
		t.Fatal("local fixture seed connection unavailable")
	}
	defer conn.Close(ctx)
	// Completed synthetic facts in this fresh local database only. The browser
	// must initialize Master and explicitly acknowledge through the real Gate.
	_, err = conn.Exec(ctx, `INSERT INTO identity.account_refs(newapi_user_id) VALUES(910000001),(910000002);
 INSERT INTO identity.migration_notice_versions(version,title,body,completed_at,evidence_ref) VALUES(1,'本地合成验收已准备完成','这是一份本地合成验收通知：目录投影已准备，尚未创建任何 API 密钥，也不会连接真实模型。',now()-interval '1 minute','synthetic-portal-catalog-browser-only');
 INSERT INTO identity.migration_notice_requirements(newapi_user_id,version) VALUES(910000001,1),(910000002,1)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, `INSERT INTO ops.admin_principals(admin_principal_id,newapi_user_id,base_role) VALUES('01990000-1111-7777-aaaa-000000000701',910000001,'SUPER_ADMIN') ON CONFLICT(newapi_user_id) DO NOTHING`)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{"..", "demo/aurora", "demo/cedar", "demo/review", "demo/星海'$(literal)"}
	slices.Sort(ids)
	price := func(input, output string) platform.CatalogPrice {
		one := "1"
		kinds := []string{"input", "output", "cache_read", "cache_write"}
		amounts := []string{input, output, "0.1", "1.25"}
		conditions := []string{"uncached_plain_text_tokens", "plain_text_output_tokens", "only_if_native_reports_billable_cache_read_tokens", "only_if_native_reports_billable_generic_cache_write_tokens"}
		dimensions := []platform.CatalogDimension{}
		for i, kind := range kinds {
			value := amounts[i]
			dimensions = append(dimensions, platform.CatalogDimension{Kind: kind, Amount: &value, Unit: "API_Credit_per_1M_tokens", Source: "native_effective", Condition: conditions[i], Support: "not_asserted"})
		}
		return platform.CatalogPrice{Mode: "ratio", Configured: true, Status: "reference", GroupMultiplier: &one, Dimensions: dimensions, Unquoted: []string{"image", "audio", "tools", "request_adjustments"}}
	}
	models := []platform.NativeCatalogModel{}
	for _, id := range ids {
		p := price("2", "6")
		if strings.Contains(id, "星海") {
			p = price("0.000000000000000002", "0")
		}
		models = append(models, platform.NativeCatalogModel{ModelID: id, EnabledConfiguration: true, NativeCatalogVisible: true, EndpointStatus: "configured_subset_not_health", Endpoints: []platform.CatalogEndpoint{{Kind: "openai", Path: "/v1/chat/completions", Method: "POST"}, {Kind: "openai-response", Path: "/v1/responses", Method: "POST"}}, Price: p})
	}
	var mu sync.Mutex
	mode := "valid"
	modelCalls, keyCreates, sourceReads, personalReads := 0, 0, 0, 0
	active := map[int64]bool{}
	keys := map[int64][]map[string]any{}
	makeToken := func(id int64) string {
		b, _ := json.Marshal(map[string]any{"iss": "new-api", "aud": []string{"new-api-dashboard"}, "token_use": "access", "sub": strconv.FormatInt(id, 10), "sid": "m3c-synthetic-" + strconv.FormatInt(id, 10), "uv": 1, "sv": 1, "exp": time.Now().Add(2 * time.Hour).Unix(), "iat": time.Now().Unix(), "nbf": time.Now().Add(-5 * time.Second).Unix(), "jti": "m3c-browser-fixture"})
		raw := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(b)
		mac := hmac.New(sha256.New, []byte("synthetic-public-browser-fixture-only"))
		mac.Write([]byte(raw))
		return raw + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
	tokens := map[int64]string{910000001: makeToken(910000001), 910000002: makeToken(910000002)}
	user := func(id int64) map[string]any {
		role := 1
		if id == 910000002 {
			role = 100
		}
		return map[string]any{"id": id, "username": "catalog-synthetic-" + strconv.FormatInt(id, 10), "display_name": "合成验收账户", "role": role, "status": 1, "quota": 0, "used_quota": 0, "request_count": 0}
	}
	bundle := func(id int64) map[string]any {
		return map[string]any{"access_token": tokens[id], "access_expires_at": time.Now().Add(2 * time.Hour).Unix(), "user": user(id), "session": map[string]string{"sid": "m3c-synthetic-" + strconv.FormatInt(id, 10)}}
	}
	native := catalogRoundTrip(func(r *http.Request) (*http.Response, error) {
		response := func(status int, value any) (*http.Response, error) {
			body, _ := json.Marshal(value)
			return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
		}
		mu.Lock()
		defer mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/v1/") || r.URL.Path == "/pg/chat/completions" {
			modelCalls++
			return response(503, map[string]any{"success": false})
		}
		if r.URL.Path == "/internal/momiao/catalog" {
			sourceReads++
			if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 64) || mode == "failed" {
				return response(503, map[string]any{"success": false})
			}
			current := models
			if mode == "empty" {
				current = []platform.NativeCatalogModel{}
			}
			data, _ := json.Marshal(map[string]any{"schema": platform.NativeCatalogSchema, "basis": "public_default_reference", "billing_authority": "native_settlement", "notices": []string{"configuration_not_call_health", "absence_is_not_retirement", "unpublished_models_require_platform_review", "extra_fees_and_integer_quota_rounding_not_included", "API_Credit_is_native_USD_denominated_accounting_unit_not_currency_conversion"}, "models": current})
			return response(200, map[string]any{"success": true, "complete": true, "observed_at": time.Now().UTC().Format(time.RFC3339Nano), "content_hash": fmt.Sprintf("sha256:%x", sha256.Sum256(data)), "data": json.RawMessage(data)})
		}
		id, _ := strconv.ParseInt(r.Header.Get("New-Api-User"), 10, 64)
		if !active[id] || r.Header.Get("Authorization") != "Bearer "+tokens[id] || r.Header.Get("X-Auth-Session") != "m3c-synthetic-"+strconv.FormatInt(id, 10) {
			return response(401, map[string]any{"success": false, "code": "AUTH_UNAUTHORIZED"})
		}
		switch r.URL.Path {
		case "/api/user/self":
			return response(200, map[string]any{"success": true, "data": user(id)})
		case "/api/momiao/catalog/prices":
			personalReads++
			p := price("1.75", "4.5")
			if id == 910000002 {
				p = price("7.25", "10.5")
			}
			p.GroupMultiplier = nil
			return response(200, platform.NativePersonalCatalog{Success: true, Schema: platform.NativeCatalogSchema, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ModelID: r.URL.Query().Get("model_id"), Basis: "current_user_group_reference_not_token_selection", BillingAuthority: "native_settlement", Quotes: []platform.NativePersonalQuote{{Candidate: 1, EnabledConfiguration: true, NativeCatalogVisible: true, Price: &p}}})
		case "/api/token/":
			if r.Method == "POST" {
				var request map[string]any
				if json.NewDecoder(r.Body).Decode(&request) != nil {
					return response(400, map[string]any{"success": false})
				}
				keyCreates++
				keys[id] = append(keys[id], map[string]any{"id": keyCreates, "name": request["name"], "key": "synthetic***masked", "status": 1, "created_time": time.Now().Unix(), "expired_time": request["expired_time"], "remain_quota": request["remain_quota"], "used_quota": 0, "unlimited_quota": request["unlimited_quota"]})
				return response(200, map[string]any{"success": true, "data": map[string]any{}})
			}
			items := keys[id]
			if items == nil {
				items = []map[string]any{}
			}
			size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
			if size < 1 {
				size = 10
			}
			return response(200, map[string]any{"success": true, "data": map[string]any{"items": items, "total": len(items), "page": 1, "page_size": size}})
		case "/api/log/self":
			return response(200, map[string]any{"success": true, "data": map[string]any{"items": []any{}, "total": 0, "page": 1, "page_size": 10}})
		}
		return response(503, map[string]any{"success": false, "message": "outside synthetic fixture scope"})
	})
	read := nativeCatalogReader{transport: native, key: strings.Repeat("a", 64)}.Read
	result, err := store.SyncCatalog(ctx, read)
	if err != nil || result.Status != "VERIFIED" {
		t.Fatal("fixture source did not pass real catalog reader", err, result.Status)
	}
	policy := platform.CatalogPolicy{StaleAfter: 10 * time.Minute, DisableAfter: 30 * time.Minute}
	names := map[string]string{"..": "Dot · 路径边界", "demo/aurora": "月海 · Kimi 合成展示", "demo/cedar": "雪松 · 文字工坊", "demo/星海'$(literal)": "星海 · 灵感连接"}
	families := map[string]string{"..": "other", "demo/aurora": "kimi", "demo/cedar": "claude", "demo/星海'$(literal)": "gpt"}
	for i, id := range ids {
		if id == "demo/review" {
			continue
		}
		p, item, e := store.OpsCatalogModel(ctx, 910000001, id, policy)
		if e != nil {
			t.Fatal(e)
		}
		if item.PublicationState != "PENDING_METADATA" {
			continue
		}
		status, e := store.CatalogSyncStatus(ctx)
		if e != nil {
			t.Fatal(e)
		}
		length := int64(128000)
		var contextLength *int64
		if id != "demo/cedar" {
			contextLength = &length
		}
		metadata := platform.CatalogMetadata{DisplayName: names[id], Family: families[id], Summary: "本地合成验收模型：用于核对目录、精确参考价与接入流程，不连接真实上游。", ContextLength: contextLength, Subtitle: "选择一种新的探索方式", Tags: []string{"writing", "general"}, UseCases: []string{"writing", "conversation"}}
		if id == "demo/aurora" {
			metadata.AssetID = "persona_kimi_master"
		}
		for _, action := range []string{"SAVE", "PUBLISH"} {
			var bytes [16]byte
			if _, e = rand.Read(bytes[:]); e != nil {
				t.Fatal(e)
			}
			hexID := hex.EncodeToString(bytes[:])
			uuid := hexID[:8] + "-" + hexID[8:12] + "-" + hexID[12:16] + "-" + hexID[16:20] + "-" + hexID[20:]
			command := platform.CatalogCommand{OperationID: uuid, Epoch: p.Epoch, Action: action, ModelID: id, ExpectedVersion: item.Version, ExpectedCatalogVersion: status.Version, Reason: "合成浏览器验收初始化"}
			if action == "SAVE" {
				command.Metadata = &metadata
				command.Recommended = id != ".."
				command.SortOrder = i
			}
			preview, e := store.PrepareCatalog(ctx, 910000001, command, read, policy)
			if e != nil {
				t.Fatal(e)
			}
			ack, e := store.ExecuteCatalog(ctx, 910000001, command, preview.ID, true, read, policy)
			if e != nil {
				t.Fatal(e)
			}
			item.Version = ack.Version
		}
	}
	web, err := filepath.Abs("../../web/dist")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	cfg := config{WebDir: web, PublicOrigin: origin, catalog: store, catalogSource: read, CatalogStaleAfter: policy.StaleAfter, CatalogDisableAfter: policy.DisableAfter, APIBaseURL: "https://synthetic-api.example/v1", announcements: store, profile: store, wallet: store, economy: store, accessGate: store, accessDeclaration: &accessDeclaration{Version: 1, Environment: "STAGING", Origin: origin, EvidenceRef: "synthetic-portal-catalog-browser-only", MigrationApplicability: "PERSISTED_COMPLETED_NOTICE", Resources: map[string]string{"ACCOUNT": "AVAILABLE", "API": "AVAILABLE", "COMMUNITY": "AVAILABLE", "OPERATIONS": "AVAILABLE", "ASSETS": "AVAILABLE"}}}
	portal := newPortalHandler(cfg, native)
	var server *http.Server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			var input struct{ Username, Password string }
			if r.Method != "POST" || json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&input) != nil || input.Password != "catalog-synthetic-only" {
				walletError(w, 401, "AUTH_UNAUTHORIZED")
				return
			}
			id := int64(0)
			if input.Username == "catalog-review-admin" {
				id = 910000001
			}
			if input.Username == "catalog-review-reader" {
				id = 910000002
			}
			if id == 0 {
				walletError(w, 401, "AUTH_UNAUTHORIZED")
				return
			}
			mu.Lock()
			active[id] = true
			mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: "m3c_synthetic_fixture", Value: strconv.FormatInt(id, 10), Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			walletSuccess(w, bundle(id))
		case "/api/user/auth/refresh":
			cookie, e := r.Cookie("m3c_synthetic_fixture")
			if e != nil {
				walletError(w, 401, "AUTH_UNAUTHORIZED")
				return
			}
			id, _ := strconv.ParseInt(cookie.Value, 10, 64)
			mu.Lock()
			valid := active[id]
			mu.Unlock()
			if !valid {
				walletError(w, 401, "AUTH_UNAUTHORIZED")
				return
			}
			walletSuccess(w, bundle(id))
		case "/api/user/auth/logout":
			if c, e := r.Cookie("m3c_synthetic_fixture"); e == nil {
				id, _ := strconv.ParseInt(c.Value, 10, 64)
				mu.Lock()
				active[id] = false
				mu.Unlock()
			}
			http.SetCookie(w, &http.Cookie{Name: "m3c_synthetic_fixture", Path: "/", HttpOnly: true, MaxAge: -1, SameSite: http.SameSiteStrictMode})
			walletSuccess(w, map[string]any{})
		case "/__catalog-fixture/status":
			mu.Lock()
			defer mu.Unlock()
			walletSuccess(w, map[string]any{"synthetic": true, "model_calls": modelCalls, "key_creates": keyCreates, "source_reads": sourceReads, "personal_reads": personalReads, "source_mode": mode})
		case "/__catalog-fixture/source":
			if r.Method != "POST" || r.Header.Get("X-Catalog-Fixture") != "synthetic-local-only" {
				walletError(w, 403, "FIXTURE_ONLY")
				return
			}
			var input struct{ Mode string }
			if json.NewDecoder(r.Body).Decode(&input) != nil || !slices.Contains([]string{"valid", "empty", "failed", "stale", "expired"}, input.Mode) {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
			if input.Mode == "stale" || input.Mode == "expired" {
				minutes := 11
				if input.Mode == "expired" {
					minutes = 31
				}
				_, e := conn.Exec(r.Context(), `UPDATE catalog.model_sync_state SET last_verified_at=clock_timestamp()-make_interval(mins=>$1) WHERE singleton`, minutes)
				if e != nil {
					walletError(w, 503, "FIXTURE_FAILED")
					return
				}
				walletSuccess(w, map[string]any{"mode": input.Mode})
				return
			}
			mu.Lock()
			mode = input.Mode
			mu.Unlock()
			result, e := store.SyncCatalog(r.Context(), read)
			if e != nil {
				walletError(w, 503, "FIXTURE_FAILED")
				return
			}
			walletSuccess(w, result)
		case "/__catalog-fixture/stop":
			if r.Method != "POST" || r.Header.Get("X-Catalog-Fixture") != "synthetic-local-only" {
				walletError(w, 403, "FIXTURE_ONLY")
				return
			}
			walletSuccess(w, map[string]any{"stopping": true})
			go server.Shutdown(context.Background())
		default:
			portal.ServeHTTP(w, r)
		}
	})
	server = &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	t.Logf("CATALOG_SYNTHETIC_FIXTURE_READY %s; real Go + PostgreSQL; synthetic native only", origin)
	if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if modelCalls != 0 {
		t.Errorf("catalog acceptance made %d inference calls", modelCalls)
	}
	t.Logf("CATALOG_FIXTURE_RECEIPT model_calls=%d key_creates=%d source_reads=%d personal_reads=%d", modelCalls, keyCreates, sourceReads, personalReads)
}
