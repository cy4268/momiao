package roulette

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// PublicRoomView is the actor-free whitelist for a complete public room. Keep
// this separate from RoomView so personal actions, intel and economy never leak
// when the browser projection gains fields.
type PublicRoomView struct {
	ID             string        `json:"id"`
	Game           string        `json:"game"`
	Title          string        `json:"title"`
	Version        int64         `json:"version,string"`
	Sequence       int64         `json:"sequence,string"`
	State          string        `json:"state"`
	TargetPlayers  int           `json:"target_players"`
	StakeUnits     int64         `json:"stake_units,string"`
	PoolUnits      int64         `json:"pool_units,string"`
	Binding        Binding       `json:"binding"`
	ServerSeedHash string        `json:"server_seed_hash"`
	TurnSeat       *int          `json:"turn_seat"`
	ServerNow      time.Time     `json:"server_now"`
	Deadline       *time.Time    `json:"deadline"`
	GameDeadline   *time.Time    `json:"game_deadline"`
	Players        []PlayerView  `json:"players"`
	Log            []PublicEvent `json:"log"`
	Devil          *DevilView    `json:"devil"`
	Pressure       *PressureView `json:"pressure"`
}

// PublicView reads one consistent snapshot without an actor, personal balance
// or cap lookup, and without advancing deadlines or pending settlement.
func (s *Service) PublicView(ctx context.Context, id string) (PublicRoomView, error) {
	if !ValidRoundID(id) {
		return PublicRoomView{}, ErrInvalidInput
	}
	var view PublicRoomView
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := readOnly(ctx, tx); err != nil {
			return err
		}
		r, err := s.load(ctx, tx, id, false)
		if err != nil {
			if r == nil || r.State != "NEEDS_REVIEW" || !errors.Is(err, ErrUnavailable) {
				return err
			}
			r.Snapshot = snapshot{}
		}
		players, err := participants(ctx, tx, id)
		if err != nil {
			return err
		}
		now, err := dbNow(ctx, tx)
		if err != nil {
			return err
		}
		public := project(r, players, 0, now)
		view = PublicRoomView{
			ID: public.ID, Game: public.Game, Title: public.Title, Version: public.Version,
			Sequence: public.Sequence, State: public.State, TargetPlayers: public.TargetPlayers,
			StakeUnits: public.StakeUnits, PoolUnits: public.PoolUnits, Binding: public.Binding,
			ServerSeedHash: public.ServerSeedHash, TurnSeat: public.TurnSeat, ServerNow: public.ServerNow,
			Deadline: public.Deadline, GameDeadline: public.GameDeadline, Players: public.Players,
			Log: []PublicEvent{}, Devil: public.Devil, Pressure: public.Pressure,
		}
		rows, err := tx.Query(ctx, `SELECT public_event FROM (SELECT sequence,public_event FROM roulette.actions WHERE round_id=$1 ORDER BY sequence DESC LIMIT 20) recent ORDER BY sequence`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var event PublicEvent
			if err = rows.Scan(&raw); err != nil {
				return err
			}
			if err = json.Unmarshal(raw, &event); err != nil {
				return err
			}
			view.Log = append(view.Log, event)
		}
		return rows.Err()
	})
	return view, err
}
