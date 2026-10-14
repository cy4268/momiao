package poker

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

// Ops projections intentionally omit password material, encrypted snapshots,
// cards, deck state and unrevealed fairness values.
type OpsOverview struct {
	GeneratedAt            time.Time  `json:"generated_at"`
	ServiceState           string     `json:"service_state"`
	RequiresAttention      bool       `json:"requires_attention"`
	Blockers               []string   `json:"blockers"`
	OpenTables             int64      `json:"open_tables,string"`
	ActiveSessions         int64      `json:"active_sessions,string"`
	HandsInProgress        int64      `json:"hands_in_progress,string"`
	NeedsReviewHands       int64      `json:"needs_review_hands,string"`
	PendingFunding         int64      `json:"pending_funding,string"`
	OldestPendingFundingAt *time.Time `json:"oldest_pending_funding_at,omitempty"`
}

type OpsTable struct {
	TableID          string     `json:"table_id"`
	OwnerUserID      string     `json:"owner_user_id"`
	Name             string     `json:"name"`
	AccessMode       string     `json:"access_mode"`
	MaxSeats         int        `json:"max_seats"`
	OccupiedSeats    int64      `json:"occupied_seats,string"`
	SpectatorCount   int64      `json:"spectator_count,string"`
	ChatEnabled      bool       `json:"chat_enabled"`
	LifecycleState   string     `json:"lifecycle_state"`
	AcceptingPlayers bool       `json:"accepting_players"`
	AllowNewHands    bool       `json:"allow_new_hands"`
	TableVersion     string     `json:"table_version"`
	RuntimeEpoch     string     `json:"runtime_epoch"`
	CurrentHandID    string     `json:"current_hand_id,omitempty"`
	NeedsReview      bool       `json:"needs_review"`
	RecoveryState    string     `json:"recovery_state,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	ClosedAt         *time.Time `json:"closed_at,omitempty"`
}

type OpsTableQuery struct {
	Search   string `json:"search,omitempty"`
	State    string `json:"state,omitempty"`
	From, To *time.Time

	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type OpsTablePage struct {
	GeneratedAt time.Time  `json:"generated_at"`
	Items       []OpsTable `json:"items"`
	NextCursor  string     `json:"next_cursor,omitempty"`
}

type OpsPlayer struct {
	SessionID      string `json:"session_id"`
	UserID         string `json:"user_id"`
	SeatNo         int    `json:"seat_no"`
	DisplayName    string `json:"display_name"`
	State          string `json:"state"`
	StackUnits     string `json:"stack_units"`
	Connected      bool   `json:"connected"`
	LeaveAfterHand bool   `json:"leave_after_hand"`
}

type OpsMessageAuthor struct {
	UserID      string `json:"user_id"`
	MemberID    string `json:"member_id"`
	DisplayName string `json:"display_name"`
}

type OpsMessage struct {
	MessageID       string            `json:"message_id"`
	Sequence        string            `json:"sequence"`
	Kind            string            `json:"kind"`
	Body            string            `json:"body"`
	CreatedAt       time.Time         `json:"created_at"`
	Author          *OpsMessageAuthor `json:"author,omitempty"`
	ModerationState string            `json:"moderation_state"`
}

type OpsSafeEvent struct {
	EventID     string          `json:"event_id"`
	EventType   string          `json:"event_type"`
	ActorKind   string          `json:"actor_kind"`
	CreatedAt   time.Time       `json:"created_at"`
	SafeDetails json.RawMessage `json:"safe_details"`
}

type OpsTableDetail struct {
	GeneratedAt  time.Time      `json:"generated_at"`
	Table        OpsTable       `json:"table"`
	Players      []OpsPlayer    `json:"players"`
	Messages     []OpsMessage   `json:"messages"`
	RecentEvents []OpsSafeEvent `json:"recent_events"`
}

type OpsSession struct {
	SessionID         string     `json:"session_id"`
	TableID           string     `json:"table_id"`
	UserID            string     `json:"user_id"`
	SeatNo            int        `json:"seat_no"`
	DisplayName       string     `json:"display_name"`
	State             string     `json:"state"`
	InitialBuyInUnits string     `json:"initial_buyin_units"`
	TotalTopUpUnits   string     `json:"total_topup_units"`
	CurrentStackUnits string     `json:"current_stack_units"`
	FinalCashOutUnits *string    `json:"final_cashout_units,omitempty"`
	RealizedPLUnits   *string    `json:"realized_pl_units,omitempty"`
	ControlEpoch      string     `json:"control_epoch"`
	StartedAt         time.Time  `json:"started_at"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
	EndReason         *string    `json:"end_reason,omitempty"`
}

