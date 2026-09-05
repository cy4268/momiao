package main

import (
	"context"
	"errors"
	"github.com/cy4268/momiao/internal/platform"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type walletTestStore struct {
	wallets []platform.Wallet
	err     error
	ensured int
	user    int64
	entries []platform.LedgerEntry
}

func (s *walletTestStore) EnsureAccount(_ context.Context, u int64) error {
	s.user = u
	s.ensured++
	return s.err
}
func (s *walletTestStore) ReadWallets(_ context.Context, u int64) ([]platform.Wallet, error) {
	s.user = u
	return s.wallets, s.err
}
func (s *walletTestStore) Ledger(_ context.Context, u int64, a platform.Asset, n int64, l int) ([]platform.LedgerEntry, error) {
	s.user = u
	return s.entries, s.err
}

type walletTransport func(*http.Request) (*http.Response, error)

func (f walletTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func walletReq(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer secret")
	r.Header.Set("New-Api-User", "9007199254740993")
	r.Header.Set("X-Auth-Session", "session")
	r.Header.Set("Origin", "https://wallet.example")
	r.Header.Set("Content-Type", "application/json")
	return r
}
func TestWalletNativeBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, body   string
		status, want int
	}{
		{"valid", `{"success":true,"data":{"id":9007199254740993,"status":1}}`, 200, 200},
		{"revoked", `{"success":false}`, 401, 401}, {"outage", `{"success":false}`, 500, 502},
		{"malformed", `{"success":true,"data":{"id":"9007199254740993","status":1}}`, 200, 502},
		{"mismatch", `{"success":true,"data":{"id":2,"status":1}}`, 200, 401},
		{"disabled", `{"success":true,"data":{"id":9007199254740993,"status":2}}`, 200, 401},
		{"missing_status", `{"success":true,"data":{"id":9007199254740993}}`, 200, 502},
		{"false_success", `{"success":false,"message":"secret"}`, 200, 502},
		{"trailing", `{"success":true,"data":{"id":9007199254740993,"status":1}} {}`, 200, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			tr := walletTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != "http://unix/api/user/self" || r.Header.Get("Cookie") != "" || r.Header.Get("X-User-Id") != "" || r.Header.Get("Authorization") != "Bearer secret" {
					t.Errorf("wrong forwarded request: %v", r)
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})
			s := &walletTestStore{}
			h := newWalletHandler("https://wallet.example", s, tr)
			for i := 0; i < 2; i++ {
				r := walletReq("GET", "/platform/v1/wallet", "")
				r.Header.Set("Cookie", "private")
				r.Header.Set("X-User-Id", "2")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != tc.want || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "secret") {
					t.Fatalf("%d %s", w.Code, w.Body)
				}
				if tc.want == 200 && !strings.Contains(w.Body.String(), `"user_id":"9007199254740993"`) {
					t.Fatal(w.Body)
				}
			}
			if calls != 2 {
				t.Fatalf("cached auth: %d", calls)
			}
		})
	}
}
func TestWalletValidationAndData(t *testing.T) {
	tr := walletTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":9007199254740993,"status":1}}`)), Header: make(http.Header)}, nil
	})
	for _, tc := range []struct {
		method, path, body string
		want               int
	}{
		{"GET", "/platform/v1/wallet", "", 200}, {"GET", "/platform/v1/wallet?user_id=1", "", 400}, {"POST", "/platform/v1/wallet/initialize", "{}", 200}, {"POST", "/platform/v1/wallet/initialize", `{"user_id":1}`, 400}, {"POST", "/platform/v1/wallet/initialize", "null", 400}, {"POST", "/platform/v1/wallet/initialize", "{} {}", 400}, {"GET", "/platform/v1/wallet/ledger?asset=AVAILABLE_CHIPS", "", 200}, {"GET", "/platform/v1/wallet/ledger?asset=ACTIVE", "", 400}, {"GET", "/platform/v1/wallet/ledger?asset=AVAILABLE_CHIPS&asset=AVAILABLE_CHIPS", "", 400}, {"GET", "/platform/v1/wallet/ledger?asset=AVAILABLE_CHIPS&limit=51", "", 400}, {"GET", "/platform/v1/wallet/ledger?asset=AVAILABLE_CHIPS&after_seq=-1", "", 400}, {"GET", "/platform/v1/wallet/ledger?asset=AVAILABLE_CHIPS&after_seq=9223372036854775808", "", 400}, {"GET", "/platform/v1/wallet/ledger?asset=AVAILABLE_CHIPS&user_id=2", "", 400},
	} {
		t.Run(tc.path+tc.body, func(t *testing.T) {
			s := &walletTestStore{}
			w := httptest.NewRecorder()
			newWalletHandler("https://wallet.example", s, tr).ServeHTTP(w, walletReq(tc.method, tc.path, tc.body))
			if w.Code != tc.want {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if tc.want != 200 && s.ensured != 0 {
				t.Fatal("invalid request wrote")
			}
		})
	}
	for _, origin := range []string{"", "null", "https://evil.example", "https://wallet.example/"} {
		s := &walletTestStore{}
		r := walletReq("POST", "/platform/v1/wallet/initialize", "{}")
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		newWalletHandler("https://wallet.example", s, tr).ServeHTTP(w, r)
		if w.Code != 403 || s.ensured != 0 {
			t.Fatalf("origin %q %d", origin, w.Code)
		}
	}
	s := &walletTestStore{err: errors.New("password secret")}
	w := httptest.NewRecorder()
	newWalletHandler("https://wallet.example", s, tr).ServeHTTP(w, walletReq("GET", "/platform/v1/wallet", ""))
	if w.Code != 503 || strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w.Code, w.Body)
	}
}
func TestWalletDisabled(t *testing.T) {
	w := httptest.NewRecorder()
	newPortalHandler(config{}, nil).ServeHTTP(w, httptest.NewRequest("GET", "/platform/v1/wallet", nil))
	if w.Code != 503 || !strings.Contains(w.Body.String(), "WALLET_UNAVAILABLE") {
		t.Fatal(w.Code, w.Body)
	}
}

