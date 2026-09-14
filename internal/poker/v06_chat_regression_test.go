package poker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Current access grants from 0022, chat columns from 0026 and moderation from 0031.
// This fixture has real PG and synthetic Native/control/lease boundaries.
func v06PokerDB(t *testing.T) (*pgxpool.Pool, *pgxpool.Pool) {
	t.Helper()
	owner, runtime := localPokerDB(t)
	role := pgx.Identifier{runtime.Config().ConnConfig.User}.Sanitize()
	for _, sql := range []string{
		"GRANT SELECT,INSERT ON poker.table_access_credentials TO " + role,
		"GRANT SELECT(table_id,newapi_user_id,member_id,display_name_snapshot,avatar_id_snapshot), INSERT(table_id,newapi_user_id,member_id,display_name_snapshot,avatar_id_snapshot) ON poker.chat_members TO " + role,
		"GRANT SELECT(table_id,message_sequence,message_id,sender_member_id,kind,body,created_at), INSERT(table_id,message_sequence,message_id,sender_member_id,kind,body,created_at) ON poker.chat_messages TO " + role,
		"GRANT SELECT,INSERT ON poker.chat_moderation_events,poker.admin_operations TO " + role,
		"GRANT UPDATE(chat_sequence) ON poker.tables TO " + role,
		"GRANT SELECT(avatar_id) ON identity.master_profiles TO " + role,
		"UPDATE identity.master_profiles SET profile_version=1 WHERE newapi_user_id=910001",
	} {
		if _, err := owner.Exec(context.Background(), sql); err != nil {
			t.Fatal(err)
		}
	}
	return owner, runtime
}

func TestV06ChatCapabilityRequiresMemberProfile(t *testing.T) {
	owner, pool := v06PokerDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := owner.Exec(ctx, "UPDATE identity.master_profiles SET profile_version=0,display_name='',normalized_name=NULL,nickname_changed_at=NULL WHERE newapi_user_id=910003")
	must(err)
	s, err := legacyTestNew(Options{Pool: pool, Keyring: Keyring{Current: "v06", Keys: map[string][]byte{"v06": make([]byte, 32)}}, Leases: &fixtureLeases{entries: map[string]leaseEntry{}}, MailboxCapacity: 8})
	must(err)
	defer s.Close()
	created, err := s.CreateTable(ctx, CreateTableCommand{UserID: 910001, Key: "v06-chat-profile-table", Name: "Chat profile", BlindPreset: "5-10", MaxSeats: 2, AllowSpectators: true, ChatEnabled: true})
	must(err)
	ref := connected(t, s, fixtureRef(created.TableID, 910003), false).Ref
	view, err := s.ViewForConnection(ctx, ref)
	must(err)
	if view.Chat == nil {
		t.Fatal("chat view missing")
	}
	_, denied := s.SendChat(ctx, ChatCommand{UserID: 910003, Key: "v06-chat-unready-send", TableID: created.TableID, Body: "Not ready", Connection: ref})
	if !errors.Is(denied, ErrDenied) {
		t.Fatalf("uninitialized first sender: %v", denied)
	}
	if view.Chat.CanSend {
		t.Error("uninitialized first sender advertised can_send=true although SendChat denies")
	}
	var count int
	must(owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM poker.chat_members)+(SELECT count(*) FROM poker.chat_messages)").Scan(&count))
	if count != 0 {
		t.Fatal("denied send or read created chat state")
	}
	_, err = owner.Exec(ctx, "UPDATE identity.master_profiles SET profile_version=1,display_name='Synthetic 910003',normalized_name='Synthetic 910003' WHERE newapi_user_id=910003")
	must(err)
	view, err = s.ViewForConnection(ctx, ref)
	must(err)
	if view.Chat == nil || !view.Chat.CanSend {
		t.Fatal("initialized observer cannot send")
	}
	r, err := s.SendChat(ctx, ChatCommand{UserID: 910003, Key: "v06-chat-ready-send", TableID: created.TableID, Body: "Ready", Connection: ref})
	must(err)
	if r.Status != "CHAT_ACCEPTED" {
		t.Fatal("ready sender receipt missing")
	}
	// Existing members use their immutable table identity snapshot.
	_, err = owner.Exec(ctx, "UPDATE identity.master_profiles SET profile_version=0,display_name='',normalized_name=NULL,nickname_changed_at=NULL WHERE newapi_user_id=910003")
	must(err)
	view, err = s.ViewForConnection(ctx, ref)
	must(err)
	if view.Chat == nil || !view.Chat.CanSend {
		t.Fatal("existing member identity snapshot ignored")
	}
	_, err = s.SendChat(ctx, ChatCommand{UserID: 910003, Key: "v06-chat-member-send", TableID: created.TableID, Body: "Existing member", Connection: ref})
	must(err)
}
