package games

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cy4268/momiao/internal/historyaccess"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/testutil/historyfixture"
)

func TestHistoryDetailRoundPG(t *testing.T) {
	f := historyfixture.Open(t)
	ctx := context.Background()
	writer, reader := newTestService(t, f.Owner), newTestService(t, f.Runtime)
	_, err := f.Owner.Apply(ctx, platform.Mutation{UserID: 101, Asset: platform.AvailableChips, DeltaUnits: 5000000000, BizType: "H1B_TEST", BizID: "h1b-direct", EntryType: "TEST_GRANT", IdempotencyKey: requestKey(t)})
	historyfixture.Check(t, err)
	for game, input := range map[string]CreateInput{"dice": {Type: "DICE", Wager: "10", Choice: "BIG"}, "scratch": {Type: "SCRATCH", Wager: "10"}, "summon": {Type: "SUMMON", BaseWager: "10", Mode: "SINGLE"}, "slot": {Type: "SLOT", TotalWager: "10"}} {
		boot, e := writer.Bootstrap(ctx, 101, game)
		historyfixture.Check(t, e)
		r, e := writer.Create(ctx, 101, game, requestKey(t), boot.Next.ID, input)
		historyfixture.Check(t, e)
		v, e := reader.HistoryDetail(ctx, historyaccess.Own(101), r.ID)
		if e != nil || v.ID != r.ID || v.Game != game || v.StakeUnits != 5000000 || v.PayoutUnits == nil || *v.PayoutUnits != r.PayoutUnits || len(v.Transactions) != 2 {
			t.Fatal("typed round/funding read", game, e)
		}
		if !map[string]bool{"dice": v.Dice != nil, "scratch": v.Scratch != nil, "summon": v.Summon != nil, "slot": v.Slot != nil}[game] {
			t.Fatal("typed result omitted", game)
		}
		proof, e := reader.HistoryVerify(ctx, historyaccess.Own(101), r.ID)
		if e != nil || !proof.Verified || proof.RevealState != "REVEALED" {
			t.Fatal("existing typed verifier not reused", game, e)
		}
		if _, e = reader.HistoryDetail(ctx, historyaccess.Own(102), r.ID); !errors.Is(e, historyaccess.ErrNotFound) {
			t.Fatal("foreign round disclosed")
		}
		f.SQL(t, `UPDATE games.game_registry SET title=title||' changed',publication_state='RETIRED' WHERE game_slug=$1`, game)
		after, e := reader.HistoryDetail(ctx, historyaccess.Own(101), r.ID)
		if e != nil || after.Metadata.GameTitle != v.Metadata.GameTitle || after.Metadata.Origin != "creation_snapshot" {
			t.Fatal("retired source/snapshot lost", e)
		}
		if game == "dice" {
			f.SQL(t, `ALTER TABLE games.dice_results DISABLE TRIGGER immutable_history; DELETE FROM games.dice_results WHERE round_id=$1; ALTER TABLE games.dice_results ENABLE TRIGGER immutable_history`, r.ID)
			if _, e = reader.HistoryDetail(ctx, historyaccess.Own(101), r.ID); !errors.Is(e, historyaccess.ErrUnavailable) {
				t.Fatal("missing required typed fact swallowed", e)
			}
		}
	}
	bjWriter, user, c := blackjackFixture(t, f.Owner, f.Owner)
	r, err := bjWriter.Create(ctx, user, "blackjack", requestKey(t), c.ID, CreateInput{Type: "BLACKJACK", InitialWager: "10"})
	historyfixture.Check(t, err)
	first, err := bjWriter.BlackjackAction(ctx, user, r.ID, BlackjackActionInput{ActionID: requestKey(t), ActionType: "SPLIT", HandID: r.Blackjack.ActiveHandID, ExpectedVersion: "1"})
	historyfixture.Check(t, err)
	next, err := bjWriter.BlackjackAction(ctx, user, r.ID, BlackjackActionInput{ActionID: requestKey(t), ActionType: "DOUBLE", HandID: first.Blackjack.ActiveHandID, ExpectedVersion: "2"})
	historyfixture.Check(t, err)
	guard, err := f.Pool.Begin(ctx)
	historyfixture.Check(t, err)
	defer guard.Rollback(ctx)
	historyfixture.Check(t, lockUserGame(ctx, guard, user, "blackjack"))
	f.SQL(t, `REVOKE SELECT(balance_units) ON economy.wallet_balances FROM `+f.PlatformRole)
	active, err := reader.HistoryDetail(ctx, historyaccess.Own(user), r.ID)
	if err != nil || active.StakeUnits != 15000000 || active.PayoutUnits != nil || len(active.Transactions) != 3 || len(active.Blackjack.Hands) != 2 || len(active.Blackjack.DealerCards) != 1 {
		t.Fatal("read-only active BJ or additional stakes", err)
	}
	proof, err := reader.HistoryVerify(ctx, historyaccess.Own(user), r.ID)
	if err != nil || proof.ServerSeed != "" || proof.RevealState != "NOT_YET_AVAILABLE" {
		t.Fatal("early BJ proof leak", err)
	}
	raw, _ := json.Marshal(active)
	for _, forbidden := range []string{"legal_actions", "active_hand_id", "auto_resolve_at", "server_seed\"", "request_hash", "ciphertext", "original_response"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("history contains authority", forbidden)
		}
	}
	historyfixture.Check(t, guard.Rollback(ctx))
	final, err := bjWriter.BlackjackAction(ctx, user, r.ID, BlackjackActionInput{ActionID: requestKey(t), ActionType: "STAND", HandID: next.Blackjack.ActiveHandID, ExpectedVersion: "3"})
	historyfixture.Check(t, err)
	settled, err := reader.HistoryDetail(ctx, historyaccess.Own(user), r.ID)
	if err != nil || len(settled.Transactions) != 4 || settled.PayoutUnits == nil || *settled.PayoutUnits != final.PayoutUnits || len(settled.Blackjack.Actions) != 3 {
		t.Fatal("settled split/double funding incomplete", err)
	}
	seen := map[string]bool{}
	for _, tx := range settled.Transactions {
		if seen[tx.ID] || len(tx.Links) != 1 || tx.Links[0].SourceID != r.ID {
			t.Fatal("duplicate/foreign source backlink")
		}
		seen[tx.ID] = true
		v, e := f.Runtime.HistoryTransaction(ctx, historyaccess.Own(user), tx.ID)
		if e != nil || v.ID != tx.ID {
			t.Fatal("wallet round trip", e)
		}
	}
	proof, err = reader.HistoryVerify(ctx, historyaccess.Own(user), r.ID)
	if err != nil || !proof.Verified || proof.BlackjackAudit == nil {
		t.Fatal("terminal BJ verification", err)
	}
	// Dealer ten+ten beats a non-ace 2..9 pair: a genuine zero-payout settlement.
	loser, zeroUser, zeroCommit := blackjackFixtureMatching(t, f.Owner, f.Owner, func(s [312]uint16) bool {
		return s[0]%13 >= 1 && s[0]%13 <= 8 && s[2]%13 >= 1 && s[2]%13 <= 8 && s[1]%13 >= 9 && s[3]%13 >= 9
	})
	z, err := loser.Create(ctx, zeroUser, "blackjack", requestKey(t), zeroCommit.ID, CreateInput{Type: "BLACKJACK", InitialWager: "10"})
	historyfixture.Check(t, err)
	z, err = loser.BlackjackAction(ctx, zeroUser, z.ID, BlackjackActionInput{ActionID: requestKey(t), ActionType: "STAND", HandID: z.Blackjack.ActiveHandID, ExpectedVersion: "1"})
	historyfixture.Check(t, err)
	zero, err := reader.HistoryDetail(ctx, historyaccess.Own(zeroUser), z.ID)
	if err != nil || zero.PayoutUnits == nil || *zero.PayoutUnits != 0 || len(zero.Transactions) != 2 || len(zero.Transactions[1].Effects) != 0 {
		t.Fatal("zero payout transaction lost", err)
	}
	f.SQL(t, `TRUNCATE games.history_index,games.history_ingestion_cursors`)
	if _, err = reader.HistoryDetail(ctx, historyaccess.Own(user), r.ID); err != nil {
		t.Fatal("index became detail authority", err)
	}
	t.Log("ROUNDS: five typed paths; retired metadata; no gameplay lock/current balance/control; split/double, zero payout, verified wallet backlinks")
}

