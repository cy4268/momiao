package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestHealthHTTP(t *testing.T) {
	ts := httptest.NewServer(healthHandler())
	t.Cleanup(ts.Close)
	client := ts.Client()
	client.Timeout = 2 * time.Second
	for _, tc := range []struct {
		method, path string
		status       int
		body         string
	}{
		{"GET", "/healthz", 200, "{\"status\":\"ok\"}\n"},
		{"HEAD", "/healthz", 200, ""},
		{"GET", "/healthz?probe=1", 200, "{\"status\":\"ok\"}\n"},
		{"POST", "/healthz", 405, "method not allowed\n"},
		{"OPTIONS", "/healthz", 405, "method not allowed\n"},
		{"GET", "/", 404, "404 page not found\n"},
		{"GET", "/healthz/", 404, "404 page not found\n"},
		{"GET", "/healthz/nested", 404, "404 page not found\n"},
		{"GET", "/api/auth/login", 404, "404 page not found\n"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.status || string(body) != tc.body {
				t.Fatalf("got status=%d body=%q", resp.StatusCode, body)
			}
			if tc.status == 200 {
				if resp.Header.Get("Content-Type") != "application/json" || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
					t.Fatalf("missing health response headers: %v", resp.Header)
				}
			}
			if tc.status == 405 && resp.Header.Get("Allow") != "GET, HEAD" {
				t.Fatalf("wrong allowed methods: %q", resp.Header.Get("Allow"))
			}
		})
	}
}

func TestServerHasBoundedHTTPTimeouts(t *testing.T) {
	srv := newServer(config{})
	for name, timeout := range map[string]time.Duration{
		"read header": srv.ReadHeaderTimeout,
		"read":        srv.ReadTimeout,
		"write":       srv.WriteTimeout,
		"idle":        srv.IdleTimeout,
	} {
		if timeout <= 0 || timeout > time.Minute {
			t.Errorf("%s timeout is outside (0, 1m]: %v", name, timeout)
		}
	}
	if srv.MaxHeaderBytes <= 0 || srv.MaxHeaderBytes > 1<<20 {
		t.Errorf("header size bound missing or excessive: %d", srv.MaxHeaderBytes)
	}
	response := httptest.NewRecorder()
	srv.Handler.ServeHTTP(response, httptest.NewRequest("GET", "/healthz", nil))
	if response.Code != 200 {
		t.Fatalf("server not wired to health endpoint: %d", response.Code)
	}
}

