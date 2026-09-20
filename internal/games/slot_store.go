package games

import (
	"context"
	"github.com/cy4268/momiao/internal/games/fairness"
	"github.com/cy4268/momiao/internal/games/slot"
	"github.com/jackc/pgx/v5"
)

func deriveSlot(round *GameRound, seed []byte, fair FairRound, c Config, wager int64) (int64, error) {
	if err := c.checkRound(fair, "slot"); err != nil {
		return 0, err
	}
	spin := slot.Spin
	if c.binding.RulesetVersion == slot.FairRulesetVersion {
		spin = slot.SpinFair
	}
	if c.binding.RulesetVersion == slot.FrequentRulesetVersion {
		spin = slot.SpinFrequent
	}
	r, err := spin(wager, func(domain string, n uint32) (uint32, error) {
		stream, e := fairness.NewStream(seed, fair, domain)
		if e != nil {
			return 0, e
		}
		v, e := fairness.UniformInt(stream, uint64(n))
		return uint32(v), e
	})
	if err != nil {
		return 0, err
	}
	round.Slot = &r
	return r.TotalPayoutUnits, nil
}

func persistSlot(ctx context.Context, tx pgx.Tx, r GameRound) error {
	d := r.Slot
	grid := make([]string, 0, 15)
	for _, reel := range d.Grid {
		for _, symbol := range reel {
			grid = append(grid, string(symbol))
		}
	}
	strips, paytable := slot.ReelStripVersion, slot.PaytableVersion
	if r.Ruleset == slot.FairRulesetVersion {
		paytable = slot.FairPaytableVersion
	}
	if r.Ruleset == slot.FrequentRulesetVersion {
		strips, paytable = slot.FrequentReelStripVersion, slot.FrequentPaytableVersion
	}
	_, err := tx.Exec(ctx, `INSERT INTO games.slot_results(round_id,stop_1,stop_2,stop_3,stop_4,stop_5,full_grid,total_wager_units,line_stake_units,total_payout_units,net_change_units,result_detail,reel_strip_version,payline_version,paytable_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'slot-paylines-v1',$14)`, r.ID, d.Stops[0], d.Stops[1], d.Stops[2], d.Stops[3], d.Stops[4], grid, d.TotalWagerUnits, d.LineStakeUnits, d.TotalPayoutUnits, d.NetChangeUnits, d.Detail, strips, paytable)
	if err != nil {
		return err
	}
	for _, l := range d.Lines {
		if _, err = tx.Exec(ctx, `INSERT INTO games.slot_line_results(round_id,line_number,interpreted_symbol,match_length,multiplier,line_stake_units,line_payout_units) VALUES($1,$2,$3,$4,$5,$6,$7)`, r.ID, l.LineNumber, l.Symbol, l.MatchLength, l.Multiplier, l.LineStakeUnits, l.PayoutUnits); err != nil {
			return err
		}
	}
	return nil
}

func readSlot(ctx context.Context, tx pgx.Tx, r GameRound) (*slot.Result, error) {
	d := &slot.Result{Class: string(r.Outcome)}
	var grid []string
	err := tx.QueryRow(ctx, `SELECT stop_1,stop_2,stop_3,stop_4,stop_5,full_grid,total_wager_units,line_stake_units,total_payout_units,net_change_units,result_detail FROM games.slot_results WHERE round_id=$1`, r.ID).Scan(&d.Stops[0], &d.Stops[1], &d.Stops[2], &d.Stops[3], &d.Stops[4], &grid, &d.TotalWagerUnits, &d.LineStakeUnits, &d.TotalPayoutUnits, &d.NetChangeUnits, &d.Detail)
	if err != nil {
		return nil, err
	}
	if len(grid) != 15 {
		return nil, ErrUnavailable
	}
	for i, s := range grid {
		d.Grid[i/3][i%3] = slot.Symbol(s)
	}
	rows, err := tx.Query(ctx, `SELECT line_number,interpreted_symbol,match_length,multiplier,line_stake_units,line_payout_units FROM games.slot_line_results WHERE round_id=$1 ORDER BY line_number`, r.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var l slot.LineResult
		if err = rows.Scan(&l.LineNumber, &l.Symbol, &l.MatchLength, &l.Multiplier, &l.LineStakeUnits, &l.PayoutUnits); err != nil {
			return nil, err
		}
		if count >= 10 || l.LineNumber != count+1 {
			return nil, ErrUnavailable
		}
		d.Lines[count] = l
		count++
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	if count != 10 {
		return nil, ErrUnavailable
	}
	return d, nil
}
