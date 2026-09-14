package poker

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/history"
	"github.com/cy4268/momiao/internal/historyaccess"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/poker/fairness"
	"github.com/cy4268/momiao/internal/testutil/pokerhistoryfixture"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func v07Must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// All current migrations are installed by localPokerDB. Extra roles exist only
// inside its new network-isolated fixture and receive one History function each.
func v07HistoryRole(t *testing.T, owner, runtime *pgxpool.Pool, suffix, capability string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	cfg := owner.Config().Copy()
	cfg.AfterConnect = nil
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	v07Must(t, err)
	role := runtime.Config().ConnConfig.User + suffix
	quoted := pgx.Identifier{role}.Sanitize()
	password := runtime.Config().ConnConfig.Password
	created := false
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		if created {
			_, e := admin.Exec(context.Background(), "DROP OWNED BY "+quoted+"; DROP ROLE "+quoted, pgx.QueryExecModeSimpleProtocol)
			if e != nil {
				t.Error("owned History fixture role cleanup failed")
			}
		}
		admin.Close()
	})
	_, err = admin.Exec(ctx, "CREATE ROLE "+quoted+" LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '"+password+"'")
	v07Must(t, err)
	created = true
	_, err = admin.Exec(ctx, "GRANT USAGE ON SCHEMA games TO "+quoted+"; GRANT EXECUTE ON FUNCTION "+capability+" TO "+quoted, pgx.QueryExecModeSimpleProtocol)
	v07Must(t, err)
	cfg.ConnConfig.User, cfg.ConnConfig.Password = role, password
	pool, err = pgxpool.NewWithConfig(ctx, cfg)
	v07Must(t, err)
	return pool
}

func v07OwnerStore(t *testing.T, owner *pgxpool.Pool) *platform.Store {
	t.Helper()
	var role string
	v07Must(t, owner.QueryRow(context.Background(), "SELECT current_user").Scan(&role))
	cfg := owner.Config().ConnConfig.Copy()
	dsn := url.URL{Scheme: "postgres", Host: fmt.Sprintf("%s:%d", cfg.Host, cfg.Port), Path: "/" + cfg.Database, User: url.UserPassword(cfg.User, cfg.Password)}
	dsn.RawQuery = url.Values{"sslmode": {"disable"}, "options": {"-c role=" + role}}.Encode()
	store, err := platform.Open(context.Background(), dsn.String())
	v07Must(t, err)
	t.Cleanup(store.Close)
	return store
}

