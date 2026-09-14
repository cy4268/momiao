package poker

import (
	"context"
	"errors"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
)

const hostSpectatorLimit = 256

type HostView struct {
	IsHost               bool                 `json:"is_host"`
	Capabilities          []string             `json:"capabilities"`
	Players               []HostPlayerView     `json:"players"`
	Spectators            []HostSpectatorView  `json:"spectators"`
	SpectatorsTruncated   bool                 `json:"spectators_truncated"`
	ChatTargets           []HostChatTargetView `json:"chat_targets"`
	ChatTargetsTruncated  bool                 `json:"chat_targets_truncated"`
}

type HostPlayerView struct {
	TargetSessionID string `json:"target_session_id"`
	TargetUserID    string `json:"target_user_id"`
	SeatNo          int    `json:"seat_no"`
	DisplayName     string `json:"display_name"`
	RemovalPending  bool   `json:"removal_pending"`
	Muted           bool   `json:"muted"`
}

type HostSpectatorView struct {
	TargetUserID string `json:"target_user_id"`
	DisplayName  string `json:"display_name"`
	AvatarID     string `json:"avatar_id"`
	Muted        bool   `json:"muted"`
}

type HostChatTargetView struct {
	TargetUserID string `json:"target_user_id"`
	MemberID     string `json:"member_id"`
	DisplayName  string `json:"display_name"`
	AvatarID     string `json:"avatar_id"`
	Muted        bool   `json:"muted"`
}

func (s *Service) tableConnections(table string) []ControlRef {
	s.mu.Lock()
	defer s.mu.Unlock()
	refs := make([]ControlRef, 0)
	for _, ref := range s.connections {
		if ref.TableID == table {
			refs = append(refs, ref)
		}
	}
	return refs
}

func (s *Service) liveSpectatorRef(ctx context.Context, tx pgx.Tx, t *tableRow, ref ControlRef) bool {
	if !t.AllowSpectators || ref.UserID <= 0 || ref.UserID == t.Owner || ref.SessionID != "" || !s.live(ref, false) {
		return false
	}
	if s.validateAuth(ctx, ref) != nil || s.authorizeTableRow(ctx, tx, t, ref.auth()) != nil {
		return false
	}
	var active bool
	return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM poker.sessions
	 WHERE newapi_user_id=$1 AND state<>'SETTLED')`, ref.UserID).Scan(&active) == nil && !active
}

func (s *Service) liveSpectator(ctx context.Context, tx pgx.Tx, t *tableRow, user int64) bool {
	if user <= 0 {
		return false
	}
	for _, ref := range s.tableConnections(t.ID) {
		if ref.UserID == user && s.liveSpectatorRef(ctx, tx, t, ref) {
			return true
		}
	}
	return false
}

func readMutedUsers(ctx context.Context, tx pgx.Tx, table string) (map[int64]bool, error) {
	rows, err := tx.Query(ctx, `SELECT target_newapi_user_id FROM (
	 SELECT DISTINCT ON(target_newapi_user_id) target_newapi_user_id,action
	 FROM poker.chat_moderation_events WHERE table_id=$1 AND target_kind='USER'
	 ORDER BY target_newapi_user_id,created_at DESC,event_id DESC) current WHERE action='MUTE'`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	muted := map[int64]bool{}
	for rows.Next() {
		var user int64
		if err = rows.Scan(&user); err != nil {
			return nil, err
		}
		muted[user] = true
	}
	return muted, rows.Err()
}

func (s *Service) readHostView(ctx context.Context, tx pgx.Tx, t tableRow, seats []seatRow) (*HostView, error) {
	muted, err := readMutedUsers(ctx, tx, t.ID)
	if err != nil {
		return nil, err
	}
	view := &HostView{
		IsHost: true,
		Capabilities: []string{"PAUSE_ACCEPTING_PLAYERS", "RESUME_ACCEPTING_PLAYERS", "REMOVE_PLAYER_AFTER_HAND", "REMOVE_SPECTATOR", "MUTE_CHAT_USER", "CLOSE_TABLE"},
		Players: []HostPlayerView{}, Spectators: []HostSpectatorView{}, ChatTargets: []HostChatTargetView{},
	}
	for _, seat := range seats {
		view.Players = append(view.Players, HostPlayerView{
			TargetSessionID: seat.Session, TargetUserID: strconv.FormatInt(seat.User, 10), SeatNo: seat.Seat,
			DisplayName: seat.Display, RemovalPending: seat.Leave, Muted: muted[seat.User],
		})
	}

	seen := map[int64]bool{}
	for _, ref := range s.tableConnections(t.ID) {
		if seen[ref.UserID] || !s.liveSpectatorRef(ctx, tx, &t, ref) {
			continue
		}
		var name, avatar string
		if err = tx.QueryRow(ctx, `SELECT display_name,avatar_id FROM identity.master_profiles
		 WHERE newapi_user_id=$1 AND profile_version>0`, ref.UserID).Scan(&name, &avatar); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return nil, err
		}
		seen[ref.UserID] = true
		if len(view.Spectators) >= hostSpectatorLimit {
			view.SpectatorsTruncated = true
			break
		}
		view.Spectators = append(view.Spectators, HostSpectatorView{TargetUserID: strconv.FormatInt(ref.UserID, 10), DisplayName: name, AvatarID: avatar, Muted: muted[ref.UserID]})
	}
	sort.Slice(view.Spectators, func(i, j int) bool { return view.Spectators[i].TargetUserID < view.Spectators[j].TargetUserID })

	rows, err := tx.Query(ctx, `SELECT members.newapi_user_id,members.member_id::text,members.display_name_snapshot,members.avatar_id_snapshot
	 FROM poker.chat_members members
	 WHERE members.table_id=$1 AND EXISTS(SELECT 1 FROM poker.chat_messages messages
	   WHERE messages.table_id=members.table_id AND messages.sender_member_id=members.member_id)
	 ORDER BY (SELECT max(messages.message_sequence) FROM poker.chat_messages messages
	   WHERE messages.table_id=members.table_id AND messages.sender_member_id=members.member_id) DESC
	 LIMIT 101`, t.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var user int64
		var target HostChatTargetView
		if err = rows.Scan(&user, &target.MemberID, &target.DisplayName, &target.AvatarID); err != nil {
			return nil, err
		}
		target.TargetUserID, target.Muted = strconv.FormatInt(user, 10), muted[user]
		view.ChatTargets = append(view.ChatTargets, target)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(view.ChatTargets) > 100 {
		view.ChatTargetsTruncated = true
		view.ChatTargets = append([]HostChatTargetView{}, view.ChatTargets[:100]...)
	}
	return view, nil
}
