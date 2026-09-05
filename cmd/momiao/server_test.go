package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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
