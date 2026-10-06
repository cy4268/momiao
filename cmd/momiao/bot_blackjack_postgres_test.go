package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/botgames"
	"github.com/cy4268/momiao/internal/games"
	bj "github.com/cy4268/momiao/internal/games/blackjack"
	"github.com/cy4268/momiao/internal/games/fairness"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

func requireFreshBlackjackFixture(t *testing.T) {
	t.Helper()
	path := os.Getenv("MOMIAO_GAMES_TEST_CONNECTION_FILE")
	if path == "" {
		t.Skip("fresh isolated local blackjack G1 connection required")
	}
	raw, e := os.ReadFile(path)
	var c struct{ OwnerURL, RuntimeURL, NativeURL string }
	if e != nil || json.Unmarshal(raw, &c) != nil {
		t.Fatal("blackjack fixture unreadable")
	}
	var database string
	for _, url := range []string{c.OwnerURL, c.RuntimeURL, c.NativeURL} {
		cfg, e := pgx.ParseConfig(url)
		if e != nil || cfg.Host != "127.0.0.1" || cfg.Port != 55432 || !strings.HasPrefix(cfg.Database, "momiao_test_g1_original_blackjack_") {
			t.Fatal("refusing non-blackjack loopback fixture")
		}
		if database != "" && database != cfg.Database {
			t.Fatal("split fixture databases")
		}
		database = cfg.Database
	}
	conn, e := pgx.Connect(context.Background(), c.OwnerURL)
	botGamesPGCheck(t, e, "fresh blackjack guard")
	defer conn.Close(context.Background())
	var fresh bool
	e = conn.QueryRow(context.Background(), `SELECT NOT EXISTS(SELECT FROM pg_namespace WHERE nspname IN ('games','economy','identity')) AND to_regclass('public.users') IS NULL`).Scan(&fresh)
	botGamesPGCheck(t, e, "empty blackjack database")
	if !fresh {
		t.Fatal("blackjack integration needs a NEW empty database; retain earlier evidence")
	}
}
func blackjackTestUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	_, e := rand.Read(b[:])
	botGamesPGCheck(t, e, "test UUID")
	millis := uint64(time.Now().UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(millis)
		millis >>= 8
	}
	b[6] = b[6]&15 | 0x70
	b[8] = b[8]&63 | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func (f *botGamesPGFixture) prepareBlackjack(t *testing.T, subject, wager string) botgames.BlackjackPrepared {
	t.Helper()
	f.next++
	p, e := f.service.PrepareBlackjack(context.Background(), subject, botgames.BlackjackPrepareInput{RequestID: strconv.FormatInt(f.next, 10), InitialWager: wager})
	botGamesPGCheck(t, e, "blackjack prepare")
	return p
}
func (f *botGamesPGFixture) blackjackAction(t *testing.T, subject string, r botgames.BlackjackRound, kind string) botgames.BlackjackActionResult {
	t.Helper()
	f.next++
	out, e := f.service.ActBlackjack(context.Background(), subject, botgames.BlackjackActionRequest{Quote: r.Quote, RequestID: strconv.FormatInt(f.next, 10), ActionType: kind})
	botGamesPGCheck(t, e, "blackjack "+kind)
	return out
}
func (f *botGamesPGFixture) blackjackRound(t *testing.T, subject string, p botgames.BlackjackPrepared) botgames.BlackjackRound {
	t.Helper()
	r, e := f.service.PlayBlackjack(context.Background(), subject, p.Quote)
	botGamesPGCheck(t, e, "blackjack play")
	return r
}
func (f *botGamesPGFixture) blackjackFinish(t *testing.T, subject string, r botgames.BlackjackRound) botgames.BlackjackRound {
	t.Helper()
	for i := 0; i < 4 && r.Status == "PLAYER_TURN"; i++ {
		r = f.blackjackAction(t, subject, r, "STAND").Round
	}
	if r.Status != "SETTLED" {
		t.Fatal("round not terminal")
	}
	return r
}

