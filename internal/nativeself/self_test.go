package nativeself

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func credential() string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"HS256"}`)) + "." + enc([]byte(`{"iss":"new-api","aud":["new-api-dashboard"],"token_use":"access","sub":"42","sid":"synthetic-session","uv":1,"sv":1,"exp":1900000000,"iat":1800000000,"nbf":1800000000,"jti":"synthetic-jti"}`)) + "." + enc([]byte("synthetic-signature"))
}

func TestFixedUnixSelfTransport(t *testing.T) {
	// This is the real client Unix transport, with a synthetic source HTTP server;
	// it is not a claim that the native application's auth was run in this suite.
	dir, err := os.MkdirTemp("", "m4sock-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(dir)
	socket := filepath.Join(dir, "self.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Skipf("host Unix socket support unavailable: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/user/self" || r.Host != "localhost" {
			t.Error("unexpected native request")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"id":42,"username":"synthetic-user","status":1,"role":1}}`)
	})}
	go server.Serve(listener)
	defer server.Close()
	transport := NewTransport(socket)
	defer transport.CloseIdleConnections()
	if err = Verify(context.Background(), transport, 42, "synthetic-user", credential(), "synthetic-session"); err != nil {
		t.Fatal(err)
	}
}

func TestSelfTimeoutFailsClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	start := time.Now()
	if Verify(ctx, transport, 42, "synthetic-user", credential(), "synthetic-session") == nil {
		t.Fatal("timeout accepted")
	}
	if time.Since(start) > time.Second {
		t.Fatal("caller deadline ignored")
	}
}
func TestVerifySelfStrict(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		valid      bool
	}{
		{"valid ordinary native user", `{"success":true,"data":{"id":42,"username":"synthetic-user","status":1,"role":1}}`, 200, true},
		{"wrong identity", `{"success":true,"data":{"id":43,"username":"synthetic-user","status":1}}`, 200, false},
		{"wrong username", `{"success":true,"data":{"id":42,"username":"other","status":1}}`, 200, false},
		{"disabled", `{"success":true,"data":{"id":42,"username":"synthetic-user","status":2}}`, 200, false},
		{"missing status", `{"success":true,"data":{"id":42,"username":"synthetic-user"}}`, 200, false},
		{"duplicate identity", `{"success":true,"data":{"id":43,"id":42,"username":"synthetic-user","status":1}}`, 200, false},
		{"soft deleted or nonexistent", `{"success":false,"message":"sensitive native error"}`, 200, false},
		{"bad credential", `{}`, 401, false}, {"redirect", `{}`, 302, false},
		{"oversized", strings.Repeat(" ", 65537), 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || r.URL.String() != "http://unix/api/user/self" || r.Host != "localhost" || r.Header.Get("New-Api-User") != "42" || r.Header.Get("Authorization") != "Bearer "+credential() || r.Header.Get("Cookie") != "" {
					t.Fatal("unexpected native request")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			err := Verify(context.Background(), transport, 42, "synthetic-user", credential(), "synthetic-session")
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if err != nil && strings.Contains(err.Error(), "sensitive") {
				t.Fatal("raw error leaked")
			}
		})
	}
}
func TestVerifySelfFailsClosedBeforeTransport(t *testing.T) {
	calls := 0
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, fmt.Errorf("private credential transport error")
	})
	for _, token := range []string{"pat-opaque", "sk-relay", credential() + "\n", ""} {
		if Verify(context.Background(), transport, 42, "synthetic-user", token, "synthetic-session") == nil {
			t.Fatal("non-session accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid credential reached source")
	}
	if err := Verify(context.Background(), transport, 42, "synthetic-user", credential(), "synthetic-session"); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("transport error not redacted")
	}
}
