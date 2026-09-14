package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/poker"
)

const pokerOpsRPCPrefix = pokerInternalPrefix + "ops/"
const pokerOpsRPCResponseLimit = 512 << 10

var errPokerOpsNotFound = errors.New("poker ops operation not found")

// PokerOpsPort is shared by embedded Poker and the signed private-process
// client. The browser-facing handler never receives a database handle.
type PokerOpsPort interface {
	ReadOpsOverview(context.Context) (poker.OpsOverview, error)
	ListOpsTables(context.Context, poker.OpsTableQuery) (poker.OpsTablePage, error)
	ReadOpsTable(context.Context, string) (poker.OpsTableDetail, error)
	ListOpsSessions(context.Context, poker.OpsSessionQuery) (poker.OpsSessionPage, error)
	ReadOpsSession(context.Context, string) (poker.OpsSession, error)
	ReadOpsHand(context.Context, string) (poker.OpsHand, error)
	ReadOpsOperation(context.Context, string) (poker.OpsReceipt, error)
	ExecuteOpsCommand(context.Context, poker.OpsCommand) (poker.OpsReceipt, error)
}

type pokerOpsRPC struct{ service *poker.Service }

// newPokerOpsRPCHandler owns only /internal/v1/poker/ops/*. The enclosing
// Poker-process handler verifies the service assertion before dispatching here.
func newPokerOpsRPCHandler(service *poker.Service) http.Handler {
	return &pokerOpsRPC{service: service}
}

func (h *pokerOpsRPC) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r == nil || r.URL == nil || !strings.HasPrefix(r.URL.Path, pokerOpsRPCPrefix) ||
		r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.Fragment != "" {
		walletError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if h == nil || h.service == nil {
		walletError(w, http.StatusServiceUnavailable, "POKER_SERVICE_UNAVAILABLE")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	path := strings.TrimPrefix(r.URL.Path, pokerOpsRPCPrefix)
	switch {
	case path == "overview":
		if !pokerOpsRPCRead(w, r) {
			return
		}
		result, err := h.service.ReadOpsOverview(r.Context())
		pokerOpsRPCResult(w, result, err, false)
	case path == "tables":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var query poker.OpsTableQuery
		if !decodeOpsBody(r, &query) {
			walletError(w, http.StatusBadRequest, "POKER_INVALID_COMMAND")
			return
		}
		result, err := h.service.ListOpsTables(r.Context(), query)
		pokerOpsRPCResult(w, result, err, false)
	case strings.HasPrefix(path, "tables/"):
		if !pokerOpsRPCRead(w, r) {
			return
		}
		id := strings.TrimPrefix(path, "tables/")
		if id == "" || strings.ContainsRune(id, '/') {
			walletError(w, http.StatusNotFound, "NOT_FOUND")
			return
		}
		result, err := h.service.ReadOpsTable(r.Context(), id)
		pokerOpsRPCResult(w, result, err, true)
	case path == "sessions":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var query poker.OpsSessionQuery
		if !decodeOpsBody(r, &query) {
			walletError(w, http.StatusBadRequest, "POKER_INVALID_COMMAND")
			return
		}
		result, err := h.service.ListOpsSessions(r.Context(), query)
		pokerOpsRPCResult(w, result, err, false)
	case strings.HasPrefix(path, "sessions/"):
		if !pokerOpsRPCRead(w, r) {
			return
		}
		id := strings.TrimPrefix(path, "sessions/")
		if id == "" || strings.ContainsRune(id, '/') {
			walletError(w, http.StatusNotFound, "NOT_FOUND")
			return
		}
		result, err := h.service.ReadOpsSession(r.Context(), id)
		pokerOpsRPCResult(w, result, err, true)
	case strings.HasPrefix(path, "hands/"):
		if !pokerOpsRPCRead(w, r) {
			return
		}
		id := strings.TrimPrefix(path, "hands/")
		if id == "" || strings.ContainsRune(id, '/') {
			walletError(w, http.StatusNotFound, "NOT_FOUND")
			return
		}
		result, err := h.service.ReadOpsHand(r.Context(), id)
		pokerOpsRPCResult(w, result, err, true)
	case path == "operations":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var command poker.OpsCommand
		if !decodeOpsBody(r, &command) {
			walletError(w, http.StatusBadRequest, "POKER_INVALID_COMMAND")
			return
		}
		result, err := h.service.ExecuteOpsCommand(r.Context(), command)
		pokerOpsRPCResult(w, result, err, false)
	case strings.HasPrefix(path, "operations/"):
		if !pokerOpsRPCRead(w, r) {
			return
		}
		id := strings.TrimPrefix(path, "operations/")
		if id == "" || strings.ContainsRune(id, '/') {
			walletError(w, http.StatusNotFound, "NOT_FOUND")
			return
		}
		result, err := h.service.ReadOpsOperation(r.Context(), id)
		pokerOpsRPCResult(w, result, err, true)
	default:
		walletError(w, http.StatusNotFound, "NOT_FOUND")
	}
}

func pokerOpsRPCRead(w http.ResponseWriter, r *http.Request) bool {
	if !requireMethod(w, r, http.MethodGet) {
		return false
	}
	if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		walletError(w, http.StatusBadRequest, "POKER_INVALID_COMMAND")
		return false
	}
	return true
}