func TestV07HistoryProjectionRecovery(t *testing.T) {
	owner, runtime := localPokerDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	readerPool := v07HistoryRole(t, owner, runtime, "_reader", "games.history_list(bigint,jsonb)")
	workerPool := v07HistoryRole(t, owner, runtime, "_worker", "games.history_ingest_batch(text,integer)")
	r, w := history.NewReader(readerPool), history.NewWorker(workerPool)
	step := func() history.Progress {
		t.Helper()
		p, err := w.Step(ctx, history.Session, 1)
		v07Must(t, err)
		return p
	}
	list := func(user int64, q history.Query) history.Page {
		t.Helper()
		p, err := r.List(ctx, user, q)
		v07Must(t, err)
		return p
	}
	digest := func(sql string) string {
		t.Helper()
		var value string
		v07Must(t, owner.QueryRow(ctx, sql).Scan(&value))
		return value
	}
	_, err := owner.Exec(ctx, `INSERT INTO poker.tables(table_id,owner_newapi_user_id,name,max_seats,blind_preset_version,ruleset_version)
	 VALUES($1,910001,'V07 history fixture',3,'5-10','poker-cash-v1-20260906');
	 INSERT INTO poker.seats(table_id,seat_no) SELECT $1,n FROM generate_series(1,3)n`, pgx.QueryExecModeSimpleProtocol, historyID(701))
	v07Must(t, err)
	insert := `INSERT INTO poker.sessions(session_id,newapi_user_id,table_id,seat_no,display_name_snapshot,state,
	 initial_buyin_units,current_stack_units,final_cashout_units,realized_pl_units,started_at,ended_at)
	 VALUES($1,$2,$3,$4,'V07 Player','SETTLED',9007199255000000,0,9007199255000000,0,$5,$5::timestamptz+interval '1 hour')`
	late, err := owner.Begin(ctx)
	v07Must(t, err)
	defer late.Rollback(context.Background())
	_, err = late.Exec(ctx, insert, historyID(711), int64(910001), historyID(701), 1, "2026-09-09T00:00:00Z")
	v07Must(t, err)
	_, err = owner.Exec(ctx, insert, historyID(712), int64(910002), historyID(701), 2, "2026-09-10T00:00:00Z")
	v07Must(t, err)
	p := step()
	if p.SourceID == nil || *p.SourceID != historyID(712) || p.WrittenRows != 1 {
		t.Fatal("newer committed source did not advance alone")
	}
	v07Must(t, late.Commit(ctx))
	p = step()
	if p.MissingSources != 1 || p.SourceID == nil || *p.SourceID != historyID(712) {
		t.Fatal("late commit missing or cursor regressed")
	}
	page := list(910001, history.Query{RecordType: history.Session})
	if len(page.Items) != 1 || page.Items[0].NetChangeUnits == nil || *page.Items[0].NetChangeUnits != "0" || *page.Items[0].InitialBuyInUnits != "9007199255000000" || *page.Items[0].Result != "BREAK_EVEN" {
		t.Fatal("zero result or large integer detail lost")
	}
	stableIndex := `SELECT md5(jsonb_agg(to_jsonb(i)-ARRAY['checked_at','updated_at'] ORDER BY record_type,source_id,newapi_user_id)::text) FROM games.history_index i`
	before := digest(stableIndex)
	step()
	step()
	if digest(stableIndex) != before {
		t.Fatal("duplicate projection changed settled facts")
	}

	lock, err := owner.Begin(ctx)
	v07Must(t, err)
	defer lock.Rollback(context.Background())
	_, err = lock.Exec(ctx, "SELECT pg_advisory_xact_lock(1212765012,2)")
	v07Must(t, err)
	if !step().Busy {
		t.Fatal("competing worker was not excluded")
	}
	v07Must(t, lock.Rollback(ctx))
	_, err = owner.Exec(ctx, insert, historyID(713), int64(910001), historyID(701), 1, "2026-09-10T00:00:00Z")
	v07Must(t, err)
	business := `SELECT md5(jsonb_build_object('sessions',(SELECT jsonb_agg(to_jsonb(s) ORDER BY session_id) FROM poker.sessions s),'wallets',(SELECT jsonb_agg(to_jsonb(w) ORDER BY newapi_user_id,asset_type) FROM economy.wallet_balances w),'snapshots',(SELECT jsonb_agg(to_jsonb(d) ORDER BY record_type,source_id,newapi_user_id) FROM games.history_display_snapshots d))::text)`
	unchanged := digest(business)
	lock, err = owner.Begin(ctx)
	v07Must(t, err)
	defer lock.Rollback(context.Background())
	_, err = lock.Exec(ctx, "LOCK games.history_index IN ACCESS EXCLUSIVE MODE")
	v07Must(t, err)
	started := time.Now()
	_, err = w.Step(ctx, history.Session, 1)
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != "57014" || time.Since(started) > 4*time.Second {
		t.Fatal("worker SQL timeout is not bounded", err)
	}
	v07Must(t, lock.Rollback(ctx))
	w = history.NewWorker(workerPool)
	if step().WrittenRows != 1 {
		t.Fatal("new worker did not recover pending source after timeout rollback")
	}
	first := list(910001, history.Query{RecordType: history.Session, Limit: 1})
	if len(first.Items) != 1 || !first.HasMore || first.NextCursor == nil || first.Items[0].SourceID != historyID(713) {
		t.Fatal("first owner page differs")
	}
	second := list(910001, history.Query{RecordType: history.Session, Limit: 1, Cursor: *first.NextCursor})
	if len(second.Items) != 1 || second.HasMore || second.Items[0].SourceID != historyID(711) {
		t.Fatal("keyset continuation repeated or skipped a source")
	}
	for _, request := range []struct {
		user  int64
		query history.Query
	}{
		{910002, history.Query{RecordType: history.Session, Limit: 1, Cursor: *first.NextCursor}},
		{910001, history.Query{RecordType: history.Session, Result: "BREAK_EVEN", Limit: 1, Cursor: *first.NextCursor}},
	} {
		if _, err = r.List(ctx, request.user, request.query); !errors.Is(err, history.ErrQuery) {
			t.Fatal("foreign/rebound cursor accepted")
		}
	}
	filtered := list(910001, history.Query{Result: "BREAK_EVEN", TimeFrom: "2026-09-10T00:00:00Z", TimeTo: "2026-09-11T00:00:00Z"})
	if len(filtered.Items) != 1 || filtered.Items[0].SourceID != historyID(713) {
		t.Fatal("zero/time filters lost the source")
	}
	for pool, sql := range map[*pgxpool.Pool]string{readerPool: "SELECT games.history_ingest_batch('POKER_SESSION',1)", workerPool: "SELECT games.history_list(910001,'{}')"} {
		_, err = pool.Exec(ctx, sql)
		if !errors.As(err, &pgerr) || pgerr.Code != "42501" {
			t.Fatal("History capabilities were not split")
		}
		_, err = pool.Exec(ctx, "SELECT 1 FROM games.history_index")
		if !errors.As(err, &pgerr) || pgerr.Code != "42501" {
			t.Fatal("History role has raw index access")
		}
	}
	before = digest(stableIndex)
	_, err = owner.Exec(ctx, "TRUNCATE games.history_index,games.history_ingestion_cursors")
	v07Must(t, err)
	_, err = w.Step(ctx, history.Session, 200)
	v07Must(t, err)
	if digest(stableIndex) != before || digest(business) != unchanged {
		t.Fatal("projection recovery changed source, wallet or immutable display data")
	}
	t.Log("CURRENT33_HISTORY: delayed commit, duplicate ingestion, busy worker, bounded timeout/restart, zero/large integer, keyset ownership/filter and full index reconstruction passed")
}

