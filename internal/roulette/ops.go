package roulette

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
 (f.funded-f.refunded-f.paid-coalesce(sum(escrow_units::numeric),0))::text,
 count(*) FILTER(WHERE state='FINISHED'),count(*) FILTER(WHERE outcome->>'reason'='FORFEIT'),
 count(*) FILTER(WHERE state='FINISHED' AND jsonb_array_length(outcome->'eligible_seats')>1),
 count(*) FILTER(WHERE EXISTS(SELECT 1 FROM roulette.actions a WHERE a.round_id=r.round_id AND a.input->>'kind' IN('TIMEOUT','GAME_LIMIT'))),
 count(*) FILTER(WHERE state='NEEDS_REVIEW') FROM f LEFT JOIN r ON true GROUP BY f.funded,f.refunded,f.paid`, slug).Scan(&out.EscrowUnits, &out.FundedUnits, &out.RefundedUnits, &out.PaidUnits, &out.ConservationDelta, &out.Completed, &out.Surrendered, &out.Draws, &out.Timeouts, &out.NeedsReview)
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

const configPreview = "ROULETTE_CONFIG_PREVIEW"
const configActivate = "ROULETTE_CONFIG_ACTIVATE"
const systemVoid = "ROULETTE_SYSTEM_VOID"

type opsHandler struct {
	service *Service
	kind    string
}
type opsInput struct {
	Game string `json:"game_slug,omitempty"`
}

func OpsBindings(s *Service) []platform.OpsOperationBinding {
	out := []platform.OpsOperationBinding{}
	for _, kind := range []string{configPreview, configActivate, systemVoid} {
		d := platform.OpsOperationDescriptor{OperationType: kind, Risk: platform.OpsRiskCritical, RequiredPermission: "games.config.activate", AllowedRoles: []string{"SUPER_ADMIN"}, TargetType: "game_config", InputSchemaVersion: "roulette-config-reference.v1", ImpactSchemaVersion: "roulette-impact.v1", ExecutionMode: platform.OpsSameDatabase, ConfirmationMode: platform.OpsConfirmTyped, RequiresReason: true, RequiresFreshAuth: true}
		if kind == configPreview {
			d.Risk = platform.OpsRiskRoutine
			d.RequiredPermission = "games.config.validate"
			d.AllowedRoles = []string{"SUPER_ADMIN", "OPERATOR"}
			d.ConfirmationMode = platform.OpsConfirmNone
			d.RequiresReason = false
			d.RequiresFreshAuth = false
		}
		if kind == systemVoid {
			d.RequiredPermission = "games.runtime.write"
			d.TargetType = "roulette_round"
			d.InputSchemaVersion = "roulette-void.v1"
		}
		var h platform.OpsOperationHandler
		if s != nil {
			h = opsHandler{s, kind}
		}
		out = append(out, platform.OpsOperationBinding{Descriptor: d, Handler: h})
	}
	return out
}
func (h opsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var in opsInput
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || d.Decode(&struct{}{}) != io.EOF || h.kind == systemVoid && in.Game != "" || h.kind != systemVoid && !IsGame(in.Game) {
		return nil, platform.ErrOpsInvalid
	}
	return json.Marshal(in)
}
func opsRaw(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func opsImpact(current, proposed any, id string) platform.OpsImpact {
	return platform.OpsImpact{CurrentState: opsRaw(current), ProposedChange: opsRaw(proposed), BlockingFacts: []string{}, ContinuingAcceptedWork: []string{"other accepted rounds retain their original binding"}, RelatedIDs: []string{id}, UnavailableMeasurements: []string{}}
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
func (h opsHandler) Prepare(ctx context.Context, tx pgx.Tx, _ platform.OpsPrincipal, in platform.OpsPrepareRequest, lock bool) (platform.OpsPreparedMaterial, error) {
	empty := platform.OpsPreparedMaterial{}
	canonical, e := h.Canonicalize(in.Input)
	if e != nil {
		return empty, e
	}
	var input opsInput
	_ = json.Unmarshal(canonical, &input)
	if h.kind == systemVoid {
		if in.Target.Type != "roulette_round" || !ValidRoundID(in.Target.ID) {
			return empty, platform.ErrOpsInvalid
		}
		r, _, refund, e := voidMaterial(ctx, tx, in.Target.ID, lock)
		if e != nil {
			return empty, e
		}
		version := strconv.FormatInt(r.Version, 10)
		if version != in.Target.ExpectedVersion {
			return empty, platform.ErrOpsPreviewStale
		}
		return platform.OpsPreparedMaterial{Impact: opsImpact(map[string]any{"state": r.State, "escrow_units": strconv.FormatInt(r.Escrow, 10)}, refund, r.ID), TargetVersion: version, TargetLocator: r.Game}, nil
	}
	if in.Target.Type != "game_config" || !ValidRoundID(in.Target.ID) {
		return empty, platform.ErrOpsInvalid
	}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	var impl string
	var registryVersion int64
	var active *string
	e = tx.QueryRow(ctx, `SELECT implementation_key,version,active_config_version_id::text FROM games.game_registry WHERE game_slug=$1`+suffix, input.Game).Scan(&impl, &registryVersion, &active)
	if e != nil {
		return empty, e
	}
	if impl != implementation(input.Game) {
		return empty, platform.ErrOpsConflict
	}
	var status string
	var version int64
	e = tx.QueryRow(ctx, `SELECT status,version_number FROM games.game_config_versions WHERE game_slug=$1 AND config_version_id=$2`+suffix, input.Game, in.Target.ID).Scan(&status, &version)
	if e != nil {
		return empty, e
	}
	if strconv.FormatInt(version, 10) != in.Target.ExpectedVersion {
		return empty, platform.ErrOpsPreviewStale
	}
	if h.kind == configPreview && status != "VALIDATED" || h.kind == configActivate && status != "PREVIEWED" {
		return empty, platform.ErrOpsConflict
	}
	c, e := ReadConfig(ctx, tx, input.Game, in.Target.ID)
	if e != nil {
		return empty, platform.ErrOpsConflict
	}
	next := "PREVIEWED"
	if h.kind == configActivate {
		next = "ACTIVE"
	}
	impact := opsImpact(map[string]any{"config": c.ID, "status": status, "registry_version": strconv.FormatInt(registryVersion, 10), "active_config": active}, map[string]any{"config": c.ID, "status": next}, c.ID)
	return platform.OpsPreparedMaterial{Impact: impact, TargetVersion: strconv.FormatInt(version, 10), TargetLocator: input.Game}, nil
}
func (h opsHandler) Execute(ctx context.Context, tx pgx.Tx, _ platform.OpsPrincipal, op platform.OpsOperation, m platform.OpsPreparedMaterial) (platform.OpsExecutionResult, error) {
	result := platform.OpsExecutionResult{BeforeSnapshot: m.Impact.CurrentState, AfterSnapshot: m.Impact.ProposedChange, Result: m.Impact.ProposedChange, RelatedBusinessID: op.Target.ID}
	if op.OperationType != h.kind {
		return result, platform.ErrOpsInvalid
	}
	if h.kind == systemVoid {
		r, ps, o, e := voidMaterial(ctx, tx, op.Target.ID, true)
		if e != nil {
			return result, e
		}
		if strconv.FormatInt(r.Version, 10) != op.Target.ExpectedVersion {
			return result, platform.ErrOpsPreviewStale
		}
		if e = lockWallets(ctx, tx, ps); e != nil {
			return result, e
		}
		for _, a := range o.Awards {
			var p participant
			for _, v := range ps {
				if v.User == a.User {
					p = v
					break
				}
			}
			if e = fund(ctx, tx, r, p, "VOID_REFUND", a.Amount); e != nil {
				return result, e
			}
		}
		if e = markReview(ctx, tx, r); e != nil {
			return result, e
		}
		_, e = tx.Exec(ctx, `UPDATE roulette.rounds SET state='CANCELLED',version=version+1,escrow_units=0,ended_at=clock_timestamp(),deadline=NULL,void_outcome=$2 WHERE round_id=$1`, r.ID, opsRaw(o))
		if e != nil {
			return result, e
		}
		_, e = tx.Exec(ctx, `UPDATE roulette.participants SET active=false WHERE round_id=$1`, r.ID)
		return result, e
	}
	if h.kind == configPreview {
		_, e := tx.Exec(ctx, `UPDATE games.game_config_versions SET status='PREVIEWED',previewed_at=clock_timestamp() WHERE config_version_id=$1 AND status='VALIDATED'`, op.Target.ID)
		return result, e
	}
	if _, e := tx.Exec(ctx, `UPDATE games.game_config_versions SET status='SUPERSEDED',superseded_at=clock_timestamp() WHERE game_slug=$1 AND status='ACTIVE'`, m.TargetLocator); e != nil {
		return result, e
	}
	if _, e := tx.Exec(ctx, `UPDATE games.game_config_versions SET status='ACTIVE',activated_at=clock_timestamp() WHERE config_version_id=$1 AND status='PREVIEWED'`, op.Target.ID); e != nil {
		return result, e
	}
	_, e := tx.Exec(ctx, `UPDATE games.game_registry SET active_config_version_id=$2,version=version+1 WHERE game_slug=$1`, m.TargetLocator, op.Target.ID)
	return result, e
}
