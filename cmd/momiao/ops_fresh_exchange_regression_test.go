package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"
)

// Opt-in against the explicitly owned local V04 BFF. The stage runner supplies
// two PREPARED fresh operations for one actor/epoch and a READY challenge/proof
// for ExpectedOperationID. This test never prepares or executes business work.
func TestOpsFreshExchangeMismatchPreservesSessionAndProof(t *testing.T) {
	if os.Getenv("V04_FRESH_REGRESSION_STDIN") != "owned-local-fixture" {
		t.Skip("set V04_FRESH_REGRESSION_STDIN=owned-local-fixture and provide restricted JSON on stdin")
	}
	var fixture struct {
		BaseURL             string `json:"base_url"`
		Origin              string `json:"origin"`
		RootCAFile          string `json:"root_ca_file"`
		Cookie              string `json:"cookie"`
		CSRF                string `json:"csrf"`
		ExpectedOperationID string `json:"expected_operation_id"`
		WrongOperationID    string `json:"wrong_operation_id"`
		AuthzEpoch          string `json:"authz_epoch"`
		ChallengeID         string `json:"challenge_id"`
		Proof               string `json:"proof"`
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 16385))
	if err != nil || len(raw) > 16384 || json.Unmarshal(raw, &fixture) != nil {
		t.Fatal("restricted fixture input unavailable or malformed")
	}
	uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	opaque := regexp.MustCompile(`^[A-Za-z0-9_-]{16,512}$`)
	if fixture.BaseURL != "https://localhost:18444" || fixture.Origin != fixture.BaseURL ||
		fixture.Cookie == "" || fixture.CSRF == "" || fixture.ExpectedOperationID == fixture.WrongOperationID ||
		!uuid.MatchString(fixture.ExpectedOperationID) || !uuid.MatchString(fixture.WrongOperationID) ||
		!opaque.MatchString(fixture.ChallengeID) || !opaque.MatchString(fixture.Proof) {
		t.Fatal("fixture must name an explicit loopback BFF and distinct prepared operations")
	}
	ca, err := os.ReadFile(fixture.RootCAFile)
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(ca) {
		t.Fatal("owned local BFF public CA unavailable or malformed")
	}
	transport := &http.Transport{Proxy: nil,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "localhost"},
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", "127.0.0.1:18444")
		},
	}
	client := &http.Client{Timeout: 20 * time.Second, Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	t.Cleanup(client.CloseIdleConnections)
	type reply struct {
		Success bool `json:"success"`
		Data    struct {
			AuthenticationState string `json:"authentication_state"`
			PlatformSession     string `json:"platform_session"`
			NativeSession       string `json:"native_session"`
			CSRF                string `json:"csrf_token"`
			Session             struct {
				CSRF         string     `json:"csrf_token"`
				Fresh        *time.Time `json:"fresh_auth_at"`
				Confirmation *struct {
					OperationID string `json:"operation_id"`
				} `json:"confirmation"`
			} `json:"session"`
		} `json:"data"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	request := func(method, endpoint string, body any) (*http.Response, reply) {
		t.Helper()
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal("request encoding failed")
		}
		var input io.Reader
		if body != nil {
			input = bytes.NewReader(encoded)
		}
		r, err := http.NewRequest(method, fixture.BaseURL+endpoint, input)
		if err != nil {
			t.Fatal("request construction failed")
		}
		r.Header.Set("Cookie", fixture.Cookie)
		r.Header.Set("Origin", fixture.Origin)
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set("X-CSRF-Token", fixture.CSRF)
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal("owned BFF request failed")
		}
		defer response.Body.Close()
		var result reply
		if json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&result) != nil {
			t.Fatalf("BFF response was not JSON: HTTP %d", response.StatusCode)
		}
		return response, result
	}
	assertSession := func() {
		t.Helper()
		response, result := request(http.MethodGet, "/api/v1/session/bootstrap", nil)
		if response.StatusCode != http.StatusOK || !result.Success || result.Data.AuthenticationState != "AUTHENTICATED" ||
			result.Data.CSRF != fixture.CSRF || result.Data.Session.Fresh != nil || len(response.Header.Values("Set-Cookie")) != 0 {
			t.Fatalf("original non-Fresh session was not preserved: HTTP %d", response.StatusCode)
		}
	}
	assertSession()
	body := map[string]string{"operation_id": fixture.WrongOperationID, "authz_epoch": fixture.AuthzEpoch,
		"challenge_id": fixture.ChallengeID, "proof": fixture.Proof}
	response, result := request(http.MethodPost, "/api/v1/ops/fresh/exchange", body)
	if response.StatusCode != http.StatusConflict || result.Success || result.Error.Code != "SESSION_CONFLICT" {
		t.Errorf("operation mismatch must be rejected with SESSION_CONFLICT before proof consumption: got HTTP %d", response.StatusCode)
	}
	if len(response.Header.Values("Set-Cookie")) != 0 {
		t.Error("operation mismatch changed the browser session cookie")
	}
	assertSession()
	if t.Failed() {
		t.FailNow() // Preserve the correct proof for runner recovery after an unexpected rejection.
	}
	body["operation_id"] = fixture.ExpectedOperationID
	response, result = request(http.MethodPost, "/api/v1/ops/fresh/exchange", body)
	if response.StatusCode != http.StatusOK || !result.Success || result.Data.Session.Fresh == nil ||
		result.Data.Session.CSRF == "" || result.Data.Session.CSRF == fixture.CSRF ||
		result.Data.Session.Confirmation == nil || result.Data.Session.Confirmation.OperationID != fixture.ExpectedOperationID {
		t.Fatalf("original proof could not complete its correct operation: HTTP %d", response.StatusCode)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || cookies[0].Value == "" || cookies[0].Name+"="+cookies[0].Value == fixture.Cookie ||
		!cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatal("correct completion did not rotate the secure session cookie")
	}
	fixture.Cookie = cookies[0].Name + "=" + cookies[0].Value
	fixture.CSRF = result.Data.Session.CSRF
	response, result = request(http.MethodPost, "/api/v1/auth/logout", struct{}{})
	if response.StatusCode != http.StatusOK || !result.Success || result.Data.NativeSession != "REVOKED" || result.Data.PlatformSession != "REVOKED" {
		t.Fatalf("rotated session logout was not confirmed: HTTP %d", response.StatusCode)
	}
	response, result = request(http.MethodGet, "/api/v1/ops/bootstrap", nil)
	if response.StatusCode != http.StatusUnauthorized || result.Success || result.Error.Code != "SESSION_UNAUTHORIZED" {
		t.Fatalf("revoked rotated session retained private access: HTTP %d", response.StatusCode)
	}
	t.Log("mismatch preserved original session and proof; correct completion rotated SID/CSRF and bound expected operation")
	t.Log("rotated Native and platform sessions revoked; same-cookie private access rejected")
}
