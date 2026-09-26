package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
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
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/platform"
)

type catalogHTTPStore struct {
	*platform.Store
	publicCalls, authorityCalls, writes int
	denied, withdrawn                   bool
	readOnly                            bool
	uploads                             int
	uploadReceipt                       *platform.CatalogCoverUploadResult
	uploadCommand                       platform.CatalogCoverUploadCommand
	uploadImage                         platform.CatalogCoverUploadImage
	filter                              platform.CatalogFilter
}

func (s *catalogHTTPStore) CatalogAuthority(_ context.Context, user int64) (platform.AnnouncementPrincipal, error) {
	s.authorityCalls++
	if s.denied {
		return platform.AnnouncementPrincipal{}, platform.ErrCatalogForbidden
	}
	if s.readOnly {
		return platform.AnnouncementPrincipal{UserID: user, Epoch: 2, Permissions: []string{"models.read"}}, nil
	}
	return platform.AnnouncementPrincipal{UserID: user, Role: "OPERATOR", Epoch: 2, Permissions: []string{"models.read", "models.write", "models.publish"}}, nil
}
func (s *catalogHTTPStore) PublicCatalog(_ context.Context, f platform.CatalogFilter, _ platform.CatalogPolicy) (platform.CatalogPage, error) {
	s.publicCalls++
	s.filter = f
	return platform.CatalogPage{Items: []platform.CatalogModel{}}, nil
}
func (s *catalogHTTPStore) PublicCatalogModel(_ context.Context, id string, _ platform.CatalogPolicy) (platform.CatalogModel, error) {
	s.publicCalls++
	if s.withdrawn {
		return platform.CatalogModel{}, platform.ErrCatalogNotFound
	}
	return platform.CatalogModel{ModelID: id, PublicationState: "PUBLISHED", CanUse: true}, nil
}
func (s *catalogHTTPStore) OpsCatalog(ctx context.Context, user int64, _ platform.CatalogOpsFilter, _ platform.CatalogPolicy) (platform.CatalogOpsPage, error) {
	p, e := s.CatalogAuthority(ctx, user)
	return platform.CatalogOpsPage{Principal: p, Items: []platform.CatalogModel{}}, e
}
func (s *catalogHTTPStore) PrepareCatalog(_ context.Context, _ int64, c platform.CatalogCommand, _ platform.CatalogSource, _ platform.CatalogPolicy) (platform.CatalogPreview, error) {
	s.writes++
	return platform.CatalogPreview{ID: c.OperationID}, nil
}
func (s *catalogHTTPStore) ExecuteCatalog(_ context.Context, _ int64, c platform.CatalogCommand, id string, confirmed bool, _ platform.CatalogSource, _ platform.CatalogPolicy) (platform.CatalogResult, error) {
	s.writes++
	if !confirmed || id == "" {
		return platform.CatalogResult{}, platform.ErrCatalogConfirmation
	}
	return platform.CatalogResult{OperationID: c.OperationID}, nil
}
func catalogHTTPConfig(s catalogStore) config {
	return config{catalog: s, PublicOrigin: "https://wallet.example", CatalogStaleAfter: 10 * time.Minute, CatalogDisableAfter: 30 * time.Minute, APIBaseURL: "https://api.example/v1"}
}

