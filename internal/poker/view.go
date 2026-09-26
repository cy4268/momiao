package poker

import (
	"context"
 "github.com/cy4268/momiao/internal/platform"
	"encoding/hex"
	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/jackc/pgx/v5"
	"strconv"
	"time"
)

// Public DTOs contain decimal-string Atomic Units and never a deck/server seed.
type TableView struct {
	RuntimeEpoch      uint64     `json:"-"`
	TableID           string     `json:"table_id"`
	Name              string     `json:"name"`
	LifecycleState    string     `json:"lifecycle_state"`
	TableVersion      string     `json:"table_version"`
	MaxSeats          int        `json:"max_seats"`
	SmallBlindUnits   string     `json:"small_blind_units"`
	BigBlindUnits     string     `json:"big_blind_units"`
	SettingsLocked    bool       `json:"settings_locked"`
	ServerNow         time.Time  `json:"server_now"`
	IntermissionUntil *time.Time `json:"intermission_until,omitempty"`
	Seats             []SeatView `json:"seats"`
	Hand              *HandView  `json:"hand,omitempty"`
	Viewer            ViewerView `json:"viewer"`
	Chat              *ChatView  `json:"chat,omitempty"`
	Timeline          TimelineView `json:"timeline"`
	Host              *HostView  `json:"host,omitempty"`
}
type SeatView struct {
	SeatNo               int        `json:"seat_no"`
	DisplayName          string     `json:"display_name"`
	State                string     `json:"state"`
	IsSelf               bool       `json:"is_self"`
	Connected            bool       `json:"connected"`
	StackUnits           string     `json:"stack_units"`
	StreetCommittedUnits string     `json:"street_committed_units"`
	TotalCommittedUnits  string     `json:"total_committed_units"`
	IsFolded             bool       `json:"is_folded"`
	IsAllIn              bool       `json:"is_all_in"`
	HoleCardCount        int        `json:"hole_card_count"`
	HoleCards            []int      `json:"hole_cards,omitempty"`
	PublicHoleCards      []int      `json:"public_hole_cards,omitempty"`
	HoleCardsReleased    bool       `json:"hole_cards_released"`
	SitOutNextHand       bool       `json:"sit_out_next_hand"`
	LeaveAfterHand       bool       `json:"leave_after_hand"`
	PendingTopUpUnits    string     `json:"pending_top_up_units"`
	RebuyDeadlineAt      *time.Time `json:"rebuy_deadline_at,omitempty"`
}
type HandView struct {
 EconomySettlement *platform.PayoutCapView `json:"economy_settlement,omitempty"`
	HandID           string     `json:"hand_id"`
	HandVersion      string     `json:"hand_version"`
	Street           string     `json:"street"`
	ButtonSeat       int        `json:"button_seat"`
	ActorSeat        int        `json:"actor_seat"`
	BoardCards       []int      `json:"board_cards"`
	PotUnits         string     `json:"pot_units"`
	Pots             []PotView  `json:"pots"`
	ActionSequence   string     `json:"action_sequence"`
	ActionDeadlineAt *time.Time `json:"action_deadline_at,omitempty"`
	Recovering       bool       `json:"recovering"`
	RecoveryUntil    *time.Time `json:"recovery_until,omitempty"`
	ServerSeedHash   string     `json:"server_seed_hash"`
	DeckHash         string     `json:"deck_hash"`
}
type PotView struct {
	Index         int         `json:"index"`
	AmountUnits   string      `json:"amount_units"`
	EligibleSeats []int       `json:"eligible_seats"`
	Awards        []AwardView `json:"awards"`
}
type AwardView struct {
	SeatNo      int    `json:"seat_no"`
	AmountUnits string `json:"amount_units"`
}
type ViewerView struct {
	Control       *SocketControlView `json:"control,omitempty"`
	SessionID     string             `json:"session_id,omitempty"`
	SeatNo        int                `json:"seat_no,omitempty"`
	ControlEpoch  string             `json:"control_epoch"`
	CanAct        bool               `json:"can_act"`
	CanTopUp      bool               `json:"can_top_up"`
	CanLeave      bool               `json:"can_leave"`
	CanResume     bool               `json:"can_resume"`
	CanSitOut     bool               `json:"can_sit_out"`
	CanStart      bool               `json:"can_start"`
	TopUpMinUnits string             `json:"top_up_min_units"`
	TopUpMaxUnits string             `json:"top_up_max_units"`
	Legal         *LegalView         `json:"legal,omitempty"`
}
type LegalView struct {
	Actions             []string       `json:"actions"`
	ToCallUnits         string         `json:"to_call_units"`
	CallAppliedUnits    string         `json:"call_applied_units"`
	MinimumBetUnits     string         `json:"minimum_bet_units"`
	MinimumRaiseToUnits string         `json:"minimum_raise_to_units"`
	MaximumRaiseToUnits string         `json:"maximum_raise_to_units"`
	RaiseRights         bool           `json:"raise_rights"`
	Shortcuts           []ShortcutView `json:"shortcuts"`
}
type ShortcutView struct {
	Name          string `json:"name"`
	ActionType    string `json:"action_type"`
	TargetToUnits string `json:"target_to_units"`
}

