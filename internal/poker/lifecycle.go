package poker

import (
	"context"
	"encoding/json"
	"strconv"
	"github.com/cy4268/momiao/internal/poker/fairness"
	"github.com/jackc/pgx/v5"
)

type sessionChange func(context.Context, pgx.Tx, *tableRow, seatRow) (string, error)

func (s *Service) changeSession(ctx context.Context, c SessionCommand, scope string, change sessionChange) (Receipt, error) {
	return s.mutate(ctx, c.TableID, func(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
		seat, err := userSeat(ctx, tx, t.ID, c.UserID)
		if err != nil {
			return Receipt{}, err
		}
		if err = s.validateControl(ctx, tx, t, seat, c.Control); err != nil {
			return Receipt{}, err
		}
		intent := sessionIntent{c.UserID, c.TableID, seat.Session}
		if r, ok, err := findReceipt(ctx, tx, c.UserID, scope, c.Key, intent); err != nil || ok {
			return r, err
		}
		if err = s.guardMutation(ctx, tx, t, seat, c.Control, c.ExpectedTableVersion, c.ExpectedHandVersion, c.HandID); err != nil {
			return Receipt{}, err
		}
		if seat.Leave || t.State == "CLOSING" || t.State == "CLOSED" {
			return Receipt{}, ErrDenied
		}
		status, err := change(ctx, tx, t, seat)
		if err != nil {
			return Receipt{}, err
		}
		r := Receipt{TableID: t.ID, SessionID: seat.Session, Status: status, Version: t.Version + 1}
		return r, saveReceipt(ctx, tx, c.UserID, scope, c.Key, intent, r)
	})
}

func (s *Service) RequestSitOut(ctx context.Context, c SessionCommand) (Receipt, error) {
	return s.changeSession(ctx, c, "poker.sitout.v1", func(ctx context.Context, tx pgx.Tx, t *tableRow, seat seatRow) (string, error) {
		active, err := ownsHand(ctx, tx, seat.Session)
		if err != nil {
			return "", err
		}
		if active {
			_, err = tx.Exec(ctx, "UPDATE poker.seats SET sit_out_next_hand=true WHERE table_id=$1 AND seat_no=$2", t.ID, seat.Seat)
			return "SIT_OUT_NEXT_HAND", err
		}
		if seat.State == "REBUY_WINDOW" {
			return "", ErrDenied
		}
		_, err = tx.Exec(ctx, "UPDATE poker.seats SET state='SIT_OUT',sit_out_next_hand=true,sit_out_since=coalesce(sit_out_since,$3) WHERE table_id=$1 AND seat_no=$2", t.ID, seat.Seat, t.Now)
		return "SIT_OUT", err
	})
}
func (s *Service) ResumeSeat(ctx context.Context, c SessionCommand) (Receipt, error) {
	return s.changeSession(ctx, c, "poker.resume.v1", func(ctx context.Context, tx pgx.Tx, t *tableRow, seat seatRow) (string, error) {
		if seat.Stack <= 0 || !seat.Connected || seat.State != "SIT_OUT" {
			return "", ErrDenied
		}
		active, err := ownsHand(ctx, tx, seat.Session)
		if err != nil {
			return "", err
		}
		if active {
			return "", ErrDenied
		}
		_, err = tx.Exec(ctx, "UPDATE poker.seats SET state='WAITING_BIG_BLIND',sit_out_next_hand=false,sit_out_since=NULL,timeout_count=0,timeout_sit_out=false WHERE table_id=$1 AND seat_no=$2", t.ID, seat.Seat)
		return "WAITING_BIG_BLIND", err
	})
}
func (s *Service) connection(ctx context.Context, tx pgx.Tx, t *tableRow, seat seatRow, connected bool) error {
	active, err := ownsHand(ctx, tx, seat.Session)
	if err != nil {
		return err
	}
	if active {
		var status string
		if err = tx.QueryRow(ctx, "SELECT state FROM poker.hands WHERE hand_id=$1", t.HandID).Scan(&status); err != nil {
			return err
		}
		if status == "COMMITTED" {
			return ErrBusy
		}
		e, err := s.restoreEngine(ctx, tx, t.HandID)
		if err != nil {
			return err
		}
		if _, err = e.SetConnected(seat.Seat, connected, t.Now); err != nil {
			return err
		}
		return s.persistEngine(ctx, tx, t, e)
	}
	_, err = tx.Exec(ctx, "UPDATE poker.seats SET connected=$3,disconnected_since=CASE WHEN $3 THEN NULL ELSE coalesce(disconnected_since,$4) END WHERE table_id=$1 AND seat_no=$2", t.ID, seat.Seat, connected, t.Now)
	return err
}
func (s *Service) TakeControl(ctx context.Context, c SessionCommand) (Receipt, error) {
	// An account-level command never identifies a target socket. Retained only
	// to fail closed for older internal callers; use the typed TakeOver method.
	return Receipt{}, ErrNoControl
}
func (s *Service) SetConnected(ctx context.Context, c ConnectionCommand) (Receipt, error) {
	return Receipt{}, ErrNoControl
}