func (s *catalogHTTPStore) CatalogFamilyCovers(ctx context.Context, user int64) (platform.CatalogFamilyCoverPage, error) {
	p, e := s.CatalogAuthority(ctx, user)
	return platform.CatalogFamilyCoverPage{Principal: p, Items: []platform.CatalogFamilyCover{}}, e
}
func (s *catalogHTTPStore) CatalogCoverUploadReceipt(ctx context.Context, user int64, c platform.CatalogCoverUploadCommand, f platform.CatalogCoverUploadImage) (*platform.CatalogCoverUploadResult, error) {
	if _, err := s.CatalogAuthority(ctx, user); err != nil {
		return nil, err
	}
	if s.uploadReceipt == nil {
		return nil, nil
	}
	if c != s.uploadCommand || f != s.uploadImage {
		return nil, platform.ErrCatalogOperation
	}
	return s.uploadReceipt, nil
}
func (s *catalogHTTPStore) RegisterCatalogCoverUpload(ctx context.Context, user int64, c platform.CatalogCoverUploadCommand, f platform.CatalogCoverUploadImage) (platform.CatalogCoverUploadResult, error) {
	if _, err := s.CatalogAuthority(ctx, user); err != nil {
		return platform.CatalogCoverUploadResult{}, err
	}
	if c.Epoch != 2 {
		return platform.CatalogCoverUploadResult{}, platform.ErrAnnouncementStale
	}
	s.uploads++
	key, _ := platform.CatalogCoverObjectKey(c.Family, f.SHA256, f.Extension)
	result := platform.CatalogCoverUploadResult{OperationID: c.OperationID, AssetID: "00000000-0000-4000-8000-000000000002", Image: platform.CatalogCoverImage{Src: "/" + key, Alt: c.Alt, Width: f.Width, Height: f.Height}}
	s.uploadReceipt = &result
	s.uploadCommand = c
	s.uploadImage = f
	return result, nil
}
func (s *catalogHTTPStore) PrepareCatalogFamilyCover(_ context.Context, _ int64, c platform.CatalogFamilyCoverCommand) (platform.CatalogFamilyCoverPreview, error) {
	s.writes++
	return platform.CatalogFamilyCoverPreview{ID: c.OperationID}, nil
}
func (s *catalogHTTPStore) ExecuteCatalogFamilyCover(_ context.Context, _ int64, c platform.CatalogFamilyCoverCommand, id string, confirmed bool) (platform.CatalogFamilyCoverResult, error) {
	s.writes++
	if !confirmed || id == "" {
		return platform.CatalogFamilyCoverResult{}, platform.ErrCatalogConfirmation
	}
	return platform.CatalogFamilyCoverResult{OperationID: c.OperationID}, nil
}

