package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
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

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
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
		cfg.Session.Enabled = true
		cfg.localIdentity = auth
		cfg.relay = store
		h := newPortalHandler(cfg, catalogRoundTrip(func(*http.Request) (*http.Response, error) {
			t.Error("Ops catalog reached native")
			return nil, errors.New("unexpected native call")
		}))
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
		t.Run("users", func(t *testing.T) {
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
		h := newPortalHandler(cfg, catalogRoundTrip(func(*http.Request) (*http.Response, error) {
			t.Error("local catalog reached native transport")
			return nil, errors.New("unexpected native call")
		}))
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
	calls := 0
	cfg := catalogHTTPConfig(&catalogHTTPStore{})
	cfg.WebDir = dir
	h := newPortalHandler(cfg, catalogRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 201, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("native"))}, nil
	}))
	for _, path := range []string{"/api/access?model_id=x", "/models/" + url.PathEscape("组织/x%2F'"), "/models/~Lg", "/models/~Li4", "/models/%7ELg", "/ops/models"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "catalog SPA") {
			t.Fatal(path, w.Code, w.Body)
		}
	}
	if calls != 0 {
		t.Fatal("access page was proxied")
	}
	for _, path := range []string{"/api/access/", "/api/user/self"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 201 {
			t.Fatal(path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/access", nil))
	if w.Code != 405 || calls != 2 {
		t.Fatal("access method boundary", w.Code, calls)
	}
}