func TestWalletDTOAndPagination(t *testing.T) {
	tr := walletTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":9007199254740993,"status":1}}`)), Header: make(http.Header)}, nil
	})
	s := &walletTestStore{wallets: []platform.Wallet{{UserID: 9007199254740993, Asset: platform.ReserveAPICredit, BalanceUnits: 18600, LedgerSeq: 9007199254740993, Version: 9007199254740993}, {UserID: 9007199254740993, Asset: platform.AvailableChips}}, entries: []platform.LedgerEntry{{ID: "first", UserID: 9007199254740993, Asset: platform.ReserveAPICredit, LedgerSeq: 9007199254740993, DeltaUnits: -1, BalanceAfterUnits: 18600}, {ID: "second", LedgerSeq: 9007199254740994}}}
	h := newWalletHandler("https://wallet.example", s, tr)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, walletReq("GET", "/platform/v1/wallet", ""))
	for _, text := range []string{`"initialized":true`, `"amount":"0.0372"`, `"total_assets":null`, `"scope":"LOCAL_WALLETS_ONLY"`, `"ledger_seq":"9007199254740993"`} {
		if !strings.Contains(w.Body.String(), text) {
			t.Fatal(w.Body)
		}
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, walletReq("GET", "/platform/v1/wallet/ledger?asset=RESERVE_API_CREDIT&limit=1", ""))
	for _, text := range []string{`"has_more":true`, `"next_after_seq":"9007199254740993"`, `"delta_amount":"-0.000002"`, `"balance_after_amount":"0.0372"`} {
		if !strings.Contains(w.Body.String(), text) {
			t.Fatal(w.Body)
		}
	}
	if strings.Contains(w.Body.String(), "second") {
		t.Fatal(w.Body)
	}
	for i := 0; i < 2; i++ {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, walletReq("POST", "/platform/v1/wallet/initialize", " { } \n"))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	if s.ensured != 2 || s.user != 9007199254740993 {
		t.Fatal(s)
	}
}
func TestWalletRejectsAmbiguousHeaders(t *testing.T) {
	tr := walletTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid headers reached native")
		return nil, nil
	})
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Authorization") }, func(r *http.Request) { r.Header.Set("Authorization", "secret") }, func(r *http.Request) { r.Header.Add("Authorization", "Bearer other") }, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 8192)) }, func(r *http.Request) { r.Header.Add("New-Api-User", "1") }, func(r *http.Request) { r.Header.Set("New-Api-User", "-1") }, func(r *http.Request) { r.Header.Set("X-Auth-Session", strings.Repeat("x", 513)) }} {
		r := walletReq("GET", "/platform/v1/wallet", "")
		edit(r)
		w := httptest.NewRecorder()
		newWalletHandler("https://wallet.example", &walletTestStore{}, tr).ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
}

func TestWalletNativeTransportAndLargeResponseFailure(t *testing.T) {
	for _, tr := range []walletTransport{func(*http.Request) (*http.Response, error) { return nil, errors.New("private secret") }, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat(" ", 65537)))}, nil
	}} {
		w := httptest.NewRecorder()
		newWalletHandler("https://wallet.example", &walletTestStore{}, tr).ServeHTTP(w, walletReq("GET", "/platform/v1/wallet", ""))
		if w.Code != 502 || strings.Contains(w.Body.String(), "secret") {
			t.Fatal(w.Code, w.Body)
		}
	}
}
func TestWalletStoreIntegrityError(t *testing.T) {
	tr := walletTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":9007199254740993,"status":1}}`))}, nil
	})
	for _, path := range []string{"/platform/v1/wallet", "/platform/v1/wallet/ledger?asset=AVAILABLE_CHIPS"} {
		w := httptest.NewRecorder()
		newWalletHandler("https://wallet.example", &walletTestStore{err: platform.ErrIncompleteWallets}, tr).ServeHTTP(w, walletReq("GET", path, ""))
		if w.Code != 503 {
			t.Fatal(w.Code, w.Body)
		}
	}
}

func TestNativeForbiddenPreservesSession(t *testing.T) {
	tr := walletTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{"success":false,"message":"private"}`))}, nil
	})
	w := httptest.NewRecorder()
	newWalletHandler("https://wallet.example", &walletTestStore{}, tr).ServeHTTP(w, walletReq("GET", "/platform/v1/wallet", ""))
	if w.Code != 403 || !strings.Contains(w.Body.String(), `"code":"AUTH_FORBIDDEN"`) {
		t.Fatal(w.Code, w.Body)
	}
}

func TestUnavailableLazyWalletIs503AfterNativeValidation(t *testing.T) {
	s, err := platform.OpenLazy(context.Background(), "host=127.0.0.1 port=1 user=wallet dbname=wallet sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	tr := walletTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":9007199254740993,"status":1}}`))}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	newWalletHandler("https://wallet.example", s, tr).ServeHTTP(w, walletReq("GET", "/platform/v1/wallet", "").WithContext(ctx))
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"code":"WALLET_UNAVAILABLE"`) {
		t.Fatal(w.Code, w.Body)
	}
}
