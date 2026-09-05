package main

import (
	"context"
	"github.com/cy4268/momiao/internal/platform"
	"net/http/httptest"
	"testing"
)

type quotaFixture struct {
	calls        int
	user, amount int64
	key          string
}

func (f *quotaFixture) CreateQuotaTransfer(_ context.Context, u int64, k string, a int64) (platform.QuotaTransfer, error) {
	f.calls++
	f.user = u
	f.amount = a
	f.key = k
	return platform.QuotaTransfer{ID: k, UserID: u, AmountUnits: a, Status: "PENDING"}, nil
}
func (f *quotaFixture) QuotaTransferByKey(context.Context, int64, string) (*platform.QuotaTransfer, error) {
	return nil, nil
}
func (f *quotaFixture) QuotaTransfers(context.Context, int64) ([]platform.QuotaTransfer, error) {
	return []platform.QuotaTransfer{}, nil
}
func (f *quotaFixture) ReadNativeQuota(_ context.Context, u int64) (platform.NativeQuotaSnapshot, error) {
	return platform.NativeQuotaSnapshot{UserID: u, RawQuota: 0, Amount: "0", Enabled: true}, nil
}
func TestQuotaTransferHTTP(t *testing.T) {
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/platform/v1/quota-transfers", `{"idempotency_key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","amount":"0.000002"}`, 202},
		{"POST", "/platform/v1/quota-transfers", `{"idempotency_key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","amount":"1","amount":"2"}`, 400},
		{"POST", "/platform/v1/quota-transfers", `{"idempotency_key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","amount":"1","user_id":"2"}`, 400},
		{"POST", "/platform/v1/quota-transfers", `{"idempotency_key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","amount":"0.000001"}`, 400},
		{"GET", "/platform/v1/quota-transfers/by-key?key=aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "", 200},
		{"GET", "/platform/v1/native-quota", "", 200},
		{"GET", "/platform/v1/native-quota?user_id=2", "", 400},
		{"DELETE", "/platform/v1/quota-transfers", "", 405},
	} {
		f := &quotaFixture{}
		h := newQuotaHandler("https://wallet.example", f, f, profileTransport())
		r := walletReq(tc.method, tc.path, tc.body)
		r.Header.Set("Origin", "https://wallet.example")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
		if tc.status == 202 && (f.calls != 1 || f.user != 9007199254740993 || f.amount != 1) {
			t.Fatal(f)
		}
	}
}
