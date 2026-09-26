package poker

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/historyaccess"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/cy4268/momiao/internal/poker/fairness"
	"github.com/jackc/pgx/v5"
)

var ErrHistoryCursorStale = errors.New("poker history: hand changed; restart pagination")

func (r *HistoryReader) HandDetail(ctx context.Context, access historyaccess.Access, id string, q HistoryHandQuery) (HistoryHandDetail, error) {
	var d HistoryHandDetail
	var stale bool
	err := r.read(ctx, access, id, func(ctx context.Context, tx pgx.Tx, viewer, subject int64) error {
		e := r.historyHand(ctx, tx, viewer, subject, id, q, &d)
		stale = errors.Is(e, ErrHistoryCursorStale)
		return e
	})
	if stale {
		err = ErrHistoryCursorStale
	}
	if err != nil {
		return HistoryHandDetail{}, err
	}
	return d, nil
}

func (r *HistoryReader) HandFairness(ctx context.Context, access historyaccess.Access, id string) (FairnessView, error) {
	var v FairnessView
	err := r.read(ctx, access, id, func(ctx context.Context, tx pgx.Tx, viewer, subject int64) error {
		var participant bool
		if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM poker.hand_participants WHERE hand_id=$1 AND newapi_user_id=$2)", id, viewer).Scan(&participant); e != nil {
			return e
		}
		if !participant {
			return historyaccess.ErrNotFound
		}
		var d HistoryHandDetail
		if e := r.historyHand(ctx, tx, viewer, subject, id, HistoryHandQuery{}, &d); e != nil {
			return e
		}
		var e error
		v, e = readFairness(ctx, tx, r.keys, id, viewer)
		return e
	})
	if err != nil {
		return FairnessView{}, err
	}
	return v, nil
}

