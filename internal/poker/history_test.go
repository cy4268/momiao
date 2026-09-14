package poker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/historyaccess"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/testutil/pokerhistoryfixture"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPokerHistoryFundingPG(t *testing.T) {
	h := newPokerHistoryFixture(t)
	ctx := context.Background()
	id := h.fund(t, 1, "TOP_UP", 500000)
	var raw []byte
	pokerhistoryfixture.Check(t, h.pg.Poker.QueryRow(ctx, "SELECT poker.history_funding_transactions($1,$2,$3)", 101, h.sessions[1], []string{id}).Scan(&raw))
	var transactions []platform.HistoryTransaction
	pokerhistoryfixture.Check(t, json.Unmarshal(raw, &transactions))
	if len(transactions) != 1 || transactions[0].ID != id || len(transactions[0].Effects) != 1 || transactions[0].Effects[0].DeltaUnits != "-500000" {
		t.Fatal("owned confirmed funding and exact wallet effect missing")
	}
	h.pg.SQL(t, "UPDATE poker.funding_operations SET amount_units=1000000 WHERE confirmed_transaction_id=$1", id)
	if err := h.pg.Poker.QueryRow(ctx, "SELECT poker.history_funding_transactions(101,$1,$2)", h.sessions[1], []string{id}).Scan(&raw); err == nil {
		t.Fatal("formal amount disagreed with genuine wallet leg but passed")
	}
	var now time.Time
	pokerhistoryfixture.Check(t, h.pg.Pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now))
	for _, delta := range []time.Duration{-time.Microsecond, 0, time.Microsecond} {
		at := now.Add(delta)
		if fairnessReleased("SETTLED", &now, at) != (delta >= 0) || fairnessReleased("PREFLOP", &now, at) {
			t.Fatal("proof PG-time boundary changed")
		}
	}
}

func TestPokerHistorySessionPG(t *testing.T) {
	h := newPokerHistoryFixture(t)
	h.fund(t, 1, "TOP_UP", 500000)
	r, err := NewHistoryReader(h.pg.Poker, h.keys)
	pokerhistoryfixture.Check(t, err)
	d, err := r.SessionDetail(context.Background(), historyaccess.Own(101), h.sessions[1], HistorySessionQuery{FundingLimit: 1})
	pokerhistoryfixture.Check(t, err)
	if d.InitialBuyIn != 200000000 || d.TopUp != 500000 || d.Rebuy != 0 || d.RealizedPL != nil || d.FundingCount != 2 || len(d.Funding) != 1 || d.NextFundingCursor == "" || d.Metadata.ActorName == nil || *d.Metadata.ActorName != "Player A" {
		t.Fatal("Session header, immutable metadata or page missing")
	}
	last, err := r.SessionDetail(context.Background(), historyaccess.Own(101), h.sessions[1], HistorySessionQuery{FundingLimit: 1, FundingCursor: d.NextFundingCursor})
	pokerhistoryfixture.Check(t, err)
	if len(last.Funding) != 1 || last.Funding[0].Kind != "BUY_IN" || last.NextFundingCursor != "" {
		t.Fatal("continuation did not reach original buy-in")
	}
	for _, state := range []string{"PENDING", "FAILED_NO_EFFECT"} {
		h.pg.SQL(t, `INSERT INTO poker.funding_operations(funding_operation_id,table_id,seat_no,session_id,newapi_user_id,kind,amount_units,request_hash,planned_transaction_id,planned_ledger_id,state)
		 VALUES($1,$2,1,$3,101,'TOP_UP',500000,decode(repeat('00',32),'hex'),$4,$5,$6)`, uuid(), h.table, h.sessions[1], uuid(), uuid(), state)
	}
	all, err := r.SessionDetail(context.Background(), historyaccess.Own(101), h.sessions[1], HistorySessionQuery{})
	pokerhistoryfixture.Check(t, err)
	if all.FundingCount != 4 || all.TopUp != 500000 || all.Funding[0].Transaction != nil || all.Funding[1].Transaction != nil {
		t.Fatal("unconfirmed funding invented wallet effects or changed totals")
	}
	again, err := r.SessionDetail(context.Background(), historyaccess.Own(101), h.sessions[1], HistorySessionQuery{FundingCursor: d.NextFundingCursor})
	if err != nil || len(again.Funding) != 1 || again.Funding[0].Kind != "BUY_IN" {
		t.Fatal("newer funding entered old cursor range")
	}
	raw, err := base64.RawURLEncoding.DecodeString(d.NextFundingCursor)
	pokerhistoryfixture.Check(t, err)
	for _, change := range []func(*historyCursor){func(c *historyCursor) { c.Subject++ }, func(c *historyCursor) { c.Parent = h.sessions[2] }, func(c *historyCursor) { c.Kind = "hands" }, func(c *historyCursor) { c.UpperAt = c.UpperAt.Add(time.Microsecond) }, func(c *historyCursor) { c.LastID = historyID(999) }} {
		var c historyCursor
		pokerhistoryfixture.Check(t, json.Unmarshal(raw, &c))
		change(&c)
		bad, err := json.Marshal(c)
		pokerhistoryfixture.Check(t, err)
		got, err := r.SessionDetail(context.Background(), historyaccess.Own(101), h.sessions[1], HistorySessionQuery{FundingCursor: base64.RawURLEncoding.EncodeToString(bad)})
		if !errors.Is(err, historyaccess.ErrInvalid) || got.ID != "" {
			t.Fatal("unbound funding cursor accepted")
		}
	}
}