func pokerOpsRPCResult(w http.ResponseWriter, result any, err error, deniedMeansMissing bool) {
	if err == nil {
		walletJSON(w, http.StatusOK, result)
		return
	}
	if deniedMeansMissing && errors.Is(err, poker.ErrDenied) {
		walletError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	fault := pokerDomainError(err)
	status, code := http.StatusServiceUnavailable, "POKER_SERVICE_UNAVAILABLE"
	if value, ok := fault.(interface {
		Error() string
	}); ok {
		code = value.Error()
	}
	if typed, ok := fault.(interface{ HTTPStatus() int }); ok {
		status = typed.HTTPStatus()
	} else {
		switch code {
		case "POKER_INVALID_COMMAND":
			status = http.StatusBadRequest
		case "POKER_COMMAND_DENIED", "TABLE_ACCESS_REQUIRED", "TABLE_PASSWORD_INVALID", "POKER_CHAT_MUTED", "POKER_CONTROL_NOT_OWNED":
			status = http.StatusForbidden
		case "RATE_LIMITED":
			status = http.StatusTooManyRequests
		case "POKER_CHAT_DISABLED", "POKER_STALE_VERSION", "POKER_REQUEST_CONFLICT", "POKER_HAND_NEEDS_REVIEW", "POKER_ACTION_ILLEGAL", "POKER_ACTION_DEADLINE", "STALE_RUNTIME_EPOCH":
			status = http.StatusConflict
		}
	}
	walletError(w, status, code)
}

func (p *pokerRemote) callOps(ctx context.Context, method, path string, input, output any) error {
	if p == nil || p.transport == nil || !strings.HasPrefix(path, "ops/") {
		return errPokerStartup
	}
	var raw []byte
	if input != nil {
		var err error
		raw, err = json.Marshal(input)
		if err != nil || len(raw) == 0 || len(raw) > 64<<10 {
			return poker.ErrInvalid
		}
	}
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(bounded, method, "http://unix"+pokerInternalPrefix+path, bytes.NewReader(raw))
	if err != nil {
		return errPokerStartup
	}
	r.Host = "localhost"
	r.Header.Set("Accept", "application/json")
	if input != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if signPokerRequest(r, raw, p.signing) != nil {
		return errPokerStartup
	}
	response, err := p.transport.RoundTrip(r)
	if err != nil || response == nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return errPokerStartup
	}
	if response.Body == nil {
		return errPokerStartup
	}
	defer response.Body.Close()
	raw, err = io.ReadAll(io.LimitReader(response.Body, pokerOpsRPCResponseLimit+1))
	if err != nil || len(raw) == 0 || len(raw) > pokerOpsRPCResponseLimit {
		return errPokerStartup
	}
	if response.StatusCode == http.StatusOK {
		if output == nil || json.Unmarshal(raw, output) == nil {
			return nil
		}
		return errPokerStartup
	}
	var fault struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	if json.Unmarshal(raw, &fault) != nil || fault.Success || fault.Code == "" {
		return errPokerStartup
	}
	switch {
	case response.StatusCode == http.StatusNotFound && fault.Code == "NOT_FOUND":
		return errPokerOpsNotFound
	case response.StatusCode == http.StatusBadRequest && fault.Code == "POKER_INVALID_COMMAND":
		return poker.ErrInvalid
	case response.StatusCode == http.StatusForbidden && fault.Code == "POKER_COMMAND_DENIED":
		return poker.ErrDenied
	case response.StatusCode == http.StatusConflict && fault.Code == "POKER_CHAT_DISABLED":
		return poker.ErrChatDisabled
	case response.StatusCode == http.StatusConflict && fault.Code == "POKER_STALE_VERSION":
		return poker.ErrStaleVersion
	case response.StatusCode == http.StatusConflict && fault.Code == "POKER_REQUEST_CONFLICT":
		return poker.ErrConflict
	case response.StatusCode == http.StatusConflict && fault.Code == "POKER_HAND_NEEDS_REVIEW":
		return poker.ErrNeedsReview
	case response.StatusCode == http.StatusServiceUnavailable && fault.Code == "POKER_TABLE_BUSY":
		return poker.ErrBusy
	case response.StatusCode == http.StatusServiceUnavailable && fault.Code == "POKER_SERVICE_CLOSED":
		return poker.ErrClosed
	default:
		return errPokerStartup
	}
}