func (r *HistoryReader) historyHand(ctx context.Context, tx pgx.Tx, viewer, subject int64, id string, q HistoryHandQuery, d *HistoryHandDetail) error {
	var sealed, snapshot, hash, seedHash, deckHash, effectiveHash []byte
	var sequence uint64
	var deckCursor, owners int
	var owned bool
	var economicVersion string
	err := tx.QueryRow(ctx, `SELECT h.hand_id::text,h.table_id::text,h.hand_no,h.state,h.button_seat,h.hand_version,h.event_sequence,
	 h.created_at,h.settled_at,h.setup_cipher,h.snapshot_cipher,h.snapshot_hash,p.session_id::text,p.seat_no,
	 s.newapi_user_id=p.newapi_user_id AND s.seat_no=p.seat_no AND s.table_id=h.table_id,count(*) OVER(),
	 f.server_seed_hash,f.deck_hash,f.effective_client_seed_hash,f.algorithm_version,f.deck_version,f.deal_sequence_version,
	 f.next_deck_index,f.full_fairness_reveal_at,clock_timestamp(),coalesce(h.economic_policy_version,'')
	 FROM poker.hands h JOIN poker.hand_participants p USING(hand_id) JOIN poker.sessions s USING(session_id)
	 LEFT JOIN poker.hand_fairness f USING(hand_id) WHERE h.hand_id=$1 AND p.newapi_user_id=$2`, id, subject).Scan(
		&d.ID, &d.TableID, &d.Number, &d.State, &d.Button, &d.Version, &sequence, &d.CreatedAt, &d.SettledAt,
		&sealed, &snapshot, &hash, &d.SessionID, &d.Seat, &owned, &owners, &seedHash, &deckHash, &effectiveHash,
		&d.Fairness.AlgorithmVersion, &d.Configuration.DeckVersion, &d.Configuration.DealVersion, &deckCursor, &d.Fairness.RevealAt, &d.ReadAt, &economicVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return historyaccess.ErrNotFound
	}
	if err != nil {
		return err
	}
	if !owned || owners != 1 || d.Number < 1 || len(seedHash) != 32 || len(deckHash) != 32 || len(effectiveHash) != 32 ||
		(d.State == "SETTLED" && (d.SettledAt == nil || d.Fairness.RevealAt == nil || !d.Fairness.RevealAt.Equal(d.SettledAt.Add(24*time.Hour)))) ||
		(d.State != "SETTLED" && (d.SettledAt != nil || d.Fairness.RevealAt != nil)) {
		return historyaccess.ErrUnavailable
	}
	var setup handSetup
	plain, err := openCipher(r.keys, id+":setup", sealed)
	if err != nil || json.Unmarshal(plain, &setup) != nil {
		return historyaccess.ErrUnavailable
	}
	if setup.Config.HandID != id || setup.Config.ButtonSeat != d.Button || setup.Config.EvaluatorVersion != engine.EvaluatorVersion ||
		len(setup.Config.Deck) != 52 || len(setup.Seats) < 2 || len(setup.Seats) > 9 || len(setup.Sessions) != len(setup.Seats) || len(setup.Contributions) != len(setup.Seats) ||
		d.Fairness.AlgorithmVersion != fairness.AlgorithmVersion || d.Configuration.DeckVersion != fairness.AlgorithmVersion || d.Configuration.DealVersion != "poker-deal-v1" {
		return historyaccess.ErrUnavailable
	}
	cards, seen := make([]byte, 52), [52]bool{}
	for i, card := range setup.Config.Deck {
		if card >= 52 || seen[card] {
			return historyaccess.ErrUnavailable
		}
		cards[i], seen[card] = byte(card), true
	}
	deckSum := sha256.Sum256(cards)
	effective, err := fairness.Effective(d.TableID, id, setup.Contributions)
	if err != nil || !equal(deckSum[:], deckHash) || !equal(effective[:], effectiveHash) {
		return historyaccess.ErrUnavailable
	}
	if setup.EconomicVersion != economicVersion {
		return historyaccess.ErrUnavailable
	}
	validBlind := false
	for _, bb := range []int64{10, 20, 50, 100, 200, 1000} {
		validBlind = validBlind || (setup.Config.BigBlind == bb*engine.UnitsPerChip && setup.Config.SmallBlind == bb/2*engine.UnitsPerChip)
	}
	if !validBlind {
		return historyaccess.ErrUnavailable
	}
	state := engine.State{Config: setup.Config, Street: engine.Street(d.State)}
	if d.State == "COMMITTED" {
		if len(snapshot) != 0 || len(hash) != 0 || d.Version != 0 || sequence != 0 || deckCursor != 0 {
			return historyaccess.ErrUnavailable
		}
	} else {
		e, err := restoreHistoryEngine(ctx, tx, r.keys, id)
		if err != nil {
			return err
		}
		state = e.State()
		if !reflect.DeepEqual(state.Config, setup.Config) || state.DeckCursor != deckCursor || len(state.Players) != len(setup.Seats) ||
			(d.State == "SETTLED" && !state.Events[len(state.Events)-1].At.Equal(*d.SettledAt)) {
			return historyaccess.ErrUnavailable
		}
	}
	d.Metadata, err = historyMetadata(ctx, tx, subject, "POKER_HAND", id)
	if err != nil {
		return err
	}
	deckVersion, dealVersion := d.Configuration.DeckVersion, d.Configuration.DealVersion
	d.Configuration = historyConfiguration()
	d.Configuration.BlindOrigin, d.Configuration.SmallBlind, d.Configuration.BigBlind = "persisted_hand_setup", &setup.Config.SmallBlind, &setup.Config.BigBlind
	d.Configuration.EvaluatorVersion, d.Configuration.DeckVersion, d.Configuration.DealVersion = setup.Config.EvaluatorVersion, deckVersion, dealVersion
	d.Fairness.HandID, d.Fairness.ServerSeedHash, d.Fairness.DeckHash = id, hex.EncodeToString(seedHash), hex.EncodeToString(deckHash)
	d.Participants, d.Board, d.Pots, d.Returns = []HistoryParticipant{}, []int{}, []HistoryPot{}, []HistoryReturn{}
	public, viewerSeat, users, buttonFound := releasedHoles(state), 0, map[int64]bool{}, false
	rows, err := tx.Query(ctx, `SELECT p.seat_no,p.session_id::text,p.newapi_user_id,p.hand_start_stack_units,p.street_committed_units,p.total_committed_units,
	 p.folded,p.contribution_version,p.contribution_cipher,s.display_name_snapshot,
	 s.newapi_user_id=p.newapi_user_id AND s.seat_no=p.seat_no AND s.table_id=$2
	 FROM poker.hand_participants p JOIN poker.sessions s USING(session_id) WHERE p.hand_id=$1 ORDER BY p.seat_no`, id, d.TableID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p HistoryParticipant
		var session string
		var user int64
		var street, total int64
		var contribution uint64
		var cipher []byte
		var valid bool
		if err = rows.Scan(&p.Seat, &session, &user, &p.Initial, &street, &total, &p.Folded, &contribution, &cipher, &p.DisplayName, &valid); err != nil {
			break
		}
		i := len(d.Participants)
		value, e := openCipher(r.keys, fmt.Sprint(id, ":contribution:", p.Seat), cipher)
		if i >= len(setup.Seats) || e != nil || !valid || users[user] || user <= 0 || p.Initial <= 0 || p.Initial%engine.UnitsPerChip != 0 ||
			setup.Seats[i].SeatNo != p.Seat || setup.Seats[i].PlayerID != strconv.FormatInt(user, 10) || setup.Seats[i].Stack != p.Initial ||
			setup.Sessions[p.Seat] != session || setup.Contributions[i].Seat != p.Seat || setup.Contributions[i].Version != contribution || setup.Contributions[i].Value != string(value) {
			err = historyaccess.ErrUnavailable
			break
		}
		users[user], buttonFound = true, buttonFound || p.Seat == d.Button
		if d.State == "COMMITTED" {
			if street != 0 || total != 0 || p.Folded || p.Seat < 1 || p.Seat > 9 || setup.Seats[i].TimeoutSitOut || setup.Seats[i].ConsecutiveTimeouts < 0 {
				err = historyaccess.ErrUnavailable
				break
			}
		} else {
			actual := state.Players[i]
			if actual.SeatNo != p.Seat || actual.PlayerID != strconv.FormatInt(user, 10) || actual.InitialStack != p.Initial || actual.StreetCommitted != street || actual.TotalCommitted != total || actual.Folded != p.Folded {
				err = historyaccess.ErrUnavailable
				break
			}
			if user == viewer {
				p.HoleCards, viewerSeat = []int{int(actual.Hole[0]), int(actual.Hole[1])}, p.Seat
			}
			p.PublicHoleCards = public[p.Seat]
			if d.State == "SETTLED" {
				ending, net := actual.Stack, actual.Stack-p.Initial
				p.Ending, p.Net = &ending, &net
			}
		}
		p.NameOrigin = "persisted_session_snapshot"
		d.Participants = append(d.Participants, p)
	}
	rows.Close()
	if err != nil {
		return err
	}
	if rows.Err() != nil || len(d.Participants) != len(setup.Seats) || !buttonFound {
		return historyaccess.ErrUnavailable
	}
	for _, card := range state.Board {
		d.Board = append(d.Board, int(card))
	}
	if err = r.historyHandFacts(ctx, tx, id, state, d); err != nil {
		return err
	}

	if d.State == "SETTLED" {
		policy, err := platform.ResolveEconomicPolicyInTx(ctx, tx, economicVersion)
		if err != nil {
			return err
		}
		paid, gross := handContributions(state)
		for i, player := range state.Players {
			user, err := strconv.ParseInt(player.PlayerID, 10, 64)
			if err != nil {
				return err
			}
			c, err := platform.LoadCapSettlementInTx(ctx, tx, "POKER_HAND", id, user)
			if err != nil {
				return err
			}
			if policy.Version == "" {
				if c != nil {
					return historyaccess.ErrUnavailable
				}
				continue
			}
			if c == nil || c.PolicyVersion != policy.Version || c.PolicyHash != policy.Hash || c.StakeUnits != paid[player.SeatNo] || c.GrossPayoutUnits != gross[player.SeatNo] || c.WithheldUnits > player.Stack {
				return historyaccess.ErrUnavailable
			}
			ending, net := player.Stack-c.WithheldUnits, player.Stack-c.WithheldUnits-player.InitialStack
			d.Participants[i].Ending, d.Participants[i].Net = &ending, &net
			if user == subject {
				v := c.PublicView()
				d.EconomySettlement = &v
			}
		}
	}
	return historyTimeline(subject, id, d.Version, state.Events, public, viewerSeat, q, d)
}

