package poker

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Reuses the empty-chat regression against the existing isolated dev database.
// All new table, chat, moderation and audit rows stay inside one rolled-back transaction.
func TestReadOpsTableWithEmptyChat(t *testing.T) {
	path := os.Getenv("MOMIAO_POKER_DSN_FILE")
	if path == "" {
		t.Skip("requires isolated dev Poker database")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read dev Poker DSN")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := &Service{opts: Options{Pool: pool}}
	var emptyID string
	if err = pool.QueryRow(ctx, `SELECT table_id::text FROM poker.tables t WHERE NOT EXISTS(SELECT 1 FROM poker.chat_messages m WHERE m.table_id=t.table_id) LIMIT 1`).Scan(&emptyID); err != nil {
		t.Fatal(err)
	}
	detail, err := s.ReadOpsTable(ctx, emptyID)
	if err != nil || len(detail.Messages) != 0 {
		t.Fatal("empty chat projection", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var admin, regular int64
	if err = tx.QueryRow(ctx, `SELECT min(id) FILTER(WHERE role>=10),min(id) FILTER(WHERE role<10) FROM identity.users WHERE status=1 AND deleted_at IS NULL`).Scan(&admin, &regular); err != nil {
		t.Fatal(err)
	}
	tableID := uuid()
	if _, err = tx.Exec(ctx, `INSERT INTO poker.tables(table_id,owner_newapi_user_id,name,max_seats,blind_preset_version,ruleset_version,chat_enabled)
 SELECT $1,owner_newapi_user_id,'Ops rollback table',2,blind_preset_version,ruleset_version,true FROM poker.tables ORDER BY table_id LIMIT 1`, tableID); err != nil {
		t.Fatal(err)
	}
	table, err := loadTable(ctx, tx, tableID, true)
	if err != nil {
		t.Fatal(err)
	}
	denied := OpsCommand{ActorUserID: regular, TableID: tableID, CommandType: "POKER_TABLE_PAUSE"}
	if _, err = s.applyOpsCommand(ctx, tx, &table, denied, 0); !errors.Is(err, platform.ErrCatalogForbidden) {
		t.Fatal("regular actor", err)
	}
	run := func(command OpsCommand) Receipt {
		t.Helper()
		command.ActorUserID, command.TableID = admin, tableID
		target, e := validateOpsCommand(command)
		if e != nil {
			t.Fatal(e)
		}
		table, e := loadTable(ctx, tx, tableID, true)
		if e != nil {
			t.Fatal(e)
		}
		table.Now = time.Now().UTC()
		result, e := s.applyOpsCommand(ctx, tx, &table, command, target)
		if e != nil {
			t.Fatal(command.CommandType, e)
		}
		// The production actor performs this increment before its commit.
		if _, e = tx.Exec(ctx, `UPDATE poker.tables SET table_version=$2 WHERE table_id=$1`, tableID, result.Version); e != nil {
			t.Fatal(e)
		}
		return result
	}
	off, on := false, true
	run(OpsCommand{CommandType: "POKER_ACCEPTING_PLAYERS_SET", Accepting: &off})
	run(OpsCommand{CommandType: "POKER_TABLE_PAUSE"})
	state, err := s.readOpsTable(ctx, tx, tableID)
	if err != nil || state.AcceptingPlayers || state.AllowNewHands || state.LifecycleState != "PAUSED" {
		t.Fatal("pause", state, err)
	}
	run(OpsCommand{CommandType: "POKER_TABLE_RESUME"})
	member, err := ensureChatMember(ctx, tx, tableID, table.Owner)
	if err != nil {
		t.Fatal(err)
	}
	message := uuid()
	if _, err = tx.Exec(ctx, `INSERT INTO poker.chat_messages(table_id,message_sequence,message_id,sender_member_id,kind,body) VALUES($1,1,$2,$3,'USER_TEXT','Ops rollback message')`, tableID, message, member.ID); err != nil {
		t.Fatal(err)
	}
	systemMessage := uuid()
	if _, err = tx.Exec(ctx, `INSERT INTO poker.chat_messages(table_id,message_sequence,message_id,kind,body) VALUES($1,2,$2,'SYSTEM','System event')`, tableID, systemMessage); err != nil {
		t.Fatal(err)
	}
	current, err := loadTable(ctx, tx, tableID, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.applyOpsCommand(ctx, tx, &current, OpsCommand{ActorUserID: admin, TableID: tableID, CommandType: "POKER_CHAT_VISIBILITY_SET", MessageID: systemMessage, Hidden: &on}, 0); !errors.Is(err, ErrDenied) {
		t.Fatal("system message must remain visible", err)
	}
	run(OpsCommand{CommandType: "POKER_CHAT_MUTE_SET", TargetUserID: strconv.FormatInt(table.Owner, 10), Muted: &on})
	run(OpsCommand{CommandType: "POKER_CHAT_VISIBILITY_SET", MessageID: message, Hidden: &on})
	moderated, err := s.opsChangeState(ctx, tx, tableID, OpsCommand{MessageID: message}, table.Owner)
	if err != nil || !moderated.Muted || !moderated.MessageHidden {
		t.Fatal("moderation", err)
	}
	run(OpsCommand{CommandType: "POKER_CHAT_VISIBILITY_SET", MessageID: message, Hidden: &off})
	run(OpsCommand{CommandType: "POKER_EMERGENCY_PAUSE"})
	if r := run(OpsCommand{CommandType: "POKER_RECOVERY_REQUEST", RecoveryAction: "RECOVER"}); r.Status != "RECOVERY_RELEASED" {
		t.Fatal(r)
	}
	run(OpsCommand{CommandType: "POKER_CLOSE_AFTER_HAND"})
	state, err = s.readOpsTable(ctx, tx, tableID)
	if err != nil || state.LifecycleState != "CLOSED" {
		t.Fatal("closed", state, err)
	}
	table, err = loadTable(ctx, tx, tableID, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.applyOpsCommand(ctx, tx, &table, OpsCommand{ActorUserID: admin, TableID: tableID, CommandType: "POKER_TABLE_RESUME"}, 0); !errors.Is(err, ErrConflict) {
		t.Fatal("closed table", err)
	}
	var count, legacy int
	var before, after string
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM ops.audit_log WHERE target=$1`, tableID).Scan(&count); err != nil || count != 9 {
		t.Fatal("one audit per write", count, err)
	}
	if err = tx.QueryRow(ctx, `SELECT before_state->'table'->>'accepting_players',after_state->'state'->'table'->>'accepting_players' FROM ops.audit_log WHERE target=$1 AND action='POKER_ACCEPTING_PLAYERS_SET'`, tableID).Scan(&before, &after); err != nil || before != "true" || after != "false" {
		t.Fatal("audit before/after", before, after, err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM poker.admin_operations WHERE table_id=$1`, tableID).Scan(&legacy); err != nil || legacy != 0 {
		t.Fatal("legacy receipt created", err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM poker.tables WHERE table_id=$1)+(SELECT count(*) FROM ops.audit_log WHERE target=$1::text)`, tableID).Scan(&count); err != nil || count != 0 {
		t.Fatal("rollback", count, err)
	}
	t.Log("existing Poker runtime role: local actor; pause/resume, mute/hide/restore, emergency/recovery, empty-table close; nine audited writes, zero legacy receipts; rollback removed all new rows")
}