func TestHistoryDetailCorruptReadOnlyPG(t *testing.T) {
	f := historyfixture.Open(t)
	ctx := context.Background()
	writer, user, c := blackjackFixture(t, f.Owner, f.Owner)
	r, err := writer.Create(ctx, user, "blackjack", requestKey(t), c.ID, CreateInput{Type: "BLACKJACK", InitialWager: "10"})
	historyfixture.Check(t, err)
	foreign, err := f.Owner.Apply(ctx, platform.Mutation{UserID: 102, Asset: platform.AvailableChips, DeltaUnits: 5000000, BizType: "H1B_FOREIGN", BizID: "foreign-link", EntryType: "TEST_GRANT", IdempotencyKey: requestKey(t)})
	historyfixture.Check(t, err)
	f.SQL(t, `ALTER TABLE games.game_rounds DISABLE TRIGGER blackjack_round_guard; ALTER TABLE games.game_rounds DISABLE TRIGGER blackjack_persistence_complete; UPDATE games.game_rounds SET wager_transaction_id=$2 WHERE round_id=$1; SET CONSTRAINTS ALL IMMEDIATE; ALTER TABLE games.game_rounds ENABLE TRIGGER blackjack_round_guard; ALTER TABLE games.game_rounds ENABLE TRIGGER blackjack_persistence_complete`, r.ID, foreign.TransactionID)
	if _, err = newTestService(t, f.Runtime).HistoryDetail(ctx, historyaccess.Own(user), r.ID); !errors.Is(err, historyaccess.ErrUnavailable) {
		t.Fatal("owned round accepted foreign money link", err)
	}
	if _, err = f.Runtime.HistoryTransaction(ctx, historyaccess.Own(102), foreign.TransactionID); !errors.Is(err, historyaccess.ErrUnavailable) {
		t.Fatal("wallet backlink accepted foreign round owner", err)
	}
	f.SQL(t, `ALTER TABLE games.game_rounds DISABLE TRIGGER blackjack_round_guard; ALTER TABLE games.game_rounds DISABLE TRIGGER blackjack_persistence_complete; UPDATE games.game_rounds SET wager_transaction_id=$2 WHERE round_id=$1; SET CONSTRAINTS ALL IMMEDIATE; ALTER TABLE games.game_rounds ENABLE TRIGGER blackjack_round_guard; ALTER TABLE games.game_rounds ENABLE TRIGGER blackjack_persistence_complete`, r.ID, r.WagerTransactionID)
	f.SQL(t, `ALTER TABLE games.blackjack_round_state DISABLE TRIGGER blackjack_state_guard; UPDATE games.blackjack_round_state SET shoe_hash=decode(repeat('00',32),'hex') WHERE round_id=$1; SET CONSTRAINTS ALL IMMEDIATE; ALTER TABLE games.blackjack_round_state ENABLE TRIGGER blackjack_state_guard`, r.ID)
	fingerprint := func() string {
		var value string
		historyfixture.Check(t, f.Pool.QueryRow(ctx, `SELECT md5(concat((SELECT jsonb_agg(to_jsonb(r) ORDER BY round_id)::text FROM games.game_rounds r),(SELECT jsonb_agg(to_jsonb(j) ORDER BY round_id)::text FROM games.round_jobs j),(SELECT jsonb_agg(to_jsonb(w) ORDER BY newapi_user_id,asset_type)::text FROM economy.wallet_balances w),(SELECT jsonb_agg(to_jsonb(l) ORDER BY ledger_entry_id)::text FROM economy.wallet_ledger l)))`).Scan(&value))
		return value
	}
	before := fingerprint()
	reader := newTestService(t, f.Runtime)
	missingKey, err := NewService(f.Runtime, Keyring{Active: "other", Keys: map[string][32]byte{"other": {1}}})
	historyfixture.Check(t, err)
	if _, err = missingKey.HistoryDetail(ctx, historyaccess.Own(user), r.ID); !errors.Is(err, historyaccess.ErrUnavailable) {
		t.Fatal("missing historical key did not fail safely", err)
	}
	if _, err = reader.HistoryDetail(ctx, historyaccess.Own(user), r.ID); !errors.Is(err, historyaccess.ErrUnavailable) {
		t.Fatal("corrupt BJ history not safe", err)
	}
	if _, err = reader.HistoryVerify(ctx, historyaccess.Own(user), r.ID); !errors.Is(err, historyaccess.ErrUnavailable) {
		t.Fatal("corrupt BJ verification not safe", err)
	}
	if before != fingerprint() {
		t.Fatal("history changed source, job, or money")
	}
	original, err := reader.Read(ctx, user, r.ID)
	if err != nil || original.RecoveryState != "NEEDS_REVIEW" || before == fingerprint() {
		t.Fatal("original gameplay repair behavior changed", err)
	}
	for _, id := range []string{r.ID, "00000000-0000-4000-8000-000000000099"} {
		if _, err = reader.HistoryDetail(ctx, historyaccess.Own(user+1), id); !errors.Is(err, historyaccess.ErrNotFound) {
			t.Fatal("ownership must precede corrupt decrypt")
		}
	}
	var repaired bool
	historyfixture.Check(t, f.Pool.QueryRow(ctx, `SELECT count(*)=2 AND bool_and(status='NEEDS_REVIEW' AND last_error='RECOVERY_MISMATCH') FROM games.round_jobs WHERE round_id=$1`, r.ID).Scan(&repaired))
	if !repaired {
		t.Fatal("original job repair lost")
	}
	t.Log("READ_ONLY: corruption caused no writes; original gameplay Read still marks Round/job for review")
}
