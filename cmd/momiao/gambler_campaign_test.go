package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The one campaign regression grows with the snapshot/reset implementation;
// reuse the existing quota fixture and real-PG harness, not a second framework.
func TestGamblerCampaignSnapshotResetLifecycle(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys := pokerTicketKeys{active: "economy-read-test", private: priv, public: map[string]ed25519.PublicKey{"economy-read-test": pub}}
	f := &quotaFixture{}
	h := newEconomyQuotaReadHandler(f, keys.public)
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Result(), nil
	})
	observer, err := newEconomyQuotaObserver(transport, keys)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := observer.ReadNativeQuota(context.Background(), 701)
	if err != nil || snapshot.UserID != 701 || snapshot.ObservedAt.IsZero() {
		t.Fatal(snapshot, err)
	}
	id := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	receipt, err := observer.QueryQuotaOperation(context.Background(), id, 701)
	if err != nil || receipt.ID != id || receipt.UserID != 701 || receipt.Result != "NOT_APPLIED" {
		t.Fatal(receipt, err)
	}
	for _, mode := range []string{"unsigned", "direction", "body", "apply", "query", "cookie", "authorization"} {
		path := "/internal/v1/economy/quota/read"
		if mode == "apply" {
			path = "/internal/v1/economy/quota/apply"
		}
		if mode == "query" {
			path += "?user_id=702"
		}
		body := []byte(`{"user_id":"701"}`)
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if mode == "direction" {
			err = signPokerRequest(r, body, keys)
		} else if mode != "unsigned" {
			err = signServiceRequest(r, body, keys, "poker", "platform")
		}
		if err != nil {
			t.Fatal(err)
		}
		if mode == "body" {
			r.Body = http.NoBody
		}
		if mode == "cookie" {
			r.Header.Set("Cookie", "session=synthetic")
		}
		if mode == "authorization" {
			r.Header.Set("Authorization", "Bearer synthetic")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code < 400 {
			t.Fatalf("%s accepted: %d", mode, w.Code)
		}
	}
	forward := httptest.NewRequest(http.MethodPost, "/internal/v1/poker/ready", http.NoBody)
	if signPokerRequest(forward, nil, keys) != nil || verifyPokerRequest(forward, nil, keys.public) != nil {
		t.Fatal("existing forward assertion changed")
	}
	if verifyServiceRequest(forward, nil, keys.public, "poker", "platform") == nil {
		t.Fatal("forward signer accepted on reverse port")
	}
	if f.calls != 0 {
		t.Fatal("read RPC performed quota mutation")
	}
}
