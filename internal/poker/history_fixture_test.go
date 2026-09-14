package poker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/cy4268/momiao/internal/poker/fairness"
	"github.com/cy4268/momiao/internal/testutil/pokerhistoryfixture"
	"github.com/jackc/pgx/v5"
)

type pokerHistoryFixture struct {
	pg       *pokerhistoryfixture.Fixture
	keys     Keyring
	table    string
	sessions map[int]string
	ageHours int
}

func historyID(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

func newPokerHistoryFixture(t *testing.T) *pokerHistoryFixture {
	f := pokerhistoryfixture.Open(t)
	h := &pokerHistoryFixture{pg: f, keys: Keyring{Current: "history", Keys: map[string][]byte{"history": bytes.Repeat([]byte{7}, 32)}}, table: historyID(1), sessions: map[int]string{}, ageHours: 48}
	f.SQL(t, `INSERT INTO identity.master_profiles(newapi_user_id,display_name,normalized_name) VALUES(103,'Player C','player c'); INSERT INTO poker.tables(table_id,owner_newapi_user_id,name,max_seats,blind_preset_version,ruleset_version) VALUES($1,101,'History Table',3,'5-10','poker-cash-v1-20260906')`, h.table)
	for seat := 1; seat <= 3; seat++ {
		user := int64(100 + seat)
		_, err := f.Owner.Apply(context.Background(), platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: 5000000000, BizType: "HISTORY_FIXTURE", BizID: fmt.Sprint(user), EntryType: "GRANT", IdempotencyKey: "poker-history-" + fmt.Sprint(user)})
		pokerhistoryfixture.Check(t, err)
		h.sessions[seat] = historyID(10 + seat)
		f.SQL(t, `INSERT INTO poker.seats(table_id,seat_no) VALUES($1,$2)`, h.table, seat)
		h.fund(t, seat, "BUY_IN", int64(150+50*seat)*1000000)
	}
	return h
}

func (h *pokerHistoryFixture) fund(t *testing.T, seat int, kind string, amount int64) string {
	t.Helper()
	op, transaction, ledger, reservation := uuid(), uuid(), uuid(), uuid()
	if kind == "BUY_IN" {
		h.pg.SQL(t, `INSERT INTO poker.seat_reservations(reservation_id,table_id,seat_no,newapi_user_id,lease_token,state,expires_at) VALUES($1,$2,$3,$4,'fixture','LEASE_ACTIVE',clock_timestamp()+interval '1 hour')`, reservation, h.table, seat, 100+seat)
	} else {
		reservation = ""
	}
	h.pg.SQL(t, `INSERT INTO poker.funding_operations(funding_operation_id,table_id,seat_no,session_id,newapi_user_id,kind,amount_units,reservation_id,request_hash,planned_transaction_id,planned_ledger_id) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,decode(repeat('00',32),'hex'),$9,$10)`, op, h.table, seat, h.sessions[seat], 100+seat, kind, amount, reservation, transaction, ledger)
	fn := map[string]string{"BUY_IN": "poker_buy_in_apply", "TOP_UP": "poker_top_up_apply", "REBUY": "poker_top_up_apply", "CASH_OUT": "poker_cash_out_apply"}[kind]
	var raw []byte
	pokerhistoryfixture.Check(t, h.pg.Pool.QueryRow(context.Background(), "SELECT economy."+fn+"($1,0,$2)", h.table, op).Scan(&raw))
	var receipt struct {
		TransactionID string `json:"transaction_id"`
	}
	pokerhistoryfixture.Check(t, json.Unmarshal(raw, &receipt))
	if receipt.TransactionID != transaction {
		t.Fatal("real funding gateway did not confirm expected transaction")
	}
	return transaction
}

