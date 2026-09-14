package history

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func proves(t *testing.T, f *historyFixture, query string, args ...any) {
	t.Helper()
	var ok bool
	checked(t, f.owner.QueryRow(context.Background(), query, args...).Scan(&ok))
	if !ok {
		t.Fatalf("history invariant failed: %s", query)
	}
}

func rejects(t *testing.T, pool *pgxpool.Pool, query, code string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), query)
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != code {
		t.Fatalf("expected %s for %s: %v", code, query, err)
	}
}

func TestPrivateHistory(t *testing.T) {
	f := newHistoryFixture(t)
	for _, permission := range []string{"TRUNCATE ON games.history_index", "CREATE ON SCHEMA games", "EXECUTE ON FUNCTION economy.poker_lobby_balance(bigint)"} {
		f.sql(t, "GRANT "+permission+" TO "+f.readerRole)
		f.grants(t, false)
		f.sql(t, "REVOKE "+permission+" FROM "+f.readerRole)
	}
	f.grants(t, true)
	ctx, cancelTest := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelTest()
	w, r := NewWorker(f.worker), NewReader(f.reader)
	list := func(user int64, query Query) Page {
		t.Helper()
		result, err := r.List(ctx, user, query)
		checked(t, err)
		return result
	}
	step := func(source Source, batch int) Progress {
		t.Helper()
		result, err := w.Step(ctx, source, batch)
		checked(t, err)
		return result
	}
	proves(t, f, "SELECT count(*)=21 FROM platform_meta.schema_migrations")
	id := func(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }
	sessionSQL := `INSERT INTO poker.sessions(session_id,newapi_user_id,table_id,seat_no,display_name_snapshot,
initial_buyin_units,current_stack_units,started_at) SELECT $1,$2,table_id,$3,'Player',200000000,200000000,$4 FROM poker.tables`
	f.sql(t, sessionSQL, id(12), 102, 2, "2026-09-02T00:00:00Z")
	for range 2 {
		step(Session, 1)
	}
	page := list(101, Query{})
	if len(page.Items) != 1 || page.Items[0].Snapshot.MetadataOrigin != "migration_metadata_backfill" || page.Items[0].NetChangeUnits != nil || page.Items[0].FinalCashOutUnits != nil {
		t.Fatal("legacy snapshot or owner projection")
	}
	proves(t, f, "SELECT metadata_origin='creation_snapshot' FROM games.history_display_snapshots WHERE source_id=$1", id(12))
	f.sql(t, `UPDATE poker.tables SET name='Renamed'; UPDATE games.game_registry SET publication_state='RETIRED' WHERE game_slug='texas-holdem';
INSERT INTO games.game_registry(game_slug,title,sort_order,publication_state,configured_runtime_state,implementation_key) VALUES('history-future','Future',99,'PUBLISHED','UNAVAILABLE','unimplemented')`)
	proves(t, f, "SELECT bool_and(table_name='Original table') FROM games.history_display_snapshots")
	for _, query := range []string{"UPDATE games.history_display_snapshots SET game_title='bad'", "DELETE FROM games.history_display_snapshots", "TRUNCATE games.history_display_snapshots"} {
		rejects(t, f.owner, query, "55000")
	}
	t.Log("SNAPSHOT: source-time metadata and honest legacy origin; renamed and retired source retained")

	f.sql(t, `UPDATE poker.sessions SET state='SETTLED',final_cashout_units=0,realized_pl_units=-200000000,
ended_at='2026-09-02T01:00:00Z' WHERE session_id=$1`, id(11))
	for range 3 {
		step(Session, 1)
	}
	proves(t, f, "SELECT display_status='SETTLED' AND result_class='LOSS' AND final_cashout_units=0 FROM games.history_index WHERE source_id=$1", id(11))
	a := f.transaction(t)
	_, err := a.Exec(ctx, sessionSQL, id(13), 101, 1, "2026-08-31T00:00:00Z")
	checked(t, err)
	f.sql(t, sessionSQL, id(14), 103, 3, "2026-09-03T00:00:00Z")
	progress := step(Session, 1)
	if progress.SourceID == nil || *progress.SourceID != id(14) {
		t.Fatal("newer committed source did not advance")
	}
	checked(t, a.Commit(ctx))
	progress = step(Session, 1)
	if progress.MissingSources != 1 || *progress.SourceID != id(14) {
		t.Fatal("late commit was lost or cursor moved backwards")
	}
	proves(t, f, "SELECT count(*)=1 FROM games.history_index WHERE source_id=$1", id(13))
	t.Log("LATE_COMMIT: A older/uncommitted; B newer/committed; cursor passed A; independent gap lane recovered A")

	f.sql(t, `INSERT INTO poker.hands(hand_id,table_id,hand_no,state,button_seat,runtime_epoch,setup_cipher,settled_at)
SELECT $1,table_id,1,'SETTLED',1,0,'\x00',clock_timestamp() FROM poker.tables;
INSERT INTO poker.hand_participants(hand_id,seat_no,session_id,newapi_user_id,hand_start_stack_units,contribution_version,contribution_cipher)
SELECT $1,seat_no,session_id,newapi_user_id,200000000,1,'\x00' FROM poker.sessions WHERE session_id IN($2,$3);
INSERT INTO poker.actions(hand_id,event_sequence,hand_version,event_type,actor_seat,applied_delta_units,payload_cipher,created_at)
SELECT $1,n,1,kind,seat,amount,'\x00',clock_timestamp() FROM (VALUES
(1,'POST_SB',1,5000000),(2,'POST_BB',2,10000000),(3,'CALL',1,10000000),(4,'BET',1,15000000),
(5,'RAISE',1,20000000),(6,'ALL_IN',1,30000000),(7,'ALL_IN',2,40000000),(8,'RETURN_UNCALLED',1,10000000),
(9,'IGNORED_EVENT',1,500000000)) v(n,kind,seat,amount);
INSERT INTO poker.pots VALUES($1,0,100000000,0,50000000),($1,1,20000000,50000000,70000000);
INSERT INTO poker.pot_eligible_players VALUES($1,0,1),($1,0,2),($1,1,1);
INSERT INTO poker.pot_awards VALUES($1,0,1,80000000,0,80000000),($1,0,2,20000000,0,20000000),($1,1,1,20000000,0,20000000);`, id(21), id(12), id(13))
	progress = step(Hand, 1)
	if progress.WrittenRows != 2 {
		t.Fatal("a Hand was split at the source cursor boundary")
	}
	page = list(101, Query{RecordType: Hand})
	if len(page.Items) != 1 || *page.Items[0].StakeUnits != "80000000" || *page.Items[0].NetChangeUnits != "30000000" {
		t.Fatal("hand amount whitelist or awards multiplicity")
	}
	proves(t, f, "SELECT sum(net_change_units)=0 AND count(*)=2 FROM games.history_index WHERE record_type='POKER_HAND'")
	t.Log("HAND: both participants, batch=1; six bet types, only RETURN_UNCALLED, each pot award once")

	guard := f.transaction(t)
	_, err = guard.Exec(ctx, "SELECT pg_advisory_xact_lock(1212765012,1)")
	checked(t, err)
	svc, err := games.NewService(f.store, games.Keyring{Active: "h1a-fixture", Keys: map[string][32]byte{"h1a-fixture": {1}}})
	checked(t, err)
	_, err = f.store.Apply(ctx, platform.Mutation{UserID: 101, Asset: platform.AvailableChips, DeltaUnits: 5000000000, BizType: "TEST_H1A", BizID: id(90), EntryType: "TEST_GRANT", IdempotencyKey: id(90)})
	checked(t, err)
	for game, input := range map[string]games.CreateInput{
		"dice": {Type: "DICE", Wager: "10", Choice: "BIG"}, "scratch": {Type: "SCRATCH", Wager: "10"},
		"summon": {Type: "SUMMON", BaseWager: "10", Mode: "SINGLE"}, "slot": {Type: "SLOT", TotalWager: "10"},
		"blackjack": {Type: "BLACKJACK", InitialWager: "10"},
	} {
		boot, e := svc.Bootstrap(ctx, 101, game)
		checked(t, e)
		_, e = svc.Create(ctx, 101, game, id(91), boot.Next.ID, input)
		checked(t, e)
	}
	checked(t, guard.Rollback(ctx))
	step(Round, 200)
	proves(t, f, "SELECT count(*)=5 AND count(DISTINCT game_slug)=5 FROM games.history_index WHERE record_type='DIRECT_PLAY_ROUND'")
	t.Log("ROUND: five real domain-created sources; snapshot INSERT completed while history advisory lock held")
	var diceAt time.Time
	for _, q := range []Query{{RecordType: Round, Mode: "DIRECT_PLAY", GameSlug: "dice", Status: "SETTLED"}, {RecordType: Session, Result: "LOSS", ID: id(11), TimeFrom: "2026-09-01T00:00:00+00:00", TimeTo: "2026-09-02T00:00:00Z"}, {ParentSourceID: id(13)}} {
		filtered := list(101, q)
		if len(filtered.Items) != 1 {
			t.Fatal("record/mode/game/status/result/id/time/parent filters")
		}
		if q.GameSlug == "dice" {
			diceAt = filtered.Items[0].OccurredAt
		}
	}

	page = list(101, Query{Limit: 1})
	if !page.HasMore || page.NextCursor == nil {
		t.Fatal("first keyset page")
	}
	for _, q := range []Query{{Limit: 101}, {Limit: -1}, {TimeFrom: "2026-09-01T00:00:00+08:00"}, {Limit: 1, Cursor: *page.NextCursor, Mode: "POKER"}, {Cursor: "invalid"}} {
		if _, err = r.List(ctx, 101, q); !errors.Is(err, ErrQuery) {
			t.Fatal("invalid or rebound query accepted")
		}
	}
	if _, err = r.List(ctx, 102, Query{Limit: 1, Cursor: *page.NextCursor}); !errors.Is(err, ErrQuery) {
		t.Fatal("foreign cursor")
	}
	retired, future := false, false
	for _, o := range page.GameOptions {
		retired = retired || (o.GameSlug == "texas-holdem" && o.Retired)
		future = future || o.GameSlug == "history-future"
	}
	if !retired || !future {
		t.Fatal("retired or dynamic future option missing")
	}
	for p, role := range map[*pgxpool.Pool]string{f.reader: f.readerRole, f.worker: f.workerRole} {
		var correctRole bool
		checked(t, p.QueryRow(ctx, "SELECT current_user=$1", role).Scan(&correctRole))
		if !correctRole {
			t.Fatal("fixture role mismatch")
		}
		for _, q := range []string{"SELECT setup_cipher FROM poker.hands", "SELECT * FROM economy.wallet_ledger", "SELECT * FROM games.history_index",
			"UPDATE games.history_ingestion_cursors SET source_id=NULL", "CREATE TABLE games.forbidden(id int)"} {
			rejects(t, p, q, "42501")
		}
	}
	rejects(t, f.reader, "SELECT games.history_ingest_batch('POKER_SESSION',1)", "42501")
	rejects(t, f.worker, "SELECT games.history_list(101,'{}')", "42501")
	t.Log("ACL: independent reader/worker, no source/history/ledger DML or SELECT, no cross-capability EXECUTE")

	lock := f.transaction(t)
	_, err = lock.Exec(ctx, "SELECT pg_advisory_xact_lock(1212765012,2); SELECT table_id FROM poker.tables FOR UPDATE")
	checked(t, err)
	progress = step(Session, 1)
	if !progress.Busy {
		t.Fatal("concurrent worker was not excluded")
	}
	list(101, Query{})
	checked(t, lock.Rollback(ctx))
	lock = f.transaction(t)
	_, err = lock.Exec(ctx, "SELECT session_id FROM poker.sessions FOR UPDATE; SELECT table_id FROM poker.tables FOR UPDATE")
	checked(t, err)
	step(Session, 1)
	list(101, Query{})
	checked(t, lock.Rollback(ctx))
	lock = f.transaction(t)
	_, err = lock.Exec(ctx, "LOCK games.history_index IN ACCESS EXCLUSIVE MODE")
	checked(t, err)
	startedTimeout := time.Now()
	_, err = w.Step(ctx, Session, 1)
	var timeout *pgconn.PgError
	if !errors.As(err, &timeout) || timeout.Code != "57014" || time.Since(startedTimeout) > 4*time.Second {
		t.Fatal("SQL timeout was not bounded", err)
	}
	checked(t, lock.Rollback(ctx))
	atomic := f.transaction(t)
	_, err = atomic.Exec(ctx, `UPDATE poker.sessions SET state='SETTLED',ended_at=clock_timestamp(),final_cashout_units=0,realized_pl_units=-200000000 WHERE session_id=$1;
INSERT INTO poker.sessions(session_id,newapi_user_id,table_id,seat_no,display_name_snapshot,initial_buyin_units,current_stack_units,started_at)
SELECT $2,newapi_user_id,table_id,seat_no,display_name_snapshot,initial_buyin_units,current_stack_units,'1900-01-01' FROM poker.sessions WHERE session_id=$1;
DELETE FROM games.history_index WHERE record_type='POKER_SESSION'; DELETE FROM games.history_ingestion_cursors WHERE source='POKER_SESSION';
SELECT games.history_ingest_batch('POKER_SESSION',1)`, pgx.QueryExecModeSimpleProtocol, id(13), id(15))
	checked(t, err)
	checked(t, atomic.Rollback(ctx))
	proves(t, f, "SELECT count(*)=4 FROM games.history_index WHERE record_type='POKER_SESSION'")
	proves(t, f, "SELECT source_id=$1 FROM games.history_ingestion_cursors WHERE source='POKER_SESSION'", id(14))
	proves(t, f, "SELECT NOT EXISTS(SELECT 1 FROM games.history_display_snapshots WHERE source_id=$1)", id(15))
	f.sql(t, "UPDATE games.history_index SET checked_at=occurred_at WHERE record_type='POKER_SESSION'; UPDATE poker.sessions SET state='NEEDS_REVIEW' WHERE session_id=$1", id(14))
	for range 3 {
		step(Session, 1)
	}
	proves(t, f, "SELECT display_status='RECOVERING' AND occurred_at='2026-09-03T00:00:00Z' FROM games.history_index WHERE source_id=$1", id(14))
	f.sql(t, `INSERT INTO poker.sessions(session_id,newapi_user_id,table_id,seat_no,display_name_snapshot,state,initial_buyin_units,current_stack_units,final_cashout_units,realized_pl_units,started_at,ended_at)
SELECT ('00000000-0000-4000-8000-'||lpad(n::text,12,'0'))::uuid,101,table_id,1,'Bulk','SETTLED',9007199255000000,0,9007199255000000,0,$1,$1 FROM poker.tables CROSS JOIN generate_series(200,251) n`, diceAt)
	step(Session, 200)
	page = list(101, Query{})
	if len(page.Items) != 50 || !page.HasMore {
		t.Fatal("default 50 page size")
	}
	page = list(101, Query{Limit: 100})
	if len(page.Items) != 59 || page.HasMore {
		t.Fatal("100 limit or default Hand exclusion")
	}
	q := Query{TimeFrom: diceAt.UTC().Format(time.RFC3339Nano), TimeTo: diceAt.Add(time.Microsecond).UTC().Format(time.RFC3339Nano), Limit: 1}
	page = list(101, q)
	q.Cursor = *page.NextCursor
	next := list(101, q)
	if page.Items[0].RecordType != Round || next.Items[0].SourceID != id(251) || *next.Items[0].InitialBuyInUnits != "9007199255000000" || *next.Items[0].NetChangeUnits != "0" {
		t.Fatal("mixed keyset order or large/zero integer strings")
	}
	if next.NextCursor == nil {
		t.Fatal("Session251 cursor missing")
	}
	q.Cursor = *next.NextCursor
	third := list(101, q)
	if len(third.Items) != 1 || third.Items[0].RecordType != Session || third.Items[0].SourceID != id(250) || third.Items[0].SourceID == next.Items[0].SourceID || !third.Items[0].OccurredAt.Equal(diceAt) {
		t.Fatal("same-time Session251 to Session250 keyset boundary")
	}
	t.Log("KEYSET: same timestamp, limit=1, Round -> Session251 -> Session250; no repeated source")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = w.Run(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal("Run swallowed cancellation")
	}
	started := time.Now()
	step(Session, 200)
	t.Logf("MEASURE: 5 Rounds/56 Sessions/1 Hand/2 participants; Step=%s, batch cap is not a scan-cost bound", time.Since(started))
	var before string
	checked(t, f.owner.QueryRow(ctx, "SELECT md5(jsonb_agg(to_jsonb(i)-ARRAY['updated_at','checked_at'] ORDER BY record_type,source_id,newapi_user_id)::text) FROM games.history_index i").Scan(&before))
	f.sql(t, "BEGIN; SELECT pg_advisory_xact_lock(1212765012,n) FROM generate_series(1,3) n; TRUNCATE games.history_index,games.history_ingestion_cursors; COMMIT")
	for _, source := range []Source{Round, Session, Hand} {
		step(source, 200)
	}
	proves(t, f, "SELECT md5(jsonb_agg(to_jsonb(i)-ARRAY['updated_at','checked_at'] ORDER BY record_type,source_id,newapi_user_id)::text)=$1 FROM games.history_index i", before)
	t.Log("REBUILD: all business columns identical after wiping only index/cursors")
}