type NextSeedCommand struct {
	SessionCommand
	Contribution string
}

func (s *Service) SetNextSeed(ctx context.Context, c NextSeedCommand) (Receipt, error) {
	if !fairness.ValidContribution(c.Contribution) {
		return Receipt{}, ErrInvalid
	}
	// Include the new contribution in the idempotency fingerprint without
	// recording it as an event, log, audit field or public payload.
	return s.mutate(ctx, c.TableID, func(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
		seat, err := userSeat(ctx, tx, t.ID, c.UserID)
		if err != nil {
			return Receipt{}, err
		}
		if err = s.validateControl(ctx, tx, t, seat, c.Control); err != nil {
			return Receipt{}, err
		}
		intent := nextSeedIntent{sessionIntent{c.UserID, c.TableID, seat.Session}, c.Contribution}
		if r, ok, e := findReceipt(ctx, tx, c.UserID, "poker.nextseed.v1", c.Key, intent); e != nil || ok {
			return r, e
		}
		if err = s.guardMutation(ctx, tx, t, seat, c.Control, c.ExpectedTableVersion, c.ExpectedHandVersion, c.HandID); err != nil {
			return Receipt{}, err
		}
		if seat.Leave || t.State == "CLOSING" || t.State == "CLOSED" {
			return Receipt{}, ErrDenied
		}
		_, err = tx.Exec(ctx, "UPDATE poker.seats SET next_client_seed_contribution=$3,contribution_version=contribution_version+1 WHERE table_id=$1 AND seat_no=$2", t.ID, seat.Seat, c.Contribution)
		if err != nil {
			return Receipt{}, err
		}
		r := Receipt{TableID: t.ID, SessionID: seat.Session, Status: "NEXT_SEED_UPDATED", Version: t.Version + 1}
		return r, saveReceipt(ctx, tx, c.UserID, "poker.nextseed.v1", c.Key, intent, r)
	})
}
func (s *Service) HostControl(ctx context.Context, c HostCommand) (Receipt, error) {
	if !validIdentityKey(c.UserID, c.Key) || !canonicalSessionID(c.TableID) {
		return Receipt{}, ErrInvalid
	}
	return s.mutate(ctx, c.TableID, func(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
		if r, ok, err := findReceipt(ctx, tx, c.UserID, "poker.host.v1", c.Key, c); err != nil || ok {
			return r, err
		}
		if c.UserID != t.Owner || t.CloseRequested || t.State == "CLOSED" {
			return Receipt{}, ErrDenied
		}
		status := t.State
		var auditSession any
		removedUser := int64(0)
		switch c.Kind {
		case "PAUSE_ACCEPTING_PLAYERS":
			if c.TargetSessionID != "" || c.TargetUserID != 0 {
				return Receipt{}, ErrInvalid
			}
			t.Accepting = false
		case "RESUME_ACCEPTING_PLAYERS":
			if c.TargetSessionID != "" || c.TargetUserID != 0 {
				return Receipt{}, ErrInvalid
			}
			t.Accepting = true
		case "PAUSE":
			if c.TargetSessionID != "" || c.TargetUserID != 0 {
				return Receipt{}, ErrInvalid
			}
			t.AllowHands = false
			if t.State == "WAITING" || t.State == "INTERMISSION" {
				t.State = "PAUSED"
			}
			status = t.State
		case "RESUME":
			if c.TargetSessionID != "" || c.TargetUserID != 0 {
				return Receipt{}, ErrInvalid
			}
			t.AllowHands = true
			if t.State == "PAUSED" {
				t.State = "INTERMISSION"
			}
			status = t.State
		case "CLOSE", "CLOSE_TABLE":
			if c.TargetSessionID != "" || c.TargetUserID != 0 {
				return Receipt{}, ErrInvalid
			}
			t.State = "CLOSING"
			t.AllowHands = false
			t.Accepting = false
			status = "CLOSING"
		case "REMOVE_PLAYER_AFTER_HAND":
			if !canonicalSessionID(c.TargetSessionID) || c.TargetUserID <= 0 {
				return Receipt{}, ErrInvalid
			}
			seats, err := loadSeats(ctx, tx, t.ID)
			if err != nil {
				return Receipt{}, err
			}
			var target seatRow
			found := false
			for _, seat := range seats {
				if seat.Session == c.TargetSessionID && seat.User == c.TargetUserID {
					target, found = seat, true
					break
				}
			}
			if !found {
				return Receipt{}, ErrDenied
			}
			active, err := ownsHand(ctx, tx, target.Session)
			if err != nil {
				return Receipt{}, err
			}
			if _, err = tx.Exec(ctx, `UPDATE poker.sessions SET end_reason=coalesce(end_reason,'HOST_REMOVED')
			 WHERE session_id=$1`, target.Session); err != nil {
				return Receipt{}, err
			}
			if active {
				if _, err = tx.Exec(ctx, `UPDATE poker.seats SET state='LEAVE_AFTER_HAND',leave_after_hand=true
				 WHERE table_id=$1 AND seat_no=$2`, t.ID, target.Seat); err != nil {
					return Receipt{}, err
				}
				status = "PLAYER_REMOVE_AFTER_HAND"
			} else {
				if _, err = tx.Exec(ctx, `UPDATE poker.funding_operations SET state='FAILED_NO_EFFECT',failure_code='HOST_REMOVED'
				 WHERE session_id=$1 AND state='PENDING' AND kind IN('TOP_UP','REBUY')`, target.Session); err != nil {
					return Receipt{}, err
				}
				funding, err := cashout(ctx, tx, t, target, "HOST_REMOVED")
				if err != nil {
					return Receipt{}, err
				}
				if funding.Status != "CONFIRMED" {
					return Receipt{}, ErrDenied
				}
				if err = updateEmpty(ctx, tx, t); err != nil {
					return Receipt{}, err
				}
				status, removedUser = "PLAYER_REMOVED", target.User
			}
			auditSession = target.Session
		case "REMOVE_SPECTATOR":
			if c.TargetSessionID != "" || c.TargetUserID <= 0 {
				return Receipt{}, ErrInvalid
			}
			if !s.liveSpectator(ctx, tx, t, c.TargetUserID) {
				return Receipt{}, ErrDenied
			}
			status, removedUser = "SPECTATOR_REMOVED", c.TargetUserID
		case "MUTE_CHAT_USER":
			if c.TargetUserID <= 0 || c.TargetSessionID != "" && !canonicalSessionID(c.TargetSessionID) {
				return Receipt{}, ErrInvalid
			}
			if !t.ChatEnabled {
				return Receipt{}, ErrChatDisabled
			}
			validTarget := false
			if c.TargetSessionID != "" {
				var exists bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM poker.sessions s JOIN poker.seats seat
				 ON seat.table_id=s.table_id AND seat.seat_no=s.seat_no AND seat.session_id=s.session_id
				 WHERE s.table_id=$1 AND s.session_id=$2 AND s.newapi_user_id=$3 AND s.state<>'SETTLED')`, t.ID, c.TargetSessionID, c.TargetUserID).Scan(&exists); err != nil {
					return Receipt{}, err
				}
				validTarget, auditSession = exists, c.TargetSessionID
			} else {
				validTarget = s.liveSpectator(ctx, tx, t, c.TargetUserID)
				if !validTarget {
					if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM poker.chat_members members
					 WHERE members.table_id=$1 AND members.newapi_user_id=$2 AND EXISTS(
					  SELECT 1 FROM poker.chat_messages messages WHERE messages.table_id=members.table_id
					   AND messages.sender_member_id=members.member_id))`, t.ID, c.TargetUserID).Scan(&validTarget); err != nil {
						return Receipt{}, err
					}
				}
			}
			if !validTarget {
				return Receipt{}, ErrDenied
			}
			var muted bool
			if err := tx.QueryRow(ctx, `SELECT coalesce((SELECT action='MUTE' FROM poker.chat_moderation_events
			 WHERE table_id=$1 AND target_kind='USER' AND target_newapi_user_id=$2
			 ORDER BY created_at DESC,event_id DESC LIMIT 1),false)`, t.ID, c.TargetUserID).Scan(&muted); err != nil {
				return Receipt{}, err
			}
			if muted {
				return Receipt{}, ErrDenied
			}
			if _, err := tx.Exec(ctx, `INSERT INTO poker.chat_moderation_events(
			 table_id,target_kind,target_newapi_user_id,action,actor_user_id,actor_kind)
			 VALUES($1,'USER',$2,'MUTE',$3,'HOST')`, t.ID, c.TargetUserID, c.UserID); err != nil {
				return Receipt{}, err
			}
			status = "CHAT_USER_MUTED"
		default:
			return Receipt{}, ErrInvalid
		}
		_, err := tx.Exec(ctx, "UPDATE poker.tables SET lifecycle_state=$2,allow_new_hands=$3,accepting_players=$4,intermission_until=CASE WHEN $5='RESUME' THEN $6::timestamptz+interval '5 seconds' ELSE intermission_until END WHERE table_id=$1", t.ID, t.State, t.AllowHands, t.Accepting, c.Kind, t.Now)
		if err != nil {
			return Receipt{}, err
		}
		if c.Kind == "CLOSE" || c.Kind == "CLOSE_TABLE" {
			if _, err = tx.Exec(ctx, "UPDATE poker.seats SET leave_after_hand=true WHERE table_id=$1 AND session_id IS NOT NULL", t.ID); err != nil {
				return Receipt{}, err
			}
			if err = s.boundary(ctx, tx, t); err != nil {
				return Receipt{}, err
			}
		}
		_, requestHash := hashes(c.Key, c)
		details, _ := json.Marshal(struct {
			Command         string `json:"command"`
			TargetSessionID string `json:"target_session_id,omitempty"`
			TargetUserID    string `json:"target_user_id,omitempty"`
			Status          string `json:"status"`
		}{Command: c.Kind, TargetSessionID: c.TargetSessionID, TargetUserID: func() string {
			if c.TargetUserID <= 0 {
				return ""
			}
			return strconv.FormatInt(c.TargetUserID, 10)
		}(), Status: status})
		if _, err = tx.Exec(ctx, `INSERT INTO poker.audit_events(event_id,table_id,session_id,actor_user_id,actor_kind,event_type,request_hash,details)
		 VALUES($1,$2,$3,$4,'USER',$5,$6,$7)`, uuid(), t.ID, auditSession, c.UserID, "POKER_HOST_"+c.Kind, requestHash[:], details); err != nil {
			return Receipt{}, err
		}
		if removedUser > 0 && s.opts.AfterSpectatorRemoved != nil {
			t.onCommit = func(context.Context) { s.opts.AfterSpectatorRemoved(t.ID, removedUser) }
		}
		r := Receipt{TableID: t.ID, Status: status, Version: t.Version + 1}
		return r, saveReceipt(ctx, tx, c.UserID, "poker.host.v1", c.Key, c, r)
	})
}
