package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/roulette"
)

func rouletteBrowserRoute(path string) bool {
	if path == "/roulette/devil-roulette" || path == "/roulette/pressure-roulette" {
		return true
	}
	for _, prefix := range []string{"/roulette/rooms/", "/history/roulette/"} {
		if strings.HasPrefix(path, prefix) && roulette.ValidRoundID(strings.TrimPrefix(path, prefix)) {
			return true
		}
	}
	return false
}
func rouletteError(w http.ResponseWriter, e error) {
	status, code := 503, "ROULETTE_UNAVAILABLE"
	switch {
	case errors.Is(e, roulette.ErrInvalidInput):
		status, code = 400, e.Error()
	case errors.Is(e, roulette.ErrNotFound):
		status, code = 404, e.Error()
	case errors.Is(e, roulette.ErrVersionConflict), errors.Is(e, roulette.ErrActionInvalid), errors.Is(e, roulette.ErrAlreadySeated), errors.Is(e, roulette.ErrIdempotencyConflict):
		status, code = 409, e.Error()
	case errors.Is(e, platform.ErrInsufficientBalance):
		status, code = 409, "INSUFFICIENT_CHIPS"
	case errors.Is(e, platform.ErrBalanceOverflow):
		status, code = 400, "GAME_AMOUNT_OVERFLOW"
	}
	walletError(w, status, code)
}
func decodeRouletteJSON(w http.ResponseWriter, r *http.Request, out any) error {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if err != nil || !utf8.Valid(raw) {
		return roulette.ErrInvalidInput
	}
	unique := json.NewDecoder(bytes.NewReader(raw))
	if !nativeself.UniqueJSON(unique, 0) {
		return roulette.ErrInvalidInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return roulette.ErrInvalidInput
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return roulette.ErrInvalidInput
	}
	return nil
}
func newRouletteHandler(origin string, service *roulette.Service, csrf *games.Service, transport http.RoundTripper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.EscapedPath() != r.URL.Path || strings.Contains(r.URL.Path, "//") {
			walletError(w, 404, "NOT_FOUND")
			return
		}
		if service == nil || csrf == nil {
			rouletteError(w, roulette.ErrUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		user, status := verifyWalletUser(r, transport)
		if status != 0 {
			code := "AUTH_UNAVAILABLE"
			if status == 401 || status == 403 {
				code = "AUTH_UNAUTHORIZED"
			}
			walletError(w, status, code)
			return
		}
		session, ok := authHeader(r, "X-Auth-Session", 512, true)
		if !ok {
			walletError(w, 401, "AUTH_UNAUTHORIZED")
			return
		}
		q, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil {
			rouletteError(w, roulette.ErrInvalidInput)
			return
		}
		for key, values := range q {
			if len(values) != 1 || r.URL.Path != "/api/v1/roulette" || (key != "game" && key != "cursor") {
				rouletteError(w, roulette.ErrInvalidInput)
				return
			}
		}
		if r.Method != "GET" && r.Method != "POST" {
			w.Header().Set("Allow", "GET, POST")
			walletError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		key := ""
		if r.Method == "POST" {
			if origins := r.Header.Values("Origin"); len(origins) != 1 || origins[0] != origin {
				walletError(w, 403, "ORIGIN_REJECTED")
				return
			}
			ct, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if e != nil || ct != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
				walletError(w, 415, "INVALID_CONTENT_TYPE")
				return
			}
			token, ok := authHeader(r, "X-CSRF-Token", 64, true)
			if !ok || !csrf.ValidCSRF(user, session, token) {
				walletError(w, 403, "CSRF_REJECTED")
				return
			}
			key, ok = authHeader(r, "Idempotency-Key", 128, true)
			if !ok {
				rouletteError(w, roulette.ErrInvalidInput)
				return
			}
		}
		var data any
		path := r.URL.Path
		switch {
		case path == "/api/v1/roulette" && r.Method == "GET":
			var lobby roulette.Lobby
			lobby, e = service.List(ctx, user, roulette.LobbyQuery{Game: q.Get("game"), Cursor: q.Get("cursor")})
			data = struct {
				roulette.Lobby
				CSRF string `json:"csrf_token"`
			}{lobby, csrf.CSRFToken(user, session)}
		case path == "/api/v1/roulette/rooms" && r.Method == "POST":
			var in roulette.CreateRequest
			if e = decodeRouletteJSON(w, r, &in); e == nil {
				in.Key = key
				data, e = service.Create(ctx, user, in)
			}
		case strings.HasPrefix(path, "/api/v1/roulette/receipts/") && r.Method == "GET":
			data, e = service.FindReceipt(ctx, user, strings.TrimPrefix(path, "/api/v1/roulette/receipts/"))
		case strings.HasPrefix(path, "/api/v1/roulette/rooms/"):
			parts := strings.Split(strings.TrimPrefix(path, "/api/v1/roulette/rooms/"), "/")
			if !roulette.ValidRoundID(parts[0]) {
				e = roulette.ErrNotFound
			} else if len(parts) == 1 && r.Method == "GET" {
				data, e = service.View(ctx, user, parts[0])
			} else if len(parts) == 2 && parts[1] == "commands" && r.Method == "POST" {
				var in roulette.Command
				if e = decodeRouletteJSON(w, r, &in); e == nil {
					in.Key = key
					data, e = service.Command(ctx, user, parts[0], in)
				}
			} else {
				e = roulette.ErrNotFound
			}
		default:
			e = roulette.ErrNotFound
		}
		if e != nil {
			rouletteError(w, e)
			return
		}
		walletSuccess(w, data)
	})
}