func (p *pokerRemote) ReadOpsOverview(ctx context.Context) (poker.OpsOverview, error) {
	var result poker.OpsOverview
	err := p.callOps(ctx, http.MethodGet, "ops/overview", nil, &result)
	return result, err
}

func (p *pokerRemote) ListOpsTables(ctx context.Context, query poker.OpsTableQuery) (poker.OpsTablePage, error) {
	var result poker.OpsTablePage
	err := p.callOps(ctx, http.MethodPost, "ops/tables", query, &result)
	return result, err
}

func (p *pokerRemote) ReadOpsTable(ctx context.Context, id string) (poker.OpsTableDetail, error) {
	var result poker.OpsTableDetail
	if !opsUUIDPatternHTTP(id) {
		return result, poker.ErrInvalid
	}
	err := p.callOps(ctx, http.MethodGet, "ops/tables/"+id, nil, &result)
	return result, err
}

func (p *pokerRemote) ListOpsSessions(ctx context.Context, query poker.OpsSessionQuery) (poker.OpsSessionPage, error) {
	var result poker.OpsSessionPage
	err := p.callOps(ctx, http.MethodPost, "ops/sessions", query, &result)
	return result, err
}

func (p *pokerRemote) ReadOpsSession(ctx context.Context, id string) (poker.OpsSession, error) {
	var result poker.OpsSession
	if !opsUUIDPatternHTTP(id) {
		return result, poker.ErrInvalid
	}
	err := p.callOps(ctx, http.MethodGet, "ops/sessions/"+id, nil, &result)
	return result, err
}

func (p *pokerRemote) ReadOpsHand(ctx context.Context, id string) (poker.OpsHand, error) {
	var result poker.OpsHand
	if !opsUUIDPatternHTTP(id) {
		return result, poker.ErrInvalid
	}
	err := p.callOps(ctx, http.MethodGet, "ops/hands/"+id, nil, &result)
	return result, err
}

func (p *pokerRemote) ReadOpsOperation(ctx context.Context, id string) (poker.OpsReceipt, error) {
	var result poker.OpsReceipt
	if !opsUUIDPatternHTTP(id) {
		return result, poker.ErrInvalid
	}
	err := p.callOps(ctx, http.MethodGet, "ops/operations/"+id, nil, &result)
	return result, err
}

func (p *pokerRemote) ExecuteOpsCommand(ctx context.Context, command poker.OpsCommand) (poker.OpsReceipt, error) {
	var result poker.OpsReceipt
	err := p.callOps(ctx, http.MethodPost, "ops/operations", command, &result)
	return result, err
}
