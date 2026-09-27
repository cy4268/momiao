package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/nativeself"
)

// Called only after the Ops maintenance.read gate and opaque-session projection.
// Native AdminAuth remains authoritative; no generic proxy or new credential.
func newNativeRuntimeProjectionHandler(transport http.RoundTripper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/ops/native-runtime" {
			walletError(w, 404, "NOT_FOUND")
			return
		}
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			walletError(w, 400, "OPS_INPUT_INVALID")
			return
		}
		if !nativeself.SessionCredential(r) {
			nativeSelfProjectionError(w, 401)
			return
		}
		if transport == nil {
			nativeSelfProjectionError(w, 503)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		upstream, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/api/status/test", nil)
		upstream.Host = "localhost"
		upstream.Header.Set("Accept", "application/json")
		for _, key := range []string{"Authorization", "New-Api-User", "X-Auth-Session"} {
			upstream.Header.Set(key, r.Header.Get(key))
		}
		response, err := transport.RoundTrip(upstream)
		if response != nil && response.Body != nil {
			defer response.Body.Close()
		}
		if err != nil || response == nil || response.Body == nil {
			nativeSelfProjectionError(w, 502)
			return
		}
		if response.StatusCode == 401 {
			// This is an upstream observation failure, not a rejected platform
			// session: returning 401 would log the maintenance operator out.
			walletError(w, 502, "NATIVE_RUNTIME_UNAVAILABLE")
			return
		}
		if response.StatusCode == 403 {
			walletError(w, 403, "NATIVE_RUNTIME_FORBIDDEN")
			return
		}
		if response.StatusCode != 200 {
			nativeSelfProjectionError(w, 502)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, 4097))
		decoder := json.NewDecoder(bytes.NewReader(raw))
		if err != nil || len(raw) > 4096 || !utf8.Valid(raw) || ctx.Err() != nil || !nativeself.UniqueJSON(decoder, 0) {
			nativeSelfProjectionError(w, 502)
			return
		}
		if _, err = decoder.Token(); err != io.EOF {
			nativeSelfProjectionError(w, 502)
			return
		}
		var envelope map[string]json.RawMessage
		var stats map[string]json.RawMessage
		var success bool
		var active int64
		if json.Unmarshal(raw, &envelope) != nil || !nativeSelfField(envelope, "success", &success) || !success || !nativeSelfField(envelope, "http_stats", &stats) || !nativeSelfField(stats, "active_connections", &active) || !nativeSelfSafeInteger(active) {
			nativeSelfProjectionError(w, 502)
			return
		}
		// A missing/error response is never an observed zero. Only these two fields
		// leave the server; Native messages, headers and tokens are discarded.
		sessionEnvelope(w, 200, map[string]any{"active_connections": active, "observed_at": time.Now().UTC().Format(time.RFC3339Nano)})
	})
}