func TestPokerHistoryFundingRejectedPG(t *testing.T) {
	h := newPokerHistoryFixture(t)
	id := h.fund(t, 1, "TOP_UP", 500000)
	for _, ids := range [][]string{nil, {id, id}, {historyID(900)}, {h.fund(t, 2, "TOP_UP", 500000)}} {
		var raw []byte
		if err := h.pg.Poker.QueryRow(context.Background(), "SELECT poker.history_funding_transactions(101,$1,$2)", h.sessions[1], ids).Scan(&raw); err == nil {
			t.Fatal("invalid or foreign funding accepted")
		}
	}
	for _, subject := range []int{102, 999} {
		var raw []byte
		pokerhistoryfixture.Check(t, h.pg.Poker.QueryRow(context.Background(), "SELECT poker.history_funding_transactions($1,$2,$3)", subject, h.sessions[1], []string{id}).Scan(&raw))
		if raw != nil {
			t.Fatal("foreign Session disclosed")
		}
	}
	var raw []byte
	pokerhistoryfixture.Check(t, h.pg.Poker.QueryRow(context.Background(), "SELECT poker.history_funding_transactions(101,$1,'{}'::uuid[])", h.sessions[1]).Scan(&raw))
	if string(raw) != "[]" {
		t.Fatal("empty batch changed")
	}
	if err := h.pg.Poker.QueryRow(context.Background(), "SELECT poker.history_funding_transactions(101,$1,ARRAY[NULL]::uuid[])", h.sessions[1]).Scan(&raw); err == nil {
		t.Fatal("NULL element accepted")
	}
}

func TestPokerHistoryGrantPollutionPG(t *testing.T) {
	h := newPokerHistoryFixture(t)
	capability := "poker.history_funding_transactions(bigint,uuid,uuid[])"
	h.pg.SQL(t, "GRANT EXECUTE ON FUNCTION "+capability+" TO "+pgx.Identifier{h.pg.PokerRole}.Sanitize()+" WITH GRANT OPTION")
	h.pg.Grant(t, "deploy/sql/runtime-grants-0024-history-poker-details.psql", "poker", false)
	h.pg.SQL(t, "REVOKE GRANT OPTION FOR EXECUTE ON FUNCTION "+capability+" FROM "+pgx.Identifier{h.pg.PokerRole}.Sanitize())
	for _, role := range []string{h.pg.PlatformRole, h.pg.ReaderRole, h.pg.WorkerRole} {
		h.pg.SQL(t, "GRANT EXECUTE ON FUNCTION "+capability+" TO "+pgx.Identifier{role}.Sanitize())
		h.pg.Grant(t, "deploy/sql/runtime-grants-0024-history-poker-details.psql", "poker", false)
		h.pg.SQL(t, "REVOKE EXECUTE ON FUNCTION "+capability+" FROM "+pgx.Identifier{role}.Sanitize())
	}
	h.inboundGrant(t)
}

