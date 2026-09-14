package transport

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	closed := h.closed
	h.mu.Unlock()
	if closed {
		writeError(w, &Fault{503, "POKER_SERVICE_CLOSED"})
		return
	}
	if r.URL.Path == "/ws/poker" {
		h.serveWS(w, r)
		return
	}
	if r.URL.Path != "/api/v1/poker" && !strings.HasPrefix(r.URL.Path, "/api/v1/poker/") {
		http.NotFound(w, r)
		return
	}
	// Never accept browser-selected identity, credentials in URLs, or an alternate
	// path through the shared proxy. This handler is mounted before /api/*.
	if (r.URL.RawQuery != "" || r.URL.ForceQuery) && !(r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v1/poker/tables") {
		writeError(w, errInvalid)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		writeError(w, &Fault{405, "METHOD_NOT_ALLOWED"})
		return
	}
	if r.Method == http.MethodPost && !h.validOrigin(r) {
		writeError(w, &Fault{403, "ORIGIN_DENIED"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.opts.OperationTimeout)
	defer cancel()
	r = r.WithContext(ctx)
	p, err := h.opts.AuthHTTP(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if p.UserID <= 0 {
		writeError(w, &Fault{401, "AUTH_REQUIRED"})
		return
	}
	if err = h.opts.ValidateSession(ctx, p); err != nil {
		writeError(w, err)
		return
	}
	data, err := h.routeHTTP(r, p)
	if err != nil {
		writeError(w, err)
		return
	}
	if !json.Valid(data) {
		writeError(w, &Fault{500, "POKER_INVALID_PROJECTION"})
		return
	}
	writeJSON(w, 200, struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}{true, data})
}
func (h *Handler) validOrigin(r *http.Request) bool {
	v := r.Header.Values("Origin")
	return len(v) == 1 && v[0] == h.opts.Origin
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, err error) {
	f := fault(err)
	writeJSON(w, f.Status, struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}{false, f.Code, http.StatusText(f.Status)})
}
func readBody(r *http.Request, max int64, out any, fields ...string) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return &Fault{415, "CONTENT_TYPE_REQUIRED"}
	}
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, max+1))
	if err != nil || int64(len(b)) > max {
		return &Fault{413, "MESSAGE_TOO_LARGE"}
	}
	if err = strictJSON(b, out, fields...); err != nil {
		return errInvalid
	}
	return nil
}
func readPort(ctx context.Context, p Principal, fn func(context.Context, Principal) (json.RawMessage, error)) (json.RawMessage, error) {
	if fn == nil {
		return nil, errUnavailable
	}
	return fn(ctx, p)
}
func idReadPort(ctx context.Context, p Principal, id string, fn func(context.Context, Principal, string) (json.RawMessage, error)) (json.RawMessage, error) {
	if fn == nil {
		return nil, errUnavailable
	}
	return fn(ctx, p, id)
}
func postPort[T any](h *Handler, r *http.Request, p Principal, id string, fn func(context.Context, Principal, string, T) (json.RawMessage, error), valid func(T) bool, fields ...string) (json.RawMessage, error) {
	if r.Method != http.MethodPost {
		return nil, &Fault{405, "METHOD_NOT_ALLOWED"}
	}
	var body T
	if err := readBody(r, h.opts.MaxMessageBytes, &body, fields...); err != nil {
		return nil, err
	}
	if !valid(body) {
		return nil, errInvalid
	}
	if fn == nil {
		return nil, errUnavailable
	}
	return fn(r.Context(), p, id, body)
}
func (h *Handler) routeHTTP(r *http.Request, p Principal) (json.RawMessage, error) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/poker")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	ctx := r.Context()
	ports := h.opts.Ports
	if path == "" && r.Method == http.MethodGet {
		return readPort(ctx, p, ports.ReadLobby)
	}
	if path == "/entry-receipt-query" {
		return postPort(h, r, p, "", ports.ReadEntryReceipt, func(b EntryReceiptRequest) bool {
			if !validKey(b.MutationID) {
				return false
			}
			if b.Kind == "create" {
				return len(b.TableID) == 0
			}
			var table string
			return (b.Kind == "reserve" || b.Kind == "buyin") && json.Unmarshal(b.TableID, &table) == nil && validUUID(table) && table == strings.ToLower(table)
		}, "kind", "mutation_id", "table_id")
	}
	if path == "/tables" {
		if r.Method == http.MethodGet {
			q, err := readLobbyQuery(r)
			if err != nil {
				return nil, err
			}
			if ports.ReadTables == nil {
				return nil, errUnavailable
			}
			return ports.ReadTables(ctx, p, q)
		}
		fn := func(ctx context.Context, p Principal, _ string, b CreateTableRequest) (json.RawMessage, error) {
			if ports.CreateTable == nil {
				return nil, errUnavailable
			}
			return ports.CreateTable(ctx, p, b)
		}
		return postPort(h, r, p, "", fn, func(b CreateTableRequest) bool {
			mode := b.AccessMode
			if mode == "" {
				mode = "PUBLIC"
			}
			password := mode == "PUBLIC" && len(b.Password) == 0
			if mode == "PASSWORD" && len(b.Password) != 0 {
				var value string
				password = json.Unmarshal(b.Password, &value) == nil && len(value) > 0 && len(value) <= 128
			}
			return validKey(b.RequestID) && len(b.Name) > 0 && b.MaxSeats >= 2 && b.MaxSeats <= 9 && validPreset(b.BlindPreset) && password
		}, "request_id", "name", "blind_preset", "max_seats", "allow_spectators", "access_mode", "password", "chat_enabled")
	}
	if path == "/connect-tickets" {
		fn := func(ctx context.Context, p Principal, _ string, b TicketRequest) (json.RawMessage, error) {
			if ports.MintTicket == nil {
				return nil, errUnavailable
			}
			return ports.MintTicket(ctx, p, b)
		}
		return postPort(h, r, p, "", fn, func(b TicketRequest) bool {
			return (b.TargetTableID == nil || validUUID(*b.TargetTableID)) && (b.ControlIntent == "CLAIM_CONTROL" || b.ControlIntent == "READ_ONLY")
		}, "target_table_id", "control_intent")
	}
	if path == "/sessions/active" && r.Method == http.MethodGet {
		return readPort(ctx, p, ports.ReadActiveSession)
	}
	if len(parts) < 2 || !validUUID(parts[1]) {
		return nil, &Fault{404, "POKER_ROUTE_NOT_FOUND"}
	}
	id := parts[1]
	if len(parts) == 2 && r.Method == http.MethodGet {
		switch parts[0] {
		case "tables":
			s, err := h.readSnapshot(ctx, p, ConnectionRef{TableID: id})
			return s.Payload, err
		case "sessions":
			return idReadPort(ctx, p, id, ports.ReadSession)
		}
	}
	if len(parts) != 3 {
		return nil, &Fault{404, "POKER_ROUTE_NOT_FOUND"}
	}
	if parts[0] == "hands" && parts[2] == "fairness" && r.Method == http.MethodGet {
		return idReadPort(ctx, p, id, ports.ReadFairness)
	}
	if parts[0] == "tables" {
		switch parts[2] {
		case "reservation-query":
			return postPort(h, r, p, id, ports.ReadReservation, func(b ReservationQueryRequest) bool {
				return id == strings.ToLower(id) && validUUID(b.ReservationID) && b.ReservationID == strings.ToLower(b.ReservationID)
			}, "reservation_id")
		case "receipt-query":
			return postPort(h, r, p, id, ports.ReadReceipt, func(b ReceiptQueryRequest) bool {
				return validReceiptKind(b.Kind) && validKey(b.MutationID)
			}, "kind", "mutation_id")
		case "access":
			return postPort(h, r, p, id, ports.VerifyTableAccess, func(b AccessRequest) bool { return len(b.Password) > 0 && len(b.Password) <= 128 }, "password")
		case "seat-reservations":
			return postPort(h, r, p, id, ports.ReserveSeat, func(b ReserveSeatRequest) bool { return validKey(b.RequestID) && b.SeatNo >= 1 && b.SeatNo <= 9 }, "request_id", "seat_no")
		case "buy-ins":
			return postPort(h, r, p, id, ports.BuyIn, func(b BuyInRequest) bool {
				return validKey(b.RequestID) && validUUID(b.ReservationID) && validAmount(b.AmountUnits, true)
			}, "request_id", "reservation_id", "amount_units")
		case "commands":
			return postPort(h, r, p, id, ports.HostCommand, func(b HostRequest) bool {
				return validKey(b.RequestID) && validHostRequest(b)
			}, "request_id", "command", "target_session_id", "target_user_id")
		}
	}
	if parts[0] == "sessions" {
		switch parts[2] {
		case "top-ups":
			return postPort(h, r, p, id, ports.TopUp, func(b TopUpRequest) bool { return validKey(b.RequestID) && validAmount(b.AmountUnits, true) }, "request_id", "amount_units")
		case "safe-leave":
			return postPort(h, r, p, id, ports.SafeLeave, func(b SessionRequest) bool { return validKey(b.RequestID) }, "request_id")
		case "take-over":
			return postPort(h, r, p, id, h.takeOver, func(b TakeOverRequest) bool { return validKey(b.RequestID) && validConnectionID(b.ConnectionID) }, "request_id", "connection_id")
		}
	}
	return nil, &Fault{404, "POKER_ROUTE_NOT_FOUND"}
}