func TestV07PokerHistoryReleasedProof(t *testing.T) {
	owner, runtime := localPokerDB(t)
	ctx := context.Background()
	store := v07OwnerStore(t, owner)
	role := pgx.Identifier{runtime.Config().ConnConfig.User}.Sanitize()
	_, err := owner.Exec(ctx, "GRANT USAGE ON SCHEMA games TO "+role+"; GRANT EXECUTE ON FUNCTION games.history_record_snapshot(bigint,text,uuid),poker.history_funding_transactions(bigint,uuid,uuid[]) TO "+role, pgx.QueryExecModeSimpleProtocol)
	v07Must(t, err)
	f := &pokerhistoryfixture.Fixture{Pool: owner, Poker: runtime, Owner: store}
	h := &pokerHistoryFixture{pg: f, keys: Keyring{Current: "history", Keys: map[string][]byte{"history": bytes.Repeat([]byte{7}, 32)}}, table: historyID(1), sessions: map[int]string{}, ageHours: 48}
	for seat := 1; seat <= 3; seat++ {
		user := int64(100 + seat)
		v07Must(t, store.EnsureAccount(ctx, user))
		f.SQL(t, "INSERT INTO identity.master_profiles(newapi_user_id,display_name,normalized_name) VALUES($1,$2,$2)", user, fmt.Sprint("V07 proof ", seat))
		_, err = store.Apply(ctx, platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: 5000000000, BizType: "V07_PROOF_FIXTURE", BizID: fmt.Sprint(user), EntryType: "GRANT", IdempotencyKey: "v07-proof-fixture-" + fmt.Sprint(user)})
		v07Must(t, err)
	}
	f.SQL(t, "INSERT INTO poker.tables(table_id,owner_newapi_user_id,name,max_seats,blind_preset_version,ruleset_version) VALUES($1,101,'V07 proof fixture',3,'5-10','poker-cash-v1-20260906')", h.table)
	for seat := 1; seat <= 3; seat++ {
		h.sessions[seat] = historyID(10 + seat)
		f.SQL(t, "INSERT INTO poker.seats(table_id,seat_no) VALUES($1,$2)", h.table, seat)
		h.fund(t, seat, "BUY_IN", int64(150+50*seat)*1000000)
	}
	state := h.hand(t, 1, "fold", 7)
	r, err := NewHistoryReader(runtime, h.keys)
	v07Must(t, err)
	before := h.fingerprint(t)
	v, err := r.HandFairness(ctx, historyaccess.Own(101), state.Config.HandID)
	v07Must(t, err)
	if !v.Released || len(v.Deck) != 52 {
		t.Fatal("past-due isolated hand did not reveal")
	}
	// This seed belongs only to the disposable, fixed-seed 48-hour fixture.
	publicProof, err := json.Marshal(map[string]any{"table_id": h.table, "proof": v})
	v07Must(t, err)
	t.Logf("V07_RELEASED_PROOF %s", publicProof)
	seed, err := hex.DecodeString(v.ServerSeed)
	v07Must(t, err)
	contributions := []fairness.Contribution{}
	for _, c := range v.Contributions {
		var version uint64
		_, err = fmt.Sscan(c.Version, &version)
		v07Must(t, err)
		contributions = append(contributions, fairness.Contribution{Seat: c.Seat, Version: version, Value: c.Value})
	}
	proof, err := fairness.Derive(h.table, state.Config.HandID, contributions, seed)
	v07Must(t, err)
	if hex.EncodeToString(proof.ServerSeedHash[:]) != v.ServerSeedHash || hex.EncodeToString(proof.DeckHash[:]) != v.DeckHash || hex.EncodeToString(proof.EffectiveSeed[:]) != v.EffectiveClientSeed {
		t.Fatal("released proof hashes failed recomputation")
	}
	for i, card := range proof.Deck {
		if int(card) != v.Deck[i] {
			t.Fatal("released deck differs from recomputation")
		}
	}
	d, err := r.HandDetail(ctx, historyaccess.Own(101), state.Config.HandID, HistoryHandQuery{})
	v07Must(t, err)
	if d.Fairness.Released {
		t.Fatal("ordinary hand detail released full fairness")
	}
	for _, p := range d.Participants {
		if (p.Seat == 1) != (len(p.HoleCards) == 2) || len(p.PublicHoleCards) != 0 {
			t.Fatal("folded private card visibility changed")
		}
	}
	raw, err := json.Marshal(d)
	v07Must(t, err)
	for _, forbidden := range [][]byte{[]byte(`"server_seed":`), []byte(`"deck":`), []byte(`"newapi_user_id":`), []byte(h.sessions[2]), []byte(h.sessions[3])} {
		if bytes.Contains(raw, forbidden) {
			t.Fatal("private identifier or full proof leaked in detail")
		}
	}
	_, err = r.HandFairness(ctx, historyaccess.Own(910003), state.Config.HandID)
	if !errors.Is(err, historyaccess.ErrNotFound) {
		t.Fatal("nonparticipant obtained full proof")
	}
	if h.fingerprint(t) != before {
		t.Fatal("History read or proof changed persisted business")
	}
	h.ageHours = 0
	fresh := h.hand(t, 2, "fold", 8)
	locked, err := r.HandFairness(ctx, historyaccess.Own(101), fresh.Config.HandID)
	v07Must(t, err)
	if locked.Released || locked.ServerSeed != "" || len(locked.Deck) != 0 {
		t.Fatal("fresh hand revealed before its delay")
	}
	f.SQL(t, "UPDATE poker.hands SET settled_at=settled_at-interval '48 hours' WHERE hand_id=$1; UPDATE poker.hand_fairness SET full_fairness_reveal_at=full_fairness_reveal_at-interval '48 hours' WHERE hand_id=$1", fresh.Config.HandID)
	bad, err := r.HandFairness(ctx, historyaccess.Own(101), fresh.Config.HandID)
	if !errors.Is(err, historyaccess.ErrUnavailable) || !reflect.DeepEqual(bad, FairnessView{}) {
		t.Fatal("mutable SQL dates bypassed authenticated settlement time")
	}
	t.Log("CURRENT33_PROOF: aged authenticated fixture revealed and recomputed; ordinary/private-owner isolation, read-only state and tampered-date rejection passed")
}
