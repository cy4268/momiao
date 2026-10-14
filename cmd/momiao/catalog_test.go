package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/poker"
	"github.com/cy4268/momiao/internal/poker/redislease"
	"github.com/cy4268/momiao/internal/rankings"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type catalogHTTPStore struct {
	*platform.Store
	publicCalls int
	withdrawn   bool
	filter      platform.CatalogFilter
}

func (s *catalogHTTPStore) PublicCatalog(_ context.Context, f platform.CatalogFilter) (platform.CatalogPage, error) {
	s.publicCalls++
	s.filter = f
	return platform.CatalogPage{Items: []platform.CatalogModel{}}, nil
}
func (s *catalogHTTPStore) PublicCatalogModel(_ context.Context, id string) (platform.CatalogModel, error) {
	s.publicCalls++
	if s.withdrawn {
		return platform.CatalogModel{}, platform.ErrCatalogNotFound
	}
	return platform.CatalogModel{ModelID: id, PublicationState: "PUBLISHED", CanUse: true}, nil
}
func catalogHTTPConfig(s catalogStore) config {
	return config{catalog: s, PublicOrigin: "https://wallet.example", APIBaseURL: "https://api.example/v1"}
}

func TestCatalogCoverUploadLifecycle(t *testing.T) {
	command := platform.CatalogCoverUploadCommand{Family: "gpt", Alt: "Acceptance cover", RightsStatus: "ORIGINAL_GENERATED", RightsNote: "Generated"}
	metadata, _ := json.Marshal(command)
	request := func(data []byte, mimeType, extra string) *http.Request {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		part, _ := mw.CreateFormField("metadata")
		_, _ = part.Write(metadata)
		headers := textproto.MIMEHeader{"Content-Disposition": {`form-data; name="file"; filename="not-trusted.svg"`}}
		if mimeType != "" {
			headers.Set("Content-Type", mimeType)
		}
		part, _ = mw.CreatePart(headers)
		_, _ = part.Write(data)
		if extra != "" {
			part, _ = mw.CreateFormField(extra)
			_, _ = part.Write([]byte("{}"))
		}
		_ = mw.Close()
		r := announcementReq("POST", "/platform/v1/ops/models/family-covers/upload", "")
		r.Body = io.NopCloser(bytes.NewReader(body.Bytes()))
		r.ContentLength = int64(body.Len())
		r.Header.Set("Content-Type", mw.FormDataContentType())
		return r
	}
	png, _ := os.ReadFile("../../web/public/assets/models/kimi-family-normalized-v001.png")
	webp, _ := os.ReadFile("../../web/public/assets/models/kimi-family-web-v001.webp")
	if len(png) == 0 || len(webp) == 0 {
		t.Fatal("existing raster missing")
	}
	var jpg bytes.Buffer
	_ = jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil)
	mutatePNG := func(w, h uint32) []byte {
		b := bytes.Clone(png)
		binary.BigEndian.PutUint32(b[16:20], w)
		binary.BigEndian.PutUint32(b[20:24], h)
		binary.BigEndian.PutUint32(b[29:33], crc32.ChecksumIEEE(b[12:29]))
		return b
	}
	apng := append(bytes.Clone(png[:33]), append([]byte{0, 0, 0, 0, 'a', 'c', 'T', 'L', 0, 0, 0, 0}, png[33:]...)...)
	animated := bytes.Clone(webp)
	animated = append(animated, []byte{'A', 'N', 'I', 'M', 0, 0, 0, 0}...)
	binary.LittleEndian.PutUint32(animated[4:8], uint32(len(animated)-8))
	for _, v := range []struct {
		data        []byte
		kind, extra string
	}{
		{[]byte("<svg></svg>"), "image/png", ""}, {make([]byte, 8*1024*1024+1), "image/png", ""}, {mutatePNG(9000, 1), "image/png", ""}, {mutatePNG(5000, 5000), "image/png", ""}, {apng, "image/png", ""}, {animated, "image/webp", ""}, {png[:100], "image/png", ""}, {webp[:40], "image/webp", ""}, {png, "image/jpeg", ""}, {png, "image/png", "file"}, {png, "image/png", "metadata"}, {png, "image/png", "unexpected"},
	} {

		if _, _, err := parseCatalogCoverUpload(httptest.NewRecorder(), request(v.data, v.kind, v.extra)); err == nil {
			t.Fatal("invalid upload accepted")
		}
	}
	for _, v := range []struct {
		data              []byte
		mime, actual, ext string
	}{{png, "image/png", "image/png", "png"}, {webp, "application/octet-stream", "image/webp", "webp"}, {jpg.Bytes(), "", "image/jpeg", "jpg"}} {
		c, f, err := parseCatalogCoverUpload(httptest.NewRecorder(), request(v.data, v.mime, ""))
		if err != nil || c.Family != "gpt" || !bytes.Equal(f.Bytes, v.data) || f.Image.ContentType != v.actual || f.Image.Extension != v.ext {
			t.Fatal("original raster upload changed", err)
		}
	}
}

