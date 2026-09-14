package poker

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

const chatHistoryLimit = 100

type ChatView struct {
	Enabled      bool              `json:"enabled"`
	CanSend      bool              `json:"can_send"`
	Muted        bool              `json:"muted"`
	LastSequence string            `json:"last_sequence"`
	Truncated    bool              `json:"truncated"`
	Messages     []ChatMessageView `json:"messages"`
}

type ChatMessageView struct {
	Sequence  string          `json:"sequence"`
	MessageID string          `json:"message_id"`
	Kind      string          `json:"kind"`
	Body      string          `json:"body"`
	CreatedAt time.Time       `json:"created_at"`
	Author    *ChatAuthorView `json:"author,omitempty"`
}

type ChatAuthorView struct {
	MemberID   string `json:"member_id"`
	DisplayName string `json:"display_name"`
	AvatarID    string `json:"avatar_id"`
}

type chatMember struct {
	ID, DisplayName, AvatarID string
}

func ensureChatMember(ctx context.Context, tx pgx.Tx, table string, user int64) (chatMember, error) {
	var member chatMember
	err := tx.QueryRow(ctx, `SELECT member_id::text,display_name_snapshot,avatar_id_snapshot
	 FROM poker.chat_members WHERE table_id=$1 AND newapi_user_id=$2`, table, user).Scan(&member.ID, &member.DisplayName, &member.AvatarID)
	if err == nil {
		return member, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return member, err
	}
	member.ID = uuid()
	tag, err := tx.Exec(ctx, `INSERT INTO poker.chat_members(table_id,newapi_user_id,member_id,display_name_snapshot,avatar_id_snapshot)
	 SELECT $1,$2,$3,p.display_name,p.avatar_id FROM identity.master_profiles p
	 WHERE p.newapi_user_id=$2 AND p.profile_version>0
	 ON CONFLICT(table_id,newapi_user_id) DO NOTHING`, table, user, member.ID)
	if err != nil {
		return chatMember{}, err
	}
	if tag.RowsAffected() == 0 {
		err = tx.QueryRow(ctx, `SELECT member_id::text,display_name_snapshot,avatar_id_snapshot
		 FROM poker.chat_members WHERE table_id=$1 AND newapi_user_id=$2`, table, user).Scan(&member.ID, &member.DisplayName, &member.AvatarID)
		if errors.Is(err, pgx.ErrNoRows) {
			return chatMember{}, ErrDenied
		}
		return member, err
	}
	err = tx.QueryRow(ctx, `SELECT display_name_snapshot,avatar_id_snapshot FROM poker.chat_members
	 WHERE table_id=$1 AND newapi_user_id=$2`, table, user).Scan(&member.DisplayName, &member.AvatarID)
	return member, err
}

func readChat(ctx context.Context, tx pgx.Tx, t tableRow, user int64) (*ChatView, error) {
	if !t.ChatEnabled {
		return nil, nil
	}
	view := &ChatView{Enabled: true, LastSequence: strconv.FormatUint(t.ChatSequence, 10), Messages: []ChatMessageView{}}
	if err := tx.QueryRow(ctx, `SELECT coalesce((SELECT action='MUTE' FROM poker.chat_moderation_events
	 WHERE table_id=$1 AND target_kind='USER' AND target_newapi_user_id=$2
	 ORDER BY created_at DESC,event_id DESC LIMIT 1),false)`, t.ID, user).Scan(&view.Muted); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT message_sequence::text,message_id::text,kind,body,recent.created_at,
	 coalesce(member_id::text,''),coalesce(display_name_snapshot,''),coalesce(avatar_id_snapshot,'')
	 FROM (SELECT table_id,message_sequence,message_id,sender_member_id,kind,body,created_at
	       FROM poker.chat_messages messages WHERE table_id=$1
	        AND NOT coalesce((SELECT moderation.action='HIDE' FROM poker.chat_moderation_events moderation
	         WHERE moderation.table_id=messages.table_id AND moderation.target_kind='MESSAGE'
	          AND moderation.message_id=messages.message_id
	         ORDER BY moderation.created_at DESC,moderation.event_id DESC LIMIT 1),false)
	       ORDER BY message_sequence DESC LIMIT $2) recent
	 LEFT JOIN poker.chat_members members
	   ON members.table_id=recent.table_id AND members.member_id=recent.sender_member_id
	 ORDER BY message_sequence`, t.ID, chatHistoryLimit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var message ChatMessageView
		var memberID, displayName, avatarID string
		if err = rows.Scan(&message.Sequence, &message.MessageID, &message.Kind, &message.Body, &message.CreatedAt, &memberID, &displayName, &avatarID); err != nil {
			return nil, err
		}
		if message.Kind == "USER_TEXT" {
			if memberID == "" || displayName == "" || avatarID == "" {
				return nil, ErrCorruptSnapshot
			}
			message.Author = &ChatAuthorView{MemberID: memberID, DisplayName: displayName, AvatarID: avatarID}
		}
		view.Messages = append(view.Messages, message)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(view.Messages) > chatHistoryLimit {
		view.Truncated = true
		view.Messages = append([]ChatMessageView{}, view.Messages[len(view.Messages)-chatHistoryLimit:]...)
	}
	return view, nil
}
