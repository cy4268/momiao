package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/cy4268/momiao/internal/platform"
)

const economyQuotaPrefix = "/internal/v1/economy/quota/"

type economyQuotaRequest struct {
	UserID      int64  `json:"user_id,string"`
	OperationID string `json:"operation_id,omitempty"`
}

// This port has no mutation interface and takes no account/database locks:
// the caller already holds the common account lock while observing Native.
func newEconomyQuotaReadHandler(observer platform.NativeQuotaObserver, peerKeys map[string]ed25519.PublicKey) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if observer == nil || r.Method != http.MethodPost || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.Fragment != "" || r.URL.User != nil || (r.URL.Path != economyQuotaPrefix+"read" && r.URL.Path != economyQuotaPrefix+"query") {
			walletError(w, 400, "ECONOMY_READ_DENIED")
			return
		}
		for _, key := range []string{"Cookie", "Authorization", "X-Auth-Session", "New-Api-User"} {
			if len(r.Header.Values(key)) != 0 {
				walletError(w, 400, "ECONOMY_READ_DENIED")
				return
			}
		}
		if r.Header.Get("Content-Type") != "application/json" {
			walletError(w, 400, "ECONOMY_READ_DENIED")
			return
		}
		var raw []byte
		var err error
		if r.Body != nil {
			raw, err = io.ReadAll(io.LimitReader(r.Body, 4097))
			_ = r.Body.Close()
		}
		if err != nil || len(raw) > 4096 || verifyServiceRequest(r, raw, peerKeys, "poker", "platform") != nil {
			walletError(w, 403, "ECONOMY_READ_DENIED")
			return
		}
		var in economyQuotaRequest
		fields := []string{"user_id"}
		if r.URL.Path == economyQuotaPrefix+"query" {
			fields = append(fields, "operation_id")
		}
		if decodePokerObject(raw, &in, fields...) != nil || in.UserID <= 0 || in.UserID > 1<<31-1 || (len(fields) == 2 && !platform.ValidOperationKey(in.OperationID)) {
			walletError(w, 400, "ECONOMY_READ_DENIED")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if len(fields) == 1 {
			out, e := observer.ReadNativeQuota(ctx, in.UserID)
			if e != nil || out.UserID != in.UserID || out.RawQuota < 0 || out.RawQuota > 1<<31-1 || out.ObservedAt.IsZero() {
				walletError(w, 503, "ECONOMY_READ_UNAVAILABLE")
				return
			}
			walletJSON(w, 200, out)
		} else {
			out, e := observer.QueryQuotaOperation(ctx, in.OperationID, in.UserID)
			if e != nil || out.UserID != in.UserID || out.ID != in.OperationID {
				walletError(w, 503, "ECONOMY_READ_UNAVAILABLE")
				return
			}
			walletJSON(w, 200, out)
		}
	})
}

type economyQuotaObserver struct {
	transport http.RoundTripper
	signing   pokerTicketKeys
}

func newEconomyQuotaObserver(transport http.RoundTripper, signing pokerTicketKeys) (platform.NativeQuotaObserver, error) {
	if transport == nil || len(signing.private) != ed25519.PrivateKeySize || signing.active == "" {
		return nil, errPokerConfig
	}
	return &economyQuotaObserver{transport, signing}, nil
}

func (o *economyQuotaObserver) call(ctx context.Context, path string, in economyQuotaRequest, out any) error {
	if in.UserID <= 0 || in.UserID > 1<<31-1 {
		return platform.ErrNativeQuotaDependency
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix"+economyQuotaPrefix+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	r.Host = "localhost"
	r.Header.Set("Content-Type", "application/json")
	if signServiceRequest(r, raw, o.signing, "poker", "platform") != nil {
		return platform.ErrNativeQuotaDependency
	}
	// RoundTrip deliberately does not follow redirects or carry a cookie jar.
	response, err := o.transport.RoundTrip(r)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil || response == nil || response.Body == nil || response.StatusCode != 200 {
		return platform.ErrNativeQuotaDependency
	}
	raw, err = io.ReadAll(io.LimitReader(response.Body, 16*1024+1))
	if err != nil || len(raw) > 16*1024 || json.Unmarshal(raw, out) != nil {
		return platform.ErrNativeQuotaDependency
	}
	return nil
}

func (o *economyQuotaObserver) ReadNativeQuota(ctx context.Context, user int64) (platform.NativeQuotaSnapshot, error) {
	var out platform.NativeQuotaSnapshot
	err := o.call(ctx, "read", economyQuotaRequest{UserID: user}, &out)
	if err != nil || out.UserID != user || out.RawQuota < 0 || out.RawQuota > 1<<31-1 || out.ObservedAt.IsZero() {
		return platform.NativeQuotaSnapshot{}, platform.ErrNativeQuotaDependency
	}
	return out, nil
}

func (o *economyQuotaObserver) QueryQuotaOperation(ctx context.Context, id string, user int64) (platform.NativeQuotaReceipt, error) {
	var out platform.NativeQuotaReceipt
	if !platform.ValidOperationKey(id) {
		return out, platform.ErrInvalidMutation
	}
	err := o.call(ctx, "query", economyQuotaRequest{UserID: user, OperationID: id}, &out)
	if err != nil || out.UserID != user || out.ID != id {
		return platform.NativeQuotaReceipt{}, platform.ErrNativeQuotaDependency
	}
	return out, nil
}