func TestCatalogCoverUploadLifecycle(t *testing.T) {
	s := &catalogHTTPStore{}
	cfg := catalogHTTPConfig(s)
	keyFile := filepath.Join(t.TempDir(), "r2.json")
	if err := os.WriteFile(keyFile, []byte(`{"access_key_id":"test-access","secret_access_key":"test-secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	puts := 0
	fail, revoke, delayed := false, false, false
	var expected []byte
	var expectedType, expectedExt string
	client := &http.Client{Transport: catalogRoundTrip(func(r *http.Request) (*http.Response, error) {
		puts++
		if s.uploads != 0 {
			t.Fatal("object registered before PUT acknowledgement")
		}
		if delayed {
			time.Sleep(100 * time.Millisecond)
		}
		got, e := io.ReadAll(r.Body)
		if e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(expected)
		if r.Method != "PUT" || !strings.HasSuffix(r.URL.Path, "/assets/models/uploads/gpt/"+hex.EncodeToString(sum[:])+"."+expectedExt) || !bytes.Equal(got, expected) || r.Header.Get("Content-Type") != expectedType || r.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Fatal("PUT content/key/cache contract changed", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		if revoke {
			s.denied = true
		}
		status := 200
		body := ""
		if fail {
			status = 400
			body = "<Error><Code>AccessDenied</Code><Message>SECRET_R2_DETAIL</Message></Error>"
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	var err error
	cfg.catalogAssets, err = newCatalogAssetStore(catalogAssetConfig{AccountID: strings.Repeat("a", 32), Bucket: "test-assets", CredentialsFile: keyFile}, client)
	if err != nil {
		t.Fatal(err)
	}
	h := newCatalogHandler(cfg, profileTransport())
	command := platform.CatalogCoverUploadCommand{OperationID: "00000000-0000-4000-8000-000000000001", Epoch: 2, Family: "gpt", Alt: "Acceptance cover", RightsStatus: "ORIGINAL_GENERATED", RightsNote: "Generated", Reason: "Acceptance"}
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
		w := httptest.NewRecorder()
		h.ServeHTTP(w, request(v.data, v.kind, v.extra))
		if w.Code != 400 {
			t.Fatal("invalid upload accepted", w.Code, w.Body)
		}
	}
	if puts != 0 || s.uploads != 0 || s.writes != 0 {
		t.Fatal("invalid upload reached storage")
	}
	for _, v := range []struct {
		data              []byte
		mime, actual, ext string
	}{{png, "image/png", "image/png", "png"}, {webp, "application/octet-stream", "image/webp", "webp"}, {jpg.Bytes(), "", "image/jpeg", "jpg"}} {
		s.uploadReceipt = nil
		expected = v.data
		expectedType = v.actual
		expectedExt = v.ext
		s.uploads = 0
		w := httptest.NewRecorder()
		h.ServeHTTP(w, request(v.data, v.mime, ""))
		if w.Code != 200 || s.uploads != 1 || s.writes != 0 {
			t.Fatal("valid upload lifecycle failed", w.Code, w.Body)
		}
		first := w.Body.String()
		beforePuts := puts
		fail = true
		s.uploads = 0
		w = httptest.NewRecorder()
		h.ServeHTTP(w, request(v.data, v.mime, ""))
		if w.Code != 200 || first != w.Body.String() || puts != beforePuts || s.uploads != 0 {
			t.Fatal("committed receipt retry depended on R2 or changed identity", w.Body)
		}
		before := s.uploadReceipt
		w = httptest.NewRecorder()
		// Keep the operation ID but change metadata: no new public PUT is allowed.
		command.Reason = "Different command"
		metadata, _ = json.Marshal(command)
		h.ServeHTTP(w, request(v.data, v.mime, ""))
		if w.Code != 409 || puts != beforePuts || !reflect.DeepEqual(before, s.uploadReceipt) {
			t.Fatal("conflicting operation reached R2", w.Code)
		}
		command.Reason = "Acceptance"
		metadata, _ = json.Marshal(command)
		fail = false
	}
	s.uploadReceipt = nil
	expected = png
	expectedType = "image/png"
	expectedExt = "png"
	s.uploads = 0
	fail = true
	w := httptest.NewRecorder()
	h.ServeHTTP(w, request(png, "image/png", ""))
	if w.Code != 503 || s.uploads != 0 || strings.Contains(w.Body.String(), "SECRET") {
		t.Fatal("storage failure registered or leaked", w.Body)
	}
	fail = false
	revoke = true
	w = httptest.NewRecorder()
	h.ServeHTTP(w, request(png, "image/png", ""))
	if w.Code != 403 || s.uploads != 0 || s.writes != 0 {
		t.Fatal("revocation crossed publication boundary", w.Body)
	}
	s.denied = false
	revoke = false
	delayed = true
	server := httptest.NewUnstartedServer(h)
	server.Config.ReadTimeout = 20 * time.Millisecond
	server.Config.WriteTimeout = 20 * time.Millisecond
	server.Start()
	defer server.Close()
	r := request(png, "image/png", "")
	u, _ := url.Parse(server.URL + r.URL.Path)
	r.URL = u
	r.RequestURI = ""
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal("authorized upload retained short server deadline", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || s.uploads != 1 || server.Config.ReadTimeout != 20*time.Millisecond || server.Config.WriteTimeout != 20*time.Millisecond {
		t.Fatal("upload deadline changed global configuration", response.StatusCode)
	}
}

func TestCatalogHTTPPublicAndStrictQueries(t *testing.T) {
	s := &catalogHTTPStore{}
	native := catalogRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("public read reached native identity")
		return nil, nil
	})
	h := newCatalogHandler(catalogHTTPConfig(s), native)
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
	s := &catalogHTTPStore{denied: true}
	h := newCatalogHandler(catalogHTTPConfig(s), profileTransport())
	for _, tail := range []string{"", "/upload", "/prepare", "/execute"} {
		method := "POST"
		if tail == "" {
			method = "GET"
		}
		for _, mode := range []string{"anonymous", "scope", "query", "origin", "permission"} {
			r := announcementReq(method, "/platform/v1/ops/models/family-covers"+tail, `{"command":{}}`)
			s.denied = mode == "scope"
			s.readOnly = mode == "permission"
			want := 403
			if mode == "anonymous" {
				r = httptest.NewRequest(method, r.URL.String(), nil)
				want = 401
			}
			if mode == "query" {
				r.URL.RawQuery = "x=1"
				want = 400
			}
			if mode == "origin" {
				r.Header.Set("Origin", "https://evil.example")
				if method == "GET" {
					want = 200
				}
			}
			if mode == "permission" && method == "GET" {
				want = 200
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != want {
				t.Fatal("cover HTTP boundary", tail, mode, w.Code, w.Body)
			}
		}
	}
	s.denied = false
	s.readOnly = false
	for _, body := range []string{`null`, `{"command":{},"unknown":1}`, `{"command":{"family":"gpt","family":"other"}}`} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, announcementReq("POST", "/platform/v1/ops/models/family-covers/prepare", body))
		if w.Code != 400 {
			t.Fatal("cover JSON boundary", w.Code)
		}
	}
	wc := httptest.NewRecorder()
	h.ServeHTTP(wc, announcementReq("POST", "/platform/v1/ops/models/family-covers/execute", `{"command":{},"confirmed":false}`))
	if wc.Code != 400 {
		t.Fatal("cover confirmation missing", wc.Code)
	}
	s.writes = 0
	s.denied = true
	for _, path := range []string{"/platform/v1/ops/models", "/platform/v1/models/personal-price?model_id=x"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 {
			t.Fatal(path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, announcementReq("GET", "/platform/v1/ops/models", ""))
	if w.Code != 403 {
		t.Fatal("native admin gained scope", w.Code)
	}
	s.denied = false
	for _, body := range []string{`null`, `[]`, `{"command":{},"command":{}}`, `{"command":{"action":"SAVE","action":"SYNC"}}`, `{"command":{},"other":1}`, strings.Repeat(" ", 66000)} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, announcementReq("POST", "/platform/v1/ops/models/prepare", body))
		if w.Code != 400 {
			t.Fatal(body[:min(len(body), 80)], w.Code)
		}
	}
	for _, kind := range []string{"origin", "content", "query"} {
		r := announcementReq("POST", "/platform/v1/ops/models/prepare", `{"command":{}}`)
		if kind == "origin" {
			r.Header.Set("Origin", "https://evil.example")
		}
		if kind == "content" {
			r.Header.Set("Content-Type", "text/plain")
		}
		if kind == "query" {
			r.URL.RawQuery = "x=1"
		}
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code < 400 || s.writes != 0 {
			t.Fatal(kind, w.Code, s.writes)
		}
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, announcementReq("POST", "/platform/v1/ops/models/prepare", `{"command":{"operation_id":"fixture"}}`))
	if w.Code != 200 || s.writes != 1 {
		t.Fatal(w.Code, w.Body)
	}
}
func TestCatalogHTTPPersonalPriceBoundaryAndWithdrawal(t *testing.T) {
	id := "组织/a%2F'"
	s := &catalogHTTPStore{}
	nativeCalls := 0
	withdraw := false
	native := catalogRoundTrip(func(r *http.Request) (*http.Response, error) {
		nativeCalls++
		if r.URL.Path == "/api/user/self" {
			return profileTransport().RoundTrip(r)
		}
		if r.Method != "GET" || r.URL.Scheme != "http" || r.URL.Host != "unix" || r.URL.Path != "/api/momiao/catalog/prices" || r.URL.Query().Get("model_id") != id || len(r.URL.Query()) != 1 {
			t.Fatal("personal request broadened", r.URL)
		}
		for k := range r.Header {
			if k != "Authorization" && k != "New-Api-User" && k != "X-Auth-Session" && k != "Accept" {
				t.Fatal("forwarded private header", k)
			}
		}
		if r.Header.Get("X-Auth-Session") != "fixture-session" || r.Header.Get("New-Api-User") != "9007199254740993" {
			t.Fatal("unbound subject")
		}
		if withdraw {
			s.withdrawn = true
		}
		body, _ := json.Marshal(platform.NativePersonalCatalog{Success: true, Schema: platform.NativeCatalogSchema, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ModelID: id, Basis: "current_user_group_reference_not_token_selection", BillingAuthority: "native_settlement", Quotes: []platform.NativePersonalQuote{{Candidate: 1, Reason: "model_not_enabled_in_candidate"}}})
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
	h := newCatalogHandler(catalogHTTPConfig(s), native)
	for _, want := range []int{200, 404} {
		w := httptest.NewRecorder()
		r := announcementReq("GET", "/platform/v1/models/personal-price?model_id="+url.QueryEscape(id), "")
		r.Header.Set("Cookie", "do-not-forward")
		r.Header.Set("X-Private", "do-not-forward")
		h.ServeHTTP(w, r)
		if w.Code != want || strings.Contains(w.Body.String(), "group_multiplier") || strings.Contains(w.Body.String(), "do-not-forward") {
			t.Fatal(w.Code, w.Body)
		}
		withdraw = true
	}
	if nativeCalls != 4 || s.publicCalls != 4 {
		t.Fatal("missing native validation or withdrawal fence", nativeCalls, s.publicCalls)
	}
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

func TestCatalogWorkerRecoversAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := make(chan int, 4)
	done := make(chan struct{})
	n := 0
	go func() {
		defer close(done)
		runCatalogWorker(ctx, 5*time.Millisecond, func(context.Context) (platform.CatalogSyncResult, error) {
			n++
			calls <- n
			return platform.CatalogSyncResult{}, errors.New("synthetic failure")
		})
	}()
	for i := 1; i <= 2; i++ {
		select {
		case got := <-calls:
			if got != i {
				t.Fatal(got)
			}
		case <-time.After(time.Second):
			t.Fatal("worker failed to recover")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker ignored cancellation")
	}
}
