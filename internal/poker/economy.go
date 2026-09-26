package poker

import (
	"context"
	"strconv"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/jackc/pgx/v5"
)

func handContributions(state engine.State) (map[int]int64, map[int]int64) {
	paid, gross := map[int]int64{}, map[int]int64{}
	for _, event := range state.Events {
		switch event.Type {
		case "POST_SB", "POST_BB", "CALL", "BET", "RAISE", "ALL_IN":
			paid[event.Seat] += event.Delta
		}
	}
	for _, pot := range state.Pots {
		for _, award := range pot.Awards {
			gross[award.Seat] += award.Amount
		}
	}
	for _, returned := range state.Returns {
		paid[returned.Seat] -= returned.Amount
	}
	return paid, gross
}

// Observe before replacing current stacks/committed pots. Moving a bet from a
// stack into its pot leaves T unchanged; removing the pot first would undercount.
func (s *Service) settleHandCap(ctx context.Context, tx pgx.Tx, id, version string, state engine.State) (map[int]int64, error) {
	withheld := map[int]int64{}
	users := make([]int64, 0, len(state.Players))
	for _, player := range state.Players {
		user, err := strconv.ParseInt(player.PlayerID, 10, 64)
		if err != nil || user <= 0 {
			return nil, ErrCorruptSnapshot
		}
		users = append(users, user)
	}
	if err := platform.LockEconomyUsersInTx(ctx, tx, users...); err != nil {
		return nil, err
	}
	if state.Street != engine.Settled || version == "" {
		return withheld, nil
	}
	policy, err := platform.ResolveEconomicPolicyInTx(ctx, tx, version)
	if err != nil {
		return nil, err
	}
	paid, gross := handContributions(state)
	for i, player := range state.Players {
		c, err := platform.PrepareCapSettlementInTx(ctx, tx, s.opts.EconomyObserver, policy, users[i], paid[player.SeatNo], gross[player.SeatNo], engine.UnitsPerChip)
		if err != nil {
			return nil, err
		}
		for _, returned := range state.Returns {
			if returned.Seat == player.SeatNo {
				c.NeutralReturnUnits += returned.Amount
			}
		}
		if c.WithheldUnits > player.Stack {
			return nil, ErrCorruptSnapshot
		}
		if err = platform.RecordCapSettlementInTx(ctx, tx, "POKER_HAND", id, users[i], c); err != nil {
			return nil, err
		}
		withheld[player.SeatNo] = c.WithheldUnits
	}
	return withheld, nil
}

func handCapView(ctx context.Context, tx pgx.Tx, id string, user int64) (*platform.PayoutCapView, error) {
	c, err := platform.LoadCapSettlementInTx(ctx, tx, "POKER_HAND", id, user)
	if err != nil || c == nil {
		return nil, err
	}
	v := c.PublicView()
	return &v, nil
}