// This protected test fixture builds a *new* commitment before the Bot preview.
// Production keeps the unmodified shuffle, encrypted-seed authority, engine,
// transaction guards, typed action journal, wallet ledger and settlement path.
func (f *botGamesPGFixture) blackjackPrecommit(t *testing.T, user int64, match func([312]uint16) bool) games.Commitment {
	t.Helper()
	ctx := context.Background()
	b, e := f.engine.Bootstrap(ctx, user, "blackjack")
	botGamesPGCheck(t, e, "fixture blackjack bootstrap")
	if b.Next == nil {
		t.Fatal("missing fixture commitment")
	}
	c := *b.Next
	c.ID = blackjackTestUUID(t)
	c.ReservedRoundID = blackjackTestUUID(t)
	c.Nonce++
	uuid := func(s string) [16]byte {
		raw, e := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
		botGamesPGCheck(t, e, "fixture UUID bytes")
		return [16]byte(raw)
	}
	cfg, e := hex.DecodeString(c.ConfigHash)
	botGamesPGCheck(t, e, "fixture config hash")
	var seed [32]byte
	found := false
	for i := 0; i < 100000; i++ {
		seed = sha256.Sum256([]byte(fmt.Sprintf("blackjack-bot-protected-precommit-%d", i)))
		hash := sha256.Sum256(seed[:])
		c.ServerSeedHash = hex.EncodeToString(hash[:])
		fr := fairness.Round{ID: uuid(c.ReservedRoundID), Game: "blackjack", ClientSeed: c.ClientSeed, Nonce: c.Nonce, StreamVersion: c.Stream, AlgorithmVersion: c.Algorithm, ConfigVersion: c.ConfigVersion, ConfigHash: [32]byte(cfg), ServerSeedHash: hash}
		stream, e := fairness.NewStream(seed[:], fr, bj.ShuffleDomain)
		botGamesPGCheck(t, e, "fixture fair stream")
		shoe, e := bj.Shuffle(func(n uint32) (uint32, error) { v, e := fairness.UniformInt(stream, uint64(n)); return uint32(v), e })
		botGamesPGCheck(t, e, "fixture shuffle")
		if match(shoe) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("protected blackjack precommit not found")
	}
	key := [32]byte{9}
	block, e := aes.NewCipher(key[:])
	botGamesPGCheck(t, e, "fixture AES")
	aead, e := cipher.NewGCM(block)
	botGamesPGCheck(t, e, "fixture GCM")
	nonce := make([]byte, 12)
	_, e = rand.Read(nonce)
	botGamesPGCheck(t, e, "fixture nonce")
	aad := []byte("CHALDEA-GAME-SEED-AAD-V1\x00")
	id, round := uuid(c.ID), uuid(c.ReservedRoundID)
	aad = append(aad, id[:]...)
	aad = append(aad, round[:]...)
	lp := func(v string) {
		aad = binary.BigEndian.AppendUint16(aad, uint16(len(v)))
		aad = append(aad, []byte(v)...)
	}
	lp(strconv.FormatInt(user, 10))
	lp("blackjack")
	aad = binary.BigEndian.AppendUint64(aad, uint64(c.Nonce))
	lp(c.Algorithm)
	if c.EconomicVersion != "" {
		lp(c.EconomicVersion)
	}
	encrypted := aead.Seal(nil, nonce, seed[:], aad)
	e = f.owner.WithTx(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `UPDATE games.fairness_commitments SET state='INVALIDATED' WHERE commitment_id=$1`, b.Next.ID); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE games.fairness_nonce_cursors SET next_nonce=$2 WHERE newapi_user_id=$1 AND game_slug='blackjack'`, user, c.Nonce+1); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO games.fairness_commitments(commitment_id,reserved_round_id,newapi_user_id,game_slug,nonce,client_seed,client_seed_version,state,server_seed_hash,key_version,gcm_nonce,ciphertext,ruleset_version,algorithm_version,fairness_stream_version,game_config_version_id,game_config_hash,wager_policy_version_id,wager_policy_hash,resource_versions,economic_policy_version) VALUES($1,$2,$3,'blackjack',$4,$5,$6,'AVAILABLE',decode($7,'hex'),'bot-fixture-v1',$8,$9,$10,$11,$12,$13,decode($14,'hex'),$15,decode($16,'hex'),$17,NULLIF($18,''))`, c.ID, c.ReservedRoundID, user, c.Nonce, c.ClientSeed, c.ClientVersion, c.ServerSeedHash, nonce, encrypted, c.Ruleset, c.Algorithm, c.Stream, c.ConfigVersion, c.ConfigHash, c.PolicyVersion, c.PolicyHash, []byte(c.Resources), c.EconomicVersion)
		return e
	})
	botGamesPGCheck(t, e, "protected blackjack precommit insert")
	return c
}
func blackjackPair(shoe [312]uint16) bool {
	return shoe[0]%13 == 7 && shoe[2]%13 == 7 && shoe[1]%13 >= 1 && shoe[1]%13 <= 8 && shoe[4]%13 < 8 && shoe[5]%13 < 8
}
func blackjackNatural(shoe [312]uint16) bool {
	return shoe[0]%13 == 0 && shoe[2]%13 >= 9 && shoe[1]%13 >= 1 && shoe[1]%13 <= 8
}
func (f *botGamesPGFixture) blackjackEffects(t *testing.T, user int64, round string, rounds, actions, settlements int) {
	t.Helper()
	var gotRounds, wagers, gotActions, gotSettlements int
	e := f.owner.WithTx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM games.game_rounds WHERE newapi_user_id=$1),(SELECT count(*) FROM economy.asset_transactions WHERE newapi_user_id=$1 AND biz_type='GAME_WAGER'),(SELECT count(*) FROM games.round_actions WHERE round_id=NULLIF($2,'')::uuid),(SELECT count(*) FROM economy.asset_transactions WHERE newapi_user_id=$1 AND biz_type='GAME_SETTLEMENT')`, user, round).Scan(&gotRounds, &wagers, &gotActions, &gotSettlements)
	})
	botGamesPGCheck(t, e, "blackjack effect counts")
	if gotRounds != rounds || wagers != rounds || gotActions != actions || gotSettlements != settlements {
		t.Fatalf("effects rounds=%d wagers=%d actions=%d settlements=%d", gotRounds, wagers, gotActions, gotSettlements)
	}
}
func (f *botGamesPGFixture) blackjackLedger(t *testing.T, user int64, r botgames.BlackjackRound) {
	t.Helper()
	if r.Settlement == nil {
		t.Fatal("missing settlement")
	}
	var stake, credit, supply int64
	e := f.owner.WithTx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT (SELECT coalesce(sum(l.delta_units),0)::bigint FROM economy.wallet_ledger l JOIN economy.asset_transactions a ON a.transaction_id=l.transaction_id WHERE a.newapi_user_id=$1 AND a.biz_type IN ('GAME_WAGER','GAME_ADDITIONAL_WAGER')),(SELECT coalesce(sum(l.delta_units),0)::bigint FROM economy.wallet_ledger l JOIN economy.asset_transactions a ON a.transaction_id=l.transaction_id WHERE a.newapi_user_id=$1 AND a.biz_type='GAME_SETTLEMENT'),(SELECT coalesce(sum(CASE WHEN direction='ISSUE' THEN amount_units ELSE -amount_units END),0)::bigint FROM games.supply_events WHERE round_id=$2)`, user, r.RoundID).Scan(&stake, &credit, &supply)
	})
	botGamesPGCheck(t, e, "blackjack ledger")
	wantStake, _ := strconv.ParseInt(r.StakeUnits, 10, 64)
	wantCredit, _ := strconv.ParseInt(r.Settlement.CreditedPayoutUnits, 10, 64)
	wantNet, _ := strconv.ParseInt(r.Settlement.ActualNetUnits, 10, 64)
	if stake != -wantStake || credit != wantCredit || supply != wantNet {
		t.Fatalf("ledger stake=%d credit=%d supply=%d", stake, credit, supply)
	}
	v, e := f.engine.Verify(context.Background(), user, r.RoundID)
	botGamesPGCheck(t, e, "blackjack fair replay")
	if !v.Verified {
		t.Fatal("blackjack replay not verified")
	}
}
func TestBotBlackjackPostgres(t *testing.T) {
	requireFreshBlackjackFixture(t)
	f := newBotGamesPGFixture(t)
	ctx := context.Background()
	for _, path := range []string{"../../deploy/sql/runtime-grants-0015-slot.psql", "../../deploy/sql/runtime-grants-0016-blackjack.psql"} {
		raw, e := os.ReadFile(path)
		botGamesPGCheck(t, e, "existing blackjack grants")
		lines := []string{}
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "GRANT ") {
				lines = append(lines, line)
			}
		}
		f.exec(t, strings.ReplaceAll(strings.Join(lines, "\n"), `:"runtime_role"`, pgx.Identifier{f.connection.RuntimeRole}.Sanitize()), pgx.QueryExecModeSimpleProtocol)
	}
	t.Run("preview_cancel_and_expired_absent_are_read_only", func(t *testing.T) {
		user, subject := f.player(t, true, 50000000)
		f.blackjackPrecommit(t, user, blackjackPair)
		p := f.prepareBlackjack(t, subject, "10")
		if p.Action != "DEAL" || p.Round != nil || p.InitialWager != "10" || p.Ruleset != bj.FairRulesetVersion {
			t.Fatal("preview shape/rules", p.Action, p.Ruleset)
		}
		f.blackjackEffects(t, user, "", 0, 0, 0)
		_, e := f.service.LookupBlackjack(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "NOT_FOUND")
		f.now = f.now.Add(121 * time.Second)
		defer func() { f.now = f.now.Add(-121 * time.Second) }()
		_, e = f.service.PlayBlackjack(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "QUOTE_EXPIRED")
		_, e = f.service.LookupBlackjack(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "NOT_FOUND")
		f.blackjackEffects(t, user, "", 0, 0, 0)
		if f.balance(t, user) != 50000000 {
			t.Fatal("expired preview debited")
		}
	})
	t.Run("natural_terminal_and_original_deal_recovery", func(t *testing.T) {
		user, subject := f.player(t, true, 50000000)
		f.blackjackPrecommit(t, user, blackjackNatural)
		p := f.prepareBlackjack(t, subject, "11")
		r := f.blackjackRound(t, subject, p)
		if r.Status != "SETTLED" || r.Settlement == nil || r.LastPlayerActionAt != "" || r.AutoResolveAt != "" || !r.Hands[0].Natural || r.LegalActions == nil || len(r.LegalActions) != 0 || r.DealerTotal == nil {
			t.Fatal("immediate natural fields")
		}
		if r.Settlement.GrossPayoutUnits == "13750000" || r.Settlement.FairReturnUnits == "0" {
			t.Fatal("v2 fair return omitted")
		}
		f.now = f.now.Add(121 * time.Second)
		defer func() { f.now = f.now.Add(-121 * time.Second) }()
		recovered, e := f.service.PlayBlackjack(ctx, subject, p.Quote)
		botGamesPGCheck(t, e, "expired original natural")
		if recovered.RoundID != r.RoundID || recovered.Settlement.ActualNetUnits != r.Settlement.ActualNetUnits {
			t.Fatal("natural replay changed")
		}
		f.blackjackEffects(t, user, r.RoundID, 1, 0, 1)
		f.blackjackLedger(t, user, r)
	})
	t.Run("split_double_sparse_hands_and_web_bot_continuity", func(t *testing.T) {
		user, subject := f.player(t, true, 50000000)
		f.blackjackPrecommit(t, user, blackjackPair)
		p := f.prepareBlackjack(t, subject, "10")
		r := f.blackjackRound(t, subject, p)
		if r.Status != "PLAYER_TURN" || len(r.DealerCards) != 1 || r.DealerTotal != nil || !slices.Contains(r.LegalActions, "SPLIT") {
			t.Fatal("initial controls")
		}
		initialQuote := r.Quote
		split := f.blackjackAction(t, subject, r, "SPLIT")
		if split.AdditionalStakeUnits != "5000000" || len(split.Round.Hands) != 2 || split.Round.Hands[0].Index != 0 || split.Round.Hands[1].Index != 4 {
			t.Fatal("split indices/stake")
		}
		resume := f.prepareBlackjack(t, subject, "99999")
		if resume.Action != "RESUME" || resume.Round.RoundID != r.RoundID || resume.Round.StakeUnits != "10000000" {
			t.Fatal("resume initial wager ignored")
		}
		web, e := f.engine.BlackjackAction(ctx, user, r.RoundID, games.BlackjackActionInput{ActionID: blackjackTestUUID(t), ActionType: "DOUBLE", HandID: split.Round.ActiveHandID, ExpectedVersion: split.Round.RoundVersion})
		botGamesPGCheck(t, e, "website double")
		if web.StakeUnits != 15000000 {
			t.Fatal("web double stake")
		}
		current, e := f.service.StateBlackjack(ctx, subject, initialQuote)
		botGamesPGCheck(t, e, "read original after website move")
		if current.RoundVersion != "3" || current.ActiveHandID == split.Round.ActiveHandID {
			t.Fatal("wrong current hand")
		}
		original, e := f.service.LookupBlackjackAction(ctx, subject, botgames.BlackjackActionRequest{Quote: initialQuote, RequestID: split.RequestID, ActionType: "SPLIT"})
		botGamesPGCheck(t, e, "original split lookup")
		if original.AppliedRoundVersion != "2" || original.Round.RoundVersion != "3" || original.AdditionalStakeUnits != "5000000" {
			t.Fatal("receipt/fresh state conflated")
		}
		final := f.blackjackFinish(t, subject, current)
		f.blackjackEffects(t, user, r.RoundID, 1, 3, 1)
		f.blackjackLedger(t, user, final)
	})
	t.Run("same_action_concurrency_restart_expired_original_and_hash_conflicts", func(t *testing.T) {
		user, subject := f.player(t, true, 50000000)
		f.blackjackPrecommit(t, user, blackjackPair)
		p := f.prepareBlackjack(t, subject, "10")
		r := f.blackjackRound(t, subject, p)
		in := botgames.BlackjackActionRequest{Quote: r.Quote, RequestID: subject, ActionType: "SPLIT"}
		var wg sync.WaitGroup
		errs := make(chan error, 6)
		results := make(chan botgames.BlackjackActionResult, 6)
		for range 6 {
			wg.Add(1)
			go func() { defer wg.Done(); out, e := f.service.ActBlackjack(ctx, subject, in); errs <- e; results <- out }()
		}
		wg.Wait()
		close(errs)
		close(results)
		for e := range errs {
			botGamesPGCheck(t, e, "same concurrent action")
		}
		for out := range results {
			if out.AppliedRoundVersion != "2" || out.AdditionalStakeUnits != "5000000" {
				t.Fatal("concurrent receipt")
			}
		}
		f.blackjackEffects(t, user, r.RoundID, 1, 1, 0)
		changed := in
		changed.ActionType = "HIT"
		_, e := f.service.LookupBlackjackAction(ctx, subject, changed)
		botGamesWantFault(t, e, "IDEMPOTENCY_CONFLICT")
		_, e = f.service.ActBlackjack(ctx, subject, changed)
		botGamesWantFault(t, e, "IDEMPOTENCY_CONFLICT")
		current, e := f.service.StateBlackjack(ctx, subject, r.Quote)
		botGamesPGCheck(t, e, "current state")
		changed = in
		changed.Quote = current.Quote
		_, e = f.service.LookupBlackjackAction(ctx, subject, changed)
		botGamesWantFault(t, e, "IDEMPOTENCY_CONFLICT")
		engine, e := games.NewService(f.runtime, games.Keyring{Active: "bot-fixture-v1", Keys: map[string][32]byte{"bot-fixture-v1": {9}}})
		botGamesPGCheck(t, e, "restart engine")
		service, e := botgames.NewService(engine, botGamesResolver{native: f.native, platform: f.runtime, declaration: f.declaration}, [32]byte{2}, func() time.Time { return f.now })
		botGamesPGCheck(t, e, "restart bridge")
		f.now = f.now.Add(121 * time.Second)
		defer func() { f.now = f.now.Add(-121 * time.Second) }()
		out, e := service.LookupBlackjackAction(ctx, subject, in)
		botGamesPGCheck(t, e, "expired original lookup after restart")
		if out.AppliedRoundVersion != "2" || out.Round.RoundID != r.RoundID {
			t.Fatal("restart receipt")
		}
		fresh := in
		fresh.RequestID = "999000000000000001"
		_, e = service.LookupBlackjackAction(ctx, subject, fresh)
		botGamesWantFault(t, e, "NOT_FOUND")
		_, e = service.ActBlackjack(ctx, subject, fresh)
		botGamesWantFault(t, e, "QUOTE_EXPIRED")
		f.blackjackEffects(t, user, r.RoundID, 1, 1, 0)
	})
	t.Run("different_action_ids_same_version_only_one_applies", func(t *testing.T) {
		user, subject := f.player(t, true, 50000000)
		f.blackjackPrecommit(t, user, blackjackPair)
		r := f.blackjackRound(t, subject, f.prepareBlackjack(t, subject, "10"))
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, id := range []string{"999000000000000011", "999000000000000012"} {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				_, e := f.service.ActBlackjack(ctx, subject, botgames.BlackjackActionRequest{Quote: r.Quote, RequestID: id, ActionType: "SPLIT"})
				errs <- e
			}(id)
		}
		wg.Wait()
		close(errs)
		ok, stale := 0, 0
		for e := range errs {
			if e == nil {
				ok++
			} else if e.Error() == "BLACKJACK_STALE_STATE" {
				stale++
			} else {
				t.Fatal("concurrent refusal", e)
			}
		}
		if ok != 1 || stale != 1 {
			t.Fatal("concurrency counts", ok, stale)
		}
		f.blackjackEffects(t, user, r.RoundID, 1, 1, 0)
	})
	t.Run("web_created_resume_and_other_active_deal_rejected", func(t *testing.T) {
		user, subject := f.player(t, true, 50000000)
		f.blackjackPrecommit(t, user, blackjackPair)
		p := f.prepareBlackjack(t, subject, "10")
		web, e := f.engine.Create(ctx, user, "blackjack", "website-blackjack:"+subject, p.CommitmentID, games.CreateInput{Type: "BLACKJACK", InitialWager: "11"})
		botGamesPGCheck(t, e, "web created round")
		_, e = f.service.PlayBlackjack(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "BLACKJACK_ACTIVE_ROUND")
		_, e = f.service.LookupBlackjack(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "NOT_FOUND")
		resume := f.prepareBlackjack(t, subject, "999999999999999999999999")
		if resume.Action != "RESUME" || resume.Round.RoundID != web.ID || resume.Round.InitialWager != "11" {
			t.Fatal("web resume changed wager")
		}
		f.blackjackEffects(t, user, web.ID, 1, 0, 0)
	})
	t.Run("current_balance_not_historical_after_balance", func(t *testing.T) {
		user, subject := f.player(t, true, 10000000)
		f.blackjackPrecommit(t, user, blackjackPair)
		r := f.blackjackRound(t, subject, f.prepareBlackjack(t, subject, "10"))
		_, e := f.owner.Apply(ctx, platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: -1, BizType: "BOT_TEST", BizID: "drain:" + subject, EntryType: "TEST_DRAIN", IdempotencyKey: "blackjack-drain:" + subject})
		botGamesPGCheck(t, e, "external wallet drain")
		current, e := f.service.StateBlackjack(ctx, subject, r.Quote)
		botGamesPGCheck(t, e, "fresh wallet controls")
		if slices.Contains(current.LegalActions, "DOUBLE") || slices.Contains(current.LegalActions, "SPLIT") {
			t.Fatal("historical balance controls")
		}
		for _, kind := range []string{"DOUBLE", "SPLIT"} {
			_, e = f.service.ActBlackjack(ctx, subject, botgames.BlackjackActionRequest{Quote: r.Quote, RequestID: subject, ActionType: kind})
			botGamesWantFault(t, e, "INSUFFICIENT_CHIPS")
		}
		f.blackjackEffects(t, user, r.RoundID, 1, 0, 0)
		_, e = f.owner.Apply(ctx, platform.Mutation{UserID: user, Asset: platform.AvailableChips, DeltaUnits: 7000001, BizType: "BOT_TEST", BizID: "gain:" + subject, EntryType: "TEST_GRANT", IdempotencyKey: "blackjack-gain:" + subject})
		botGamesPGCheck(t, e, "external wallet credit")
		current, e = f.service.StateBlackjack(ctx, subject, r.Quote)
		botGamesPGCheck(t, e, "fresh funded controls")
		if !slices.Contains(current.LegalActions, "DOUBLE") {
			t.Fatal("new funds hidden")
		}
		final := f.blackjackAction(t, subject, current, "DOUBLE").Round
		f.blackjackLedger(t, user, final)
		net, _ := strconv.ParseInt(final.Settlement.ActualNetUnits, 10, 64)
		if f.balance(t, user) != 17000000+net {
			t.Fatal("multi-step actual delta with unrelated credit")
		}
	})
	t.Run("long_view_expiry_does_not_stand_and_maintenance_resumes", func(t *testing.T) {
		user, subject := f.player(t, true, 50000000)
		f.blackjackPrecommit(t, user, blackjackPair)
		r := f.blackjackRound(t, subject, f.prepareBlackjack(t, subject, "10"))
		f.now = f.now.Add(121 * time.Second)
		defer func() { f.now = f.now.Add(-121 * time.Second) }()
		_, e := f.service.ActBlackjack(ctx, subject, botgames.BlackjackActionRequest{Quote: r.Quote, RequestID: subject, ActionType: "STAND"})
		botGamesWantFault(t, e, "QUOTE_EXPIRED")
		f.exec(t, `UPDATE games.runtime_gate SET maintenance=true WHERE singleton`)
		defer f.exec(t, `UPDATE games.runtime_gate SET maintenance=false WHERE singleton`)
		p := f.prepareBlackjack(t, subject, "10")
		if p.Action != "RESUME" || p.Round.RoundVersion != "1" || p.Round.LastPlayerActionAt != r.LastPlayerActionAt || p.Round.AutoResolveAt != r.AutoResolveAt {
			t.Fatal("expired view changed round")
		}
		final := f.blackjackAction(t, subject, *p.Round, "STAND").Round
		f.blackjackEffects(t, user, r.RoundID, 1, 1, 1)
		f.blackjackLedger(t, user, final)
	})
	t.Run("hole_and_authority_never_in_private_http_payload", func(t *testing.T) {
		user, subject := f.player(t, true, 50000000)
		f.blackjackPrecommit(t, user, blackjackPair)
		p := f.prepareBlackjack(t, subject, "10")
		h, e := newBotGamesHandler(f.service, botGamesTestToken)
		botGamesPGCheck(t, e, "private handler")
		raw, _ := json.Marshal(map[string]string{"quote": p.Quote})
		req := botGamesRequest("POST", "/internal/v1/bot-games/blackjack/play", string(raw))
		req.Header.Set("X-Discord-User", subject)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 200 || w.Body.Len() > 16384 {
			t.Fatalf("HTTP status=%d body=%s", w.Code, w.Body.String())
		}
		var out map[string]any
		if json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out) != 18 || len(out["dealer_cards"].([]any)) != 1 || out["dealer_total"] != nil {
			t.Fatal("private HTTP shape")
		}
		for _, secret := range []string{"server_seed", "shoe_instance_ids", "card_instance_id", "ciphertext", "balance_after_units", "available_units", "wager_transaction_id"} {
			if strings.Contains(w.Body.String(), `"`+secret+`"`) {
				t.Fatal("authority leak", secret)
			}
		}
	})
	t.Run("wrong_subject_binding_game_and_kind", func(t *testing.T) {
		user, subject := f.player(t, true, 50000000)
		f.blackjackPrecommit(t, user, blackjackPair)
		p := f.prepareBlackjack(t, subject, "10")
		other, otherSubject := f.player(t, true, 50000000)
		_, e := f.service.PlayBlackjack(ctx, otherSubject, p.Quote)
		botGamesWantFault(t, e, "UNAUTHORIZED")
		_, e = f.service.StateBlackjack(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "INVALID_REQUEST")
		_, e = f.service.Play(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "UNAUTHORIZED")
		f.exec(t, `UPDATE public.users SET discord_id='' WHERE id=$1`, user)
		f.exec(t, `UPDATE public.users SET discord_id=$2 WHERE id=$1`, other, subject)
		_, e = f.service.PlayBlackjack(ctx, subject, p.Quote)
		botGamesWantFault(t, e, "BINDING_CHANGED")
		f.blackjackEffects(t, user, "", 0, 0, 0)
		f.blackjackEffects(t, other, "", 0, 0, 0)
	})
	t.Run("needs_review_freezes_original", func(t *testing.T) {
		user, subject := f.player(t, true, 50000000)
		f.blackjackPrecommit(t, user, blackjackPair)
		r := f.blackjackRound(t, subject, f.prepareBlackjack(t, subject, "10"))
		e := f.owner.WithTx(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `ALTER TABLE games.blackjack_round_state DISABLE TRIGGER blackjack_state_guard; UPDATE games.blackjack_round_state SET shoe_hash=decode(repeat('00',32),'hex') WHERE round_id='`+r.RoundID+`'; SET CONSTRAINTS ALL IMMEDIATE; ALTER TABLE games.blackjack_round_state ENABLE TRIGGER blackjack_state_guard`, pgx.QueryExecModeSimpleProtocol)
			return e
		})
		botGamesPGCheck(t, e, "protected authority corruption fixture")
		_, e = f.service.StateBlackjack(ctx, subject, r.Quote)
		botGamesWantFault(t, e, "BLACKJACK_NEEDS_REVIEW")
		_, e = f.service.ActBlackjack(ctx, subject, botgames.BlackjackActionRequest{Quote: r.Quote, RequestID: subject, ActionType: "HIT"})
		botGamesWantFault(t, e, "BLACKJACK_NEEDS_REVIEW")
		f.blackjackEffects(t, user, r.RoundID, 1, 0, 0)
	})
	t.Run("economic_action_cap_and_raw_win_actual_break_even", func(t *testing.T) {
		f.exec(t, platform.NativeQuotaMigration)
		f.exec(t, `UPDATE momiao_quota.settings SET enabled=true; UPDATE economy.policy_runtime SET active_version='economy-cap-v1' WHERE singleton`, pgx.QueryExecModeSimpleProtocol)
		defer f.exec(t, `UPDATE economy.policy_runtime SET active_version=NULL WHERE singleton`)
		user, subject := f.player(t, true, 1000000000000)
		f.blackjackPrecommit(t, user, blackjackPair)
		p := f.prepareBlackjack(t, subject, "600000")
		if p.MaximumWagerUnits != "500000000000" {
			t.Fatal("single player cap")
		}
		r := f.blackjackRound(t, subject, p)
		if slices.Contains(r.LegalActions, "DOUBLE") || slices.Contains(r.LegalActions, "SPLIT") {
			t.Fatal("cumulative legal cap")
		}
		for _, kind := range []string{"DOUBLE", "SPLIT"} {
			_, e := f.service.ActBlackjack(ctx, subject, botgames.BlackjackActionRequest{Quote: r.Quote, RequestID: subject, ActionType: kind})
			botGamesWantFault(t, e, "BLACKJACK_ACTION_NOT_ALLOWED")
		}
		f.blackjackEffects(t, user, r.RoundID, 1, 0, 0)
		user, subject = f.player(t, true, 2000000000)
		_, e := f.owner.Apply(ctx, platform.Mutation{UserID: user, Asset: platform.ReserveAPICredit, DeltaUnits: platform.AssetCapUnits - 2000000000, BizType: "BOT_CAP_TEST", BizID: "bj-cap:" + subject, EntryType: "TEST_GRANT", IdempotencyKey: "bj-cap:" + subject})
		botGamesPGCheck(t, e, "fill unified cap")
		f.blackjackPrecommit(t, user, blackjackNatural)
		p = f.prepareBlackjack(t, subject, "11")
		r = f.blackjackRound(t, subject, p)
		if r.Settlement.Result != "WIN" || r.Settlement.ActualNetUnits != "0" || r.Settlement.CreditedPayoutUnits != "5500000" || r.Settlement.WithheldUnits == "0" || r.Settlement.FairReturnUnits == "0" {
			t.Fatalf("raw/actual cap settlement %+v", r.Settlement)
		}
		f.blackjackLedger(t, user, r)
		if f.balance(t, user) != 2000000000 {
			t.Fatal("cap wallet changed")
		}
	})
	t.Run("ordinary_hit_stand_keep_original_action_receipt", func(t *testing.T) {
		user, subject := f.player(t, true, 50000000)
		f.blackjackPrecommit(t, user, func(shoe [312]uint16) bool {
			return shoe[0]%13 == 1 && shoe[2]%13 == 2 && shoe[1]%13 >= 2 && shoe[1]%13 <= 8 && shoe[4]%13 <= 5
		})
		r := f.blackjackRound(t, subject, f.prepareBlackjack(t, subject, "10"))
		old := r.Quote
		hit := f.blackjackAction(t, subject, r, "HIT")
		if hit.Round.Status != "PLAYER_TURN" || len(hit.Round.Hands[0].Cards) != 3 || hit.AdditionalStakeUnits != "0" || slices.Contains(hit.Round.LegalActions, "DOUBLE") {
			t.Fatal("ordinary hit state")
		}
		final := f.blackjackAction(t, subject, hit.Round, "STAND").Round
		recovered, e := f.service.LookupBlackjackAction(ctx, subject, botgames.BlackjackActionRequest{Quote: old, RequestID: hit.RequestID, ActionType: "HIT"})
		botGamesPGCheck(t, e, "historical hit current terminal")
		if recovered.AppliedRoundVersion != "2" || recovered.Round.RoundVersion != "3" || recovered.Round.Status != "SETTLED" {
			t.Fatal("historical hit replaced by stand")
		}
		f.blackjackEffects(t, user, r.RoundID, 1, 2, 1)
		f.blackjackLedger(t, user, final)
	})
	t.Run("four_hands_use_sparse_order_and_reject_fifth", func(t *testing.T) {
		user, subject := f.player(t, true, 100000000)
		f.blackjackPrecommit(t, user, func(shoe [312]uint16) bool {
			return shoe[0]%13 >= 9 && shoe[2]%13 >= 9 && shoe[4]%13 >= 9 && shoe[6]%13 >= 9 && shoe[1]%13 >= 1 && shoe[1]%13 <= 8 && shoe[8]%13 >= 1
		})
		r := f.blackjackRound(t, subject, f.prepareBlackjack(t, subject, "10"))
		for range 3 {
			r = f.blackjackAction(t, subject, r, "SPLIT").Round
		}
		indexes := []int{}
		for _, h := range r.Hands {
			indexes = append(indexes, h.Index)
		}
		if !slices.Equal(indexes, []int{0, 1, 2, 4}) || r.StakeUnits != "20000000" || slices.Contains(r.LegalActions, "SPLIT") {
			t.Fatal("four-hand sparse order", indexes)
		}
		_, e := f.service.ActBlackjack(ctx, subject, botgames.BlackjackActionRequest{Quote: r.Quote, RequestID: subject, ActionType: "SPLIT"})
		botGamesWantFault(t, e, "BLACKJACK_ACTION_NOT_ALLOWED")
		final := f.blackjackFinish(t, subject, r)
		f.blackjackLedger(t, user, final)
	})
	t.Run("original_action_reused_on_another_round_conflicts", func(t *testing.T) {
		user, subject := f.player(t, true, 100000000)
		f.blackjackPrecommit(t, user, blackjackPair)
		r := f.blackjackRound(t, subject, f.prepareBlackjack(t, subject, "10"))
		split := f.blackjackAction(t, subject, r, "SPLIT")
		f.blackjackFinish(t, subject, split.Round)
		f.blackjackPrecommit(t, user, blackjackPair)
		next := f.blackjackRound(t, subject, f.prepareBlackjack(t, subject, "10"))
		in := botgames.BlackjackActionRequest{Quote: next.Quote, RequestID: split.RequestID, ActionType: "SPLIT"}
		_, e := f.service.LookupBlackjackAction(ctx, subject, in)
		botGamesWantFault(t, e, "IDEMPOTENCY_CONFLICT")
		_, e = f.service.ActBlackjack(ctx, subject, in)
		botGamesWantFault(t, e, "IDEMPOTENCY_CONFLICT")
		f.blackjackEffects(t, user, next.RoundID, 2, 0, 1)
	})
	t.Run("committed_action_failed_current_read_recovers_without_write", func(t *testing.T) {
		user, subject := f.player(t, true, 50000000)
		f.blackjackPrecommit(t, user, blackjackPair)
		r := f.blackjackRound(t, subject, f.prepareBlackjack(t, subject, "10"))
		g := &blackjackReadLossEngine{Service: f.engine}
		service, e := botgames.NewService(g, botGamesResolver{native: f.native, platform: f.runtime, declaration: f.declaration}, [32]byte{2}, func() time.Time { return f.now })
		botGamesPGCheck(t, e, "loss bridge")
		in := botgames.BlackjackActionRequest{Quote: r.Quote, RequestID: subject, ActionType: "SPLIT"}
		_, e = service.ActBlackjack(ctx, subject, in)
		botGamesWantFault(t, e, "UPSTREAM_UNAVAILABLE")
		if g.actions != 1 {
			t.Fatal("action count")
		}
		f.blackjackEffects(t, user, r.RoundID, 1, 1, 0)
		recovered, e := f.service.LookupBlackjackAction(ctx, subject, in)
		botGamesPGCheck(t, e, "original after committed read loss")
		if recovered.AppliedRoundVersion != "2" || recovered.Round.StakeUnits != "10000000" || g.actions != 1 {
			t.Fatal("recovery replayed write")
		}
		f.blackjackEffects(t, user, r.RoundID, 1, 1, 0)
	})
	t.Run("postcommit_business_read_failures_are_http_503", func(t *testing.T) {
		for _, write := range []bool{false, true} {
			for _, failure := range []error{games.ErrNotFound, bj.ErrNeedsReview, games.ErrUnavailable} {
				t.Run(fmt.Sprintf("write_%t/%s", write, failure), func(t *testing.T) {
					user, subject := f.player(t, true, 50000000)
					f.blackjackPrecommit(t, user, blackjackPair)
					r := f.blackjackRound(t, subject, f.prepareBlackjack(t, subject, "10"))
					in := botgames.BlackjackActionRequest{Quote: r.Quote, RequestID: subject, ActionType: "SPLIT"}
					if !write {
						_, e := f.service.ActBlackjack(ctx, subject, in)
						botGamesPGCheck(t, e, "existing original action")
					}
					g := &blackjackReadLossEngine{Service: f.engine, lose: !write, readErr: failure}
					service, e := botgames.NewService(g, botGamesResolver{native: f.native, platform: f.runtime, declaration: f.declaration}, [32]byte{2}, func() time.Time { return f.now })
					botGamesPGCheck(t, e, "postcommit failure bridge")
					h, e := newBotGamesHandler(service, botGamesTestToken)
					botGamesPGCheck(t, e, "postcommit failure handler")
					body, _ := json.Marshal(in)
					req := botGamesRequest("POST", "/internal/v1/bot-games/blackjack/action", string(body))
					req.Header.Set("X-Discord-User", subject)
					w := httptest.NewRecorder()
					h.ServeHTTP(w, req)
					if w.Code != 503 || w.Body.String() != `{"error":"UPSTREAM_UNAVAILABLE"}`+"\n" {
						t.Fatalf("committed response must stay UNKNOWN HTTP 503: %d %s", w.Code, w.Body.String())
					}
					wantWrites := 0
					if write {
						wantWrites = 1
					}
					if g.actions != wantWrites {
						t.Fatal("committed write replay", g.actions)
					}
					recovered, e := f.service.LookupBlackjackAction(ctx, subject, in)
					botGamesPGCheck(t, e, "recover original after HTTP 503")
					if recovered.AppliedRoundVersion != "2" || recovered.Round.StakeUnits != "10000000" {
						t.Fatal("wrong original receipt")
					}
					f.blackjackEffects(t, user, r.RoundID, 1, 1, 0)
				})
			}
		}
	})
	t.Run("accepted_deal_current_read_failures_are_http_503", func(t *testing.T) {
		for _, operation := range []string{"create", "replay", "lookup"} {
			for _, failure := range []error{games.ErrNotFound, bj.ErrNeedsReview} {
				t.Run(operation+"/"+failure.Error(), func(t *testing.T) {
					user, subject := f.player(t, true, 50000000)
					commitment := f.blackjackPrecommit(t, user, blackjackPair)
					p := f.prepareBlackjack(t, subject, "10")
					if operation != "create" {
						f.blackjackRound(t, subject, p)
						f.now = f.now.Add(121 * time.Second)
						defer func() { f.now = f.now.Add(-121 * time.Second) }()
					}
					g := &blackjackReadLossEngine{Service: f.engine, lose: operation != "create", readErr: failure}
					service, e := botgames.NewService(g, botGamesResolver{native: f.native, platform: f.runtime, declaration: f.declaration}, [32]byte{2}, func() time.Time { return f.now })
					botGamesPGCheck(t, e, "accepted deal failure bridge")
					h, e := newBotGamesHandler(service, botGamesTestToken)
					botGamesPGCheck(t, e, "accepted deal failure handler")
					body, _ := json.Marshal(map[string]string{"quote": p.Quote})
					endpoint := "play"
					if operation == "lookup" {
						endpoint = "lookup"
					}
					req := botGamesRequest("POST", "/internal/v1/bot-games/blackjack/"+endpoint, string(body))
					req.Header.Set("X-Discord-User", subject)
					w := httptest.NewRecorder()
					h.ServeHTTP(w, req)
					wantCreates := 0
					if operation == "create" {
						wantCreates = 1
					}
					if g.creates != wantCreates || g.actions != 0 {
						t.Fatal("accepted deal repeated write", g.creates, g.actions)
					}
					recovered, e := f.service.LookupBlackjack(ctx, subject, p.Quote)
					botGamesPGCheck(t, e, "recover original deal after failed read")
					if recovered.RoundID != commitment.ReservedRoundID || recovered.RoundVersion != "1" || recovered.StakeUnits != "5000000" || f.balance(t, user) != 45000000 {
						t.Fatal("original deal recovery changed round or wager")
					}
					f.blackjackEffects(t, user, recovered.RoundID, 1, 0, 0)
					if w.Code != 503 || w.Body.String() != `{"error":"UPSTREAM_UNAVAILABLE"}`+"\n" {
						t.Fatalf("accepted deal must stay UNKNOWN HTTP 503: %d %s", w.Code, w.Body.String())
					}
				})
			}
		}
	})
	t.Run("four_existing_adapters_still_work", func(t *testing.T) {
		user, subject := f.player(t, true, 500000000)
		dice := f.prepare(t, subject)
		_, e := f.service.Play(ctx, subject, dice.Quote)
		botGamesPGCheck(t, e, "legacy dice")
		slot, e := f.service.PrepareSlot(ctx, subject, botgames.SlotPrepareInput{RequestID: subject, TotalWager: "10"})
		botGamesPGCheck(t, e, "legacy slot prepare")
		_, e = f.service.PlaySlot(ctx, subject, slot.Quote)
		botGamesPGCheck(t, e, "legacy slot play")
		summon, e := f.service.PrepareSummon(ctx, subject, botgames.SummonPrepareInput{RequestID: subject, BaseWager: "10", Mode: "SINGLE"})
		botGamesPGCheck(t, e, "legacy summon prepare")
		_, e = f.service.PlaySummon(ctx, subject, summon.Quote)
		botGamesPGCheck(t, e, "legacy summon play")
		scratch, e := f.service.PrepareScratch(ctx, subject, botgames.ScratchPrepareInput{RequestID: subject, Wager: "10"})
		botGamesPGCheck(t, e, "legacy scratch prepare")
		_, e = f.service.PlayScratch(ctx, subject, scratch.Quote)
		botGamesPGCheck(t, e, "legacy scratch play")
		f.effects(t, user, 4)
	})
}

// A delivery/current-read failure seam, never an engine or wallet substitute.
type blackjackReadLossEngine struct {
	readErr error
	*games.Service
	lose             bool
	actions, creates int
}

func (g *blackjackReadLossEngine) Create(ctx context.Context, user int64, game, key, commitment string, in games.CreateInput) (games.GameRound, error) {
	g.creates++
	r, e := g.Service.Create(ctx, user, game, key, commitment, in)
	if e == nil {
		g.lose = true
	}
	return r, e
}
func (g *blackjackReadLossEngine) BlackjackAction(ctx context.Context, user int64, round string, in games.BlackjackActionInput) (games.GameRound, error) {
	g.actions++
	r, e := g.Service.BlackjackAction(ctx, user, round, in)
	if e == nil {
		g.lose = true
	}
	return r, e
}
func (g *blackjackReadLossEngine) Read(ctx context.Context, user int64, round string) (games.GameRound, error) {
	if g.lose {
		g.lose = false
		if g.readErr != nil {
			return games.GameRound{}, g.readErr
		}
		return games.GameRound{}, games.ErrUnavailable
	}
	return g.Service.Read(ctx, user, round)
}
