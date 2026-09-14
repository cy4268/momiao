package poker

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const chatShortLimit int64 = 5
const chatMinuteLimit int64 = 30

type ChatCommand struct {
	UserID             int64
	Key, TableID, Body string
	Connection         ControlRef `json:"-"`
}

type chatIntent struct {
	UserID        int64
	TableID, Body string
}

func normalizeChatBody(body string) (string, bool) {
	if !utf8.ValidString(body) {
		return "", false
	}
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 2048 {
		return "", false
	}
	for _, r := range body {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "", false
		}
	}
	return body, true
}

func (s *Service) authorizeChatConnection(ctx context.Context, tx pgx.Tx, t *tableRow, ref ControlRef) error {
	if !s.live(ref, false) || ref.RuntimeEpoch != t.Epoch {
		return ErrNoControl
	}
	if err := s.validateAuth(ctx, ref); err != nil {
		return err
	}
	if err := s.authorizeTableRow(ctx, tx, t, ref.auth()); err != nil {
		return err
	}
	seat, err := userSeat(ctx, tx, t.ID, ref.UserID)
	if err == nil {
		if ref.SessionID == "" || ref.SessionID != seat.Session {
			return ErrNoControl
		}
		return nil
	}
	if !errors.Is(err, ErrDenied) {
		return err
	}
	if ref.SessionID != "" || !t.AllowSpectators && ref.UserID != t.Owner {
		return ErrDenied
	}
	var activeElsewhere bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM poker.sessions
	 WHERE newapi_user_id=$1 AND state<>'SETTLED')`, ref.UserID).Scan(&activeElsewhere); err != nil {
		return err
	}
	if activeElsewhere {
		return ErrDenied
	}
	return nil
}

func (s *Service) canSendChat(ctx context.Context, ref ControlRef) bool {
	tx, err := s.opts.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return false
	}
	defer rollback(tx)
	t, err := loadTable(ctx, tx, ref.TableID, false)
	if err != nil || !t.ChatEnabled || s.authorizeChatConnection(ctx, tx, &t, ref) != nil {
		return false
	}
	// Match ensureChatMember without creating a member during a read. Existing
	// table members retain their identity snapshot; first senders need a profile.
	var ready bool
	err = tx.QueryRow(ctx, `SELECT
	 EXISTS(SELECT 1 FROM poker.chat_members WHERE table_id=$1 AND newapi_user_id=$2)
	 OR EXISTS(SELECT 1 FROM identity.master_profiles WHERE newapi_user_id=$2 AND profile_version>0)`, t.ID, ref.UserID).Scan(&ready)
	return err == nil && ready
}

// SendChat serializes chat with table mutations. The connection, Native
// session, password grant and table membership are all rechecked immediately
// before commit; a browser-selected identity is never accepted.
func (s *Service) SendChat(ctx context.Context, c ChatCommand) (Receipt, error) {
	body, ok := normalizeChatBody(c.Body)
	if !ok || !validIdentityKey(c.UserID, c.Key) || !canonicalSessionID(c.TableID) || c.Connection.UserID != c.UserID || c.Connection.TableID != c.TableID {
		return Receipt{}, ErrInvalid
	}
	c.Body = body
	ref, err := s.connectionRef(c.Connection)
	if err != nil {
		return Receipt{}, err
	}
	if err = s.AuthorizeTable(ctx, c.TableID, ref.auth()); err != nil {
		return Receipt{}, err
	}
	if err = s.validateAuth(ctx, ref); err != nil {
		return Receipt{}, err
	}
	intent := chatIntent{UserID: c.UserID, TableID: c.TableID, Body: c.Body}
	return s.mutate(ctx, c.TableID, func(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
		if err := s.authorizeChatConnection(ctx, tx, t, ref); err != nil {
			return Receipt{}, err
		}
		if !t.ChatEnabled {
			return Receipt{}, ErrChatDisabled
		}
		if receipt, found, err := findReceipt(ctx, tx, c.UserID, "poker.chat.v1", c.Key, intent); err != nil || found {
			return receipt, err
		}
		var muted bool
		if err := tx.QueryRow(ctx, `SELECT coalesce((SELECT action='MUTE' FROM poker.chat_moderation_events
		 WHERE table_id=$1 AND target_kind='USER' AND target_newapi_user_id=$2
		 ORDER BY created_at DESC,event_id DESC LIMIT 1),false)`, t.ID, c.UserID).Scan(&muted); err != nil {
			return Receipt{}, err
		}
		if muted {
			return Receipt{}, ErrChatMuted
		}
		member, err := ensureChatMember(ctx, tx, t.ID, c.UserID)
		if err != nil {
			return Receipt{}, err
		}
		var recent, minute int64
		if err = tx.QueryRow(ctx, `SELECT
		 count(*) FILTER(WHERE created_at>$3::timestamptz-interval '10 seconds'),
		 count(*) FILTER(WHERE created_at>$3::timestamptz-interval '60 seconds')
		 FROM poker.chat_messages WHERE table_id=$1 AND sender_member_id=$2`, t.ID, member.ID, t.Now).Scan(&recent, &minute); err != nil {
			return Receipt{}, err
		}
		if recent >= chatShortLimit || minute >= chatMinuteLimit {
			return Receipt{}, ErrRateLimited
		}
		if err = tx.QueryRow(ctx, `UPDATE poker.tables SET chat_sequence=chat_sequence+1
		 WHERE table_id=$1 RETURNING chat_sequence`, t.ID).Scan(&t.ChatSequence); err != nil {
			return Receipt{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO poker.chat_messages(table_id,message_sequence,message_id,sender_member_id,kind,body,created_at)
		 VALUES($1,$2,$3,$4,'USER_TEXT',$5,$6)`, t.ID, t.ChatSequence, uuid(), member.ID, c.Body, t.Now); err != nil {
			return Receipt{}, err
		}
		receipt := Receipt{TableID: t.ID, Status: "CHAT_ACCEPTED", ChatSequence: decimal(int64(t.ChatSequence)), Version: t.Version + 1}
		if err = saveReceipt(ctx, tx, c.UserID, "poker.chat.v1", c.Key, intent, receipt); err != nil {
			return Receipt{}, err
		}
		t.beforeCommit = func(ctx context.Context, _ *Receipt) error {
			return s.authorizeChatConnection(ctx, tx, t, ref)
		}
		return receipt, nil
	})
}