func TestPokerHistoryLongSessionPG(t *testing.T) {
	for _, count := range []int{121, 501} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			h := newPokerHistoryFixture(t)
			for i := 0; i < count-2; i++ {
				h.fund(t, 1, "TOP_UP", 500000)
			}
			h.fund(t, 1, "CASH_OUT", 0)
			r, err := NewHistoryReader(h.pg.Poker, h.keys)
			pokerhistoryfixture.Check(t, err)
			cursor, seen, ids := "", map[string]bool{}, []string{}
			for page := 0; ; page++ {
				started := time.Now()
				d, err := r.SessionDetail(context.Background(), historyaccess.Own(101), h.sessions[1], HistorySessionQuery{FundingLimit: 100, FundingCursor: cursor})
				pokerhistoryfixture.Check(t, err)
				t.Logf("SCALE total=%d page=%d funding=%d elapsed=%s", count, page, len(d.Funding), time.Since(started))
				if d.FundingCount != int64(count) || d.RealizedPL == nil || *d.RealizedPL != 0 || d.FinalCashOut == nil || *d.FinalCashOut != 200000000+int64(count-2)*500000 {
					t.Fatal("long Session aggregate or terminal result wrong")
				}
				for _, f := range d.Funding {
					if seen[f.ID] || f.Transaction == nil || f.Transaction.Links[0].SourceID != h.sessions[1] {
						t.Fatal("funding duplicate, missing wallet transaction or wrong backlink")
					}
					seen[f.ID] = true
					ids = append(ids, f.Transaction.ID)
				}
				cursor = d.NextFundingCursor
				if cursor == "" {
					break
				}
				if page > count/100 {
					t.Fatal("cursor did not terminate")
				}
			}
			if len(seen) != count {
				t.Fatal("long Session truncated")
			}
			var raw []byte
			pokerhistoryfixture.Check(t, h.pg.Poker.QueryRow(context.Background(), "SELECT poker.history_funding_transactions(101,$1,$2)", h.sessions[1], ids[:100]).Scan(&raw))
			if err := h.pg.Poker.QueryRow(context.Background(), "SELECT poker.history_funding_transactions(101,$1,$2)", h.sessions[1], ids[:101]).Scan(&raw); err == nil {
				t.Fatal("101 input IDs accepted by bounded batch capability")
			}
		})
	}
}

func TestPokerHistorySourceRejectedPG(t *testing.T) {
	for i, mutation := range []string{
		"UPDATE poker.hands SET snapshot_cipher=set_byte(snapshot_cipher,0,0)",
		"UPDATE poker.pots SET amount_units=amount_units+500000",
		"UPDATE poker.pot_awards SET base_share_units=base_share_units+500000,award_units=award_units+500000",
		"DELETE FROM poker.hand_fairness",
		"UPDATE poker.hand_participants SET hand_start_stack_units=hand_start_stack_units+500000",
		"DELETE FROM poker.actions WHERE event_sequence=1",
		"UPDATE poker.actions SET actor_seat=9 WHERE event_sequence=1",
		"DELETE FROM poker.dealt_cards WHERE deck_index=0",
		"UPDATE poker.dealt_cards SET card_cipher=(SELECT card_cipher FROM poker.dealt_cards WHERE deck_index=1) WHERE deck_index=0",
		"DELETE FROM poker.pot_eligible_players WHERE seat_no=1",
		"DELETE FROM poker.pot_awards",
		"DELETE FROM poker.settlements",
		"UPDATE poker.settlements SET biz_id='wrong-business'",
		"UPDATE poker.hand_fairness SET full_fairness_reveal_at=full_fairness_reveal_at-interval '1 second'",
		"UPDATE poker.hand_fairness SET encrypted_server_seed=set_byte(encrypted_server_seed,2,120)",
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			h := newPokerHistoryFixture(t)
			state := h.hand(t, 3, "allin", 7)
			r, err := NewHistoryReader(h.pg.Poker, h.keys)
			pokerhistoryfixture.Check(t, err)
			_, err = r.HandFairness(context.Background(), historyaccess.Own(101), state.Config.HandID)
			pokerhistoryfixture.Check(t, err)
			h.corrupt(t, mutation)
			before := h.fingerprint(t)
			d, err := r.HandDetail(context.Background(), historyaccess.Own(101), state.Config.HandID, HistoryHandQuery{})
			proof, proofErr := r.HandFairness(context.Background(), historyaccess.Own(101), state.Config.HandID)
			if (i < 14 && (!errors.Is(err, historyaccess.ErrUnavailable) || d.ID != "")) || (i == 14 && (err != nil || d.ID == "")) ||
				!errors.Is(proofErr, historyaccess.ErrUnavailable) || proof.HandID != "" || h.fingerprint(t) != before {
				t.Fatalf("damaged source contract failed: detail=%v proof=%v", err, proofErr)
			}
		})
	}
}

