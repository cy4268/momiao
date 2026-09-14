package authbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/connectticket"
)

func TestNativeReaderRealHTTPStrictBoundary(t *testing.T) {
	ref := NativeRef{"42", strings.Repeat("a", 64), 1}
	key := strings.Repeat("b", 64)
	created, expires := time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix()
	good := fmt.Sprintf(`{"user_id":"42","session_id_hash":"%s","expected_session_version":"1","valid":true,"user_auth_version":"2","session_created_at":"%d","session_expires_at":"%d"}`, ref.SessionIDHash, created, expires)
	var mu sync.RWMutex
	body := good
	status := 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/internal/momiao/poker/session-check" || r.URL.RawQuery != "" || r.Host != "localhost" || r.Header.Get("Authorization") != "Bearer "+key || r.Header.Get("Cookie") != "" || r.Header.Get("New-Api-User") != "" || r.Header.Get("X-Auth-Session") != "" {
			t.Error("private native request boundary violated")
		}
		var got map[string]any
		if json.NewDecoder(r.Body).Decode(&got) != nil || len(got) != 3 || got["user_id"] != "42" || got["session_id_hash"] != ref.SessionIDHash || got["expected_session_version"] != "1" {
			t.Error("private native request DTO")
		}
		mu.RLock()
		defer mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	// The actual HTTP exchange uses an isolated localhost server. Only this test
	// replaces the fixed native Unix dialer; the native session rows are synthetic.
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	defer transport.CloseIdleConnections()
	reader, e := NewNativeReader(transport, key)
	if e != nil {
		t.Fatal(e)
	}
	n, e := reader.Check(context.Background(), ref)
	if e != nil || n.Ref != ref || n.UserAuthVersion != 2 || n.CreatedAt.Unix() != created || n.ExpiresAt.Unix() != expires {
		t.Fatal("native positive DTO", n, e)
	}
	bad := []struct {
		name, body string
		status     int
		want       error
	}{
		{"revoked", fmt.Sprintf(`{"user_id":"42","session_id_hash":"%s","expected_session_version":"1","valid":false}`, ref.SessionIDHash), 200, connectticket.ErrRevoked},
		{"wrong user", strings.Replace(good, `"42"`, `"43"`, 1), 200, connectticket.ErrUnavailable},
		{"wrong version", strings.Replace(good, `"expected_session_version":"1"`, `"expected_session_version":"2"`, 1), 200, connectticket.ErrUnavailable},
		{"duplicate key", strings.Replace(good, `"valid":true`, `"valid":false,"valid":true`, 1), 200, connectticket.ErrUnavailable},
		{"unknown field", strings.Replace(good, `"valid":true`, `"valid":true,"role":100`, 1), 200, connectticket.ErrUnavailable},
		{"null version", strings.Replace(good, `"user_auth_version":"2"`, `"user_auth_version":null`, 1), 200, connectticket.ErrUnavailable},
		{"noncanonical counter", strings.Replace(good, `"user_auth_version":"2"`, `"user_auth_version":"02"`, 1), 200, connectticket.ErrUnavailable},
		{"number not string", strings.Replace(good, `"user_auth_version":"2"`, `"user_auth_version":2`, 1), 200, connectticket.ErrUnavailable},
		{"invalid boolean", strings.Replace(good, `"valid":true`, `"valid":null`, 1), 200, connectticket.ErrUnavailable},
		{"oversize", good + strings.Repeat(" ", 8193), 200, connectticket.ErrUnavailable},
		{"reader auth error", `{"secret":"must-not-escape"}`, 401, connectticket.ErrUnavailable},
		{"database outage", `{"secret":"must-not-escape"}`, 503, connectticket.ErrUnavailable},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			mu.Lock()
			body, status = tc.body, tc.status
			mu.Unlock()
			if _, e = reader.Check(context.Background(), ref); e != tc.want {
				t.Fatal("strict native failure", e)
			}
		})
	}
	if strings.Contains(fmt.Sprintf("%v %+v %#v", reader, reader, reader), key) {
		t.Fatal("reader key exposed in formatting")
	}
}

func TestNativeReaderConfigAndDeadline(t *testing.T) {
	if _, e := NewNativeReader(nil, strings.Repeat("a", 64)); e != connectticket.ErrConfig {
		t.Fatal(e)
	}
	if _, e := NewNativeReader(http.DefaultTransport, "secret"); e != connectticket.ErrConfig {
		t.Fatal(e)
	}
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer server.Close()
	defer close(release)
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	defer transport.CloseIdleConnections()
	reader, _ := NewNativeReader(transport, strings.Repeat("a", 64))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, e := reader.Check(ctx, NativeRef{"42", strings.Repeat("a", 64), 1}); e != connectticket.ErrUnavailable {
		t.Fatal(e)
	}
	if time.Since(start) > time.Second {
		t.Fatal("caller deadline lost")
	}
}