func TestCatalogHTTPPublicAndStrictQueries(t *testing.T) {
	s := &catalogHTTPStore{}
	h := newCatalogHandler(catalogHTTPConfig(s))
	for _, path := range []string{"/platform/v1/models", "/platform/v1/models/detail?model_id=" + url.QueryEscape("组织/x%2F'"), "/platform/v1/models/access-config"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer ignored-public-credential")
		h.ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(path, w.Code, w.Body)
		}
	}
	for _, query := range []string{"?user_id=1", "?limit=-1", "?limit=101", "?limit=0", "?offset=1000001", "?q=a&q=b", "?recommended=maybe", "?min_context=1e3", "?unknown_context=false", "?x=%GG", "?group_by=model", "?group_by=", "?group_by=family&group_by=family"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/platform/v1/models"+query, nil))
		if w.Code != 400 {
			t.Fatal(query, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/platform/v1/models?q=100%25&recommended=true&min_context=100&price_dimension=input&min_price=0.000000001&sort=price&group_by=family", nil))
	if w.Code != 200 || s.filter.Search != "100%" || !s.filter.RecommendedOnly || s.filter.MinPrice == nil || *s.filter.MinPrice != "0.000000001" || s.filter.GroupBy != "family" {
		t.Fatal(w.Code, s.filter)
	}
	for _, path := range []string{"/platform/v1/models/detail", "/platform/v1/models/detail?model_id=x&group=default", "/platform/v1/models/access-config?base_url=https://evil.example"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
}
func TestCatalogHTTPProtectedWrites(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires the existing dev database and Redis")
	}
	dsn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read dev DSN")
	}
	store, err := platform.Open(t.Context(), strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("open dev database")
	}
	defer store.Close()
	options, err := readSessionRedisConfig(os.Getenv("MOMIAO_SESSION_REDIS_CONFIG_FILE"))
	if err != nil {
		t.Fatal("read dev session Redis config")
	}
	client := redis.NewClient(options)
	defer client.Close()
	auth := &identity.HTTP{Store: &identity.Store{Pool: store.Pool()}, Sessions: identity.Sessions{Redis: client}, Origin: "https://dev.momiao.win"}

	var admin, regular int64
	if err = store.Pool().QueryRow(t.Context(), `SELECT min(id) FILTER(WHERE role>=10),min(id) FILTER(WHERE role<10) FROM identity.users WHERE status=1 AND deleted_at IS NULL`).Scan(&admin, &regular); err != nil {
		t.Fatal(err)
	}
	var model string
	if err = store.Pool().QueryRow(t.Context(), `SELECT model_id FROM catalog.model_catalog_publication WHERE publication_state='PUBLISHED' ORDER BY model_id LIMIT 1`).Scan(&model); err != nil {
		t.Fatal(err)
	}
	sessions := map[int64]identity.Session{}
	for _, user := range []int64{admin, regular} {
		value, e := auth.Sessions.Create(t.Context(), user)
		if e != nil {
			t.Fatal(e)
		}
		sessions[user] = value
		defer func() {
			if e := auth.Sessions.Revoke(context.Background(), value); e != nil {
				t.Error(e)
			}
		}()
	}
	for _, webDir := range []string{"", "/unused-catalog-web"} {
		cfg := catalogHTTPConfig(store)
		cfg.PublicOrigin, cfg.WebDir = auth.Origin, webDir
		cfg.OpsEnvironment = "DEVELOPMENT"
		cfg.Session.Enabled = true
		cfg.localIdentity = auth
		cfg.relay = store
		cfg.rpUsage = store
		cfg.RankingActivationTime = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
		cfg.rankings = rankings.NewService(store, rankings.Options{ActivationTime: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)})
		cfg.games, err = games.NewService(store, games.Keyring{Active: "ops-read-only", Keys: map[string][32]byte{"ops-read-only": {}}})
		if err != nil {
			t.Fatal(err)
		}
		var pokerPool *pgxpool.Pool
		if path := os.Getenv("MOMIAO_OPS_POKER_DSN_FILE"); path != "" {
			raw, e := os.ReadFile(path)
			if e != nil {
				t.Fatal("read dev Poker DSN")
			}
			pokerPool, e = pgxpool.New(t.Context(), strings.TrimSpace(string(raw)))
			if e != nil {
				t.Fatal(e)
			}
			defer pokerPool.Close()
			lease, e := redislease.Open(t.Context(), redislease.Config{Addr: options.Addr, Username: options.Username, Password: options.Password, TLSConfig: options.TLSConfig})
			if e != nil {
				t.Fatal(e)
			}
			defer lease.Close()
			service, e := poker.New(poker.Options{Pool: pokerPool, Leases: lease, Keyring: poker.Keyring{Current: "read-only", Keys: map[string][]byte{"read-only": make([]byte, 32)}}})
			if e != nil {
				t.Fatal(e)
			}
			defer service.Close()
			cfg.pokerOps = service

		}
		h := newPortalHandler(cfg)
		request := func(user int64, method, path, body string, csrf bool) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, path, strings.NewReader(body))
			r.Header.Set("New-Api-User", "1")
			r.Header.Set("X-Auth-Session", "forged")
			r.Header.Set("Origin", auth.Origin)
			r.Header.Set("Content-Type", "application/json")
			if value, ok := sessions[user]; ok {
				r.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: value.Token})
				if csrf {
					r.Header.Set("X-CSRF-Token", value.CSRFToken)
				}
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			return w
		}

		t.Run("native-dependencies", func(t *testing.T) {
			for _, user := range []int64{admin, regular} {
				r := httptest.NewRequest("GET", "/local-wallet", nil)
				r.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: sessions[user].Token})
				prepared, err := auth.Request(r, false)
				if err != nil {
					t.Fatal(err)
				}
				if id, status := verifyWalletUser(prepared); status != 0 || id != user {
					t.Fatal("local wallet identity", id, status)
				}
				prepared.URL.Path = "/platform/v1/wallet"
				wallet := httptest.NewRecorder()
				newWalletHandler(auth.Origin, store).ServeHTTP(wallet, prepared)
				if wallet.Code != 200 || !strings.Contains(wallet.Body.String(), `"user_id":"`+strconv.FormatInt(user, 10)+`"`) {
					t.Fatal("local wallet read", wallet.Code, wallet.Body.String())
				}
				w := request(user, "GET", "/api/v1/rankings/me", "", false)
				if w.Code != 200 {
					t.Fatalf("web=%t local personal ranking: status=%d body=%s", webDir != "", w.Code, w.Body.String())
				}
				var actual struct {
					Data rankings.Page `json:"data"`
				}
				if json.Unmarshal(w.Body.Bytes(), &actual) != nil {
					t.Fatal("ranking response")
				}
				expected, err := cfg.rankings.Read(t.Context(), rankings.Query{Metric: "TOTAL_ASSETS", Period: "CURRENT", Page: 1}, user)
				if err != nil {
					t.Fatal(err)
				}
				a, _ := json.Marshal(actual.Data)
				b, _ := json.Marshal(expected)
				if !bytes.Equal(a, b) {
					t.Fatal("ranking does not match the local session user")
				}
			}
			if w := request(0, "GET", "/api/v1/rankings/me", "", false); w.Code != 401 {
				t.Fatal("ranking anonymous", w.Code)
			}
			if w := request(admin, "POST", "/api/v1/rankings/me", `{}`, true); w.Code != 405 {
				t.Fatal("ranking read only", w.Code)
			}
			if w := request(admin, "GET", "/api/v1/rankings/me?page=0", "", false); w.Code != 400 {
				t.Fatal("ranking invalid page", w.Code)
			}
		})

		t.Run("rp", func(t *testing.T) {
			for _, user := range []int64{admin, regular} {
				w := request(user, "GET", "/api/v1/usage/rp", "", false)
				if w.Code != 200 {
					t.Fatalf("local RP usage: status=%d body=%s", w.Code, w.Body.String())
				}
				var body struct {
					Data struct {
						platform.RPUsagePage
						ObservedAt time.Time `json:"observed_at"`
					} `json:"data"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				var total int64
				if err := store.Pool().QueryRow(t.Context(), `SELECT count(DISTINCT logical_request_id) FROM economy.request_attributions WHERE newapi_user_id=$1 AND key_purpose_snapshot='ROLEPLAY' AND entered_model_flow AND requested_at>=$2`, user, cfg.RankingActivationTime).Scan(&total); err != nil {
					t.Fatal(err)
				}
				if body.Data.Total != total || body.Data.ObservedAt.IsZero() {
					t.Fatal("RP account isolation or observation", body.Data.Total, total)
				}
				w = request(user, "GET", "/api/v1/usage/rp?model=no-such-model&status=SUCCESS&page=2&from=2026-09-01T00:00:00Z&to=2026-10-14T00:00:00Z", "", false)
				if w.Code != 200 || !strings.Contains(w.Body.String(), `"total":"0"`) || !strings.Contains(w.Body.String(), `"page":2`) {
					t.Fatal("RP filters", w.Code, w.Body.String())
				}
			}
			if w := request(0, "GET", "/api/v1/usage/rp", "", false); w.Code != 401 {
				t.Fatal("RP anonymous", w.Code)
			}
			if w := request(admin, "POST", "/api/v1/usage/rp", `{}`, true); w.Code != 405 {
				t.Fatal("RP read only", w.Code)
			}
			if w := request(admin, "GET", "/api/v1/usage/rp?page=0", "", false); w.Code != 400 {
				t.Fatal("RP bad page", w.Code)
			}
		})

		t.Run("runtime", func(t *testing.T) {
			for _, path := range []string{"/api/v1/ops/bootstrap", "/api/v1/ops/admin-principals", "/api/v1/ops/operations", "/api/v1/ops/operations/00000000-0000-4000-8000-000000000001", "/api/v1/ops/fresh/password", "/api/v1/ops/fresh/discord/start"} {
				if w := request(admin, "GET", path, "", false); w.Code != 404 {
					t.Fatal("removed ops pipeline", path, w.Code, w.Body.String())
				}
				if w := request(admin, "POST", path, `{}`, true); w.Code != 404 {
					t.Fatal("removed ops write", path, w.Code, w.Body.String())
				}
			}
			if gateRouteDomain("/ops/access-control") == "OPERATIONS" || gateRouteDomain("/ops/operations/00000000-0000-4000-8000-000000000001") == "OPERATIONS" {
				t.Fatal("removed legacy Ops browser routes")
			}
			for _, path := range []string{"/api/v1/ops/jobs?page=1", "/api/v1/ops/attention?page=1", "/api/v1/ops/service-health"} {
				w := request(admin, "GET", path, "", false)
				if w.Code != 200 {
					t.Fatal("local runtime", path, w.Code, w.Body.String())
				}
				for _, user := range []int64{regular, 0} {
					want := 403
					if user == 0 {
						want = 401
					}
					if w := request(user, "GET", path, "", false); w.Code != want {
						t.Fatal("runtime role", path, w.Code, w.Body.String())
					}
				}
				if w := request(admin, "POST", path, `{}`, true); w.Code != 405 {
					t.Fatal("runtime read only", w.Code, w.Body.String())
				}
			}
			for _, path := range []string{"/api/v1/ops/jobs?page=0", "/api/v1/ops/attention?from=bad", "/api/v1/ops/jobs?from=2026-10-14T00:00:00Z&to=2026-10-13T00:00:00Z"} {
				if w := request(admin, "GET", path, "", false); w.Code != 400 {
					t.Fatal("runtime filter input", w.Code, w.Body.String())
				}
			}
			for _, path := range []string{"/api/v1/ops/jobs?kind=ANNOUNCEMENTS&status=PENDING&from=2026-01-01T00:00:00Z&to=2027-01-01T00:00:00Z&page=2", "/api/v1/ops/attention?kind=GAME_ROUNDS"} {
				if w := request(admin, "GET", path, "", false); w.Code != 200 || !strings.Contains(w.Body.String(), `"items":`) || !strings.Contains(w.Body.String(), `"summary":`) {
					t.Fatal("runtime filters", w.Code, w.Body.String())
				}
			}
			if w := request(admin, "GET", "/api/v1/ops/native-runtime", "", false); w.Code != 404 {
				t.Fatal("removed native runtime", w.Code, w.Body.String())
			}
		})
		t.Run("support-incidents", func(t *testing.T) {
			for _, path := range []string{"/api/v1/ops/support-cases?page=1", "/api/v1/ops/incidents?page=1"} {
				w := request(admin, "GET", path, "", false)
				if w.Code != 200 || !strings.Contains(w.Body.String(), `"items":`) {
					t.Fatal("local support/incidents", path, w.Code, w.Body.String())
				}
				for _, user := range []int64{regular, 0} {
					want := 403
					if user == 0 {
						want = 401
					}
					if w := request(user, "GET", path, "", false); w.Code != want {
						t.Fatal("support role", w.Code, w.Body.String())
					}
				}
			}
			for _, root := range []string{"/api/v1/ops/support-cases", "/api/v1/ops/incidents"} {
				if w := request(admin, "POST", root, `{}`, false); w.Code != 403 {
					t.Fatal("support CSRF", w.Code, w.Body.String())
				}
				if w := request(admin, "POST", root, `{}`, true); w.Code != 400 {
					t.Fatal("support input", w.Code, w.Body.String())
				}
				if w := request(regular, "POST", root, `{}`, true); w.Code != 403 {
					t.Fatal("support write role", w.Code, w.Body.String())
				}
				if w := request(admin, "GET", root+"?page=0", "", false); w.Code != 400 {
					t.Fatal("support page", w.Code, w.Body.String())
				}
				if w := request(admin, "GET", root+"?from=bad", "", false); w.Code != 400 {
					t.Fatal("support date", w.Code, w.Body.String())
				}
				if w := request(admin, "GET", root+"?q=not-found&from=2026-09-01T00:00:00Z&to=2026-10-14T00:00:00Z", "", false); w.Code != 200 {
					t.Fatal("support filters", w.Code, w.Body.String())
				}
				missing := root + "/10000000-0000-7000-8000-000000000099"
				if w := request(admin, "GET", missing, "", false); w.Code != 404 {
					t.Fatal("support missing", w.Code, w.Body.String())
				}
				if w := request(admin, "PUT", missing, `{"next_state":"CLOSED"}`, true); w.Code != 404 {
					t.Fatal("support update missing", w.Code, w.Body.String())
				}
			}
		})
		t.Run("poker", func(t *testing.T) {
			if pokerPool == nil {
				t.Skip("requires existing dev Poker DSN")
			}
			for _, path := range []string{"/api/v1/ops/poker", "/api/v1/ops/poker/tables?limit=2", "/api/v1/ops/poker/sessions?limit=2"} {
				if w := request(admin, "GET", path, "", false); w.Code != 200 {
					t.Fatal("local poker", w.Code, w.Body.String())
				}
				if w := request(regular, "GET", path, "", false); w.Code != 403 {
					t.Fatal("poker role", w.Code)
				}
				if w := request(0, "GET", path, "", false); w.Code != 401 {
					t.Fatal("poker anonymous", w.Code)
				}
			}
			for _, query := range []string{"?limit=-1", "?from=bad", "?from=2026-10-14T00:00:00Z&to=2026-09-01T00:00:00Z"} {
				if w := request(admin, "GET", "/api/v1/ops/poker/tables"+query, "", false); w.Code != 400 {
					t.Fatal("poker invalid filter", w.Code)
				}
			}
			if w := request(admin, "GET", "/api/v1/ops/poker/tables?state=CLOSED&q=not-found&from=2026-09-01T00:00:00Z&to=2026-10-14T00:00:00Z", "", false); w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
				t.Fatal("poker filter", w.Code, w.Body.String())
			}
			var tableID, sessionID, handID string
			if err := pokerPool.QueryRow(t.Context(), `SELECT table_id::text,session_id::text FROM poker.sessions ORDER BY session_id LIMIT 1`).Scan(&tableID, &sessionID); err != nil {
				t.Fatal(err)
			}
			if err := pokerPool.QueryRow(t.Context(), `SELECT hand_id::text FROM poker.hands ORDER BY hand_id LIMIT 1`).Scan(&handID); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"/api/v1/ops/poker/tables/" + tableID, "/api/v1/ops/poker/sessions/" + sessionID, "/api/v1/ops/poker/hands/" + handID} {
				w := request(admin, "GET", path, "", false)
				if w.Code != 200 {
					t.Fatal("poker detail", w.Code, w.Body.String())
				}
				if strings.Contains(w.Body.String(), `"server_seed"`) || strings.Contains(w.Body.String(), `"encrypted_snapshot"`) {
					t.Fatal("private poker material")
				}
			}
			path := "/api/v1/ops/poker/tables/00000000-0000-7000-8000-000000000001"
			if w := request(admin, "POST", path, `{}`, false); w.Code != 403 {
				t.Fatal("poker CSRF", w.Code)
			}
			if w := request(regular, "POST", path, `{}`, true); w.Code != 403 {
				t.Fatal("poker write role", w.Code)
			}
			if w := request(admin, "POST", path, `{}`, true); w.Code != 400 {
				t.Fatal("poker invalid command", w.Code, w.Body.String())
			}
			if w := request(admin, "POST", path, `{"command_type":"POKER_TABLE_PAUSE"}`, true); w.Code != 404 {
				t.Fatal("poker missing", w.Code, w.Body.String())
			}
			t.Log("poker both routers: admin200 regular403 anonymous401 CSRF403 invalid400 missing404; global filters and public details; no native")
		})

		t.Run("games", func(t *testing.T) {
			for _, path := range []string{"/api/v1/ops/games", "/api/v1/ops/games/scratch", "/api/v1/ops/games/devil-roulette"} {
				w := request(admin, "GET", path, "", false)
				if w.Code != 200 {
					t.Fatal("local games", w.Code, w.Body.String())
				}
				for _, user := range []int64{regular, 0} {
					want := 403
					if user == 0 {
						want = 401
					}
					if w := request(user, "GET", path, "", false); w.Code != want {
						t.Fatal("game read role", w.Code, w.Body.String())
					}
				}
			}
			if w := request(admin, "GET", "/api/v1/ops/games/scratch?page=2&status=ACTIVE&from=2026-09-01T00:00:00Z&to=2026-10-14T00:00:00Z", "", false); w.Code != 200 || !strings.Contains(w.Body.String(), `"page":2`) || !strings.Contains(w.Body.String(), `"config_versions":[]`) {
				t.Fatal("config history paging", w.Code, w.Body.String())
			}
			if w := request(admin, "GET", "/api/v1/ops/games/scratch?from=bad", "", false); w.Code != 400 {
				t.Fatal("config time filter", w.Code, w.Body.String())
			}
			for _, p := range []struct{ path, method string }{
				{"/api/v1/ops/games/scratch", "PUT"},
				{"/api/v1/ops/games/scratch/configs/00000000-0000-7000-8000-000000000001/activate", "POST"},
				{"/api/v1/ops/games/devil-roulette/rounds/00000000-0000-7000-8000-000000000001/void", "POST"},
			} {
				if w := request(admin, p.method, p.path, `{}`, false); w.Code != 403 {
					t.Fatal("games CSRF", w.Code, w.Body.String())
				}
				if w := request(regular, p.method, p.path, `{}`, true); w.Code != 403 {
					t.Fatal("games write role", w.Code, w.Body.String())
				}
				want := 404
				if p.method == "PUT" {
					want = 400
				}
				if w := request(admin, p.method, p.path, `{}`, true); w.Code != want {
					t.Fatal("games invalid or missing", w.Code, w.Body.String())
				}
			}
			if w := request(admin, "GET", "/api/v1/ops/games/invalid", "", false); w.Code != 404 {
				t.Fatal(w.Code)
			}
			if w := request(admin, "POST", "/api/v1/ops/games/scratch/configs/invalid/validate", `{}`, true); w.Code != 404 {
				t.Fatal(w.Code)
			}
			t.Log("games both routers: local admin200 regular403 anonymous401 CSRF403 invalid400 missing404; no native")
		})

		t.Run("rankings", func(t *testing.T) {
			w := request(admin, "GET", "/api/v1/ops/rankings", "", false)
			if w.Code != 200 {
				t.Fatal("local rankings", w.Code, w.Body.String())
			}

			if w := request(admin, "GET", "/api/v1/ops/rankings?domain=GAMES&page=2&from=2026-09-01T00:00:00Z&to=2026-10-14T00:00:00Z", "", false); w.Code != 200 || !strings.Contains(w.Body.String(), `"page":2`) {
				t.Fatal("rankings paging", w.Code, w.Body.String())
			}
			for _, query := range []string{"?domain=bad", "?page=-1", "?from=bad", "?from=2026-10-14T00:00:00Z&to=2026-09-01T00:00:00Z"} {
				if w := request(admin, "GET", "/api/v1/ops/rankings"+query, "", false); w.Code != 400 {
					t.Fatal("ranking filter", w.Code)
				}
			}
			var snapshotID string
			if err := store.Pool().QueryRow(t.Context(), `SELECT snapshot_id::text FROM rankings.published_pointers WHERE domain='GAMES' ORDER BY published_at DESC LIMIT 1`).Scan(&snapshotID); err != nil {
				t.Fatal(err)
			}
			if w := request(admin, "GET", "/api/v1/ops/rankings/snapshots/"+snapshotID, "", false); w.Code != 200 || !strings.Contains(w.Body.String(), `"diff"`) {
				t.Fatal("ranking snapshot detail", w.Code, w.Body.String())
			}
			for _, user := range []int64{regular, 0} {
				code := 403
				if user == 0 {
					code = 401
				}
				if w := request(user, "GET", "/api/v1/ops/rankings", "", false); w.Code != code {
					t.Fatal("rankings role", w.Code, w.Body.String())
				}
			}
			for _, path := range []string{"/api/v1/ops/rankings/rebuilds", "/api/v1/ops/rankings/snapshots/00000000-0000-4000-8000-000000000001/publish"} {
				if w := request(admin, "POST", path, `{}`, false); w.Code != 403 {
					t.Fatal("rankings CSRF", w.Code, w.Body.String())
				}
				if w := request(regular, "POST", path, `{}`, true); w.Code != 403 {
					t.Fatal("rankings write role", w.Code, w.Body.String())
				}
			}
			if w := request(admin, "POST", "/api/v1/ops/rankings/rebuilds", `{"domain":"invalid"}`, true); w.Code != 400 {
				t.Fatal("rankings invalid rebuild", w.Code, w.Body.String())
			}
			if w := request(admin, "POST", "/api/v1/ops/rankings/snapshots/00000000-0000-4000-8000-000000000001/publish", `{}`, true); w.Code != 404 {
				t.Fatal("rankings missing candidate", w.Code, w.Body.String())
			}
		})

		t.Run("economy", func(t *testing.T) {
			gamblerPath := "/api/v1/ops/economy/gambler-campaigns/" + platform.GamblerCampaignID
			if w := request(admin, "GET", gamblerPath+"?page=1", "", false); w.Code != 200 || !strings.Contains(w.Body.String(), `"page_size":20`) {
				t.Fatal("local gambler", w.Code, w.Body.String())
			}
			for _, u := range []int64{regular, 0} {
				want := 403
				if u == 0 {
					want = 401
				}
				if w := request(u, "GET", gamblerPath, "", false); w.Code != want {
					t.Fatal("gambler role", w.Code)
				}
			}
			if w := request(admin, "POST", gamblerPath, `{}`, false); w.Code != 403 {
				t.Fatal("gambler CSRF", w.Code)
			}
			if w := request(regular, "POST", gamblerPath, `{}`, true); w.Code != 403 {
				t.Fatal("gambler write role", w.Code)
			}
			if w := request(admin, "POST", gamblerPath, `{}`, true); w.Code != 400 {
				t.Fatal("gambler input", w.Code, w.Body.String())
			}
			if w := request(admin, "GET", gamblerPath+"?page=0", "", false); w.Code != 400 {
				t.Fatal("gambler page", w.Code)
			}
			if w := request(admin, "GET", gamblerPath+"?from=bad", "", false); w.Code != 400 {
				t.Fatal("gambler date", w.Code)
			}
			if w := request(admin, "GET", gamblerPath+"?state=BAD", "", false); w.Code != 400 {
				t.Fatal("gambler state", w.Code)
			}
			if w := request(admin, "GET", gamblerPath+"?eligible=true&from=2026-01-01T00:00:00Z&to=2027-01-01T00:00:00Z&page=2", "", false); w.Code != 200 {
				t.Fatal("gambler filters", w.Code, w.Body.String())
			}
			if w := request(admin, "POST", gamblerPath, `{"action":"snapshot","requests_drained":true}`, true); w.Code != 409 {
				t.Fatal("completed campaign restart", w.Code, w.Body.String())
			}
			assetPath := "/api/v1/ops/economy/user?newapi_user_id=" + strconv.FormatInt(regular, 10)
			assetResponse := request(admin, "GET", assetPath, "", false)
			if assetResponse.Code != 200 || !strings.Contains(assetResponse.Body.String(), `"assets":`) || !strings.Contains(assetResponse.Body.String(), `"single_player_in_flight_units":`) {
				t.Fatal("local assets missing", assetResponse.Code)
			}
			if strings.Contains(assetResponse.Body.String(), `"active_quota_units":`) || strings.Contains(assetResponse.Body.String(), `"native_observed_at":`) {
				t.Fatal("native assets remain")
			}
			chipPath := "/api/v1/ops/users/" + strconv.FormatInt(regular, 10) + "/chips"
			if w := request(admin, "POST", chipPath, `{}`, false); w.Code != 403 {
				t.Fatal("chips CSRF", w.Code)
			}
			if w := request(regular, "POST", chipPath, `{}`, true); w.Code != 403 {
				t.Fatal("chips role", w.Code)
			}
			if w := request(admin, "POST", chipPath, `{}`, true); w.Code != 400 {
				t.Fatal("chips input", w.Code, w.Body.String())
			}
			user := strconv.FormatInt(regular, 10)
			for _, path := range []string{"/api/v1/ops/economy/overview", "/api/v1/ops/economy/user?newapi_user_id=" + user, "/api/v1/ops/economy/ledger?limit=2", "/api/v1/ops/economy/transfers?limit=2", "/api/v1/ops/rewards/policies", "/api/v1/ops/rewards/claims?limit=2"} {
				w := request(admin, "GET", path, "", false)
				if w.Code != 200 {
					t.Fatal("local economy/rewards", path, w.Code, w.Body.String())
				}
				if strings.Contains(w.Body.String(), `"authz_epoch"`) || strings.Contains(w.Body.String(), `"active_quota_units"`) {
					t.Fatal("legacy economy response", w.Body.String())
				}
				if w = request(regular, "GET", path, "", false); w.Code != 403 {
					t.Fatal("regular", w.Code)
				}
				if w = request(0, "GET", path, "", false); w.Code != 401 {
					t.Fatal("anonymous", w.Code)
				}
			}
			for _, path := range []string{"/api/v1/ops/economy/ledger?newapi_user_id=-1", "/api/v1/ops/economy/ledger?asset=bad", "/api/v1/ops/rewards/claims?program=bad"} {
				if w := request(admin, "GET", path, "", false); w.Code != 400 {
					t.Fatal("invalid filter", w.Code, w.Body.String())
				}
			}
			path := "/api/v1/ops/rewards/claims/00000000-0000-4000-8000-000000000001/retry"
			if w := request(admin, "POST", path, `{}`, false); w.Code != 403 {
				t.Fatal("retry CSRF", w.Code)
			}
			if w := request(admin, "POST", path, `{}`, true); w.Code != 404 {
				t.Fatal("missing claim", w.Code, w.Body.String())
			}
			if w := request(regular, "POST", path, `{}`, true); w.Code != 403 {
				t.Fatal("regular retry", w.Code)
			}
			if w := request(admin, "POST", "/api/v1/ops/economy/transfers", `{}`, true); w.Code != 405 {
				t.Fatal("historical transfers readonly", w.Code)
			}
			if gateRouteDomain("/ops/gambler-campaign") != "OPERATIONS" {
				t.Fatal("preserved campaign navigation")
			}
			t.Log("economy/rewards both routers: local admin200 regular403 anonymous401 CSRF403 invalid400 missing404; transfers readonly405; no native")
		})
		t.Run("users", func(t *testing.T) {
			masterPath := "/api/v1/ops/users/" + strconv.FormatInt(regular, 10) + "/master"
			w := request(admin, "GET", masterPath, "", false)
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"rename_required":`) {
				t.Fatal("local Master", w.Code, w.Body.String())
			}
			for _, actor := range []int64{regular, 0} {
				want := 403
				if actor == 0 {
					want = 401
				}
				if w := request(actor, "GET", masterPath, "", false); w.Code != want {
					t.Fatal("Master role", w.Code)
				}
			}
			for _, action := range []string{"master-rename-required", "master-force-rename"} {
				path := "/api/v1/ops/users/" + strconv.FormatInt(regular, 10) + "/" + action
				if w := request(admin, "POST", path, `{}`, false); w.Code != 403 {
					t.Fatal("Master CSRF", w.Code)
				}
				if w := request(regular, "POST", path, `{}`, true); w.Code != 403 {
					t.Fatal("Master write role", w.Code)
				}
				if w := request(admin, "POST", path, `{}`, true); w.Code != 400 {
					t.Fatal("Master input", w.Code, w.Body.String())
				}
			}
			if w := request(admin, "POST", "/api/v1/ops/users/"+strconv.FormatInt(regular, 10)+"/master-force-rename", `{"display_name":"Admin"}`, true); w.Code != 403 {
				t.Fatal("reserved Master name", w.Code, w.Body.String())
			}
			if w := request(admin, "GET", "/api/v1/ops/users/9223372036854775807/master", "", false); w.Code != 404 {
				t.Fatal("missing Master", w.Code)
			}
			for _, path := range []string{"/api/v1/ops/users", "/api/v1/ops/users/" + strconv.FormatInt(regular, 10), "/api/v1/ops/users/" + strconv.FormatInt(regular, 10) + "/ledger", "/api/v1/ops/users/" + strconv.FormatInt(regular, 10) + "/keys", "/api/v1/ops/logs"} {
				if w := request(admin, "GET", path, "", false); w.Code != 200 {
					t.Fatal("local users Ops", path, w.Code, w.Body.String())
				}
				if w := request(regular, "GET", path, "", false); w.Code != 403 {
					t.Fatal("regular role", w.Code)
				}
				if w := request(0, "GET", path, "", false); w.Code != 401 {
					t.Fatal("anonymous role", w.Code)
				}
			}
			path := "/api/v1/ops/users/" + strconv.FormatInt(regular, 10)
			if w := request(admin, "PUT", path, `{}`, false); w.Code != 403 {
				t.Fatal("user CSRF", w.Code)
			}
			if w := request(admin, "PUT", path, `{}`, true); w.Code != 400 {
				t.Fatal("invalid user input", w.Code, w.Body.String())
			}
			if w := request(admin, "GET", "/api/v1/ops/logs?dimension=arbitrary", "", false); w.Code != 400 {
				t.Fatal("invalid log dimension", w.Code)
			}
			if gateRouteDomain("/ops/users/2") != "OPERATIONS" || gateRouteDomain("/ops/logs") != "OPERATIONS" {
				t.Fatal("local users/logs navigation gate")
			}
			t.Log("users/logs both routers: local admin200 regular403 anonymous401 CSRF403 invalid400; no native")
		})
		t.Run("catalog-and-relay", func(t *testing.T) {
			for _, path := range []string{"/api/v1/ops/channels", "/api/v1/ops/groups", "/api/v1/ops/groups/availability"} {
				w := request(admin, "GET", path, "", false)
				if w.Code != 200 {
					t.Fatal("local relay Ops", path, w.Code, w.Body.String())
				}
				if strings.Contains(w.Body.String(), `"key":`) || strings.Contains(w.Body.String(), `"channel_info":`) {
					t.Fatal("channel secret returned")
				}
				if w = request(regular, "GET", path, "", false); w.Code != 403 {
					t.Fatal("regular role", path, w.Code)
				}
				if w = request(0, "GET", path, "", false); w.Code != 401 {
					t.Fatal("anonymous role", path, w.Code)
				}
			}
			if w := request(admin, "PUT", "/api/v1/ops/channels", `{}`, false); w.Code != 403 {
				t.Fatal("channel CSRF", w.Code)
			}
			if w := request(admin, "PUT", "/api/v1/ops/channels", `{}`, true); w.Code != 400 {
				t.Fatal("channel input", w.Code)
			}
			if w := request(admin, "GET", "/api/channel/", "", false); w.Code != 404 {
				t.Fatal("old channel proxy remains", w.Code)
			}
			if gateRouteDomain("/ops/channels/20") != "OPERATIONS" || gateRouteDomain("/ops/groups") != "OPERATIONS" {
				t.Fatal("local Ops navigation gate")
			}
			w := request(admin, "GET", "/platform/v1/ops/models", "", false)
			if w.Code != 200 {
				t.Fatal("local Ops catalog", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), `"principal"`) || strings.Contains(w.Body.String(), `"sync"`) {
				t.Fatal("legacy catalog authority or sync in response")
			}
			w = request(admin, "GET", "/platform/v1/ops/models/detail?"+url.Values{"model_id": []string{model}}.Encode(), "", false)
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"price_settings"`) {
				t.Fatal("editable local catalog price", w.Code, w.Body.String())
			}
			if w = request(admin, "GET", catalogCoversPath, "", false); w.Code != 200 {
				t.Fatal("local family covers", w.Code, w.Body.String())
			}
			for _, user := range []int64{0, regular} {
				want := 403
				if user == 0 {
					want = 401
				}
				if w = request(user, "GET", "/platform/v1/ops/models", "", false); w.Code != want {
					t.Fatal("local Ops role", user, w.Code)
				}
			}
			if w = request(admin, "PUT", "/platform/v1/ops/models", `{}`, false); w.Code != 403 {
				t.Fatal("missing CSRF accepted", w.Code)
			}
			if w = request(admin, "PUT", "/platform/v1/ops/models", `{}`, true); w.Code != 400 {
				t.Fatal("invalid save accepted", w.Code, w.Body.String())
			}
			if w = request(admin, "POST", "/platform/v1/ops/models/prepare", `{}`, true); w.Code != 404 {
				t.Fatal("legacy prepare remains", w.Code)
			}
			if webDir == "" {
				var probeID string
				if err = store.Pool().QueryRow(t.Context(), `SELECT id::text FROM relay.channels WHERE enabled AND type=1 AND models[1]='gpt-5.6-sol' ORDER BY id LIMIT 1`).Scan(&probeID); err != nil {
					t.Fatal("existing small dev channel", err)
				}
				w = request(admin, "POST", "/api/v1/ops/channels/"+probeID+"/models", `{}`, true)
				var models struct {
					Data []string `json:"data"`
				}
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &models) != nil || len(models.Data) == 0 {
					t.Fatal("upstream model discovery", w.Code, w.Body.String())
				}
				w = request(admin, "POST", "/api/v1/ops/channels/"+probeID+"/test", `{}`, true)
				var probe struct {
					Data struct {
						OK       bool   `json:"ok"`
						Status   int    `json:"status_code"`
						Duration int64  `json:"duration_ms"`
						Model    string `json:"model"`
						Message  string `json:"message"`
					} `json:"data"`
				}
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &probe) != nil || !probe.Data.OK || probe.Data.Status != 200 || probe.Data.Model != "gpt-5.6-sol" {
					t.Fatal("manual first-model probe", w.Code, w.Body.String())
				}
				t.Logf("saved channel %s: discovery=%d models; first-model %s max_tokens=1 HTTP%d %dms; no Reserve debit", probeID, len(models.Data), probe.Data.Model, probe.Data.Status, probe.Data.Duration)
			}
		})
	}
}
func TestCatalogHTTPPersonalPriceBoundaryAndWithdrawal(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires the existing dev database and Redis")
	}
	dsn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read dev DSN")
	}
	store, err := platform.Open(t.Context(), strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("open dev database")
	}
	defer store.Close()
	options, err := readSessionRedisConfig(os.Getenv("MOMIAO_SESSION_REDIS_CONFIG_FILE"))
	if err != nil {
		t.Fatal("read dev session Redis config")
	}
	client := redis.NewClient(options)
	defer client.Close()
	auth := &identity.HTTP{Store: &identity.Store{Pool: store.Pool()}, Sessions: identity.Sessions{Redis: client}, Origin: "https://dev.momiao.win"}
	var user int64
	var model, hidden string
	tx, err := store.Pool().Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err = tx.QueryRow(t.Context(), `SELECT id FROM identity.users WHERE status=1 AND deleted_at IS NULL AND group_name='default' ORDER BY id LIMIT 1 FOR UPDATE`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	session, err := auth.Sessions.Create(t.Context(), user)
	if err != nil {
		t.Fatal("create isolated dev test session")
	}
	defer func() {
		if err := auth.Sessions.Revoke(context.Background(), session); err != nil {
			t.Error("revoke dev test session", err)
		}
	}()
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = store.Pool().QueryRow(t.Context(), `SELECT model_id FROM catalog.model_catalog_publication WHERE publication_state='PUBLISHED' AND EXISTS(SELECT 1 FROM relay.channels WHERE enabled AND model_id=ANY(models) AND 'default'=ANY(groups)) ORDER BY model_id LIMIT 1`).Scan(&model); err != nil {
		t.Fatal(err)
	}
	if err = store.Pool().QueryRow(t.Context(), `SELECT model_id FROM catalog.model_catalog_publication WHERE publication_state<>'PUBLISHED' ORDER BY model_id LIMIT 1`).Scan(&hidden); err != nil {
		t.Fatal(err)
	}
	for _, webDir := range []string{"", "/unused-catalog-web"} {
		cfg := catalogHTTPConfig(store)
		cfg.PublicOrigin, cfg.WebDir = auth.Origin, webDir
		cfg.Session.Enabled = true
		cfg.localIdentity = auth
		h := newPortalHandler(cfg)
		request := func(id, extra string, authenticated bool) *httptest.ResponseRecorder {
			t.Helper()
			r := httptest.NewRequest("GET", "/platform/v1/models/personal-price?"+url.Values{"model_id": []string{id}}.Encode()+extra, nil)
			// Only the local cookie may select the user and hence the group.
			r.Header.Set("New-Api-User", "999999")
			r.Header.Set("X-Auth-Session", "forged-native-session")
			if authenticated {
				r.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: session.Token})
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			return w
		}
		w := request(model, "", true)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"source":"local_price"`) {
			t.Fatal("local personal price", w.Code, w.Body.String())
		}
		for _, secret := range []string{"group_multiplier", "native_catalog_visible", "forged-native-session", "api_key", "base_url"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("personal price leaked internal data", secret)
			}
		}
		if w = request(model, "&group=工作", true); w.Code != 400 {
			t.Fatal("group query override", w.Code)
		}
		if w = request(hidden, "", true); w.Code != 404 {
			t.Fatal("unpublished personal price", w.Code)
		}
		if w = request(model, "", false); w.Code != 401 {
			t.Fatal("native headers authorized personal price", w.Code)
		}
	}
	t.Log("personal price uses existing local Redis session and account group; both routers 200/400/404/401; native transport unused; test session revoked")
}
func TestCatalogBrowserOpaqueRoutesAndExactAccessProxy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("catalog SPA"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := catalogHTTPConfig(&catalogHTTPStore{})
	cfg.WebDir = dir
	h := newPortalHandler(cfg)
	for _, path := range []string{"/api/access?model_id=x", "/models/" + url.PathEscape("组织/x%2F'"), "/models/~Lg", "/models/~Li4", "/models/%7ELg", "/ops/models"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "catalog SPA") {
			t.Fatal(path, w.Code, w.Body)
		}
	}
	for _, tc := range []struct {
		path string
		want int
	}{{"/api/access/", http.StatusNotFound}, {"/api/user/self", http.StatusUnauthorized}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.want || strings.Contains(w.Body.String(), "catalog SPA") {
			t.Fatal(tc.path, w.Code, w.Body)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/access", nil))
	if w.Code != 405 || w.Header().Get("Allow") != "GET, HEAD" {
		t.Fatal("access method boundary", w.Code, w.Header())
	}
}
