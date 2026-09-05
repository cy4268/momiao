package main

import (
	"context"
	"github.com/cy4268/momiao/internal/platform"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type economyFake struct {
	economyStore
	user  int64
	units int64
	calls int
}

func (s *economyFake) Exchange(_ context.Context, u int64, k string, a platform.Asset, n int64) (platform.Transaction, error) {
	s.user = u
	s.units = n
	s.calls++
	return platform.Transaction{UserID: u}, nil
}
func (s *economyFake) ClaimDaily(_ context.Context, u int64, k string) (platform.Transaction, error) {
	s.user = u
	s.calls++
	return platform.Transaction{UserID: u}, nil
}
func TestEconomyHTTPBoundary(t *testing.T) {
	tr := walletTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":9007199254740993,"status":1}}`))}, nil
	})
	for _, tc := range []struct {
		path, body, origin string
		want               int
	}{
		{"/platform/v1/wallet/exchange", `{"idempotency_key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","from_asset":"RESERVE_API_CREDIT","amount":"0.000002"}`, "https://wallet.example", 200},
		{"/platform/v1/wallet/exchange", `{"idempotency_key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","from_asset":"RESERVE_API_CREDIT","amount":"0.000001"}`, "https://wallet.example", 400},
		{"/platform/v1/rewards/daily/claim", `{"idempotency_key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}`, "https://wrong.example", 403},
		{"/platform/v1/rewards/daily/claim?user_id=2", `{"idempotency_key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}`, "https://wallet.example", 400},
		{"/platform/v1/rewards/daily/claim", `{"idempotency_key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","amount":"1000"}`, "https://wallet.example", 400},
	} {
		s := &economyFake{}
		h := newEconomyHandler("https://wallet.example", s, tr)
		r := walletReq("POST", tc.path, tc.body)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatal(w.Code, w.Body)
		}
		if tc.want == 200 {
			if s.user != 9007199254740993 || s.units != 1 {
				t.Fatal(s)
			}
		} else if s.calls != 0 {
			t.Fatal("invalid request mutated store")
		}
	}
}

func TestEconomyRequest(t *testing.T) {
	for _, tc := range []struct {
		body            string
		exchange, valid bool
	}{
		{`{"idempotency_key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}`, false, true},
		{`{"idempotency_key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","from_asset":"RESERVE_API_CREDIT","amount":"12.000002"}`, true, true},
		{`{"idempotency_key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","amount":"500"}`, false, false},
		{`{"idempotency_key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","period_key":"2026-09-05"}`, false, false},
		{`{"idempotency_key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","user_id":"2"}`, false, false},
		{`{"idempotency_key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","idempotency_key":"bbbbbbbb-bbbb-cccc-dddd-eeeeeeeeeeee"}`, false, false},
		{`{"idempotency_key":null}`, false, false}, {`{"idempotency_key":123}`, false, false},
		{`{"idempotency_key":"short"}`, false, false}, {`{} {}`, false, false},
	} {
		_, err := decodeEconomyRequest(strings.NewReader(tc.body), tc.exchange)
		if (err == nil) != tc.valid {
			t.Errorf("body %s: %v", tc.body, err)
		}
	}
}