func TestPokerHistorySessionRejectedPG(t *testing.T) {
	h := newPokerHistoryFixture(t)
	r, err := NewHistoryReader(h.pg.Poker, h.keys)
	pokerhistoryfixture.Check(t, err)
	for _, access := range []historyaccess.Access{historyaccess.Own(102), historyaccess.Records(102, 101, nil)} {
		d, err := r.SessionDetail(context.Background(), access, h.sessions[1], HistorySessionQuery{})
		if !errors.Is(err, historyaccess.ErrNotFound) || d.ID != "" {
			t.Fatal("foreign or unscoped Session read")
		}
	}
	for _, q := range []HistorySessionQuery{{FundingLimit: 101}, {HandLimit: -1}, {FundingCursor: "broken"}} {
		d, err := r.SessionDetail(context.Background(), historyaccess.Own(101), h.sessions[1], q)
		if !errors.Is(err, historyaccess.ErrInvalid) || d.ID != "" {
			t.Fatal("invalid page accepted or partial DTO returned")
		}
	}
	h.pg.SQL(t, "UPDATE poker.sessions SET initial_buyin_units=initial_buyin_units+500000 WHERE session_id=$1", h.sessions[1])
	d, err := r.SessionDetail(context.Background(), historyaccess.Own(101), h.sessions[1], HistorySessionQuery{})
	if !errors.Is(err, historyaccess.ErrUnavailable) || d.ID != "" {
		t.Fatal("contradictory Session money displayed")
	}
}

func TestPokerHistoryHandPG(t *testing.T) {
	h := newPokerHistoryFixture(t)
	state := h.hand(t, 1, "fold", 7)
	r, err := NewHistoryReader(h.pg.Poker, h.keys)
	pokerhistoryfixture.Check(t, err)
	d, err := r.HandDetail(context.Background(), historyaccess.Own(101), state.Config.HandID, HistoryHandQuery{})
	pokerhistoryfixture.Check(t, err)
	if d.State != "SETTLED" || d.SessionID != h.sessions[1] || d.Settlement == nil || len(d.Participants) != 3 || len(d.Pots) != 1 || d.Fairness.Released {
		t.Fatal("formal Hand detail missing or proof released in ordinary detail")
	}
	for i, p := range d.Participants {
		if p.Ending == nil || *p.Ending != state.Players[i].Stack || p.Initial != state.Players[i].InitialStack || (p.Seat == 1) != (len(p.HoleCards) == 2) || len(p.PublicHoleCards) != 0 {
			t.Fatal("hand terminal stack or folded-hole policy wrong")
		}
	}
}