func TestPortalServesOnlyApprovedSPAAndCompiledFiles(t *testing.T) {
	webDir := t.TempDir()
	assetsDir := filepath.Join(webDir, "assets")
	if err := os.Mkdir(assetsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"index.html":             "<!doctype html><title>portal</title>",
		"assets/app-CfXwKDEl.js": "export default 'portal'",
		".env":                   "SECRET=leak",
		"vite.config.ts":         "source config",
	} {
		if err := os.WriteFile(filepath.Join(webDir, filepath.FromSlash(name)), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(webDir, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "src", "main.tsx"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(assetsDir, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "src", "settings.json"), []byte(`{"secret":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	handler := newPortalHandler(config{WebDir: webDir, NewAPISocket: filepath.Join(t.TempDir(), "newapi.sock")}, nil)
	for _, route := range []string{"/", "/login", "/sign-in", "/dashboard", "/keys", "/logs"} {
		t.Run("shell "+route, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "portal") {
				t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
			for key, want := range map[string]string{
				"Cache-Control":          "no-store",
				"X-Content-Type-Options": "nosniff",
				"X-Frame-Options":        "DENY",
				"Referrer-Policy":        "no-referrer",
			} {
				if got := response.Header().Get(key); got != want {
					t.Errorf("%s=%q, want %q", key, got, want)
				}
			}
			if csp := response.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
				t.Errorf("unsafe or missing CSP: %q", csp)
			}
		})
	}

	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodHead, "/login", http.StatusOK},
		{http.MethodGet, "/assets/app-CfXwKDEl.js", http.StatusOK},
		{http.MethodHead, "/assets/app-CfXwKDEl.js", http.StatusOK},
		{http.MethodPost, "/login", http.StatusMethodNotAllowed},
		{http.MethodPost, "/assets/app-CfXwKDEl.js", http.StatusMethodNotAllowed},
		{http.MethodGet, "/assets/missing.css", http.StatusNotFound},
		{http.MethodGet, "/unknown", http.StatusNotFound},
		{http.MethodGet, "/assets/", http.StatusNotFound},
		{http.MethodGet, "/.env", http.StatusNotFound},
		{http.MethodGet, "/vite.config.ts", http.StatusNotFound},
		{http.MethodGet, "/src/main.tsx", http.StatusNotFound},
		{http.MethodGet, "/assets/src/settings.json", http.StatusNotFound},
		{http.MethodGet, "/%2e%2e/config.go", http.StatusNotFound},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			if response.Code != tc.status {
				t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
			if tc.status == http.StatusMethodNotAllowed && response.Header().Get("Allow") != "GET, HEAD" {
				t.Fatalf("Allow=%q", response.Header().Get("Allow"))
			}
		})
	}

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/assets/app-CfXwKDEl.js", nil))
	if got := asset.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("asset MIME=%q", got)
	}
	if got := asset.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("fingerprinted asset cache policy=%q", got)
	}
}

func TestProxyPreservesNativeContractAndRejectsSpoofedForwarding(t *testing.T) {
	var captured *http.Request
	var capturedBody string
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		captured = request.Clone(request.Context())
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		capturedBody = string(body)
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header: http.Header{
				"Content-Type":   []string{"application/json"},
				"Set-Cookie":     []string{"session=native; Secure; HttpOnly"},
				"Connection":     []string{"X-Internal-Hop"},
				"X-Internal-Hop": []string{"secret"},
			},
			Body: io.NopCloser(strings.NewReader(`{"ok":true}`)),
		}, nil
	})
	handler := newPortalHandler(config{WebDir: t.TempDir(), NewAPISocket: filepath.Join(t.TempDir(), "newapi.sock")}, transport)
	request := httptest.NewRequest(http.MethodPost, "/api/items/a%2Fb?cursor=x%2Fy", strings.NewReader(`{"name":"key"}`))
	request.Header.Set("Authorization", "Bearer token")
	request.Header.Set("New-Api-User", "42")
	request.Header.Set("Cookie", "session=native")
	request.Header.Set("Origin", "https://portal.example")
	request.Header.Set("Forwarded", "for=attacker")
	request.Header.Set("X-Forwarded-For", "203.0.113.10")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Real-IP", "203.0.113.11")
	request.Header.Set("CF-Connecting-IP", "203.0.113.12")
	request.Header.Set("True-Client-IP", "203.0.113.13")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || response.Body.String() != `{"ok":true}` {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if captured == nil {
		t.Fatal("request did not reach transport")
	}
	if captured.Method != http.MethodPost || captured.URL.EscapedPath() != "/api/items/a%2Fb" || captured.URL.RawQuery != "cursor=x%2Fy" || capturedBody != `{"name":"key"}` {
		t.Fatalf("native request changed: method=%s path=%s query=%s body=%q", captured.Method, captured.URL.EscapedPath(), captured.URL.RawQuery, capturedBody)
	}
	for key, want := range map[string]string{
		"Authorization": "Bearer token",
		"New-Api-User":  "42",
		"Cookie":        "session=native",
		"Origin":        "https://portal.example",
	} {
		if got := captured.Header.Get(key); got != want {
			t.Errorf("%s=%q, want %q", key, got, want)
		}
	}
	if captured.Header.Get("Forwarded") != "" || captured.Header.Get("X-Forwarded-For") != "" || captured.Header.Get("X-Forwarded-Proto") != "" || captured.Header.Get("X-Real-IP") != "" || captured.Header.Get("CF-Connecting-IP") != "" || captured.Header.Get("True-Client-IP") != "" {
		t.Fatalf("spoofed forwarding headers reached upstream: %v", captured.Header)
	}
	if captured.URL.Scheme != "http" || captured.URL.Host != "unix" {
		t.Fatalf("proxy target was not fixed: %s", captured.URL)
	}
	if got := response.Header().Get("Set-Cookie"); got != "session=native; Secure; HttpOnly" {
		t.Fatalf("Set-Cookie=%q", got)
	}
	if response.Header().Get("X-Internal-Hop") != "" {
		t.Fatal("hop-by-hop response header leaked")
	}
}

func TestProxyFailureIsBoundedAndUpgradeDoesNotLeak(t *testing.T) {
	called := 0
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		called++
		deadline, ok := request.Context().Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > time.Minute {
			t.Fatalf("missing bounded request deadline: %v %v", deadline, ok)
		}
		return nil, errors.New("Authorization: Bearer secret")
	})
	handler := newPortalHandler(config{WebDir: t.TempDir(), NewAPISocket: filepath.Join(t.TempDir(), "newapi.sock")}, transport)

	response := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if response.Code != http.StatusBadGateway || response.Header().Get("Content-Type") != "application/json" || response.Body.String() != "{\"error\":\"upstream unavailable\"}\n" || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("unsafe proxy error: status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
	if remaining := time.Until(response.writeDeadline); !response.deadlineSet || remaining < 20*time.Second || remaining > 30*time.Second {
		t.Fatalf("api write deadline not aligned to request bound: set=%v deadline=%v", response.deadlineSet, response.writeDeadline)
	}

	upgrade := httptest.NewRequest(http.MethodGet, "/api/live", nil)
	upgrade.Header.Set("Connection", "keep-alive, Upgrade")
	upgrade.Header.Set("Upgrade", "websocket")
	upgradeResponse := httptest.NewRecorder()
	handler.ServeHTTP(upgradeResponse, upgrade)
	if upgradeResponse.Code != http.StatusBadRequest || called != 1 {
		t.Fatalf("upgrade reached transport: status=%d calls=%d", upgradeResponse.Code, called)
	}
}

func TestV1ProxyAlignsWriteDeadlineAndKeepsOverallBound(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		deadline, ok := request.Context().Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 5*time.Minute {
			t.Fatalf("missing bounded v1 request deadline: %v %v", deadline, ok)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data: done\n\n"))}, nil
	})
	handler := newPortalHandler(config{WebDir: t.TempDir(), NewAPISocket: filepath.Join(t.TempDir(), "newapi.sock")}, transport)
	response := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`)))
	if remaining := time.Until(response.writeDeadline); response.Code != http.StatusOK || !response.deadlineSet || remaining < 4*time.Minute || remaining > 5*time.Minute {
		t.Fatalf("v1 write deadline not aligned to stream bound: status=%d set=%v deadline=%v", response.Code, response.deadlineSet, response.writeDeadline)
	}
}

func TestNativeTransportUsesRouteDeadlineForDelayedHeaders(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "newapi.sock")
	transport := newNativeTransport(socket)
	defer transport.CloseIdleConnections()
	if transport.ResponseHeaderTimeout != 0 {
		t.Fatalf("native transport imposed an independent response-header timeout: %v", transport.ResponseHeaderTimeout)
	}

	address, err := net.ResolveUnixAddr("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenUnix("unix", address)
	if err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Unix sockets are unavailable on this Windows host: %v", err)
		}
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = io.WriteString(w, "native")
	})}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close()
		<-done
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/v1/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); response.StatusCode != http.StatusOK || string(body) != "native" || elapsed < 40*time.Millisecond || elapsed >= 500*time.Millisecond {
		t.Fatalf("delayed native response ignored route context: status=%d body=%q elapsed=%v", response.StatusCode, body, elapsed)
	}
}

func TestUnixListenerLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix sockets are accepted on the Linux deployment host")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "portal.sock")
	listener, err := openListener(config{ListenSocket: path})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode=%o", info.Mode().Perm())
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("owned socket path remains after close: %v", err)
	}

	if err := os.WriteFile(path, []byte("do not delete"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openListener(config{ListenSocket: path}); err == nil {
		t.Fatal("listener replaced preexisting filesystem object")
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "do not delete" {
		t.Fatalf("preexisting object changed: body=%q error=%v", body, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	writeDeadline time.Time
	deadlineSet   bool
}

func (recorder *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	recorder.writeDeadline = deadline
	recorder.deadlineSet = true
	return nil
}

func TestServeGracefullyDrainsActiveRequest(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "finished")
	})}
	listener := localListener(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() { _ = srv.Close() })
	done := make(chan error, 1)
	go func() { done <- serve(ctx, srv, listener, time.Second) }()
	response := make(chan string, 1)
	go func() {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get("http://" + listener.Addr().String())
		if err != nil {
			response <- err.Error()
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			response <- err.Error()
			return
		}
		response <- string(body)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("request never reached server")
	}
	cancel()
	select {
	case err := <-done:
		close(release)
		t.Fatalf("shutdown returned while request was active: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case body := <-response:
		if body != "finished" {
			t.Fatalf("active request was not drained: %q", body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request did not finish")
	}
	if err := awaitServe(t, done); err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), 100*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatal("server accepted a connection after shutdown")
	}
}

func TestServeDeadlineClosesActiveConnection(t *testing.T) {
	started, closed := make(chan struct{}), make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(closed)
	})}
	listener := localListener(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() { _ = srv.Close() })
	done := make(chan error, 1)
	go func() { done <- serve(ctx, srv, listener, 20*time.Millisecond) }()
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request never reached server")
	}
	cancel()
	if err := awaitServe(t, done); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected shutdown deadline error, got %v", err)
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("active connection was not force-closed")
	}
}

func TestServeReportsListenerFailure(t *testing.T) {
	listener := localListener(t)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	err := serve(context.Background(), newServer(config{}), listener, time.Second)
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("listener failure not reported: %v", err)
	}
}

func TestServeAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	listener := localListener(t)
	go func() { done <- serve(ctx, newServer(config{}), listener, time.Second) }()
	if err := awaitServe(t, done); err != nil {
		t.Fatal(err)
	}
}

func localListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func awaitServe(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop within test deadline")
		return nil
	}
}