func (h *pokerHistoryFixture) hand(t *testing.T, number int, mode string, seedByte byte) engine.State {
	t.Helper()
	ctx, id := context.Background(), historyID(1000+number)
	var now time.Time
	pokerhistoryfixture.Check(t, h.pg.Pool.QueryRow(ctx, "SELECT clock_timestamp()-$1::integer*interval '1 hour'", h.ageHours).Scan(&now))
	setup := handSetup{Sessions: h.sessions}
	for seat := 1; seat <= 3; seat++ {
		var stack int64
		pokerhistoryfixture.Check(t, h.pg.Pool.QueryRow(ctx, "SELECT current_stack_units FROM poker.sessions WHERE session_id=$1", h.sessions[seat]).Scan(&stack))
		setup.Seats = append(setup.Seats, engine.Seat{SeatNo: seat, PlayerID: fmt.Sprint(100 + seat), Stack: stack})
		setup.Contributions = append(setup.Contributions, fairness.Contribution{Seat: seat, Version: 1, Value: fmt.Sprint("fixture-", seat)})
	}
	if mode == "advance" {
		var sealed []byte
		pokerhistoryfixture.Check(t, h.pg.Pool.QueryRow(ctx, "SELECT setup_cipher,created_at FROM poker.hands WHERE hand_id=$1", id).Scan(&sealed, &now))
		plain, err := openCipher(h.keys, id+":setup", sealed)
		pokerhistoryfixture.Check(t, err)
		pokerhistoryfixture.Check(t, json.Unmarshal(plain, &setup))
	}
	seed := bytes.Repeat([]byte{seedByte}, 32)
	proof, err := fairness.Derive(h.table, id, setup.Contributions, seed)
	pokerhistoryfixture.Check(t, err)
	deck := make([]engine.Card, 52)
	for i, card := range proof.Deck {
		deck[i] = engine.Card(card)
	}
	setup.Config = engine.Config{HandID: id, ButtonSeat: (number-1)%3 + 1, SmallBlind: 2500000, BigBlind: 5000000, Deck: deck, EvaluatorVersion: engine.EvaluatorVersion}
	e, err := engine.New(setup.Config, setup.Seats, now, engine.Evaluate)
	pokerhistoryfixture.Check(t, err)
	for step := 0; mode != "committed" && e.State().Street != engine.Settled; step++ {
		if mode == "active" || (mode == "advance" && step == 1) {
			break
		}
		if step > 1000 {
			t.Fatal("legal hand fixture failed to terminate")
		}
		state, legal, kind, to := e.State(), e.LegalActions(), engine.Check, int64(0)
		if legal.ToCall > 0 {
			kind = engine.Call
		}
		switch mode {
		case "fold":
			kind = engine.Fold
		case "allin":
			kind = engine.AllIn
		case "odd":
			if state.Actor == state.SmallBlindSeat && state.Street == engine.Preflop {
				kind = engine.Fold
			}
		case "long":
			if step%3 != 0 && legal.RaiseRights && legal.MinimumRaiseTo <= legal.MaximumRaiseTo {
				kind, to = engine.Raise, legal.MinimumRaiseTo
			}
		}
		_, err = e.Apply(engine.Action{ID: fmt.Sprint("history-", step), Seat: state.Actor, ExpectedVersion: state.Version, Kind: kind, To: to}, now.Add(time.Duration(step+1)*time.Microsecond))
		pokerhistoryfixture.Check(t, err)
	}
	state := e.State()
	snapshot, err := e.Snapshot()
	pokerhistoryfixture.Check(t, err)
	_, err = engine.Restore(snapshot, engine.Evaluate)
	pokerhistoryfixture.Check(t, err) // Every dealt source is valid before any reader assertion.
	sealer := &Service{opts: Options{Keyring: h.keys}}
	seal := func(purpose string, plain []byte) []byte {
		cipher, err := sealer.seal(id+":"+purpose, plain)
		pokerhistoryfixture.Check(t, err)
		return cipher
	}
	raw, err := json.Marshal(setup)
	pokerhistoryfixture.Check(t, err)
	status, version, sequence, cursor := string(state.Street), state.Version, len(state.Events), state.DeckCursor
	var settled *time.Time
	var cipher, hash []byte
	if mode == "committed" {
		status, version, sequence, cursor = "COMMITTED", 0, 0, 0
	} else {
		sum := sha256.Sum256(snapshot)
		cipher, hash = seal("snapshot", snapshot), sum[:]
		if state.Street == engine.Settled {
			at := state.Events[len(state.Events)-1].At
			settled = &at
		}
	}
	h.pg.SQL(t, `INSERT INTO poker.hands(hand_id,table_id,hand_no,state,button_seat,runtime_epoch,setup_cipher,snapshot_cipher,snapshot_hash,hand_version,event_sequence,created_at,settled_at)
	 VALUES($1,$2,$3,$4,$5,0,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT(hand_id) DO UPDATE SET
	 state=excluded.state,snapshot_cipher=excluded.snapshot_cipher,snapshot_hash=excluded.snapshot_hash,hand_version=excluded.hand_version,event_sequence=excluded.event_sequence,settled_at=excluded.settled_at`, id, h.table, number, status, setup.Config.ButtonSeat, seal("setup", raw), cipher, hash, version, sequence, now, settled)
	h.pg.SQL(t, `INSERT INTO poker.hand_fairness(hand_id,server_seed_hash,encrypted_server_seed,server_seed_key_version,effective_client_seed_hash,algorithm_version,deck_version,deal_sequence_version,deck_hash,next_deck_index,full_fairness_reveal_at)
	 VALUES($1,$2,$3,'history',$4,$5,$5,'poker-deal-v1',$6,$7,$8::timestamptz+interval '24 hours') ON CONFLICT(hand_id) DO UPDATE SET
	 next_deck_index=excluded.next_deck_index,full_fairness_reveal_at=excluded.full_fairness_reveal_at`, id, proof.ServerSeedHash[:], seal("server_seed", seed), proof.EffectiveSeed[:], fairness.AlgorithmVersion, proof.DeckHash[:], cursor, settled)
	for i, p := range state.Players {
		street, total, folded := p.StreetCommitted, p.TotalCommitted, p.Folded
		if mode == "committed" {
			street, total, folded = 0, 0, false
		}
		h.pg.SQL(t, `INSERT INTO poker.hand_participants(hand_id,seat_no,session_id,newapi_user_id,hand_start_stack_units,street_committed_units,total_committed_units,folded,contribution_version,contribution_cipher)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,1,$9) ON CONFLICT(hand_id,seat_no) DO UPDATE SET
		 street_committed_units=excluded.street_committed_units,total_committed_units=excluded.total_committed_units,folded=excluded.folded`, id, p.SeatNo, h.sessions[p.SeatNo], 100+p.SeatNo, p.InitialStack, street, total, folded, seal(fmt.Sprint("contribution:", p.SeatNo), []byte(setup.Contributions[i].Value)))
		if mode != "committed" {
			h.pg.SQL(t, "UPDATE poker.sessions SET current_stack_units=$2 WHERE session_id=$1", h.sessions[p.SeatNo], p.Stack)
		}
	}
	if mode == "committed" {
		return state
	}
	var paid, awarded, returned int64
	for _, event := range state.Events {
		raw, err = json.Marshal(event)
		pokerhistoryfixture.Check(t, err)
		h.pg.SQL(t, `INSERT INTO poker.actions(hand_id,event_sequence,hand_version,event_type,actor_seat,applied_delta_units,to_units,payload_cipher,created_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT DO NOTHING`, id, event.Sequence, event.Version, event.Type, event.Seat, event.Delta, event.To, seal(fmt.Sprint("event:", event.Sequence), raw), event.At)
		if event.Card != nil {
			h.pg.SQL(t, "INSERT INTO poker.dealt_cards(hand_id,deck_index,recipient_seat,visibility,card_cipher) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING", id, *event.DeckIndex, event.Seat, event.Visibility, seal(fmt.Sprint("card:", *event.DeckIndex), []byte{byte(*event.Card)}))
		}
		switch event.Type {
		case "POST_SB", "POST_BB", "CALL", "BET", "RAISE", "ALL_IN":
			paid += event.Delta
		}
	}
	if state.Street != engine.Settled {
		return state
	}
	for _, pot := range state.Pots {
		h.pg.SQL(t, "INSERT INTO poker.pots(hand_id,pot_index,amount_units,contribution_floor,contribution_ceiling) VALUES($1,$2,$3,$4,$5)", id, pot.Index, pot.Amount, pot.Floor, pot.Ceiling)
		for _, seat := range pot.Eligible {
			h.pg.SQL(t, "INSERT INTO poker.pot_eligible_players(hand_id,pot_index,seat_no) VALUES($1,$2,$3)", id, pot.Index, seat)
		}
		for _, a := range pot.Awards {
			awarded += a.Amount
			h.pg.SQL(t, "INSERT INTO poker.pot_awards(hand_id,pot_index,seat_no,base_share_units,odd_chip_units,award_units) VALUES($1,$2,$3,$4,$5,$6)", id, pot.Index, a.Seat, a.Base, a.Odd, a.Amount)
		}
	}
	for _, r := range state.Returns {
		returned += r.Amount
	}
	h.pg.SQL(t, "INSERT INTO poker.settlements(hand_id,biz_id,total_commitment_units,total_award_units,uncalled_return_units,settled_at) VALUES($1,$2,$3,$4,$5,$6)", id, "poker_hand_settlement:"+id, paid, awarded, returned, settled)
	return state
}