func TestPokerHistoryRevealSourcePG(t *testing.T) {
	h := newPokerHistoryFixture(t)
	h.ageHours = 0
	state := h.hand(t, 1, "fold", 7)
	r, err := NewHistoryReader(h.pg.Poker, h.keys)
	pokerhistoryfixture.Check(t, err)
	v, err := r.HandFairness(context.Background(), historyaccess.Own(101), state.Config.HandID)
	pokerhistoryfixture.Check(t, err)
	if v.Released {
		t.Fatal("fresh Hand proof already released")
	}
	h.pg.SQL(t, "UPDATE poker.hands SET settled_at=settled_at-interval '48 hours' WHERE hand_id=$1; UPDATE poker.hand_fairness SET full_fairness_reveal_at=full_fairness_reveal_at-interval '48 hours' WHERE hand_id=$1", state.Config.HandID)
	v, err = r.HandFairness(context.Background(), historyaccess.Own(101), state.Config.HandID)
	if !errors.Is(err, historyaccess.ErrUnavailable) || v.HandID != "" {
		t.Fatal("shifted mutable timestamps bypassed authenticated settlement time")
	}
}

func TestPokerHistoryPrivacyReadOnlyPG(t *testing.T) {
	h := newPokerHistoryFixture(t)
	state := h.hand(t, 1, "fold", 7)
	r, err := NewHistoryReader(h.pg.Poker, Keyring{Keys: h.keys.Keys})
	pokerhistoryfixture.Check(t, err)
	before := h.fingerprint(t)
	held, err := h.pg.Pool.Begin(context.Background())
	pokerhistoryfixture.Check(t, err)
	defer rollback(held)
	_, err = held.Exec(context.Background(), "SELECT 1 FROM poker.tables FOR UPDATE; SELECT 1 FROM poker.sessions FOR UPDATE; SELECT 1 FROM economy.wallet_balances FOR UPDATE", pgx.QueryExecModeSimpleProtocol)
	pokerhistoryfixture.Check(t, err)
	for _, viewer := range []int64{101, 102, 999} {
		access := historyaccess.Records(viewer, 101, func(context.Context, int64, int64) error { return nil })
		d, err := r.HandDetail(context.Background(), access, state.Config.HandID, HistoryHandQuery{})
		pokerhistoryfixture.Check(t, err)
		_, err = r.SessionDetail(context.Background(), access, h.sessions[1], HistorySessionQuery{})
		pokerhistoryfixture.Check(t, err)
		for _, p := range d.Participants {
			if (len(p.HoleCards) == 2) != (int64(p.Seat)+100 == viewer) || len(p.PublicHoleCards) != 0 {
				t.Fatal("Records Scope changed private-hole ownership")
			}
		}
		for _, a := range d.Actions {
			if a.Card != nil && int64(a.Seat)+100 != viewer {
				t.Fatal("private card escaped through timeline payload")
			}
		}
		raw, err := json.Marshal(d)
		pokerhistoryfixture.Check(t, err)
		for _, forbidden := range []string{`"server_seed":`, `"deck":`, `"newapi_user_id":`, `"control_epoch":`, `"legal_actions":`, h.sessions[2], h.sessions[3]} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatal("internal identity, proof or live control escaped whitelist")
			}
		}
		v, err := r.HandFairness(context.Background(), access, state.Config.HandID)
		if viewer == 999 {
			if !errors.Is(err, historyaccess.ErrNotFound) || v.HandID != "" {
				t.Fatal("Scope created proof participant membership")
			}
		} else if err != nil || !v.Released || len(v.Deck) != 52 {
			t.Fatal("durable participant proof missing after release")
		}
	}
	rollback(held)
	if h.fingerprint(t) != before {
		t.Fatal("successful historical reads changed durable state")
	}
	for seat := 1; seat <= 3; seat++ {
		h.fund(t, seat, "CASH_OUT", 0)
	}
	h.pg.SQL(t, "UPDATE poker.tables SET lifecycle_state='CLOSED' WHERE table_id=$1", h.table)
	v, err := r.HandFairness(context.Background(), historyaccess.Own(101), state.Config.HandID)
	if err != nil || !v.Released {
		t.Fatal("cashout or closed table revoked durable proof eligibility")
	}
}

