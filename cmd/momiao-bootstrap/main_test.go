package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func sourceFixture(t *testing.T) (func(string) string, *atomic.Value, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "m4cli-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "native.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Skipf("host Unix socket support unavailable: %v", err)
	}
	var body atomic.Value
	body.Store(`{"success":true,"data":{"id":42,"username":"synthetic-user","status":1,"role":1}}`)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body.Load().(string))
	})}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	enc := base64.RawURLEncoding.EncodeToString
	token := enc([]byte(`{"alg":"HS256"}`)) + "." + enc([]byte(`{"iss":"new-api","aud":["new-api-dashboard"],"token_use":"access","sub":"42","sid":"synthetic-session","uv":1,"sv":1,"exp":1900000000,"iat":1800000000,"nbf":1800000000,"jti":"synthetic-jti"}`)) + "." + enc([]byte("synthetic-signature"))
	files := map[string]string{"MOMIAO_BOOTSTRAP_DEPLOYMENT_FILE": filepath.Join(dir, "deployment.json"), "MOMIAO_BOOTSTRAP_CREDENTIAL_FILE": filepath.Join(dir, "credential.json"), "MOMIAO_BOOTSTRAP_DSN_FILE": filepath.Join(dir, "dsn")}
	d := deployment{Environment: "STAGING", Database: "momiao_test_m4_cli", NativeSourceTree: nativeSourceTree, NativeSocket: socket, ReleaseBuild: "synthetic-build"}
	for path, value := range map[string]any{files["MOMIAO_BOOTSTRAP_DEPLOYMENT_FILE"]: d, files["MOMIAO_BOOTSTRAP_CREDENTIAL_FILE"]: credential{AccessToken: token, SessionID: "synthetic-session"}} {
		raw, _ := json.Marshal(value)
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return func(key string) string { return files[key] }, &body, dir
}
func cliArgs() []string {
	return []string{"--environment", "STAGING", "--newapi-user-id", "42", "--expected-username", "synthetic-user", "--expected-empty"}
}

func TestBootstrapCLISourceFailureDoesNotReadDatabaseSecret(t *testing.T) {
	env, body, _ := sourceFixture(t)
	for _, response := range []string{`{"success":false}`, `{"success":true,"data":{"id":43,"username":"synthetic-user","status":1}}`, `{"success":true,"data":{"id":42,"username":"synthetic-user","status":2}}`, `{"success":true,"data":{"id":42,"status":1}}`} {
		body.Store(response)
		var out bytes.Buffer
		code := run(context.Background(), cliArgs(), env, strings.NewReader(""), &out, false, "synthetic-build")
		// The database credential file deliberately does not exist. Failure must
		// happen at source verification, before even attempting to read that mount.
		if code == 0 || !strings.Contains(out.String(), "BOOTSTRAP_TARGET_VERIFY_FAILED") || strings.Contains(out.String(), "PRIVATE_INPUT") {
			t.Fatalf("source failure touched DB secret: %d %s", code, out.String())
		}
	}
}

func TestBootstrapArgumentsRequireExplicitTarget(t *testing.T) {
	base := []string{"--environment", "STAGING", "--newapi-user-id", "42", "--expected-username", "synthetic-user", "--expected-empty"}
	if _, err := parseOptions(base); err != nil {
		t.Fatal(err)
	}
	cases := [][]string{nil, base[:6], append(append([]string{}, base...), "--force"), append(append([]string{}, base...), "--environment", "PRODUCTION")}
	for _, id := range []string{"0", "-1", "01", "+42", "1e3", "9223372036854775808", ""} {
		v := append([]string{}, base...)
		v[3] = id
		cases = append(cases, v)
	}
	for _, args := range cases {
		if _, err := parseOptions(args); err == nil {
			t.Fatalf("invalid arguments accepted: %q", args)
		}
	}
}
func TestBootstrapPreflightAndOutputRedaction(t *testing.T) {
	dir := t.TempDir()
	manifest := map[string]string{"environment": "STAGING", "database": "momiao_test_m4_cli", "native_source_tree": "wrong-source", "native_socket": filepath.Join(dir, "native.sock"), "release_build": "synthetic-build"}
	raw, _ := json.Marshal(manifest)
	path := filepath.Join(dir, "deployment.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	env := func(k string) string {
		if k == "MOMIAO_BOOTSTRAP_DEPLOYMENT_FILE" {
			return path
		}
		return filepath.Join(dir, "private-token-or-password")
	}
	var out bytes.Buffer
	code := run(context.Background(), []string{"--environment", "STAGING", "--newapi-user-id", "42", "--expected-username", "synthetic-user", "--expected-empty"}, env, strings.NewReader(""), &out, false, "synthetic-build")
	if code == 0 || !strings.Contains(out.String(), "BOOTSTRAP_SOURCE_UNVERIFIED") || strings.Contains(out.String(), dir) {
		t.Fatalf("preflight/redaction: %d %s", code, out.String())
	}
}
func TestProductionConfirmationNeedsTTYAndExactTarget(t *testing.T) {
	for _, tc := range []struct {
		tty   bool
		input string
		want  bool
	}{
		{false, "PRODUCTION BOOTSTRAP_SUPER_ADMIN 42\n", false},
		{true, "PRODUCTION BOOTSTRAP_SUPER_ADMIN 43\n", false},
		{true, "PRODUCTION BOOTSTRAP_SUPER_ADMIN 42\n", true},
		{true, "PRODUCTION BOOTSTRAP_SUPER_ADMIN 42", false},
	} {
		if confirmProduction(strings.NewReader(tc.input), tc.tty, 42) != tc.want {
			t.Fatalf("confirmation contract: %+v", tc)
		}
	}
}
