package poker

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// SessionSummary is a viewer-owned read model, not an engine snapshot or wallet.
type SessionSummary struct {
	SessionID         string     `json:"session_id"`
	TableID           string     `json:"table_id"`
	TableName         string     `json:"table_name"`
	State             string     `json:"state"`
	SeatNo            int        `json:"seat_no"`
	StackUnits        string     `json:"stack_units"`
	CommittedUnits    string     `json:"committed_units"`
	PokerInPlayUnits  string     `json:"poker_in_play_units"`
	ControlEpoch      string     `json:"control_epoch"`
	InitialBuyInUnits string     `json:"initial_buy_in_units"`
	TotalTopUpUnits   string     `json:"total_top_up_units"`
	FinalCashOutUnits *string    `json:"final_cash_out_units,omitempty"`
	RealizedPLUnits   *string    `json:"realized_pl_units,omitempty"`
	StartedAt         time.Time  `json:"started_at"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
	EndReason         *string    `json:"end_reason,omitempty"`

	smallBlind, bigBlind int64
}

func (s *Service) ActiveSession(ctx context.Context, user int64) (*SessionSummary, error) {
	return s.readSession(ctx, user, "")
}
func (s *Service) Session(ctx context.Context, user int64, session string) (SessionSummary, error) {
	if session == "" {
		return SessionSummary{}, ErrInvalid
	}
	v, err := s.readSession(ctx, user, session)
	if err != nil {
		return SessionSummary{}, err
	}
	if v == nil {
		return SessionSummary{}, ErrDenied
	}
	return *v, nil
}
func (s *Service) readSession(ctx context.Context, user int64, session string) (*SessionSummary, error) {
	return readSessionFrom(ctx, s.opts.Pool, user, session)
}

type sessionQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readSessionFrom(ctx context.Context, q sessionQuerier, user int64, session string) (*SessionSummary, error) {
	if user <= 0 {
		return nil, ErrInvalid
	}
	query := `SELECT s.session_id::text,s.table_id::text,t.name,s.state,s.seat_no,s.current_stack_units,
 coalesce((SELECT sum(p.total_committed_units) FROM poker.hand_participants p JOIN poker.hands h USING(hand_id) WHERE p.session_id=s.session_id AND h.state<>'SETTLED'),0)::bigint,
 s.control_epoch,s.initial_buyin_units,s.total_topup_units,s.final_cashout_units,s.realized_pl_units,s.started_at,s.ended_at,s.end_reason,b.small_blind_units,b.big_blind_units
 FROM poker.sessions s JOIN poker.tables t USING(table_id) JOIN poker.blind_preset_versions b ON b.version=t.blind_preset_version WHERE s.newapi_user_id=$1`
	args := []any{user}
	if session == "" {
		query += " AND s.state<>'SETTLED'"
	} else {
		query += " AND s.session_id=$2"
		args = append(args, session)
	}
	var v SessionSummary
	var stack, committed, initial, topup int64
	var epoch uint64
	var final, pl *int64
	err := q.QueryRow(ctx, query, args...).Scan(&v.SessionID, &v.TableID, &v.TableName, &v.State, &v.SeatNo, &stack, &committed, &epoch, &initial, &topup, &final, &pl, &v.StartedAt, &v.EndedAt, &v.EndReason, &v.smallBlind, &v.bigBlind)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v.StackUnits = decimal(stack)
	v.CommittedUnits = decimal(committed)
	v.PokerInPlayUnits = decimal(stack + committed)
	v.ControlEpoch = strconv.FormatUint(epoch, 10)
	v.InitialBuyInUnits = decimal(initial)
	v.TotalTopUpUnits = decimal(topup)
	if final != nil {
		value := decimal(*final)
		v.FinalCashOutUnits = &value
	}
	if pl != nil {
		value := decimal(*pl)
		v.RealizedPLUnits = &value
	}
	return &v, nil
}

func (s *Service) connectionRef(identity ControlRef) (ControlRef, error) {
	if !validIdentity(identity) {
		return ControlRef{}, ErrNoControl
	}
	s.mu.Lock()
	ref, ok := s.connections[identity.ConnectionID]
	closed := s.closed
	s.mu.Unlock()
	if closed || !ok || !sameAuth(identity.auth(), ref.auth()) || identity.TableID != ref.TableID {
		return ControlRef{}, ErrNoControl
	}
	if identity.SessionID != "" && identity.SessionID != ref.SessionID || identity.RuntimeEpoch != 0 && identity.RuntimeEpoch != ref.RuntimeEpoch {
		return ControlRef{}, ErrNoControl
	}
	return ref, nil
}

// ResolveControl is the root adapter's narrow bridge from authenticated
// Principal+server ConnectionRef+client epoch to the complete server-only ref.
func (s *Service) ResolveControl(ctx context.Context, identity ControlRef, expectedEpoch uint64) (ControlRef, error) {
	ref, err := s.connectionRef(identity)
	if err != nil {
		return ref, err
	}
	if expectedEpoch == 0 || ref.ControlEpoch != expectedEpoch {
		return ref, ErrNoControl
	}
	if err = s.AuthorizeControl(ctx, ref); err != nil {
		return ref, err
	}
	return ref, nil
}

func (s *Service) ViewForConnection(ctx context.Context, identity ControlRef) (TableView, error) {
	ref, err := s.connectionRef(identity)
	if err != nil {
		return TableView{}, err
	}
	if err = s.AuthorizeTable(ctx, ref.TableID, ref.auth()); err != nil {
		return TableView{}, err
	}
	if err = s.validateAuth(ctx, ref); err != nil {
		s.loseGrant(ref)
		return TableView{}, err
	}
	v, err := s.view(ctx, ref.TableID, ref.UserID, true)
	if err != nil {
		return v, err
	}
	if v.RuntimeEpoch != ref.RuntimeEpoch {
		return TableView{}, ErrFenced
	}
	current, err := s.connectionRef(identity)
	if err != nil || !sameConnection(current, ref) || !s.live(ref, false) {
		return TableView{}, ErrNoControl
	}
	if ref.SessionID == "" && v.Viewer.SessionID != "" || ref.SessionID != "" && ref.SessionID != v.Viewer.SessionID {
		return TableView{}, ErrNoControl
	}
	if err = s.validateAuth(ctx, ref); err != nil {
		s.loseGrant(ref)
		return TableView{}, err
	}
	if err = s.AuthorizeTable(ctx, ref.TableID, ref.auth()); err != nil {
		return TableView{}, err
	}
	if v.Chat != nil && !v.Chat.Muted && s.canSendChat(ctx, ref) {
		v.Chat.CanSend = true
	}
	control := SocketControlView{ConnectionID: ref.ConnectionID, SessionID: v.Viewer.SessionID, Mode: "READ_ONLY", ControlEpoch: v.Viewer.ControlEpoch}
	// A socket authenticated before BuyIn must be explicitly rebound by the root
	// adapter; a read operation never claims control or changes PG facts.
	if ref.SessionID != "" && ref.SessionID == v.Viewer.SessionID && ref.ControlEpoch > 0 && strconv.FormatUint(ref.ControlEpoch, 10) == v.Viewer.ControlEpoch && s.AuthorizeControl(ctx, ref) == nil {
		control.Mode = "CONTROLLER"
	}
	v.Viewer.Control = &control
	if control.Mode != "CONTROLLER" {
		v.Viewer.CanAct = false
		v.Viewer.Legal = nil
		v.Viewer.CanSitOut = false
		v.Viewer.CanResume = false
	}
	return v, nil
}