type OpsSessionQuery struct {
	UserID   int64  `json:"user_id,omitempty"`
	State    string `json:"state,omitempty"`
	From, To *time.Time

	TableID string `json:"table_id,omitempty"`
	Cursor  string `json:"cursor,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type OpsSessionPage struct {
	GeneratedAt time.Time    `json:"generated_at"`
	Items       []OpsSession `json:"items"`
	NextCursor  string       `json:"next_cursor,omitempty"`
}

type OpsHand struct {
	HandID           string     `json:"hand_id"`
	TableID          string     `json:"table_id"`
	HandNo           string     `json:"hand_no"`
	State            string     `json:"state"`
	HandVersion      string     `json:"hand_version"`
	EventSequence    string     `json:"event_sequence"`
	PublicBoardCount int64      `json:"public_board_count,string"`
	ParticipantCount int64      `json:"participant_count,string"`
	TotalPotUnits    string     `json:"total_pot_units"`
	CreatedAt        time.Time  `json:"created_at"`
	SettledAt        *time.Time `json:"settled_at,omitempty"`
	RecoveryState    string     `json:"recovery_state,omitempty"`
}

type OpsCommand struct {
	ActorUserID     int64  `json:"actor_user_id,string"`
	CommandType     string `json:"command_type"`
	TableID         string `json:"table_id"`
	Accepting       *bool  `json:"accepting,omitempty"`
	AllowNewHands   *bool  `json:"allow_new_hands,omitempty"`
	TargetSessionID string `json:"target_session_id,omitempty"`
	TargetUserID    string `json:"target_user_id,omitempty"`
	MessageID       string `json:"message_id,omitempty"`
	Muted           *bool  `json:"muted,omitempty"`
	Hidden          *bool  `json:"hidden,omitempty"`
	RecoveryAction  string `json:"recovery_action,omitempty"`
}

type OpsResult struct {
	TableID string `json:"table_id"`
	Status  string `json:"status"`
}

func (s *Service) opsAvailable() bool {
	if s == nil || s.opts.Pool == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed
}

func (s *Service) ReadOpsOverview(ctx context.Context) (OpsOverview, error) {
	result := OpsOverview{GeneratedAt: time.Now().UTC(), ServiceState: "UNAVAILABLE", Blockers: []string{}}
	if !s.opsAvailable() {
		return result, ErrClosed
	}
	err := s.opts.Pool.QueryRow(ctx, `SELECT
	 count(*) FILTER(WHERE lifecycle_state<>'CLOSED'),
	 (SELECT count(*) FROM poker.sessions WHERE state<>'SETTLED'),
	 (SELECT count(*) FROM poker.hands WHERE state<>'SETTLED'),
	 (SELECT count(*) FROM poker.recovery_state WHERE state='NEEDS_REVIEW'),
	 (SELECT count(*) FROM poker.funding_operations WHERE state='PENDING'),
	 (SELECT min(created_at) FROM poker.funding_operations WHERE state='PENDING'),clock_timestamp()
	 FROM poker.tables`).Scan(&result.OpenTables, &result.ActiveSessions, &result.HandsInProgress,
		&result.NeedsReviewHands, &result.PendingFunding, &result.OldestPendingFundingAt, &result.GeneratedAt)
	if err != nil {
		return result, err
	}
	result.ServiceState = "AVAILABLE"
	if result.NeedsReviewHands > 0 {
		result.Blockers = append(result.Blockers, "POKER_RECOVERY_NEEDS_REVIEW")
	}
	if result.PendingFunding > 0 {
		result.Blockers = append(result.Blockers, "POKER_FUNDING_PENDING")
	}
	result.RequiresAttention = len(result.Blockers) > 0
	return result, nil
}

func normalizeOpsPage(limit int) (int, error) {
	if limit == 0 {
		return 50, nil
	}
	if limit < 1 || limit > 100 {
		return 0, ErrInvalid
	}
	return limit, nil
}

func scanOpsTable(row pgx.Row) (OpsTable, error) {
	var result OpsTable
	var owner int64
	var version, epoch uint64
	var hand, recovery *string
	err := row.Scan(&result.TableID, &owner, &result.Name, &result.AccessMode, &result.MaxSeats,
		&result.OccupiedSeats, &result.SpectatorCount, &result.ChatEnabled, &result.LifecycleState,
		&result.AcceptingPlayers, &result.AllowNewHands, &version, &epoch, &hand, &result.NeedsReview,
		&recovery, &result.CreatedAt, &result.UpdatedAt, &result.ClosedAt)
	if err != nil {
		return result, err
	}
	result.OwnerUserID = strconv.FormatInt(owner, 10)
	result.TableVersion = strconv.FormatUint(version, 10)
	result.RuntimeEpoch = strconv.FormatUint(epoch, 10)
	if hand != nil {
		result.CurrentHandID = *hand
	}
	if recovery != nil {
		result.RecoveryState = *recovery
	}
	return result, nil
}

const opsTableSelect = `SELECT t.table_id::text,t.owner_newapi_user_id,t.name,t.access_mode,t.max_seats,
	count(seat.session_id),t.spectator_count,t.chat_enabled,t.lifecycle_state,t.accepting_players,t.allow_new_hands,
	t.table_version,t.runtime_epoch,t.current_hand_id::text,
	coalesce(recovery.state='NEEDS_REVIEW',false),recovery.state,t.created_at,t.updated_at,t.closed_at
	FROM poker.tables t LEFT JOIN poker.seats seat ON seat.table_id=t.table_id AND seat.session_id IS NOT NULL
	LEFT JOIN poker.recovery_state recovery ON recovery.table_id=t.table_id `

const opsTableGroup = ` GROUP BY t.table_id,recovery.state`

func (s *Service) ListOpsTables(ctx context.Context, query OpsTableQuery) (OpsTablePage, error) {
	result := OpsTablePage{GeneratedAt: time.Now().UTC(), Items: []OpsTable{}}
	if !s.opsAvailable() {
		return result, ErrClosed
	}
	limit, err := normalizeOpsPage(query.Limit)
	if err != nil || query.Cursor != "" && !canonicalSessionID(query.Cursor) {
		return result, ErrInvalid
	}
	rows, err := s.opts.Pool.Query(ctx, opsTableSelect+` WHERE ($1='' OR t.table_id<NULLIF($1,'')::uuid) AND ($3='' OR t.name ILIKE '%'||$3||'%') AND ($4='' OR t.lifecycle_state=$4) AND ($5::timestamptz IS NULL OR t.created_at >= $5) AND ($6::timestamptz IS NULL OR t.created_at < $6)`+opsTableGroup+
		` ORDER BY t.table_id DESC LIMIT $2`, query.Cursor, limit+1, query.Search, query.State, query.From, query.To)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		item, scanErr := scanOpsTable(rows)
		if scanErr != nil {
			return result, scanErr
		}
		result.Items = append(result.Items, item)
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	if len(result.Items) > limit {
		result.NextCursor = result.Items[limit-1].TableID
		result.Items = result.Items[:limit]
	}
	return result, nil
}

func (s *Service) readOpsTable(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, tableID string) (OpsTable, error) {
	if !canonicalSessionID(tableID) {
		return OpsTable{}, ErrInvalid
	}
	result, err := scanOpsTable(q.QueryRow(ctx, opsTableSelect+` WHERE t.table_id=$1`+opsTableGroup, tableID))
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrDenied
	}
	return result, err
}

func (s *Service) ReadOpsTable(ctx context.Context, tableID string) (OpsTableDetail, error) {
	result := OpsTableDetail{GeneratedAt: time.Now().UTC(), Players: []OpsPlayer{}, Messages: []OpsMessage{}, RecentEvents: []OpsSafeEvent{}}
	if !s.opsAvailable() {
		return result, ErrClosed
	}
	tx, err := s.opts.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ`); err != nil {
		return result, err
	}
	if result.Table, err = s.readOpsTable(ctx, tx, tableID); err != nil {
		return result, err
	}
	rows, err := tx.Query(ctx, `SELECT s.session_id::text,s.newapi_user_id,s.seat_no,s.display_name_snapshot,s.state,
	 s.current_stack_units::text,seat.connected,seat.leave_after_hand FROM poker.sessions s
	 JOIN poker.seats seat ON seat.table_id=s.table_id AND seat.seat_no=s.seat_no AND seat.session_id=s.session_id
	 WHERE s.table_id=$1 AND s.state<>'SETTLED' ORDER BY s.seat_no,s.session_id`, tableID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item OpsPlayer
		var user int64
		if err = rows.Scan(&item.SessionID, &user, &item.SeatNo, &item.DisplayName, &item.State,
			&item.StackUnits, &item.Connected, &item.LeaveAfterHand); err != nil {
			rows.Close()
			return result, err
		}
		item.UserID = strconv.FormatInt(user, 10)
		result.Players = append(result.Players, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()

	rows, err = tx.Query(ctx, `SELECT message_sequence::text,message_id::text,kind,body,messages.created_at,
	 coalesce(members.newapi_user_id,0),coalesce(members.member_id::text,''),coalesce(members.display_name_snapshot,''),
	 coalesce((SELECT CASE action WHEN 'HIDE' THEN 'HIDDEN' ELSE 'VISIBLE' END FROM poker.chat_moderation_events moderation
	  WHERE moderation.table_id=messages.table_id AND moderation.target_kind='MESSAGE' AND moderation.message_id=messages.message_id
	  ORDER BY moderation.created_at DESC,moderation.event_id DESC LIMIT 1),'VISIBLE')
	 FROM (SELECT * FROM poker.chat_messages WHERE table_id=$1 ORDER BY message_sequence DESC LIMIT 100) messages
	 LEFT JOIN poker.chat_members members ON members.table_id=messages.table_id AND members.member_id=messages.sender_member_id
	 ORDER BY message_sequence`, tableID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item OpsMessage
		var user int64
		var member, name string
		if err = rows.Scan(&item.Sequence, &item.MessageID, &item.Kind, &item.Body, &item.CreatedAt,
			&user, &member, &name, &item.ModerationState); err != nil {
			rows.Close()
			return result, err
		}
		if item.Kind == "USER_TEXT" {
			item.Author = &OpsMessageAuthor{UserID: strconv.FormatInt(user, 10), MemberID: member, DisplayName: name}
		}
		result.Messages = append(result.Messages, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()

	rows, err = tx.Query(ctx, `SELECT event_id::text,event_type,actor_kind,created_at,details
	 FROM (SELECT event_id,event_type,actor_kind,created_at,details FROM poker.audit_events WHERE table_id=$1 UNION ALL SELECT id,action,'ADMIN',occurred_at,jsonb_build_object('before',before_state,'after',after_state) FROM ops.audit_log WHERE target_type='POKER_TABLE' AND target=$1::text) events ORDER BY created_at DESC,event_id DESC LIMIT 100`, tableID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item OpsSafeEvent
		if err = rows.Scan(&item.EventID, &item.EventType, &item.ActorKind, &item.CreatedAt, &item.SafeDetails); err != nil {
			rows.Close()
			return result, err
		}
		result.RecentEvents = append(result.RecentEvents, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	if err = tx.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}

func scanOpsSession(row pgx.Row) (OpsSession, error) {
	var result OpsSession
	var user int64
	var initial, topup, stack int64
	var cashout, profit *int64
	var control uint64
	err := row.Scan(&result.SessionID, &result.TableID, &user, &result.SeatNo, &result.DisplayName,
		&result.State, &initial, &topup, &stack, &cashout, &profit, &control, &result.StartedAt,
		&result.EndedAt, &result.EndReason)
	if err != nil {
		return result, err
	}
	result.UserID = strconv.FormatInt(user, 10)
	result.InitialBuyInUnits = strconv.FormatInt(initial, 10)
	result.TotalTopUpUnits = strconv.FormatInt(topup, 10)
	result.CurrentStackUnits = strconv.FormatInt(stack, 10)
	result.ControlEpoch = strconv.FormatUint(control, 10)
	if cashout != nil {
		value := strconv.FormatInt(*cashout, 10)
		result.FinalCashOutUnits = &value
	}
	if profit != nil {
		value := strconv.FormatInt(*profit, 10)
		result.RealizedPLUnits = &value
	}
	return result, nil
}

const opsSessionSelect = `SELECT session_id::text,table_id::text,newapi_user_id,seat_no,display_name_snapshot,state,
	initial_buyin_units,total_topup_units,current_stack_units,final_cashout_units,realized_pl_units,control_epoch,
	started_at,ended_at,end_reason FROM poker.sessions`

func (s *Service) ListOpsSessions(ctx context.Context, query OpsSessionQuery) (OpsSessionPage, error) {
	result := OpsSessionPage{GeneratedAt: time.Now().UTC(), Items: []OpsSession{}}
	if !s.opsAvailable() {
		return result, ErrClosed
	}
	limit, err := normalizeOpsPage(query.Limit)
	if err != nil || query.TableID != "" && !canonicalSessionID(query.TableID) || query.Cursor != "" && !canonicalSessionID(query.Cursor) {
		return result, ErrInvalid
	}
	rows, err := s.opts.Pool.Query(ctx, opsSessionSelect+`
	 WHERE ($1='' OR table_id=NULLIF($1,'')::uuid) AND ($2='' OR session_id<NULLIF($2,'')::uuid) AND ($4::bigint=0 OR newapi_user_id=$4) AND ($5='' OR state=$5) AND ($6::timestamptz IS NULL OR started_at >= $6) AND ($7::timestamptz IS NULL OR started_at < $7)
	 ORDER BY session_id DESC LIMIT $3`, query.TableID, query.Cursor, limit+1, query.UserID, query.State, query.From, query.To)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		item, scanErr := scanOpsSession(rows)
		if scanErr != nil {
			return result, scanErr
		}
		result.Items = append(result.Items, item)
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	if len(result.Items) > limit {
		result.NextCursor = result.Items[limit-1].SessionID
		result.Items = result.Items[:limit]
	}
	return result, nil
}

func (s *Service) ReadOpsSession(ctx context.Context, sessionID string) (OpsSession, error) {
	if !s.opsAvailable() {
		return OpsSession{}, ErrClosed
	}
	if !canonicalSessionID(sessionID) {
		return OpsSession{}, ErrInvalid
	}
	result, err := scanOpsSession(s.opts.Pool.QueryRow(ctx, opsSessionSelect+` WHERE session_id=$1`, sessionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrDenied
	}
	return result, err
}

func (s *Service) ReadOpsHand(ctx context.Context, handID string) (OpsHand, error) {
	var result OpsHand
	if !s.opsAvailable() {
		return result, ErrClosed
	}
	if !canonicalSessionID(handID) {
		return result, ErrInvalid
	}
	var recovery *string
	err := s.opts.Pool.QueryRow(ctx, `SELECT h.hand_id::text,h.table_id::text,h.hand_no::text,h.state,
	 h.hand_version::text,h.event_sequence::text,
	 (SELECT count(*) FROM poker.dealt_cards cards WHERE cards.hand_id=h.hand_id AND cards.visibility='PUBLIC'),
	 (SELECT count(*) FROM poker.hand_participants participants WHERE participants.hand_id=h.hand_id),
	 coalesce((SELECT sum(amount_units) FROM poker.pots pots WHERE pots.hand_id=h.hand_id),0)::text,
	 h.created_at,h.settled_at,recovery.state
	 FROM poker.hands h LEFT JOIN poker.recovery_state recovery ON recovery.hand_id=h.hand_id
	 WHERE h.hand_id=$1`, handID).Scan(&result.HandID, &result.TableID, &result.HandNo, &result.State,
		&result.HandVersion, &result.EventSequence, &result.PublicBoardCount, &result.ParticipantCount,
		&result.TotalPotUnits, &result.CreatedAt, &result.SettledAt, &recovery)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrDenied
	}
	if err == nil && recovery != nil {
		result.RecoveryState = *recovery
	}
	return result, err
}

func validateOpsCommand(command OpsCommand) (int64, error) {
	if command.ActorUserID <= 0 || !canonicalSessionID(command.TableID) {
		return 0, ErrInvalid
	}
	var targetUser int64
	if command.CommandType == "POKER_REMOVE_PLAYER_AFTER_HAND" || command.CommandType == "POKER_REMOVE_SPECTATOR" || command.CommandType == "POKER_CHAT_MUTE_SET" {
		var err error
		targetUser, err = strconv.ParseInt(command.TargetUserID, 10, 64)
		if err != nil || targetUser <= 0 {
			return 0, ErrInvalid
		}
	}
	switch command.CommandType {
	case "POKER_ACCEPTING_PLAYERS_SET":
		if command.Accepting == nil {
			return 0, ErrInvalid
		}
	case "POKER_NEW_HANDS_SET":
		if command.AllowNewHands == nil {
			return 0, ErrInvalid
		}
	case "POKER_CLOSE_AFTER_HAND", "POKER_TABLE_PAUSE", "POKER_TABLE_RESUME", "POKER_EMERGENCY_PAUSE", "POKER_REMOVE_SPECTATOR":
	case "POKER_REMOVE_PLAYER_AFTER_HAND":
		if !canonicalSessionID(command.TargetSessionID) {
			return 0, ErrInvalid
		}
	case "POKER_CHAT_MUTE_SET":
		if command.Muted == nil {
			return 0, ErrInvalid
		}
	case "POKER_CHAT_VISIBILITY_SET":
		if !canonicalSessionID(command.MessageID) || command.Hidden == nil {
			return 0, ErrInvalid
		}
	case "POKER_RECOVERY_REQUEST":
		if !containsString([]string{"RECOVER", "RESUME", "SAFE_CLOSE", "ESCALATE"}, command.RecoveryAction) {
			return 0, ErrInvalid
		}
	default:
		return 0, ErrInvalid
	}
	return targetUser, nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

type opsNeedsReviewContextKey struct{}
type opsCommandContextKey struct{}

func allowOpsNeedsReview(ctx context.Context) context.Context {
	return context.WithValue(ctx, opsNeedsReviewContextKey{}, true)
}

func opsMayEnterNeedsReview(ctx context.Context) bool {
	allowed, _ := ctx.Value(opsNeedsReviewContextKey{}).(bool)
	return allowed
}

func withOpsCommand(ctx context.Context) context.Context {
	return context.WithValue(ctx, opsCommandContextKey{}, true)
}

func isOpsCommand(ctx context.Context) bool {
	opsCommand, _ := ctx.Value(opsCommandContextKey{}).(bool)
	return opsCommand
}

func (s *Service) ExecuteOpsCommand(ctx context.Context, command OpsCommand) (OpsResult, error) {
	targetUser, err := validateOpsCommand(command)
	if err != nil {
		return OpsResult{}, err
	}
	table, err := s.readOpsTable(ctx, s.opts.Pool, command.TableID)
	if err != nil {
		return OpsResult{}, err
	}
	if table.LifecycleState == "CLOSED" {
		return OpsResult{}, ErrConflict
	}
	if command.CommandType == "POKER_RECOVERY_REQUEST" || command.CommandType == "POKER_EMERGENCY_PAUSE" {
		ctx = allowOpsNeedsReview(ctx)
	}
	receipt, err := s.mutate(withOpsCommand(ctx), command.TableID, func(ctx context.Context, tx pgx.Tx, table *tableRow) (Receipt, error) {
		return s.applyOpsCommand(ctx, tx, table, command, targetUser)
	})
	return OpsResult{TableID: command.TableID, Status: receipt.Status}, err
}

// Keep table serialization, hand-boundary settlement and recovery in the existing actor.
func (s *Service) applyOpsCommand(ctx context.Context, tx pgx.Tx, table *tableRow, command OpsCommand, targetUser int64) (Receipt, error) {
	if err := platform.RequireLocalOpsAdmin(ctx, tx, command.ActorUserID); err != nil {
		return Receipt{}, err
	}
	if table.State == "CLOSED" {
		return Receipt{}, ErrConflict
	}
	before, err := s.opsChangeState(ctx, tx, table.ID, command, targetUser)
	if err != nil {
		return Receipt{}, err
	}
	resultCode := ""
	var removedUser int64
	switch command.CommandType {
	case "POKER_ACCEPTING_PLAYERS_SET":
		table.Accepting = *command.Accepting
		resultCode = map[bool]string{true: "ACCEPTING_PLAYERS", false: "NOT_ACCEPTING_PLAYERS"}[*command.Accepting]
	case "POKER_NEW_HANDS_SET":
		table.AllowHands = *command.AllowNewHands
		if !table.AllowHands && (table.State == "WAITING" || table.State == "INTERMISSION") {
			table.State = "PAUSED"
		} else if table.AllowHands && table.State == "PAUSED" {
			table.State = "INTERMISSION"
		}
		resultCode = map[bool]string{true: "NEW_HANDS_RESUMED", false: "NEW_HANDS_PAUSED"}[*command.AllowNewHands]
	case "POKER_CLOSE_AFTER_HAND":
		table.State, table.Accepting, table.AllowHands = "CLOSING", false, false
		if _, err = tx.Exec(ctx, `UPDATE poker.seats SET leave_after_hand=true
			 WHERE table_id=$1 AND session_id IS NOT NULL`, table.ID); err != nil {
			return Receipt{}, err
		}
		resultCode = "CLOSE_AFTER_HAND"
	case "POKER_REMOVE_PLAYER_AFTER_HAND":
		seats, loadErr := loadSeats(ctx, tx, table.ID)
		if loadErr != nil {
			return Receipt{}, loadErr
		}
		var target seatRow
		found := false
		for _, seat := range seats {
			if seat.Session == command.TargetSessionID && seat.User == targetUser {
				target, found = seat, true
				break
			}
		}
		if !found {
			return Receipt{}, ErrDenied
		}
		if _, err = tx.Exec(ctx, `UPDATE poker.sessions SET end_reason=coalesce(end_reason,'OPS_REMOVED')
			 WHERE session_id=$1`, target.Session); err != nil {
			return Receipt{}, err
		}
		active, activeErr := ownsHand(ctx, tx, target.Session)
		if activeErr != nil {
			return Receipt{}, activeErr
		}
		if active {
			_, err = tx.Exec(ctx, `UPDATE poker.seats SET state='LEAVE_AFTER_HAND',leave_after_hand=true
				 WHERE table_id=$1 AND seat_no=$2`, table.ID, target.Seat)
			resultCode = "PLAYER_REMOVE_AFTER_HAND"
		} else {
			if _, err = tx.Exec(ctx, `UPDATE poker.funding_operations SET state='FAILED_NO_EFFECT',failure_code='OPS_REMOVED'
				 WHERE session_id=$1 AND state='PENDING' AND kind IN('TOP_UP','REBUY')`, target.Session); err == nil {
				var funding Receipt
				funding, err = cashout(ctx, tx, table, target, "OPS_REMOVED")
				if err == nil && funding.Status != "CONFIRMED" {
					err = ErrDenied
				}
			}
			if err == nil {
				err = updateEmpty(ctx, tx, table)
			}
			resultCode = "PLAYER_REMOVED"
		}
		if err != nil {
			return Receipt{}, err
		}
	case "POKER_REMOVE_SPECTATOR":
		if !s.liveSpectator(ctx, tx, table, targetUser) {
			return Receipt{}, ErrDenied
		}
		removedUser, resultCode = targetUser, "SPECTATOR_REMOVED"
	case "POKER_CHAT_MUTE_SET":
		var targetExists bool
		if err = tx.QueryRow(ctx, `SELECT (EXISTS(SELECT 1 FROM poker.sessions WHERE table_id=$1 AND newapi_user_id=$2)
			  OR EXISTS(SELECT 1 FROM poker.chat_members WHERE table_id=$1 AND newapi_user_id=$2))`, table.ID, targetUser).Scan(&targetExists); err != nil {
			return Receipt{}, err
		}
		if !targetExists {
			return Receipt{}, ErrDenied
		}
		action := map[bool]string{true: "MUTE", false: "UNMUTE"}[*command.Muted]
		if _, err = tx.Exec(ctx, `INSERT INTO poker.chat_moderation_events(
			 table_id,target_kind,target_newapi_user_id,action,actor_user_id,actor_kind)
 VALUES($1,'USER',$2,$3,$4,'ADMIN')`, table.ID, targetUser, action, command.ActorUserID); err != nil {
			return Receipt{}, err
		}
		resultCode = "CHAT_USER_" + action + "D"
	case "POKER_CHAT_VISIBILITY_SET":
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM poker.chat_messages
			 WHERE table_id=$1 AND message_id=$2 AND kind='USER_TEXT')`, table.ID, command.MessageID).Scan(&exists); err != nil {
			return Receipt{}, err
		}
		if !exists {
			return Receipt{}, ErrDenied
		}
		action := map[bool]string{true: "HIDE", false: "RESTORE"}[*command.Hidden]
		if _, err = tx.Exec(ctx, `INSERT INTO poker.chat_moderation_events(
			 table_id,target_kind,message_id,action,actor_user_id,actor_kind)
 VALUES($1,'MESSAGE',$2,$3,$4,'ADMIN')`, table.ID, command.MessageID, action, command.ActorUserID); err != nil {
			return Receipt{}, err
		}
		resultCode = map[bool]string{true: "CHAT_MESSAGE_HIDDEN", false: "CHAT_MESSAGE_RESTORED"}[*command.Hidden]
	case "POKER_TABLE_PAUSE":
		table.AllowHands = false
		if table.State == "WAITING" || table.State == "INTERMISSION" {
			table.State = "PAUSED"
		}
		resultCode = "TABLE_PAUSED"
	case "POKER_TABLE_RESUME":
		if table.NeedsReview || table.State == "RECOVERING" {
			return Receipt{}, ErrNeedsReview
		}
		table.AllowHands = true
		if table.State == "PAUSED" {
			table.State = "INTERMISSION"
		}
		resultCode = "TABLE_RESUMED"
	case "POKER_RECOVERY_REQUEST":
		resultCode, err = s.applyOpsRecovery(ctx, tx, table, command)
		if err != nil {
			return Receipt{}, err
		}
	case "POKER_EMERGENCY_PAUSE":
		previous := table.recoveryPreviousState()
		table.Accepting, table.AllowHands, table.State = false, false, "RECOVERING"
		_, err = tx.Exec(ctx, `INSERT INTO poker.recovery_state(
			 table_id,hand_id,runtime_epoch,state,previous_table_state,reason_code,detected_at)
			 VALUES($1,NULLIF($2,'')::uuid,$3,'NEEDS_REVIEW',$4,'ADMIN_EMERGENCY_PAUSE',$5)
			 ON CONFLICT(table_id) DO UPDATE SET hand_id=excluded.hand_id,runtime_epoch=excluded.runtime_epoch,
			 state='NEEDS_REVIEW',previous_table_state=excluded.previous_table_state,reason_code=excluded.reason_code,
			 detected_at=excluded.detected_at,started_at=NULL,grace_until=NULL,resumed_at=NULL`,
			table.ID, table.HandID, table.Epoch, previous, table.Now)
		if err != nil {
			return Receipt{}, err
		}
		table.NeedsReview = true
		resultCode = "EMERGENCY_PAUSED"
	}
	if _, err = tx.Exec(ctx, `UPDATE poker.tables SET lifecycle_state=$2,accepting_players=$3,
		 allow_new_hands=$4,intermission_until=CASE WHEN $2='INTERMISSION' THEN $5::timestamptz+interval '5 seconds' ELSE intermission_until END
		 WHERE table_id=$1`, table.ID, table.State, table.Accepting, table.AllowHands, table.Now); err != nil {
		return Receipt{}, err
	}
	// A settled boundary can write CLOSED; do not overwrite it with CLOSING afterward.
	if command.CommandType == "POKER_CLOSE_AFTER_HAND" {
		if err = s.boundary(ctx, tx, table); err != nil {
			return Receipt{}, err
		}
	}
	after, err := s.opsChangeState(ctx, tx, table.ID, command, targetUser)
	if err != nil {
		return Receipt{}, err
	}
	after.Table.TableVersion = strconv.FormatUint(table.Version+1, 10)
	if err = platform.RecordOpsChange(ctx, tx, command.ActorUserID, command.CommandType, "POKER_TABLE", table.ID, before, map[string]any{"state": after, "command": command, "result": resultCode}); err != nil {
		return Receipt{}, err
	}
	if removedUser > 0 && s.opts.AfterSpectatorRemoved != nil {
		table.onCommit = func(context.Context) { s.opts.AfterSpectatorRemoved(table.ID, removedUser) }
	}
	return Receipt{TableID: table.ID, Status: resultCode, Version: table.Version + 1}, nil
}

type opsChangeState struct {
	Table         OpsTable        `json:"table"`
	Players       json.RawMessage `json:"players"`
	Muted         bool            `json:"muted"`
	MessageHidden bool            `json:"message_hidden"`
}

func (s *Service) opsChangeState(ctx context.Context, tx pgx.Tx, tableID string, command OpsCommand, targetUser int64) (opsChangeState, error) {
	var state opsChangeState
	var err error
	if state.Table, err = s.readOpsTable(ctx, tx, tableID); err != nil {
		return state, err
	}
	err = tx.QueryRow(ctx, `SELECT coalesce((SELECT jsonb_agg(jsonb_build_object('session_id',s.session_id,'user_id',s.newapi_user_id::text,'state',s.state,'stack_units',s.current_stack_units::text,'leave_after_hand',seat.leave_after_hand)) FROM poker.sessions s JOIN poker.seats seat ON seat.session_id=s.session_id WHERE s.table_id=$1),'[]'::jsonb),
 coalesce((SELECT action='MUTE' FROM poker.chat_moderation_events WHERE table_id=$1 AND target_kind='USER' AND target_newapi_user_id=$2 ORDER BY created_at DESC,event_id DESC LIMIT 1),false),
 coalesce((SELECT action='HIDE' FROM poker.chat_moderation_events WHERE table_id=$1 AND target_kind='MESSAGE' AND message_id=NULLIF($3,'')::uuid ORDER BY created_at DESC,event_id DESC LIMIT 1),false)`, tableID, targetUser, command.MessageID).Scan(&state.Players, &state.Muted, &state.MessageHidden)
	return state, err
}

func (s *Service) applyOpsRecovery(ctx context.Context, tx pgx.Tx, table *tableRow, command OpsCommand) (string, error) {
	switch command.RecoveryAction {
	case "RECOVER":
		if !table.NeedsReview {
			return "", ErrConflict
		}
		resultCode := "RECOVERY_RELEASED"
		if table.HandID != "" {
			receipt, err := s.recoverMutation(ctx, tx, table)
			if errors.Is(err, ErrCorruptSnapshot) {
				receipt, err = s.isolateHand(ctx, tx, table)
			}
			if err != nil {
				return "", err
			}
			if receipt.Status != "RECOVERED" && receipt.Status != "NOOP" {
				return receipt.Status, nil
			}
			if receipt.Status == "RECOVERED" {
				resultCode = receipt.Status
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE poker.recovery_state SET state='RESUMED',resumed_at=$2
			 WHERE table_id=$1 AND state='NEEDS_REVIEW'`, table.ID, table.Now)
		if err != nil {
			return "", err
		}
		if tag.RowsAffected() != 1 {
			return "", ErrConflict
		}
		table.NeedsReview = false
		table.State = "PAUSED"
		return resultCode, nil
	case "RESUME":
		var state string
		var grace *time.Time
		if err := tx.QueryRow(ctx, `SELECT state,grace_until FROM poker.recovery_state WHERE table_id=$1`, table.ID).Scan(&state, &grace); errors.Is(err, pgx.ErrNoRows) {
			return "", ErrConflict
		} else if err != nil {
			return "", err
		}
		if state == "NEEDS_REVIEW" || state == "GRACE" && (grace == nil || grace.After(table.Now)) {
			return "", ErrNeedsReview
		}
		if table.HandID != "" {
			receipt, err := s.tickMutation(ctx, tx, table)
			if err == nil && receipt.Status == "NOOP" {
				return "", ErrConflict
			}
			return receipt.Status, err
		}
		table.State, table.AllowHands = "INTERMISSION", true
		return "RECOVERY_RESUMED", nil
	case "SAFE_CLOSE":
		if !table.NeedsReview && table.State != "RECOVERING" {
			return "", ErrConflict
		}
		table.State, table.Accepting, table.AllowHands = "CLOSING", false, false
		if _, err := tx.Exec(ctx, `UPDATE poker.seats SET leave_after_hand=true
		 WHERE table_id=$1 AND session_id IS NOT NULL`, table.ID); err != nil {
			return "", err
		}
		return "RECOVERY_SAFE_CLOSE_QUEUED", nil
	case "ESCALATE":
		previous := table.recoveryPreviousState()
		table.State, table.Accepting, table.AllowHands, table.NeedsReview = "RECOVERING", false, false, true
		_, err := tx.Exec(ctx, `INSERT INTO poker.recovery_state(
		 table_id,hand_id,runtime_epoch,state,previous_table_state,reason_code,detected_at)
		 VALUES($1,NULLIF($2,'')::uuid,$3,'NEEDS_REVIEW',$4,'ADMIN_ESCALATED',$5)
		 ON CONFLICT(table_id) DO UPDATE SET hand_id=excluded.hand_id,runtime_epoch=excluded.runtime_epoch,
		 state='NEEDS_REVIEW',previous_table_state=excluded.previous_table_state,reason_code=excluded.reason_code,
		 detected_at=excluded.detected_at,started_at=NULL,grace_until=NULL,resumed_at=NULL`,
			table.ID, table.HandID, table.Epoch, previous, table.Now)
		return "RECOVERY_ESCALATED", err
	default:
		return "", ErrInvalid
	}
}
