package main

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/cy4268/momiao/internal/botroulette"
)

func botRouletteRoute(path string) bool {
	switch path {
	case "/internal/v1/bot-roulette/lobby", "/internal/v1/bot-roulette/public",
		"/internal/v1/bot-roulette/state", "/internal/v1/bot-roulette/prepare",
		"/internal/v1/bot-roulette/commit", "/internal/v1/bot-roulette/lookup":
		return true
	default:
		return false
	}
}

func newBotRouletteHandler(service *botroulette.Service, token string) (http.Handler, error) {
	key, err := hex.DecodeString(token)
	if service == nil || err != nil || len(key) != 32 || hex.EncodeToString(key) != token || subtle.ConstantTimeCompare(key, make([]byte, 32)) == 1 {
		return nil, errBotGamesConfig
	}
	inflight := make(chan struct{}, 16)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Vary", "Authorization, X-Discord-User")
		invalid := func() { writeBotRouletteError(w, 400, "INVALID_REQUEST") }
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.Opaque != "" {
			invalid()
			return
		}
		if !botRouletteRoute(r.URL.Path) {
			writeBotRouletteError(w, 404, "NOT_FOUND")
			return
		}
		if r.Method != http.MethodPost {
			invalid()
			return
		}
		auth := r.Header.Values("Authorization")
		if len(auth) != 1 || subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+token)) != 1 {
			writeBotRouletteError(w, 401, "UNAUTHORIZED")
			return
		}
		var subject string
		subjects := r.Header.Values("X-Discord-User")
		if r.URL.Path == "/internal/v1/bot-roulette/public" {
			if len(subjects) != 0 {
				invalid()
				return
			}
		} else {
			if len(subjects) != 1 || !botGamesSubject(subjects[0]) {
				invalid()
				return
			}
			subject = subjects[0]
		}
		for _, header := range []string{"Cookie", "Proxy-Authorization", "New-Api-User", "X-Auth-Session", "X-Native-User", "X-User-ID", "X-Forwarded-User", "X-Authenticated-User", "Idempotency-Key", "X-Fairness-Commitment"} {
			if len(r.Header.Values(header)) != 0 {
				invalid()
				return
			}
		}
		media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" || len(params) != 0 || len(r.Header.Values("Content-Type")) != 1 || r.ContentLength > 4096 {
			invalid()
			return
		}
		select {
		case inflight <- struct{}{}:
			defer func() { <-inflight }()
		default:
			writeBotRouletteError(w, 503, "UPSTREAM_UNAVAILABLE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		raw, err := io.ReadAll(io.LimitReader(r.Body, 4097))
		if err != nil || len(raw) > 4096 {
			invalid()
			return
		}
		// Each typed input's UnmarshalJSON rejects duplicate/unknown keys at
		// every nesting level, null scalars and noncanonical numeric forms.
		var result any
		switch r.URL.Path {
		case "/internal/v1/bot-roulette/lobby":
			var in botroulette.LobbyInput
			if json.Unmarshal(raw, &in) != nil {
				invalid()
				return
			}
			result, err = service.Lobby(ctx, subject, in)
		case "/internal/v1/bot-roulette/public":
			var in botroulette.RoomInput
			if json.Unmarshal(raw, &in) != nil {
				invalid()
				return
			}
			result, err = service.Public(ctx, in)
		case "/internal/v1/bot-roulette/state":
			var in botroulette.StateInput
			if json.Unmarshal(raw, &in) != nil {
				invalid()
				return
			}
			result, err = service.State(ctx, subject, in)
		case "/internal/v1/bot-roulette/prepare":
			var in botroulette.PrepareInput
			if json.Unmarshal(raw, &in) != nil {
				invalid()
				return
			}
			result, err = service.Prepare(ctx, subject, in)
		case "/internal/v1/bot-roulette/commit", "/internal/v1/bot-roulette/lookup":
			var in botroulette.QuoteInput
			if json.Unmarshal(raw, &in) != nil {
				invalid()
				return
			}
			if r.URL.Path == "/internal/v1/bot-roulette/commit" {
				result, err = service.Commit(ctx, subject, in.Quote)
			} else {
				result, err = service.Lookup(ctx, subject, in.Quote)
			}
		}
		if err != nil {
			status, code := 503, "UPSTREAM_UNAVAILABLE"
			var fault botroulette.Fault
			if errors.As(err, &fault) {
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
				case "ACCOUNT_NOT_READY", "QUOTE_EXPIRED", "COMMITMENT_INVALID", "IDEMPOTENCY_CONFLICT",
					"INSUFFICIENT_CHIPS", "MAINTENANCE", "ROULETTE_VERSION_CONFLICT", "ROULETTE_ACTION_INVALID",
					"ROULETTE_ALREADY_SEATED", "ROULETTE_NEEDS_REVIEW":
					status = 409
				}
			}
			writeBotRouletteError(w, status, code)
			return
		}
		body, err := json.Marshal(result)
		if err != nil || len(body)+1 > 65536 {
			writeBotRouletteError(w, 503, "UPSTREAM_UNAVAILABLE")
			return
		}
		_, _ = w.Write(append(body, '\n'))
	}), nil
}

func writeBotRouletteError(w http.ResponseWriter, status int, code string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		SchemaVersion string `json:"schema_version"`
		Error         string `json:"error"`
	}{SchemaVersion: "1", Error: code})
}