func readLobbyQuery(r *http.Request) (LobbyQuery, error) {
	var q LobbyQuery
	if len(r.URL.RawQuery) > 4096 {
		return q, errInvalid
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return q, errInvalid
	}
	for key, list := range values {
		if len(list) != 1 || list[0] == "" {
			return q, errInvalid
		}
		v := list[0]
		switch key {
		case "q":
			q.Query = v
		case "access_mode":
			q.AccessMode = v
		case "blind_preset":
			q.BlindPreset = v
		case "lifecycle_state":
			q.LifecycleState = v
		case "sort":
			q.Sort = v
		case "cursor":
			q.Cursor = v
		case "open_seats_only", "spectators_only":
			if v != "true" && v != "false" {
				return q, errInvalid
			}
			if key == "open_seats_only" {
				q.OpenSeatsOnly = v == "true"
			} else {
				q.SpectatorsOnly = v == "true"
			}
		case "max_seats", "limit":
			n, e := strconv.Atoi(v)
			if e != nil || strconv.Itoa(n) != v || key == "max_seats" && (n < 2 || n > 9) || key == "limit" && (n < 1 || n > 100) {
				return q, errInvalid
			}
			if key == "max_seats" {
				q.MaxSeats = n
			} else {
				q.Limit = n
			}
		default:
			return q, errInvalid
		}
	}
	return q, nil
}
func validReceiptKind(kind string) bool {
	switch kind {
	case "action", "sitout", "resume", "nextseed", "topup", "leave", "takeover", "host", "chat":
		return true
	}
	return false
}
func validPreset(s string) bool {
	switch s {
	case "5-10", "10-20", "25-50", "50-100", "100-200", "500-1000":
		return true
	}
	return false
}