func (r *HistoryReader) historyHandFacts(ctx context.Context, tx pgx.Tx, id string, state engine.State, d *HistoryHandDetail) error {
	rows, err := tx.Query(ctx, "SELECT event_sequence,hand_version,event_type,actor_seat,applied_delta_units,to_units,payload_cipher,created_at FROM poker.actions WHERE hand_id=$1 ORDER BY event_sequence", id)
	if err != nil {
		return err
	}
	count, cardEvents := 0, map[int]engine.Event{}
	for rows.Next() {
		var formal, payload engine.Event
		var cipher []byte
		if err = rows.Scan(&formal.Sequence, &formal.Version, &formal.Type, &formal.Seat, &formal.Delta, &formal.To, &cipher, &formal.At); err != nil {
			break
		}
		plain, e := openCipher(r.keys, fmt.Sprint(id, ":event:", formal.Sequence), cipher)
		if e != nil || json.Unmarshal(plain, &payload) != nil || count >= len(state.Events) || !reflect.DeepEqual(payload, state.Events[count]) ||
			formal.Sequence != payload.Sequence || formal.Version != payload.Version || formal.Type != payload.Type || formal.Seat != payload.Seat || formal.Delta != payload.Delta || formal.To != payload.To || !formal.At.Equal(payload.At) {
			err = historyaccess.ErrUnavailable
			break
		}
		if (payload.Card == nil) != (payload.DeckIndex == nil) || (payload.Type == "CARD_DEALT") != (payload.Card != nil) {
			err = historyaccess.ErrUnavailable
			break
		}
		if payload.Card != nil {
			if payload.DeckIndex == nil || *payload.DeckIndex < 0 || *payload.DeckIndex >= state.DeckCursor || *payload.Card != state.Config.Deck[*payload.DeckIndex] {
				err = historyaccess.ErrUnavailable
				break
			}
			if _, exists := cardEvents[*payload.DeckIndex]; exists {
				err = historyaccess.ErrUnavailable
				break
			}
			cardEvents[*payload.DeckIndex] = payload
		}
		count++
	}
	rows.Close()
	if err != nil {
		return err
	}
	if rows.Err() != nil || count != len(state.Events) || len(cardEvents) != state.DeckCursor {
		return historyaccess.ErrUnavailable
	}
	rows, err = tx.Query(ctx, "SELECT deck_index,recipient_seat,visibility,card_cipher FROM poker.dealt_cards WHERE hand_id=$1 ORDER BY deck_index", id)
	if err != nil {
		return err
	}
	count = 0
	for rows.Next() {
		var index, seat int
		var visibility string
		var cipher []byte
		if err = rows.Scan(&index, &seat, &visibility, &cipher); err != nil {
			break
		}
		event, ok := cardEvents[index]
		card, e := openCipher(r.keys, fmt.Sprint(id, ":card:", index), cipher)
		if !ok || e != nil || len(card) != 1 || card[0] != byte(*event.Card) || seat != event.Seat || visibility != event.Visibility {
			err = historyaccess.ErrUnavailable
			break
		}
		count++
	}
	rows.Close()
	if err != nil {
		return err
	}
	if rows.Err() != nil || count != len(cardEvents) {
		return historyaccess.ErrUnavailable
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT coalesce(jsonb_agg(jsonb_build_object('index',p.pot_index,'amount_units',p.amount_units,
	 'contribution_floor',p.contribution_floor,'contribution_ceiling',p.contribution_ceiling,
	 'eligible_seats',(SELECT jsonb_agg(seat_no ORDER BY seat_no) FROM poker.pot_eligible_players WHERE hand_id=p.hand_id AND pot_index=p.pot_index),
	 'awards',(SELECT jsonb_agg(jsonb_build_object('seat_no',seat_no,'base_share_units',base_share_units,'odd_chip_units',odd_chip_units,'award_units',award_units) ORDER BY seat_no)
	  FROM poker.pot_awards WHERE hand_id=p.hand_id AND pot_index=p.pot_index)) ORDER BY p.pot_index),'[]')
	 FROM poker.pots p WHERE p.hand_id=$1`, id).Scan(&raw)
	var pots []engine.Pot
	if err != nil || json.Unmarshal(raw, &pots) != nil || len(pots) != len(state.Pots) {
		return historyaccess.ErrUnavailable
	}
	var expected HistorySettlement
	for i, p := range pots {
		want := state.Pots[i]
		sortAwards := func(a []engine.Award) map[int]engine.Award {
			m := map[int]engine.Award{}
			for _, v := range a {
				m[v.Seat] = v
			}
			return m
		}
		if p.Index != want.Index || p.Amount != want.Amount || p.Floor != want.Floor || p.Ceiling != want.Ceiling || !reflect.DeepEqual(p.Eligible, want.Eligible) || !reflect.DeepEqual(sortAwards(p.Awards), sortAwards(want.Awards)) {
			return historyaccess.ErrUnavailable
		}
		pot := HistoryPot{Index: p.Index, Amount: p.Amount, Floor: p.Floor, Ceiling: p.Ceiling, Eligible: p.Eligible, Awards: []HistoryAward{}}
		for _, a := range p.Awards {
			pot.Awards = append(pot.Awards, HistoryAward{Seat: a.Seat, Base: a.Base, Odd: a.Odd, Amount: a.Amount})
			expected.Awarded += a.Amount
		}
		d.Pots = append(d.Pots, pot)
	}
	for _, event := range state.Events {
		switch event.Type {
		case "POST_SB", "POST_BB", "CALL", "BET", "RAISE", "ALL_IN":
			expected.Paid += event.Delta
		}
	}
	for _, r := range state.Returns {
		expected.Returned += r.Amount
		d.Returns = append(d.Returns, HistoryReturn{Seat: r.Seat, Amount: r.Amount})
	}
	var settlement HistorySettlement
	var business string
	err = tx.QueryRow(ctx, "SELECT biz_id,total_commitment_units,total_award_units,uncalled_return_units,settled_at FROM poker.settlements WHERE hand_id=$1", id).Scan(&business, &settlement.Paid, &settlement.Awarded, &settlement.Returned, &settlement.At)
	if d.State != "SETTLED" && errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil || d.State != "SETTLED" || business != "poker_hand_settlement:"+id || settlement.Paid != expected.Paid || settlement.Awarded != expected.Awarded || settlement.Returned != expected.Returned {
		return historyaccess.ErrUnavailable
	}
	d.Settlement = &settlement
	return nil
}

type historyHandCursor struct {
	Schema               int
	Subject              int64
	Hand                 string
	Version, Upper, Last uint64
}

func historyTimeline(subject int64, id string, version uint64, events []engine.Event, public map[int][]int, viewerSeat int, q HistoryHandQuery, d *HistoryHandDetail) error {
	limit, err := historyLimit(q.Limit)
	c := historyHandCursor{Schema: 1, Subject: subject, Hand: id, Version: version, Upper: uint64(len(events))}
	if err != nil || len(q.Cursor) > 2048 {
		return historyaccess.ErrInvalid
	}
	if q.Cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(q.Cursor)
		if e != nil || len(raw) > 1024 || json.Unmarshal(raw, &c) != nil || c.Schema != 1 || c.Subject != subject || c.Hand != id || c.Last == 0 || c.Last >= c.Upper {
			return historyaccess.ErrInvalid
		}
		if c.Version != version {
			return ErrHistoryCursorStale
		}
		if c.Upper != uint64(len(events)) {
			return historyaccess.ErrInvalid
		}
	}
	end := min(c.Last+uint64(limit), c.Upper)
	d.Actions = []HistoryAction{}
	for _, e := range events[c.Last:end] {
		action := HistoryAction{Sequence: e.Sequence, Version: e.Version, Type: e.Type, Street: string(e.Street), Seat: e.Seat, Delta: e.Delta, To: e.To, At: e.At}
		if e.Card != nil && (e.Visibility == "PUBLIC" || (viewerSeat != 0 && e.Seat == viewerSeat) || len(public[e.Seat]) != 0) {
			card := int(*e.Card)
			action.Card = &card
		}
		d.Actions = append(d.Actions, action)
	}
	if end < c.Upper {
		c.Last = end
		raw, _ := json.Marshal(c)
		d.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return nil
}