func TestPokerHistoryHandListPG(t *testing.T) {
	h := newPokerHistoryFixture(t)
	first := h.hand(t, 1, "fold", 7)
	for n := 2; n <= 101; n++ {
		h.hand(t, n, "fold", 7)
	}
	r, err := NewHistoryReader(h.pg.Poker, h.keys)
	pokerhistoryfixture.Check(t, err)
	page, err := r.SessionDetail(context.Background(), historyaccess.Own(101), h.sessions[1], HistorySessionQuery{HandLimit: 100})
	pokerhistoryfixture.Check(t, err)
	last, err := r.SessionDetail(context.Background(), historyaccess.Own(101), h.sessions[1], HistorySessionQuery{HandLimit: 100, HandCursor: page.NextHandCursor})
	pokerhistoryfixture.Check(t, err)
	seen := map[string]bool{}
	for _, hand := range append(page.Hands, last.Hands...) {
		if seen[hand.ID] {
			t.Fatal("duplicate Hand in continuation")
		}
		seen[hand.ID] = true
	}
	if page.HandCount != 101 || len(page.Hands) != 100 || len(last.Hands) != 1 || len(seen) != 101 || last.NextHandCursor != "" {
		t.Fatal("101 durable Hands truncated")
	}
	d, err := r.HandDetail(context.Background(), historyaccess.Own(101), first.Config.HandID, HistoryHandQuery{})
	pokerhistoryfixture.Check(t, err)
	if *d.Participants[0].Ending != first.Players[0].Stack {
		t.Fatal("historical ending stack replaced by later Session stack")
	}
}

func TestPokerHistoryHandModesPG(t *testing.T) {
	for _, mode := range []string{"committed", "allin", "long", "showdown", "active", "odd"} {
		t.Run(mode, func(t *testing.T) {
			h := newPokerHistoryFixture(t)
			if mode == "long" {
				for seat := 1; seat <= 3; seat++ {
					h.fund(t, seat, "TOP_UP", int64(350-50*seat)*1000000)
				}
			}
			number := 1
			if mode == "allin" {
				number = 3 // Largest stack opens, before both shorter stacks call all-in.
			}
			seed := byte(7)
			if mode == "odd" {
				seed = 17 // Genuine tie established by the original bounded seed search.
			}
			state := h.hand(t, number, mode, seed)
			r, err := NewHistoryReader(h.pg.Poker, h.keys)
			pokerhistoryfixture.Check(t, err)
			d, err := r.HandDetail(context.Background(), historyaccess.Own(101), state.Config.HandID, HistoryHandQuery{Limit: 100})
			pokerhistoryfixture.Check(t, err)
			switch mode {
			case "odd":
				var odd int64
				for _, pot := range d.Pots {
					for _, award := range pot.Awards {
						odd += award.Odd
					}
				}
				if odd != 500000 || d.Settlement.Paid != 12500000 || d.Settlement.Awarded != 12500000 {
					t.Fatal("genuine tied pot lost odd chip or double counted")
				}
			case "active":
				if d.State != "PREFLOP" || d.Settlement != nil || d.Participants[0].Ending != nil || len(d.Participants[0].HoleCards) != 2 {
					t.Fatal("active Hand invented terminal values or hid own holes")
				}
			case "committed":
				if d.State != "COMMITTED" || d.Settlement != nil || len(d.Actions) != 0 || len(d.Board) != 0 || d.Participants[0].Ending != nil || len(d.Participants[0].HoleCards) != 0 {
					t.Fatal("committed Hand advanced or invented an outcome")
				}
			case "allin":
				if len(d.Pots) != 2 || len(d.Returns) != 1 || d.Returns[0].Amount != 50000000 || d.Settlement.Paid != 750000000 || d.Settlement.Awarded != 700000000 {
					t.Fatal("side pot or uncalled return accounted incorrectly")
				}
				for _, p := range d.Participants {
					if len(p.PublicHoleCards) != 2 {
						t.Fatal("live all-in runout cards not public")
					}
				}
			case "long":
				events := append([]HistoryAction{}, d.Actions...)
				for cursor := d.NextCursor; cursor != ""; {
					d, err = r.HandDetail(context.Background(), historyaccess.Own(101), state.Config.HandID, HistoryHandQuery{Limit: 100, Cursor: cursor})
					pokerhistoryfixture.Check(t, err)
					events, cursor = append(events, d.Actions...), d.NextCursor
				}
				if len(events) <= 100 || len(events) != len(state.Events) {
					t.Fatal("legal over-100 timeline missing events")
				}
				for i, event := range events {
					if event.Sequence != uint64(i+1) {
						t.Fatal("timeline duplicated or skipped an event")
					}
				}
				t.Logf("LEGAL_TIMELINE events=%d", len(events))
			case "showdown":
				for _, p := range d.Participants {
					winner := false
					for _, award := range d.Pots[0].Awards {
						winner = winner || award.Seat == p.Seat
					}
					if (len(p.PublicHoleCards) == 2) != winner {
						t.Fatal("mucked loser or winner release policy wrong")
					}
				}
			}
			if mode == "committed" || mode == "active" {
				v, err := r.HandFairness(context.Background(), historyaccess.Own(101), state.Config.HandID)
				if err != nil || v.Released || len(v.Deck) != 0 || v.ServerSeed != "" {
					t.Fatal("unsettled proof released")
				}
			}
		})
	}
}

