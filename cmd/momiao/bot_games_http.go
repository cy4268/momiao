package main

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/botgames"
)

func newBotGamesHandler(service *botgames.Service, token string) (http.Handler, error) {
	key, e := hex.DecodeString(token)
	if service == nil || e != nil || len(key) != 32 || hex.EncodeToString(key) != token || subtle.ConstantTimeCompare(key, make([]byte, 32)) == 1 {
		return nil, errBotGamesConfig
	}
	inflight := make(chan struct{}, 16)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Vary", "Authorization, X-Discord-User")
		invalid := func() { writeJSONError(w, 400, "INVALID_REQUEST") }
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.Opaque != "" {
			invalid()
			return
		}
		switch r.URL.Path {
		case "/internal/v1/bot-games/dice/prepare", "/internal/v1/bot-games/dice/play", "/internal/v1/bot-games/dice/lookup":
		default:
			writeJSONError(w, 404, "NOT_FOUND")
			return
		}
		if r.Method != http.MethodPost {
			invalid()
			return
		}
		auth := r.Header.Values("Authorization")
		if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+token)) != 1 {
			writeJSONError(w, 401, "UNAUTHORIZED")
			return
		}
		subject := r.Header.Values("X-Discord-User")
		if len(subject) != 1 || !botGamesSubject(subject[0]) {
			invalid()
			return
		}
		for _, header := range []string{"Cookie", "Proxy-Authorization", "New-Api-User", "X-Auth-Session", "X-Native-User", "X-User-ID", "X-Forwarded-User", "X-Authenticated-User", "Idempotency-Key", "X-Fairness-Commitment"} {
			if len(r.Header.Values(header)) != 0 {
				invalid()
				return
			}
		}
		media, params, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if e != nil || media != "application/json" || len(params) != 0 || len(r.Header.Values("Content-Type")) != 1 || r.ContentLength > 4096 {
			invalid()
			return
		}
		select {
		case inflight <- struct{}{}:
			defer func() { <-inflight }()
		default:
			writeJSONError(w, 503, "UPSTREAM_UNAVAILABLE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		fields := []string{"quote"}
		if r.URL.Path == "/internal/v1/bot-games/dice/prepare" {
			fields = []string{"request_id", "wager", "choice"}
		}
		body, e := decodeStringFieldsLimit(r.Body, 4096, fields...)
		if e != nil {
			invalid()
			return
		}
		var result any
		switch r.URL.Path {
		case "/internal/v1/bot-games/dice/prepare":
			result, e = service.Prepare(ctx, subject[0], botgames.PrepareInput{RequestID: body["request_id"], Wager: body["wager"], Choice: body["choice"]})
		case "/internal/v1/bot-games/dice/play":
			result, e = service.Play(ctx, subject[0], body["quote"])
		case "/internal/v1/bot-games/dice/lookup":
			result, e = service.Lookup(ctx, subject[0], body["quote"])
		}
		if e != nil {
			status, code := 503, "UPSTREAM_UNAVAILABLE"
			var fault botgames.Fault
			if errors.As(e, &fault) {
				code = fault.Error()
				switch code {
				case "INVALID_REQUEST":
					status = 400
				case "UNAUTHORIZED":
					status = 401
				case "BINDING_CHANGED", "ACCOUNT_RESTRICTED":
					status = 403
				case "NOT_LINKED", "NOT_FOUND":
					status = 404
				case "ACCOUNT_NOT_READY", "QUOTE_EXPIRED", "COMMITMENT_INVALID", "IDEMPOTENCY_CONFLICT", "INSUFFICIENT_CHIPS", "MAINTENANCE":
					status = 409
				}
			}
			writeJSONError(w, status, code)
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	}), nil
}

func botGamesSubject(subject string) bool {
	v, e := strconv.ParseUint(subject, 10, 64)
	return e == nil && v != 0 && strconv.FormatUint(v, 10) == subject
}