func (h *pokerHistoryFixture) fingerprint(t *testing.T) string {
	t.Helper()
	tables := []string{
		"poker.tables", "poker.seats", "poker.sessions", "poker.hands", "poker.hand_fairness", "poker.hand_participants",
		"poker.funding_operations", "poker.actions", "poker.dealt_cards", "poker.pots", "poker.pot_eligible_players",
		"poker.pot_awards", "poker.settlements", "poker.recovery_state", "poker.audit_events", "poker.request_receipts",
		"economy.wallet_balances", "economy.wallet_ledger", "economy.asset_transactions", "platform_meta.mutation_idempotency_records",
	}
	hashes := []string{}
	for _, table := range tables {
		var hash string
		pokerhistoryfixture.Check(t, h.pg.Pool.QueryRow(context.Background(), "SELECT md5(coalesce(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text)::text,'')) FROM "+table+" t").Scan(&hash))
		hashes = append(hashes, hash)
	}
	return fmt.Sprint(hashes)
}

func (h *pokerHistoryFixture) corrupt(t *testing.T, query string) {
	table := strings.Fields(query)[1]
	if table == "FROM" {
		table = strings.Fields(query)[2]
	}
	trigger := map[string]string{"poker.actions": "poker_actions_immutable", "poker.dealt_cards": "poker_deals_immutable", "poker.settlements": "poker_settlement_immutable"}[table]
	if trigger != "" {
		query = fmt.Sprintf(`BEGIN;
		 ALTER TABLE %s DISABLE TRIGGER %s;
		 %s;
		 ALTER TABLE %s ENABLE TRIGGER %s;
		 COMMIT`, table, trigger, query, table, trigger)
	}
	h.pg.SQL(t, query)
}