func TestPokerHistoryActiveCursorPG(t *testing.T) {
	h := newPokerHistoryFixture(t)
	state := h.hand(t, 1, "active", 7)
	r, err := NewHistoryReader(h.pg.Poker, h.keys)
	pokerhistoryfixture.Check(t, err)
	d, err := r.HandDetail(context.Background(), historyaccess.Own(101), state.Config.HandID, HistoryHandQuery{Limit: 1})
	pokerhistoryfixture.Check(t, err)
	if d.NextCursor == "" {
		t.Fatal("active timeline omitted continuation")
	}
	raw, err := base64.RawURLEncoding.DecodeString(d.NextCursor)
	pokerhistoryfixture.Check(t, err)
	for _, change := range []func(*historyHandCursor){
		func(c *historyHandCursor) { c.Subject++ }, func(c *historyHandCursor) { c.Hand = historyID(999) },
		func(c *historyHandCursor) { c.Upper++ }, func(c *historyHandCursor) { c.Last = c.Upper },
	} {
		var c historyHandCursor
		pokerhistoryfixture.Check(t, json.Unmarshal(raw, &c))
		change(&c)
		bad, err := json.Marshal(c)
		pokerhistoryfixture.Check(t, err)
		got, err := r.HandDetail(context.Background(), historyaccess.Own(101), state.Config.HandID, HistoryHandQuery{Cursor: base64.RawURLEncoding.EncodeToString(bad)})
		if !errors.Is(err, historyaccess.ErrInvalid) || got.ID != "" {
			t.Fatal("unbound timeline cursor accepted")
		}
	}
	h.hand(t, 1, "advance", 7)
	got, err := r.HandDetail(context.Background(), historyaccess.Own(101), state.Config.HandID, HistoryHandQuery{Cursor: d.NextCursor})
	if !errors.Is(err, ErrHistoryCursorStale) || got.ID != "" {
		t.Fatalf("old active cursor accepted: %v", err)
	}
	_, err = r.HandDetail(context.Background(), historyaccess.Own(101), state.Config.HandID, HistoryHandQuery{})
	pokerhistoryfixture.Check(t, err)
}

