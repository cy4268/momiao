package main

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/games"
	bj "github.com/cy4268/momiao/internal/games/blackjack"
	"github.com/cy4268/momiao/internal/platform"
)

var gameRoundID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func gameBrowserRoute(path string) bool {
	switch path {
	case "/entertainment", "/games", "/games/dice", "/games/scratch", "/games/summon", "/games/slot", "/games/blackjack", "/history":
		return true
	}
	if strings.HasPrefix(path, "/history/") && gameRoundID.MatchString(strings.TrimPrefix(path, "/history/")) {
		return true
	}
	parts := strings.Split(strings.TrimPrefix(path, "/history/"), "/")
	return strings.HasPrefix(path, "/history/") && len(parts) == 2 &&
		(parts[0] == "rounds" || parts[0] == "sessions" || parts[0] == "hands") && platform.ValidOperationKey(parts[1])
}
func decodeGameCreate(slug string, body io.Reader) (games.CreateInput, error) {
	var fields []string
	switch slug {
	case "dice":
		fields = []string{"type", "wager", "choice"}
	case "scratch":
		fields = []string{"type", "wager"}
	case "summon":
		fields = []string{"type", "base_wager", "mode"}
	case "slot":
		fields = []string{"type", "total_wager"}
	case "blackjack":
		fields = []string{"type", "initial_wager"}
	default:
		return games.CreateInput{}, games.ErrNotFound
	}
	values, err := decodeStringFields(body, fields...)
	if err != nil {
		return games.CreateInput{}, games.ErrInvalidInput
	}
	return games.CreateInput{Type: values["type"], Wager: values["wager"], Choice: values["choice"], BaseWager: values["base_wager"], Mode: values["mode"], TotalWager: values["total_wager"], InitialWager: values["initial_wager"]}, nil
}
func decodeBlackjackAction(body io.Reader) (games.BlackjackActionInput, error) {
	f, err := decodeStringFields(body, "action_id", "action_type", "hand_id", "expected_round_version")
	if err != nil {
		return games.BlackjackActionInput{}, games.ErrInvalidInput
	}
	return games.BlackjackActionInput{ActionID: f["action_id"], ActionType: f["action_type"], HandID: f["hand_id"], ExpectedVersion: f["expected_round_version"]}, nil
}
func gameError(w http.ResponseWriter, err error) {
	status, code := 503, "GAME_TEMPORARILY_UNAVAILABLE"
	switch {
	case errors.Is(err, games.ErrNotFound):
		status, code = 404, "GAME_NOT_FOUND"
	case errors.Is(err, games.ErrInvalidInput), errors.Is(err, platform.ErrInvalidMutation):
		status, code = 400, "GAME_INVALID_REQUEST"
	case errors.Is(err, games.ErrMaintenance):
		status, code = 409, "GAME_MAINTENANCE"
	case errors.Is(err, games.ErrCommitmentInvalid):
		status, code = 409, "FAIRNESS_COMMITMENT_INVALID"
	case errors.Is(err, games.ErrScratchIncomplete):
		status, code = 409, "SCRATCH_PREVIOUS_REVEAL_INCOMPLETE"
	case errors.Is(err, platform.ErrIdempotencyConflict):
		status, code = 409, "IDEMPOTENCY_CONFLICT"
	case errors.Is(err, platform.ErrInsufficientBalance):
		status, code = 409, "INSUFFICIENT_CHIPS"
	case errors.Is(err, platform.ErrBalanceOverflow):
		status, code = 400, "GAME_AMOUNT_OVERFLOW"
	case errors.Is(err, games.ErrNonceExhausted):
		status, code = 409, "FAIRNESS_NONCE_EXHAUSTED"
	case errors.Is(err, games.ErrActiveRound):
		status, code = 409, "BLACKJACK_ACTIVE_ROUND_EXISTS"
	case errors.Is(err, bj.ErrOverflow):
		status, code = 400, "GAME_AMOUNT_OVERFLOW"
	case errors.Is(err, bj.ErrNeedsReview), errors.Is(err, bj.ErrStaleVersion), errors.Is(err, bj.ErrHandNotActive), errors.Is(err, bj.ErrActionNotAllowed), errors.Is(err, bj.ErrMaxHands), errors.Is(err, bj.ErrDoubleBalance), errors.Is(err, bj.ErrSplitBalance):
		status, code = 409, err.Error()
	}
	walletError(w, status, code)
}
func newGameHandler(origin string, service *games.Service, transport http.RoundTripper, readPoker games.PokerCatalogRuntime) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != r.URL.Path || strings.Contains(r.URL.Path, "//") {
			walletError(w, 404, "NOT_FOUND")
			return
		}
		if service == nil {
			walletError(w, 503, "GAME_TEMPORARILY_UNAVAILABLE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		path := r.URL.Path
		if path == "/api/v1/games" {
			if r.Method != "GET" {
				w.Header().Set("Allow", "GET")
				walletError(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
			query, err := games.ParseCatalogQuery(r.URL.RawQuery)
			if err != nil {
				gameError(w, err)
				return
			}
			entries, err := service.Catalog(ctx, readPoker)
			if err != nil {
				gameError(w, err)
				return
			}
			walletSuccess(w, map[string]any{"items": games.FilterCatalog(entries, query)})
			return
		}
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			gameError(w, games.ErrInvalidInput)
			return
		}
		for key, values := range q {
			if len(values) != 1 || (path != "/api/v1/game-rounds" && !(strings.HasSuffix(path, "/rounds/by-key") && key == "key")) || (path == "/api/v1/game-rounds" && key != "game" && key != "before" && key != "limit") {
				gameError(w, games.ErrInvalidInput)
				return
			}
		}
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
		if r.Method == "POST" || r.Method == "PUT" {
			if origins := r.Header.Values("Origin"); len(origins) != 1 || origins[0] != origin {
				walletError(w, 403, "ORIGIN_REJECTED")
				return
			}
			ct, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || ct != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
				walletError(w, 415, "INVALID_CONTENT_TYPE")
				return
			}
			token, ok := authHeader(r, "X-CSRF-Token", 64, true)
			if !ok || !service.ValidCSRF(user, session, token) {
				walletError(w, 403, "CSRF_REJECTED")
				return
			}
		}
		var data any
		if path == "/api/v1/game-rounds" {
			if r.Method != "GET" {
				walletError(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
			query := games.HistoryQuery{Game: q.Get("game"), Before: q.Get("before")}
			if q.Has("limit") {
				query.Limit, err = strconv.Atoi(q.Get("limit"))
				if err != nil {
					gameError(w, games.ErrInvalidInput)
					return
				}
			}
			data, err = service.History(ctx, user, query)
		} else if strings.HasPrefix(path, "/api/v1/games/") {
			parts := strings.Split(strings.TrimPrefix(path, "/api/v1/games/"), "/")
			if len(parts) < 2 {
				walletError(w, 404, "NOT_FOUND")
				return
			}
			slug := parts[0]
			action := strings.Join(parts[1:], "/")
			if slug != "dice" && slug != "scratch" && slug != "summon" && slug != "slot" && slug != "blackjack" {
				gameError(w, games.ErrNotFound)
				return
			}
			switch {
			case action == "bootstrap" && r.Method == "GET":
				var b games.Bootstrap
				b, err = service.Bootstrap(ctx, user, slug)
				b.CSRFToken = service.CSRFToken(user, session)
				data = b
			case action == "rounds" && r.Method == "POST":
				key, ok := authHeader(r, "Idempotency-Key", 128, true)
				commitment, valid := authHeader(r, "X-Fairness-Commitment", 36, true)
				if !ok || !valid {
					gameError(w, games.ErrInvalidInput)
					return
				}
				var input games.CreateInput
				input, err = decodeGameCreate(slug, r.Body)
				if err == nil {
					data, err = service.Create(ctx, user, slug, key, commitment, input)
				}
			case action == "rounds/by-key" && r.Method == "GET":
				var round *games.GameRound
				round, err = service.FindByKey(ctx, user, slug, q.Get("key"))
				data = map[string]any{"round": round}
			case action == "rounds/active" && r.Method == "GET":
				var b games.Bootstrap
				b, err = service.Bootstrap(ctx, user, slug)
				data = map[string]any{"round": b.Active, "scratch_presentation_blocker": b.ScratchBlocker, "latest_round": b.Latest}
			case action == "client-seed" && r.Method == "GET":
				var b games.Bootstrap
				b, err = service.Bootstrap(ctx, user, slug)
				data = b.ClientSeed
			case action == "client-seed" && r.Method == "PUT":
				var fields map[string]string
				fields, err = decodeStringFields(r.Body, "client_seed")
				if err == nil {
					data, err = service.SetClientSeed(ctx, user, slug, fields["client_seed"])
				}
			default:
				walletError(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
		} else if strings.HasPrefix(path, "/api/v1/game-rounds/") {
			parts := strings.Split(strings.TrimPrefix(path, "/api/v1/game-rounds/"), "/")
			if !gameRoundID.MatchString(parts[0]) {
				gameError(w, games.ErrNotFound)
				return
			}
			switch {
			case len(parts) == 1 && r.Method == "GET":
				data, err = service.Read(ctx, user, parts[0])
			case len(parts) == 2 && parts[1] == "fairness" && r.Method == "GET":
				data, err = service.Verify(ctx, user, parts[0])
			case len(parts) == 2 && parts[1] == "actions" && r.Method == "POST":
				var round games.GameRound
				round, err = service.Read(ctx, user, parts[0])
				if err == nil {
					switch round.Game {
					case "blackjack":
						var input games.BlackjackActionInput
						input, err = decodeBlackjackAction(r.Body)
						if err == nil {
							data, err = service.BlackjackAction(ctx, user, parts[0], input)
						}
					case "scratch":
						var fields map[string]string
						fields, err = decodeStringFields(r.Body, "action_id", "action_type")
						if err == nil && fields["action_type"] != "SCRATCH_REVEAL_COMPLETE" {
							err = games.ErrInvalidInput
						}
						if err == nil {
							data, err = service.RevealComplete(ctx, user, parts[0], fields["action_id"])
						}
					default:
						err = games.ErrInvalidInput
					}
				}
			case len(parts) == 3 && parts[1] == "actions" && r.Method == "GET":
				var round *games.GameRound
				round, err = service.FindBlackjackAction(ctx, user, parts[0], parts[2])
				data = map[string]any{"round": round}
			default:
				walletError(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
		} else {
			walletError(w, 404, "NOT_FOUND")
			return
		}
		if err != nil {
			gameError(w, err)
			return
		}
		walletSuccess(w, data)
	})
}
