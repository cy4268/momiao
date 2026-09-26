package roulette

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

func readOnly(ctx context.Context, tx pgx.Tx) error {
	_, e := tx.Exec(ctx, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY`)
	return e
}
func balance(ctx context.Context, tx pgx.Tx, user int64) (int64, error) {
	var b int64
	e := tx.QueryRow(ctx, `SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type='AVAILABLE_CHIPS'`, user).Scan(&b)
	return b, e
}
func (s *Service) View(ctx context.Context, user int64, id string) (RoomView, error) {
	if user <= 0 || !ValidRoundID(id) {
		return RoomView{}, ErrInvalidInput
	}
	var v RoomView
	e := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if e := readOnly(ctx, tx); e != nil {
			return e
		}
		r, e := s.load(ctx, tx, id, false)
		if e != nil {
			if r == nil || r.State != "NEEDS_REVIEW" || !errors.Is(e, ErrUnavailable) {
				return e
			}
			r.Snapshot = snapshot{}
		}
		ps, e := participants(ctx, tx, id)
		if e != nil {
			return e
		}
		now, e := dbNow(ctx, tx)
		if e != nil {
			return e
		}
		v = project(r, ps, user, now)
		if e = readCapView(ctx, tx, r, user, &v); e != nil {
			return e
		}
		if v.Self != nil {
			v.Self.AvailableUnits, e = balance(ctx, tx, user)
			if e != nil {
				return e
			}
		}
		rows, e := tx.Query(ctx, `SELECT public_event FROM (SELECT sequence,public_event FROM roulette.actions WHERE round_id=$1 ORDER BY sequence DESC LIMIT 100) recent ORDER BY sequence`, id)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var event PublicEvent
			if e = rows.Scan(&raw); e != nil {
				return e
			}
			if e = json.Unmarshal(raw, &event); e != nil {
				return e
			}
			v.Log = append(v.Log, event)
		}
		return rows.Err()
	})
	return v, e
}
func project(r *round, players []participant, user int64, now time.Time) RoomView {
	v := RoomView{ID: r.ID, Game: r.Game, Title: r.Title, Version: r.Version, Sequence: r.Sequence, State: r.State, TargetPlayers: r.Target, StakeUnits: r.Stake, PoolUnits: r.Escrow, Binding: binding(r.Config, r.Policy), ServerSeedHash: hex.EncodeToString(r.Commitment[:]), ServerNow: now, Deadline: r.Deadline, GameDeadline: r.GameDeadline, Players: []PlayerView{}, Actions: []Action{}, Log: []PublicEvent{}}
	if r.Economy.Version != "" {
		v.EconomicPolicy = &r.Economy
	}
	for _, p := range players {
		if p.Seat == nil {
			continue
		}
		seat := *p.Seat
		pv := PlayerView{Seat: seat, Name: p.Name, Ready: p.Ready, Alive: true}
		if d := r.Snapshot.Devil; d != nil {
			pv.HP = d.HP[seat]
			pv.Forfeited = d.Forfeited[seat]
			pv.Alive = pv.HP > 0 && !pv.Forfeited
			pv.ItemCount = len(d.Items[seat])
		}
		if state := r.Snapshot.Pressure; state != nil {
			pv.Alive = slices.Contains(state.Alive, seat)
			pv.Forfeited = state.Forfeited[seat]
		}
		v.Players = append(v.Players, pv)
		if p.User == user {
			self := &SelfView{Seat: seat, Items: []string{}, Intel: []IntelEntry{}}
			v.Self = self
			if d := r.Snapshot.Devil; d != nil {
				self.Items = append(self.Items, d.Items[seat]...)
				for index, live := range d.Known[seat] {
					self.Intel = append(self.Intel, IntelEntry{index - d.Pointer, live})
				}
				slices.SortFunc(self.Intel, func(a, b IntelEntry) int { return a.Index - b.Index })
				if r.State == "PLAYING" {
					v.Actions = d.actions(seat)
				}
			}
			if state := r.Snapshot.Pressure; state != nil && r.State == "PLAYING" {
				v.Actions = state.actions(seat)
			}
			if r.State == "WAITING" {
				v.Actions = append(v.Actions, Action{Kind: "LEAVE"})
				if p.Ready {
					v.Actions = append(v.Actions, Action{Kind: "UNREADY"})
				} else {
					v.Actions = append(v.Actions, Action{Kind: "READY"})
				}
				if p.User == r.Host {
					v.Actions = append(v.Actions, Action{Kind: "CANCEL"})
				}
			}
		}
	}
	slices.SortFunc(v.Players, func(a, b PlayerView) int { return a.Seat - b.Seat })
	if d := r.Snapshot.Devil; d != nil {
		public := d.public()
		v.Devil = &public
		if r.State == "PLAYING" {
			turn := d.Turn
			v.TurnSeat = &turn
		}
	}
	if p := r.Snapshot.Pressure; p != nil {
		public := p.public()
		v.Pressure = &public
		if r.State == "PLAYING" {
			v.TurnSeat = pressureSeat(p.Turn)
		}
	}
	if user > 0 && v.Self == nil && r.State == "WAITING" && len(v.Players) < r.Target {
		v.Actions = append(v.Actions, Action{Kind: "JOIN"})
	}
	return v
}

type lobbyCursor struct {
	Created time.Time `json:"created"`
	ID      string    `json:"id"`
}

func (s *Service) List(ctx context.Context, user int64, q LobbyQuery) (Lobby, error) {
	if user <= 0 || !IsGame(q.Game) || len(q.Cursor) > 256 {
		return Lobby{}, ErrInvalidInput
	}
	var cursor *lobbyCursor
	if q.Cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(q.Cursor)
		if e != nil {
			return Lobby{}, ErrInvalidInput
		}
		cursor = &lobbyCursor{}
		if json.Unmarshal(raw, cursor) != nil || !ValidRoundID(cursor.ID) || cursor.Created.IsZero() {
			return Lobby{}, ErrInvalidInput
		}
	}
	v := Lobby{Game: q.Game, Rooms: []RoomView{}}
	e := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if e := readOnly(ctx, tx); e != nil {
			return e
		}
		entry, c, p, e := resolve(ctx, tx, q.Game, false)
		if e != nil {
			return e
		}
		if c.ID == "" {
			return ErrUnavailable
		}
		ep, e := platform.ActiveEconomicPolicyInTx(ctx, tx)
		if e != nil {
			return e
		}
		if ep.Version != "" {
			v.EconomicPolicy = &ep
		}
		v.Binding = binding(c, p)
		v.MinimumUnits = p.Minimum
		v.StepUnits = p.Step
		v.State = entry.State
		v.AvailableUnits, e = balance(ctx, tx, user)
		if e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `SELECT round_id::text FROM roulette.participants WHERE newapi_user_id=$1 AND active`, user).Scan(&v.OwnRoundID)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		var date *time.Time
		var id *string
		if cursor != nil {
			date = &cursor.Created
			id = &cursor.ID
		}
		rows, e := tx.Query(ctx, `SELECT round_id::text,created_at FROM roulette.rounds WHERE game_slug=$1 AND state IN('WAITING','PLAYING','SETTLING','NEEDS_REVIEW') AND ($2::timestamptz IS NULL OR (created_at,round_id)<($2,$3::uuid)) ORDER BY created_at DESC,round_id DESC LIMIT 51`, q.Game, date, id)
		if e != nil {
			return e
		}
		found := []lobbyCursor{}
		for rows.Next() {
			var item lobbyCursor
			if e = rows.Scan(&item.ID, &item.Created); e != nil {
				rows.Close()
				return e
			}
			found = append(found, item)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(found) > 50 {
			last := found[49]
			raw, _ := json.Marshal(last)
			next := base64.RawURLEncoding.EncodeToString(raw)
			v.NextCursor = &next
			found = found[:50]
		}
		now, e := dbNow(ctx, tx)
		if e != nil {
			return e
		}
		for _, item := range found {
			// Lobby needs no seed/snapshot: disclose only the public seat and pool summary.
			var r round
			r.ID = item.ID
			r.Config = c
			r.Policy = p
			e = tx.QueryRow(ctx, `SELECT game_slug,title_snapshot,version,action_sequence,state,target_players,stake_units,escrow_units,deadline,game_deadline FROM roulette.rounds WHERE round_id=$1`, item.ID).Scan(&r.Game, &r.Title, &r.Version, &r.Sequence, &r.State, &r.Target, &r.Stake, &r.Escrow, &r.Deadline, &r.GameDeadline)
			if e != nil {
				return e
			}
			ps, e := participants(ctx, tx, item.ID)
			if e != nil {
				return e
			}
			rv := project(&r, ps, 0, now)
			rv.Binding = Binding{}
			rv.ServerSeedHash = ""
			v.Rooms = append(v.Rooms, rv)
		}
		return nil
	})
	return v, e
}

func readCapView(ctx context.Context, tx pgx.Tx, r *round, user int64, v *RoomView) error {
	c, e := platform.LoadCapSettlementInTx(ctx, tx, "ROULETTE_ROUND", r.ID, user)
	if e != nil {
		return e
	}
	if c != nil {
		if c.PolicyVersion != r.Economy.Version {
			return ErrUnavailable
		}
		public := c.PublicView()
		v.EconomySettlement = &public
	}
	return nil
}