func TestPokerHistoryReadControlsPG(t *testing.T) {
	h := newPokerHistoryFixture(t)
	state := h.hand(t, 1, "fold", 7)
	r, err := NewHistoryReader(h.pg.Poker, h.keys)
	pokerhistoryfixture.Check(t, err)
	h.keys.Keys["history"][0]++
	_, err = r.HandDetail(context.Background(), historyaccess.Own(101), state.Config.HandID, HistoryHandQuery{})
	pokerhistoryfixture.Check(t, err) // Constructor owns its immutable key copy.
	before, allowed := h.fingerprint(t), true
	empty, err := NewHistoryReader(h.pg.Poker, Keyring{})
	pokerhistoryfixture.Check(t, err)
	d, err := empty.HandDetail(context.Background(), historyaccess.Own(101), state.Config.HandID, HistoryHandQuery{})
	if !errors.Is(err, historyaccess.ErrUnavailable) || d.ID != "" {
		t.Fatal("missing historical key returned partial source")
	}
	access := historyaccess.Records(999, 101, func(context.Context, int64, int64) error {
		if allowed {
			return nil
		}
		return errors.New("revoked")
	})
	_, err = r.SessionDetail(context.Background(), access, h.sessions[1], HistorySessionQuery{})
	pokerhistoryfixture.Check(t, err)
	allowed = false
	s, err := r.SessionDetail(context.Background(), access, h.sessions[1], HistorySessionQuery{})
	if !errors.Is(err, historyaccess.ErrNotFound) || s.ID != "" {
		t.Fatal("revoked Scope reused")
	}
	config := h.pg.Poker.Config()
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	pokerhistoryfixture.Check(t, err)
	defer pool.Close()
	blocked, err := NewHistoryReader(pool, r.keys)
	pokerhistoryfixture.Check(t, err)
	conn, err := pool.Acquire(context.Background())
	pokerhistoryfixture.Check(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	s, err = blocked.SessionDetail(ctx, historyaccess.Own(101), h.sessions[1], HistorySessionQuery{})
	cancel()
	conn.Release()
	if !errors.Is(err, context.DeadlineExceeded) || s.ID != "" {
		t.Fatal("pool deadline returned partial data")
	}
	held, err := h.pg.Pool.Begin(context.Background())
	pokerhistoryfixture.Check(t, err)
	defer rollback(held)
	_, err = held.Exec(context.Background(), "LOCK poker.hands IN ACCESS EXCLUSIVE MODE")
	pokerhistoryfixture.Check(t, err)
	started := time.Now()
	d, err = r.HandDetail(context.Background(), historyaccess.Own(101), state.Config.HandID, HistoryHandQuery{})
	rollback(held)
	if !errors.Is(err, historyaccess.ErrUnavailable) || d.ID != "" || time.Since(started) < 1900*time.Millisecond || time.Since(started) > 4*time.Second || h.fingerprint(t) != before {
		t.Fatalf("SQL timeout or durable read-only contract failed: %v", err)
	}
}

func TestPokerHistoryZeroAndRebuyPG(t *testing.T) {
	for _, rebuy := range []bool{false, true} {
		h := newPokerHistoryFixture(t)
		state := h.hand(t, 3, "allin", 7)
		seat := 0
		for _, p := range state.Players {
			if p.Stack == 0 {
				seat = p.SeatNo
			}
		}
		if seat == 0 {
			t.Fatal("legitimate all-in did not produce a busted seat")
		}
		if rebuy {
			h.pg.SQL(t, "UPDATE poker.seats SET state='REBUY_WINDOW',rebuy_deadline_at=clock_timestamp()+interval '1 hour' WHERE table_id=$1 AND seat_no=$2", h.table, seat)
			h.fund(t, seat, "REBUY", 200000000)
		} else {
			h.fund(t, seat, "CASH_OUT", 0)
		}
		r, err := NewHistoryReader(h.pg.Poker, h.keys)
		pokerhistoryfixture.Check(t, err)
		d, err := r.SessionDetail(context.Background(), historyaccess.Own(int64(100+seat)), h.sessions[seat], HistorySessionQuery{})
		if err != nil || (rebuy && (d.Rebuy != 200000000 || d.TopUp != 0 || d.RealizedPL != nil)) || (!rebuy && (d.FinalCashOut == nil || *d.FinalCashOut != 0 || *d.RealizedPL != -d.InitialBuyIn || d.Funding[0].Transaction == nil || len(d.Funding[0].Transaction.Effects) != 0)) {
			t.Fatalf("real zero cashout or rebuy contract failed: %v", err)
		}
	}
}