func (h *pokerHistoryFixture) inboundGrant(t *testing.T) {
	name := strings.Replace(h.pg.PokerRole, "h1b2_poker_", "h1b2_outsider_", 1)
	role, poker := pgx.Identifier{name}.Sanitize(), pgx.Identifier{h.pg.PokerRole}.Sanitize()
	var exists bool
	pokerhistoryfixture.Check(t, h.pg.Pool.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)", name).Scan(&exists))
	if exists || name != "h1b2_outsider_"+strings.TrimPrefix(h.pg.Database, "h1b_poker_") {
		t.Fatal("outsider role already exists")
	}
	t.Cleanup(func() {
		h.pg.SQL(t, "BEGIN; SET LOCAL ROLE NONE; DROP ROLE IF EXISTS "+role+"; COMMIT")
		pokerhistoryfixture.Check(t, h.pg.Pool.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)", name).Scan(&exists))
		if exists {
			t.Error("outsider cleanup failed")
		}
		t.Logf("OUTSIDER_CLEANUP %s absent", name)
	})
	revoke := "BEGIN; SET LOCAL ROLE NONE; REVOKE " + poker + " FROM " + role + "; COMMIT"
	defer h.pg.SQL(t, revoke)
	h.pg.SQL(t, "BEGIN; SET LOCAL ROLE NONE; CREATE ROLE "+role+" NOLOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS; GRANT "+poker+" TO "+role+"; COMMIT")
	h.pg.Grant(t, "deploy/sql/runtime-grants-0024-history-poker-details.psql", "poker", false)
	h.pg.SQL(t, revoke)
	h.pg.Grant(t, "deploy/sql/runtime-grants-0024-history-poker-details.psql", "poker", true)
}