func validHostCommand(s string) bool {
	switch s {
	case "PAUSE_ACCEPTING_PLAYERS", "RESUME_ACCEPTING_PLAYERS", "REMOVE_PLAYER_AFTER_HAND", "REMOVE_SPECTATOR", "MUTE_CHAT_USER", "CLOSE_TABLE":
		return true
	}
	return false
}

func validPositiveID(raw string) bool {
	if raw == "" || len(raw) > 19 || len(raw) > 1 && raw[0] == '0' {
		return false
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == raw
}

func validHostRequest(b HostRequest) bool {
	if !validHostCommand(b.Command) {
		return false
	}
	switch b.Command {
	case "PAUSE_ACCEPTING_PLAYERS", "RESUME_ACCEPTING_PLAYERS", "CLOSE_TABLE":
		return b.TargetSessionID == "" && b.TargetUserID == ""
	case "REMOVE_PLAYER_AFTER_HAND":
		return validUUID(b.TargetSessionID) && validPositiveID(b.TargetUserID)
	case "REMOVE_SPECTATOR":
		return b.TargetSessionID == "" && validPositiveID(b.TargetUserID)
	case "MUTE_CHAT_USER":
		return (b.TargetSessionID == "" || validUUID(b.TargetSessionID)) && validPositiveID(b.TargetUserID)
	default:
		return false
	}
}

func (h *Handler) readSnapshot(ctx context.Context, p Principal, connection ConnectionRef) (Snapshot, error) {
	table := connection.TableID
	s, err := h.opts.Snapshot(ctx, p, connection)
	if err != nil {
		return s, err
	}
	if s.TableID != table || s.RuntimeEpoch == 0 || s.ServerTime.IsZero() || !json.Valid(s.Payload) {
		return Snapshot{}, &Fault{500, "POKER_INVALID_PROJECTION"}
	}
	if s.TableVersion > MaxSafeInteger || s.HandVersion != nil && *s.HandVersion > MaxSafeInteger {
		return Snapshot{}, errVersion
	}
	if (s.HandID == nil) != (s.HandVersion == nil) || s.HandID != nil && !validUUID(*s.HandID) {
		return Snapshot{}, &Fault{500, "POKER_INVALID_PROJECTION"}
	}
	var meta struct {
		TableID      string `json:"table_id"`
		TableVersion string `json:"table_version"`
		Hand         *struct {
			HandID      string `json:"hand_id"`
			HandVersion string `json:"hand_version"`
		} `json:"hand"`
		Viewer struct {
			SessionID    string          `json:"session_id"`
			ControlEpoch string          `json:"control_epoch"`
			CanAct       bool            `json:"can_act"`
			Legal        json.RawMessage `json:"legal"`
			Control      json.RawMessage `json:"control"`
		} `json:"viewer"`
	}
	if json.Unmarshal(s.Payload, &meta) != nil || meta.TableID != s.TableID || meta.TableVersion != strconv.FormatUint(s.TableVersion, 10) || (meta.Hand == nil) != (s.HandID == nil) || meta.Hand != nil && (meta.Hand.HandID != *s.HandID || meta.Hand.HandVersion != strconv.FormatUint(*s.HandVersion, 10)) {
		return Snapshot{}, &Fault{500, "POKER_INVALID_PROJECTION"}
	}
	invalidControl := &Fault{500, "POKER_INVALID_CONTROL_PROJECTION"}
	if connection.ID == "" {
		if s.Control != nil || len(meta.Viewer.Control) != 0 || meta.Viewer.CanAct || len(meta.Viewer.Legal) != 0 && string(meta.Viewer.Legal) != "null" {
			return Snapshot{}, invalidControl
		}
		return s, nil
	}
	var control ControlView
	if !validConnectionID(connection.ID) || s.Control == nil || strictJSON(meta.Viewer.Control, &control, "connection_id", "session_id", "mode", "control_epoch") != nil || control != *s.Control || control.ConnectionID != connection.ID || control.SessionID != meta.Viewer.SessionID || control.ControlEpoch != meta.Viewer.ControlEpoch {
		return Snapshot{}, invalidControl
	}
	epoch, ok := controlEpoch(control.ControlEpoch)
	if !ok || control.SessionID != "" && !validUUID(control.SessionID) || control.Mode != "CONTROLLER" && control.Mode != "READ_ONLY" || control.Mode == "CONTROLLER" && (control.SessionID == "" || epoch == 0 || p.ControlIntent != "CLAIM_CONTROL") || control.Mode == "READ_ONLY" && (meta.Viewer.CanAct || len(meta.Viewer.Legal) != 0 && string(meta.Viewer.Legal) != "null") {
		return Snapshot{}, invalidControl
	}
	s.Control = &control
	h.rememberControl(connection, s.Control)
	return s, nil
}
