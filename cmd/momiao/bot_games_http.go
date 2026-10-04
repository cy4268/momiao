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
		case "/internal/v1/bot-games/dice/prepare", "/internal/v1/bot-games/dice/play", "/internal/v1/bot-games/dice/lookup",
			"/internal/v1/bot-games/slot/prepare", "/internal/v1/bot-games/slot/play", "/internal/v1/bot-games/slot/lookup",
			"/internal/v1/bot-games/summon/prepare", "/internal/v1/bot-games/summon/play", "/internal/v1/bot-games/summon/lookup",
			"/internal/v1/bot-games/scratch/prepare", "/internal/v1/bot-games/scratch/play", "/internal/v1/bot-games/scratch/lookup",
			"/internal/v1/bot-games/blackjack/prepare", "/internal/v1/bot-games/blackjack/play", "/internal/v1/bot-games/blackjack/lookup",
			"/internal/v1/bot-games/blackjack/state", "/internal/v1/bot-games/blackjack/action", "/internal/v1/bot-games/blackjack/action-lookup":
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
		if r.URL.Path == "/internal/v1/bot-games/slot/prepare" {
			fields = []string{"request_id", "total_wager"}
		}
		if r.URL.Path == "/internal/v1/bot-games/summon/prepare" {
			fields = []string{"request_id", "base_wager", "mode"}
		}
		if r.URL.Path == "/internal/v1/bot-games/scratch/prepare" {
			fields = []string{"request_id", "wager"}
		}
		if r.URL.Path == "/internal/v1/bot-games/blackjack/prepare" {
			fields = []string{"request_id", "initial_wager"}
		}
		if r.URL.Path == "/internal/v1/bot-games/blackjack/action" || r.URL.Path == "/internal/v1/bot-games/blackjack/action-lookup" {
			fields = []string{"quote", "request_id", "action_type"}
		}
		body, e := decodeStringFieldsLimit(r.Body, 4096, fields...)
		if e != nil {
			invalid()
			return
		}
		var result any
		switch r.URL.Path {
		case "/internal/v1/bot-games/blackjack/prepare":
			result, e = service.PrepareBlackjack(ctx, subject[0], botgames.BlackjackPrepareInput{RequestID: body["request_id"], InitialWager: body["initial_wager"]})
		case "/internal/v1/bot-games/blackjack/play":
			result, e = service.PlayBlackjack(ctx, subject[0], body["quote"])
		case "/internal/v1/bot-games/blackjack/lookup":
			result, e = service.LookupBlackjack(ctx, subject[0], body["quote"])
		case "/internal/v1/bot-games/blackjack/state":
			result, e = service.StateBlackjack(ctx, subject[0], body["quote"])
		case "/internal/v1/bot-games/blackjack/action":
			result, e = service.ActBlackjack(ctx, subject[0], botgames.BlackjackActionRequest{Quote: body["quote"], RequestID: body["request_id"], ActionType: body["action_type"]})
		case "/internal/v1/bot-games/blackjack/action-lookup":
			result, e = service.LookupBlackjackAction(ctx, subject[0], botgames.BlackjackActionRequest{Quote: body["quote"], RequestID: body["request_id"], ActionType: body["action_type"]})
		case "/internal/v1/bot-games/scratch/prepare":
			result, e = service.PrepareScratch(ctx, subject[0], botgames.ScratchPrepareInput{RequestID: body["request_id"], Wager: body["wager"]})
		case "/internal/v1/bot-games/scratch/play":
			result, e = service.PlayScratch(ctx, subject[0], body["quote"])
		case "/internal/v1/bot-games/scratch/lookup":
			result, e = service.LookupScratch(ctx, subject[0], body["quote"])
		case "/internal/v1/bot-games/summon/prepare":
			result, e = service.PrepareSummon(ctx, subject[0], botgames.SummonPrepareInput{RequestID: body["request_id"], BaseWager: body["base_wager"], Mode: body["mode"]})
		case "/internal/v1/bot-games/summon/play":
			result, e = service.PlaySummon(ctx, subject[0], body["quote"])
		case "/internal/v1/bot-games/summon/lookup":
			result, e = service.LookupSummon(ctx, subject[0], body["quote"])
		case "/internal/v1/bot-games/slot/prepare":
			result, e = service.PrepareSlot(ctx, subject[0], botgames.SlotPrepareInput{RequestID: body["request_id"], TotalWager: body["total_wager"]})
		case "/internal/v1/bot-games/slot/play":
			result, e = service.PlaySlot(ctx, subject[0], body["quote"])
		case "/internal/v1/bot-games/slot/lookup":
			result, e = service.LookupSlot(ctx, subject[0], body["quote"])
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
				case "ACCOUNT_NOT_READY", "QUOTE_EXPIRED", "COMMITMENT_INVALID", "IDEMPOTENCY_CONFLICT", "INSUFFICIENT_CHIPS", "MAINTENANCE", "SCRATCH_PREVIOUS_REVEAL_INCOMPLETE", "BLACKJACK_ACTIVE_ROUND", "BLACKJACK_STALE_STATE", "BLACKJACK_ACTION_NOT_ALLOWED", "BLACKJACK_NEEDS_REVIEW":
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