// releasedHoles is the sole live public-hole release policy. Even in all-in
// runouts a folded hand is never released. A fold win is not a showdown.
func releasedHoles(state engine.State) map[int][]int {
	out := map[int][]int{}
	runout, showdown := false, false
	for _, event := range state.Events {
		runout = runout || event.Type == "ALL_IN_RUNOUT"
		showdown = showdown || event.Type == "SHOWDOWN"
	}
	winners := map[int]bool{}
	if showdown {
		for _, pot := range state.Pots {
			for _, award := range pot.Awards {
				winners[award.Seat] = true
			}
		}
	}
	for _, p := range state.Players {
		if !p.Folded && (runout || winners[p.SeatNo]) {
			out[p.SeatNo] = []int{int(p.Hole[0]), int(p.Hole[1])}
		}
	}
	return out
}

func (s *Service) View(ctx context.Context, table string, user int64) (TableView, error) {
	v, err := s.view(ctx, table, user, false)
	makeReadonly(&v)
	return v, err
}

func (s *Service) ViewForSession(ctx context.Context, table string, auth AuthSession) (TableView, error) {
	if !validAccessAuth(auth) {
		return TableView{}, ErrInvalid
	}
	if err := s.AuthorizeTable(ctx, table, auth); err != nil {
		return TableView{}, err
	}
	v, err := s.view(ctx, table, auth.UserID, true)
	if err != nil {
		return v, err
	}
	if err = s.AuthorizeTable(ctx, table, auth); err != nil {
		return TableView{}, err
	}
	makeReadonly(&v)
	return v, nil
}

func makeReadonly(v *TableView) {
	v.Viewer.CanAct = false
	v.Viewer.Legal = nil
	v.Viewer.CanStart = false
	v.Viewer.CanSitOut = false
	v.Viewer.CanResume = false
	if v.Chat != nil {
		v.Chat.CanSend = false
	}
}

