package roulette

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

type ReviewRoom struct {
	ID      string    `json:"id"`
	State   string    `json:"state"`
	Version int64     `json:"version,string"`
	Escrow  string    `json:"escrow_units"`
	Anomaly string    `json:"anomaly_code"`
	Created time.Time `json:"created_at"`
}
type OpsStats struct {
	WithheldUnits     string       `json:"withheld_units"`
	EscrowUnits       string       `json:"escrow_units"`
	FundedUnits       string       `json:"funded_units"`
	RefundedUnits     string       `json:"refunded_units"`
	PaidUnits         string       `json:"paid_units"`
	ConservationDelta string       `json:"conservation_delta"`
	Completed         int64        `json:"completed,string"`
	Surrendered       int64        `json:"surrendered,string"`
	Draws             int64        `json:"draws,string"`
	Timeouts          int64        `json:"timeouts,string"`
	NeedsReview       int64        `json:"needs_review,string"`
	ReviewRooms       []ReviewRoom `json:"review_rooms"`
}

func OpsStatsInTx(ctx context.Context, tx pgx.Tx, slug string) (OpsStats, error) {
	out := OpsStats{ReviewRooms: []ReviewRoom{}}
	if !IsGame(slug) {
		return out, ErrInvalidInput
	}
	e := tx.QueryRow(ctx, `WITH r AS (SELECT * FROM roulette.rounds WHERE game_slug=$1),f AS (
 SELECT coalesce(sum(amount_units::numeric) FILTER(WHERE kind='ESCROW'),0) funded,
 coalesce(sum(amount_units::numeric) FILTER(WHERE kind IN('REFUND','VOID_REFUND')),0) refunded,
 coalesce(sum(amount_units::numeric) FILTER(WHERE kind='PAYOUT'),0) paid FROM roulette.funding JOIN r USING(round_id))
 SELECT coalesce(sum(escrow_units::numeric),0)::text,f.funded::text,f.refunded::text,f.paid::text,
 (f.funded-f.refunded-f.paid-coalesce(sum(escrow_units::numeric),0)-(SELECT coalesce(sum(c.withheld_units::numeric),0) FROM economy.cap_settlements c JOIN r ON r.round_id::text=c.source_id WHERE c.source_kind='ROULETTE_ROUND'))::text,
 (SELECT coalesce(sum(c.withheld_units::numeric),0)::text FROM economy.cap_settlements c JOIN r ON r.round_id::text=c.source_id WHERE c.source_kind='ROULETTE_ROUND'),
 count(*) FILTER(WHERE state='FINISHED'),count(*) FILTER(WHERE outcome->>'reason'='FORFEIT'),
 count(*) FILTER(WHERE state='FINISHED' AND jsonb_array_length(outcome->'eligible_seats')>1),
 count(*) FILTER(WHERE EXISTS(SELECT 1 FROM roulette.actions a WHERE a.round_id=r.round_id AND a.input->>'kind' IN('TIMEOUT','GAME_LIMIT'))),
 count(*) FILTER(WHERE state='NEEDS_REVIEW') FROM f LEFT JOIN r ON true GROUP BY f.funded,f.refunded,f.paid`, slug).Scan(&out.EscrowUnits, &out.FundedUnits, &out.RefundedUnits, &out.PaidUnits, &out.ConservationDelta, &out.WithheldUnits, &out.Completed, &out.Surrendered, &out.Draws, &out.Timeouts, &out.NeedsReview)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT round_id::text,state,version,escrow_units::text,CASE WHEN outcome IS NULL THEN 'STATE_OR_BINDING' ELSE 'UNSETTLED_OUTCOME' END,created_at FROM roulette.rounds WHERE game_slug=$1 AND state='NEEDS_REVIEW' ORDER BY created_at,round_id LIMIT 100`, slug)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var r ReviewRoom
		if e = rows.Scan(&r.ID, &r.State, &r.Version, &r.Escrow, &r.Anomaly, &r.Created); e != nil {
			return out, e
		}
		out.ReviewRooms = append(out.ReviewRooms, r)
	}
	return out, rows.Err()
}

// No seed is needed for a system refund. This path never decrypts, replaces or
// repairs the forensic snapshot, even if its key has been lost.
func voidMaterial(ctx context.Context, tx pgx.Tx, id string, lock bool) (*round, []participant, *outcome, error) {
	r := &round{ID: id}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	e := tx.QueryRow(ctx, `SELECT game_slug,state,version,stake_units,escrow_units FROM roulette.rounds WHERE round_id=$1`+suffix, id).Scan(&r.Game, &r.State, &r.Version, &r.Stake, &r.Escrow)
	if e != nil {
		return nil, nil, nil, e
	}
	if terminal(r) {
		return nil, nil, nil, platform.ErrOpsConflict
	}
	var paid bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM roulette.funding WHERE round_id=$1 AND kind IN('PAYOUT','VOID_REFUND'))`, id).Scan(&paid); e != nil {
		return nil, nil, nil, e
	}
	if paid {
		return nil, nil, nil, platform.ErrOpsConflict
	}
	ps, e := participants(ctx, tx, id)
	if e != nil {
		return nil, nil, nil, e
	}
	ok, e := verifyFunding(ctx, tx, r, ps)
	if e != nil {
		return nil, nil, nil, e
	}
	if !ok {
		return nil, nil, nil, platform.ErrOpsConflict
	}
	o := &outcome{Reason: "SYSTEM_VOID", Refunds: true, Eligible: []int{}, Awards: []award{}}
	rows, e := tx.Query(ctx, `SELECT newapi_user_id,ready_cycle,sum(CASE WHEN kind='ESCROW' THEN amount_units::numeric ELSE -amount_units::numeric END)::text FROM roulette.funding WHERE round_id=$1 GROUP BY newapi_user_id,ready_cycle HAVING sum(CASE WHEN kind='ESCROW' THEN amount_units::numeric ELSE -amount_units::numeric END)<>0 ORDER BY newapi_user_id,ready_cycle`, id)
	if e != nil {
		return nil, nil, nil, e
	}
	var total int64
	for rows.Next() {
		var user, cycle int64
		var net string
		if e = rows.Scan(&user, &cycle, &net); e != nil {
			rows.Close()
			return nil, nil, nil, e
		}
		amount, e := strconv.ParseInt(net, 10, 64)
		if e != nil || amount != r.Stake || total > math.MaxInt64-amount {
			rows.Close()
			return nil, nil, nil, platform.ErrOpsConflict
		}
		found := false
		for _, p := range ps {
			if p.User == user && p.Cycle == cycle && p.Ready && p.Seat != nil {
				found = true
				o.Awards = append(o.Awards, award{user, *p.Seat, amount})
				o.Eligible = append(o.Eligible, *p.Seat)
			}
		}
		if !found {
			rows.Close()
			return nil, nil, nil, platform.ErrOpsConflict
		}
		total += amount
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, nil, nil, e
	}
	if total != r.Escrow {
		return nil, nil, nil, platform.ErrOpsConflict
	}
	return r, ps, o, nil
}
func SystemVoidInTx(ctx context.Context, tx pgx.Tx, actor int64, slug, id string) error {
	if err := platform.RequireLocalOpsAdmin(ctx, tx, actor); err != nil {
		return err
	}
	if !IsGame(slug) || !ValidRoundID(id) {
		return platform.ErrOpsInvalid
	}
	r, ps, o, err := voidMaterial(ctx, tx, id, true)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.ErrOpsNotFound
	}
	if err != nil {
		return err
	}
	if r.Game != slug {
		return platform.ErrOpsNotFound
	}
	before := map[string]any{"state": r.State, "escrow_units": strconv.FormatInt(r.Escrow, 10)}
	if err = lockWallets(ctx, tx, ps); err != nil {
		return err
	}
	for _, a := range o.Awards {
		var p participant
		for _, v := range ps {
			if v.User == a.User {
				p = v
				break
			}
		}
		if err = fund(ctx, tx, r, p, "VOID_REFUND", a.Amount); err != nil {
			return err
		}
	}
	if err = markReview(ctx, tx, r); err != nil {
		return err
	}
	raw, err := json.Marshal(o)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE roulette.rounds SET state='CANCELLED',version=version+1,escrow_units=0,ended_at=clock_timestamp(),deadline=NULL,void_outcome=$2 WHERE round_id=$1`, id, raw); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE roulette.participants SET active=false WHERE round_id=$1`, id); err != nil {
		return err
	}
	return platform.RecordOpsChange(ctx, tx, actor, "ROULETTE_SYSTEM_VOID", "roulette_round", id, before, map[string]any{"state": "CANCELLED", "escrow_units": "0", "refund": o})
}
