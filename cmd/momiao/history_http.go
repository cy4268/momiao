package main

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/history"
	"github.com/cy4268/momiao/internal/historyaccess"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/poker"
 "github.com/cy4268/momiao/internal/roulette"
	"github.com/cy4268/momiao/internal/session"
)

// Concrete dependencies keep request-controlled identities out of the authority seam.
type historyHTTP struct {
	roulette *roulette.Service
 sessions *session.Service
	list     *history.Reader
	rounds   *games.Service
	poker    *poker.HistoryReader
	wallet   *platform.Store
}

const historyPath = "/api/v1/history"

func historyAPIRoute(path string) bool {
	return path == historyPath || strings.HasPrefix(path, historyPath+"/")
}

func (h *historyHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !historyAPIRoute(r.URL.Path) {
		walletError(w, 404, "HISTORY_NOT_FOUND")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		walletError(w, 405, "HISTORY_READ_ONLY")
		return
	}
	if h == nil || h.sessions == nil {
		walletError(w, 503, "HISTORY_UNAVAILABLE")
		return
	}
	verified, err := h.sessions.VerifyPrivateRequest(r)
	if err != nil {
		var fault session.Fault
		status, code := 503, "SESSION_UNAVAILABLE"
		if errors.As(err, &fault) {
			status, code = fault.Status, fault.Code
		}
		walletError(w, status, code)
		return
	}
	user, err := strconv.ParseInt(verified.View().UserID, 10, 64)
	if err != nil || user <= 0 {
		walletError(w, 401, "SESSION_UNAUTHORIZED")
		return
	}
	kind, id, proof, q, err := parseHistoryRequest(r.URL)
	if err != nil {
		historyHTTPError(w, err)
		return
	}
	access := historyaccess.Own(user)
	var data any
	switch kind {
	case "list":
		if h.list == nil {
			err = historyaccess.ErrUnavailable
			break
		}
		data, err = h.list.List(r.Context(), user, history.Query{
			RecordType: history.Source(q.Get("record_type")), Mode: q.Get("mode"),
			GameSlug: q.Get("game_slug"), TimeFrom: q.Get("time_from"), TimeTo: q.Get("time_to"),
			Result: q.Get("result"), Status: q.Get("status"), ID: q.Get("id"),
			Limit: historyLimit(q, "limit"), Cursor: q.Get("cursor"),
		})
	case "rounds":
		if h.rounds == nil {
			err = historyaccess.ErrUnavailable
			break
		}
		if proof {
			data, err = h.rounds.HistoryVerify(r.Context(), access, id)
		} else {
			data, err = h.rounds.HistoryDetail(r.Context(), access, id)
		}
	case "roulette":
  if h.roulette==nil {err=historyaccess.ErrUnavailable;break}
  if proof { data,err=h.roulette.HistoryVerify(r.Context(),access,id) } else {data,err=h.roulette.HistoryDetail(r.Context(),access,id,roulette.HistoryQuery{Limit:historyLimit(q,"limit"),Cursor:q.Get("cursor")})}
 case "sessions":
		if h.poker == nil {
			err = historyaccess.ErrUnavailable
			break
		}
		data, err = h.poker.SessionDetail(r.Context(), access, id, poker.HistorySessionQuery{
			FundingLimit: historyLimit(q, "funding_limit"), HandLimit: historyLimit(q, "hand_limit"),
			FundingCursor: q.Get("funding_cursor"), HandCursor: q.Get("hand_cursor"),
		})
	case "hands":
		if h.poker == nil {
			err = historyaccess.ErrUnavailable
			break
		}
		if proof {
			data, err = h.poker.HandFairness(r.Context(), access, id)
		} else {
			data, err = h.poker.HandDetail(r.Context(), access, id, poker.HistoryHandQuery{
				Limit: historyLimit(q, "limit"), Cursor: q.Get("cursor"),
			})
		}
	case "transactions":
		if h.wallet == nil {
			err = historyaccess.ErrUnavailable
			break
		}
		data, err = h.wallet.HistoryTransaction(r.Context(), access, id)
	}
	if err != nil {
		historyHTTPError(w, err)
		return
	}
	walletSuccess(w, data)
}

func historyHTTPError(w http.ResponseWriter, err error) {
	status, code := 503, "HISTORY_UNAVAILABLE"
	switch {
	case errors.Is(err, poker.ErrHistoryCursorStale),errors.Is(err,roulette.ErrHistoryCursorStale):
		status, code = 409, "HISTORY_CURSOR_STALE"
	case errors.Is(err, history.ErrQuery), errors.Is(err, historyaccess.ErrInvalid):
		status, code = 400, "HISTORY_QUERY_INVALID"
	case errors.Is(err, historyaccess.ErrNotFound):
		status, code = 404, "HISTORY_NOT_FOUND"
	}
	walletError(w, status, code)
}

// ParseQuery errors and duplicates are rejected instead of silently picking a value.
// No user_id, subject, Records scope, or parent identity is accepted from the client.
func parseHistoryRequest(u *url.URL) (kind, id string, proof bool, q url.Values, err error) {
	err = historyaccess.ErrInvalid
	if u == nil || len(u.RawQuery) > 8192 {
		return
	}
	allowed := ""
	if u.Path == historyPath {
		kind = "list"
		allowed = "record_type mode game_slug time_from time_to result status id limit cursor"
	} else if strings.HasPrefix(u.Path, historyPath+"/") {
		parts := strings.Split(strings.TrimPrefix(u.Path, historyPath+"/"), "/")
		if len(parts) < 2 || len(parts) > 3 || !platform.ValidOperationKey(parts[1]) {
			return
		}
		kind, id = parts[0], parts[1]
		proof = len(parts) == 3
		if proof && ((kind != "rounds" && kind != "hands" && kind != "roulette") || parts[2] != "verify") {
			return
		}
		switch kind {
		case "rounds", "transactions":
		case "sessions":
			allowed = "funding_limit funding_cursor hand_limit hand_cursor"
		case "hands", "roulette":
			allowed = "limit cursor"
		default:
			return
		}
		if proof {
			allowed = ""
		}
	} else {
		return
	}
	q, err = url.ParseQuery(u.RawQuery)
	if err != nil {
		err = historyaccess.ErrInvalid
		return
	}
	for name, values := range q {
		if !slices.Contains(strings.Fields(allowed), name) || len(values) != 1 || len(values[0]) > 2048 {
			err = historyaccess.ErrInvalid
			return
		}
		if name == "limit" || strings.HasSuffix(name, "_limit") {
			n, e := strconv.Atoi(values[0])
			if e != nil || n < 1 || n > 100 || strconv.Itoa(n) != values[0] {
				err = historyaccess.ErrInvalid
				return
			}
		}
	}
	err = nil
	return
}

func historyLimit(q url.Values, key string) int {
	n, _ := strconv.Atoi(q.Get(key)) // Validated by parseHistoryRequest; zero uses domain default.
	return n
}