func (s *Service) view(ctx context.Context, table string, user int64, passwordAuthorized bool) (TableView, error) {
	tx, err := s.opts.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return TableView{}, err
	}
	defer rollback(tx)
	var other bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM poker.sessions WHERE newapi_user_id=$1 AND table_id<>$2 AND state<>'SETTLED')", user, table).Scan(&other); err != nil {
		return TableView{}, err
	}
	if other {
		return TableView{}, ErrDenied
	}
	t, err := loadTable(ctx, tx, table, false)
	if err != nil {
		return TableView{}, err
	}
	if t.AccessMode == "PASSWORD" && !passwordAuthorized {
		return TableView{}, ErrTableAccessRequired
	}
	if t.NeedsReview {
		return TableView{}, ErrNeedsReview
	}
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&t.Now); err != nil {
		return TableView{}, err
	}
	seats, err := loadSeats(ctx, tx, table)
	if err != nil {
		return TableView{}, err
	}
	isSeated := false
	for _, seat := range seats {
		isSeated = isSeated || seat.User == user
	}
	if !isSeated && !t.AllowSpectators && t.Owner != user {
		return TableView{}, ErrDenied
	}
	v := TableView{TableID: t.ID, Name: t.Name, LifecycleState: t.State, TableVersion: strconv.FormatUint(t.Version, 10), MaxSeats: t.MaxSeats, SmallBlindUnits: decimal(t.SmallBlind), BigBlindUnits: decimal(t.BigBlind), SettingsLocked: t.SettingsLocked != nil, ServerNow: t.Now, IntermissionUntil: t.Intermission, Seats: []SeatView{}, Viewer: ViewerView{ControlEpoch: "0", TopUpMinUnits: "0", TopUpMaxUnits: "0"}, Timeline: TimelineView{LastSequence: "0", Events: []TimelineEventView{}}}
	v.RuntimeEpoch = t.Epoch
	var e *engine.Engine
	var state engine.State
	released := map[int][]int{}
	participants := map[int]engine.Participant{}
	sessions := map[int]string{}
	if t.HandID != "" {
		v.Timeline.HandID = t.HandID
		var status string
		var seedHash, deckHash []byte
		if err = tx.QueryRow(ctx, "SELECT h.state,f.server_seed_hash,f.deck_hash FROM poker.hands h JOIN poker.hand_fairness f USING(hand_id) WHERE h.hand_id=$1", t.HandID).Scan(&status, &seedHash, &deckHash); err != nil {
			return v, err
		}
		v.Hand = &HandView{HandID: t.HandID, HandVersion: "0", Street: status, ButtonSeat: t.Button, BoardCards: []int{}, PotUnits: "0", Pots: []PotView{}, ActionSequence: "0", ServerSeedHash: hex.EncodeToString(seedHash), DeckHash: hex.EncodeToString(deckHash)}
		v.Hand.EconomySettlement,err=handCapView(ctx,tx,t.HandID,user)
 if err!=nil{return v,err}
		if status != "COMMITTED" {
			e, err = s.restoreEngine(ctx, tx, t.HandID)
			if err != nil {
				return v, err
			}
			state = e.State()
			v.Timeline = liveTimeline(t.HandID, state)
			released = releasedHoles(state)
			h := v.Hand
			h.HandVersion = strconv.FormatUint(state.Version, 10)
			h.ActionSequence = strconv.FormatUint(state.ActionSequence, 10)
			h.ActorSeat = state.Actor
			h.Recovering = state.RecoveryUntil != nil
			h.RecoveryUntil = state.RecoveryUntil
			if !state.Deadline.IsZero() {
				d := state.Deadline
				h.ActionDeadlineAt = &d
			}
			for _, card := range state.Board {
				h.BoardCards = append(h.BoardCards, int(card))
			}
			var pot int64
			for _, p := range state.Players {
				participants[p.SeatNo] = p
				pot += p.TotalCommitted
			}
			for _, p := range state.Pots {
				pot += p.Amount
				pv := PotView{Index: p.Index, AmountUnits: decimal(p.Amount), EligibleSeats: append([]int{}, p.Eligible...), Awards: []AwardView{}}
				for _, a := range p.Awards {
					pv.Awards = append(pv.Awards, AwardView{SeatNo: a.Seat, AmountUnits: decimal(a.Amount)})
				}
				h.Pots = append(h.Pots, pv)
			}
			h.PotUnits = decimal(pot)
			rows, err := tx.Query(ctx, "SELECT seat_no,session_id::text FROM poker.hand_participants WHERE hand_id=$1", t.HandID)
			if err != nil {
				return v, err
			}
			for rows.Next() {
				var seat int
				var session string
				if err = rows.Scan(&seat, &session); err != nil {
					rows.Close()
					return v, err
				}
				sessions[seat] = session
			}
			rows.Close()
			if err = rows.Err(); err != nil {
				return v, err
			}
		}
	}
	for _, seat := range seats {
		sv := SeatView{SeatNo: seat.Seat, DisplayName: seat.Display, State: seat.State, IsSelf: seat.User == user, Connected: seat.Connected, StackUnits: decimal(seat.Stack), StreetCommittedUnits: "0", TotalCommittedUnits: "0", SitOutNextHand: seat.SitOutNext, LeaveAfterHand: seat.Leave, PendingTopUpUnits: decimal(seat.Pending), RebuyDeadlineAt: seat.RebuyUntil}
		if p, ok := participants[seat.Seat]; ok && sessions[seat.Seat] == seat.Session {
			sv.StreetCommittedUnits = decimal(p.StreetCommitted)
			sv.TotalCommittedUnits = decimal(p.TotalCommitted)
			sv.IsFolded = p.Folded
			sv.IsAllIn = p.Stack == 0 && !p.Folded
			sv.HoleCardCount = 2
			sv.PublicHoleCards = released[seat.Seat]
			sv.HoleCardsReleased = len(sv.PublicHoleCards) > 0
			if sv.IsSelf && p.PlayerID == decimal(user) {
				sv.HoleCards = []int{int(p.Hole[0]), int(p.Hole[1])}
			}
		}
		v.Seats = append(v.Seats, sv)
		if sv.IsSelf {
			vw := &v.Viewer
			vw.SessionID = seat.Session
			vw.SeatNo = seat.Seat
			vw.ControlEpoch = strconv.FormatUint(seat.Control, 10)
			vw.CanLeave = !seat.Leave
			vw.CanSitOut = !seat.Leave && seat.State == "ACTIVE" && !seat.SitOutNext
			vw.CanResume = seat.Connected && !seat.Leave && seat.Stack > 0 && (seat.State == "SIT_OUT" || seat.TimeoutSitOut)
			limit := max(int64(0), 100*t.BigBlind-seat.Stack-seat.Pending)
			minimum := engine.UnitsPerChip
			if seat.State == "REBUY_WINDOW" {
				minimum = 40 * t.BigBlind
			}
			vw.TopUpMinUnits = decimal(minimum)
			vw.TopUpMaxUnits = decimal(limit)
			vw.CanTopUp = !seat.Leave && t.State != "CLOSING" && t.State != "CLOSED" && limit >= minimum && (seat.RebuyUntil == nil || seat.RebuyUntil.After(t.Now))
			vw.CanAct = e != nil && state.Actor == seat.Seat && state.RecoveryUntil == nil && seat.Connected && state.Street != engine.Settled && state.Deadline.After(t.Now) && sessions[seat.Seat] == seat.Session
			if vw.CanAct {
				l := e.LegalActions()
				lv := &LegalView{Actions: []string{}, ToCallUnits: decimal(l.ToCall), CallAppliedUnits: decimal(min(l.ToCall, l.CurrentStack)), MinimumBetUnits: decimal(l.MinimumBet), MinimumRaiseToUnits: decimal(l.MinimumRaiseTo), MaximumRaiseToUnits: decimal(l.MaximumRaiseTo), RaiseRights: l.RaiseRights, Shortcuts: []ShortcutView{}}
				for _, action := range l.Actions {
					lv.Actions = append(lv.Actions, string(action))
				}
				for _, sc := range e.ShortcutTargets() {
					lv.Shortcuts = append(lv.Shortcuts, ShortcutView{Name: sc.Name, ActionType: string(sc.Kind), TargetToUnits: decimal(sc.To)})
				}
				vw.Legal = lv
			}
		}
	}
	if v.Chat, err = readChat(ctx, tx, t, user); err != nil {
		return v, err
	}
	if t.Owner == user && passwordAuthorized {
		if v.Host, err = s.readHostView(ctx, tx, t, seats); err != nil {
			return v, err
		}
	}
	return v, nil
}
