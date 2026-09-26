package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const GamblerCampaignID = "01995000-2026-7000-8000-000000000927"
const GamblerBadgeCode = "gambler-ruler-202609"
const gamblerThreshold int64 = 500000000000000
const gamblerResetReserve int64 = 5000000000

var gamblerScopes = []string{"CHALDEA_USER_WRITES", "WALLET_EXCHANGE", "REWARDS", "DIRECT_PLAY_NEW_ROUNDS", "POKER_NEW_TABLES_NEW_HANDS", "RANKINGS_PUBLISHING"}
var ErrGamblerBlocked = errors.New("GAMBLER_CAMPAIGN_BLOCKED")

type GamblerCampaignService struct {
	store  *Store
	assets *UnifiedAssetReader
}

func NewGamblerCampaignService(store *Store, assets *UnifiedAssetReader) (*GamblerCampaignService, error) {
	if store == nil || assets == nil || assets.store != store {
		return nil, ErrOpsUnavailable
	}
	return &GamblerCampaignService{store, assets}, nil
}

type GamblerCampaignAccount struct {
	UserID             int64           `json:"user_id,string"`
	Eligible           bool            `json:"eligible"`
	QualificationUnits string          `json:"qualification_units"`
	ResetState         string          `json:"reset_state"`
	Before             json.RawMessage `json:"reset_before"`
	After              json.RawMessage `json:"reset_after"`
	Receipt            json.RawMessage `json:"reset_receipt"`
}
type GamblerCampaignView struct {
	ID                  string                   `json:"id"`
	Version             string                   `json:"version"`
	Phase               string                   `json:"phase"`
	CutoffAt            *time.Time               `json:"cutoff_at"`
	ResetCutoffAt       *time.Time               `json:"reset_cutoff_at"`
	TargetCount         string                   `json:"target_count"`
	CompletedCount      string                   `json:"completed_count"`
	EligibleCount       string                   `json:"eligible_count"`
	BlockingFacts       []string                 `json:"blocking_facts"`
	NativeUnchanged     bool                     `json:"native_unchanged"`
	PolicyActivatedAt   *time.Time               `json:"policy_activated_at"`
	RankingsPublishedAt *time.Time               `json:"rankings_published_at"`
	Accounts            []GamblerCampaignAccount `json:"accounts"`
	NextUserID          string                   `json:"next_user_id,omitempty"`
}
type gamblerRow struct {
	GamblerCampaignView
	actor, epoch                                    int64
	maintenance, operation, snapshotHash, resetHash string
	checkpoint                                      json.RawMessage
	verificationPhase                               string
	verificationCursor                              int64
	reverifyPending                                 bool
}

func readGambler(ctx context.Context, tx pgx.Tx, id string, lock bool) (gamblerRow, error) {
	c := gamblerRow{GamblerCampaignView: GamblerCampaignView{ID: id, Version: "1", Phase: "DRAFT", TargetCount: "0", CompletedCount: "0", EligibleCount: "0", BlockingFacts: []string{}, NativeUnchanged: true, Accounts: []GamblerCampaignAccount{}}}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE OF c"
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT c.version::text,c.phase,c.cutoff_at,c.reset_cutoff_at,c.blocking_facts,c.native_unchanged,c.policy_activated_at,c.rankings_published_at,
 coalesce(c.accepted_by,0),coalesce(c.accepted_epoch,0),coalesce(c.maintenance_id::text,''),coalesce(c.accepted_operation_id::text,''),coalesce(c.snapshot_target_hash,''),coalesce(c.reset_target_hash,''),c.checkpoint,c.verification_phase,c.verification_cursor,c.reverify_pending,
 (SELECT count(*)::text FROM economy.gambler_campaign_accounts a WHERE a.campaign_id=c.campaign_id AND CASE WHEN c.phase IN('RESET_PREPARING','RESET_READY','RESET_RUNNING','COMPLETED') THEN a.reset_member ELSE a.snapshot_member END),
 (SELECT count(*)::text FROM economy.gambler_campaign_accounts a WHERE a.campaign_id=c.campaign_id AND a.reset_state='COMPLETED'),
 (SELECT count(*)::text FROM economy.gambler_campaign_accounts a WHERE a.campaign_id=c.campaign_id AND a.eligible)
 FROM economy.gambler_campaigns c WHERE c.campaign_id=$1`+suffix, id).Scan(&c.Version, &c.Phase, &c.CutoffAt, &c.ResetCutoffAt, &raw, &c.NativeUnchanged, &c.PolicyActivatedAt, &c.RankingsPublishedAt, &c.actor, &c.epoch, &c.maintenance, &c.operation, &c.snapshotHash, &c.resetHash, &c.checkpoint, &c.verificationPhase, &c.verificationCursor, &c.reverifyPending, &c.TargetCount, &c.CompletedCount, &c.EligibleCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	for _, stamp := range []*time.Time{c.CutoffAt, c.ResetCutoffAt, c.PolicyActivatedAt, c.RankingsPublishedAt} {
		if stamp != nil {
			*stamp = stamp.UTC()
		}
	}
	if json.Unmarshal(raw, &c.BlockingFacts) != nil {
		return c, ErrOpsUnavailable
	}
	return c, nil
}
func (s *GamblerCampaignService) Read(ctx context.Context, actor int64, id string) (GamblerCampaignView, error) {
	return s.ReadPage(ctx, actor, id, 0)
}
func (s *GamblerCampaignService) ReadPage(ctx context.Context, actor int64, id string, after int64) (GamblerCampaignView, error) {
	var out GamblerCampaignView
	if s == nil || id != GamblerCampaignID || after < 0 {
		return out, ErrOpsInvalid
	}
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := requireOpsPermission(ctx, tx, actor, 0, "economy.read", false); err != nil {
			return err
		}
		c, err := readGambler(ctx, tx, id, false)
		if err != nil {
			return err
		}
		out = c.GamblerCampaignView
		rows, err := tx.Query(ctx, `SELECT newapi_user_id,eligible,coalesce(snapshot_assets->>'total_units',''),reset_state,reset_before,reset_after,reset_receipt FROM economy.gambler_campaign_accounts WHERE campaign_id=$1 AND newapi_user_id>$2 ORDER BY newapi_user_id LIMIT 51`, id, after)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a GamblerCampaignAccount
			if err = rows.Scan(&a.UserID, &a.Eligible, &a.QualificationUnits, &a.ResetState, &a.Before, &a.After, &a.Receipt); err != nil {
				return err
			}
			if len(out.Accounts) == 50 {
				out.NextUserID = strconv.FormatInt(out.Accounts[49].UserID, 10)
				break
			}
			out.Accounts = append(out.Accounts, a)
		}
		return rows.Err()
	})
	return out, err
}

type gamblerInput struct {
	MaintenanceID       string `json:"maintenance_id,omitempty"`
	NativePauseEvidence string `json:"native_pause_evidence,omitempty"`
	BackupReference     string `json:"backup_reference,omitempty"`
}
type gamblerOperation struct {
	service *GamblerCampaignService
	action  string
}

func (s *GamblerCampaignService) OpsBindings() []OpsOperationBinding {
	var out []OpsOperationBinding
	for _, action := range []string{"GAMBLER_SNAPSHOT_PREPARE", "GAMBLER_MEDALS_GRANT", "GAMBLER_RESET_PREPARE", "GAMBLER_RESET_START", "GAMBLER_REAUTHORIZE", "ECONOMY_POLICY_ACTIVATE"} {
		out = append(out, OpsOperationBinding{Descriptor: OpsOperationDescriptor{OperationType: action, Risk: OpsRiskCritical, RequiredPermission: "economy.adjust", AllowedRoles: []string{"SUPER_ADMIN"}, TargetType: "GAMBLER_CAMPAIGN", InputSchemaVersion: "gambler.v1", ImpactSchemaVersion: "gambler-impact.v1", RequiresReason: true, RequiresFreshAuth: true, ConfirmationMode: OpsConfirmTyped, ExecutionMode: OpsSameDatabase}, Handler: gamblerOperation{s, action}})
	}
	return out
}
func (h gamblerOperation) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var in gamblerInput
	if decodeOpsInput(raw, &in) != nil {
		return nil, ErrOpsInvalid
	}
	if h.action == "GAMBLER_SNAPSHOT_PREPARE" || h.action == "GAMBLER_RESET_PREPARE" || h.action == "GAMBLER_REAUTHORIZE" {
		if !ValidOperationKey(in.MaintenanceID) || len(in.NativePauseEvidence) < 8 || len(in.NativePauseEvidence) > 256 || strings.TrimSpace(in.NativePauseEvidence) != in.NativePauseEvidence || len(in.BackupReference) > 256 || h.action == "GAMBLER_RESET_PREPARE" && len(in.BackupReference) < 8 {
			return nil, ErrOpsInvalid
		}
	} else if in != (gamblerInput{}) {
		return nil, ErrOpsInvalid
	}
	return json.Marshal(in)
}
func gamblerMaintenance(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `SELECT ops.lock_write_scopes($1::text[])`, gamblerScopes); err != nil {
		return false, err
	}
	var ready bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ops.maintenance_windows w WHERE maintenance_id=$1 AND state='ACTIVE' AND NOT EXISTS(SELECT 1 FROM unnest($2::text[]) s WHERE NOT EXISTS(SELECT 1 FROM ops.maintenance_window_scopes x WHERE x.maintenance_id=w.maintenance_id AND x.scope=s)))`, id, gamblerScopes).Scan(&ready)
	return ready, err
}
func gamblerDrain(ctx context.Context, tx pgx.Tx) ([]string, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT economy.gambler_drain_read()`).Scan(&raw); err != nil {
		return nil, err
	}
	var counts map[string]int64
	if json.Unmarshal(raw, &counts) != nil {
		return nil, ErrOpsUnavailable
	}
	out := []string{}
	for key, n := range counts {
		if n > 0 {
			out = append(out, fmt.Sprintf("%s:%d", key, n))
		}
	}
	slices.Sort(out)
	return out, nil
}
func (h gamblerOperation) Prepare(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, req OpsPrepareRequest, lock bool) (OpsPreparedMaterial, error) {
	var m OpsPreparedMaterial
	if req.Target.ID != GamblerCampaignID {
		return m, ErrOpsInvalid
	}
	var in gamblerInput
	if decodeOpsInput(req.Input, &in) != nil {
		return m, ErrOpsInvalid
	}
	// Maintenance guards precede the campaign and account locks in all paths.
	prior, err := readGambler(ctx, tx, req.Target.ID, false)
	if err != nil {
		return m, err
	}
	maintenance := prior.maintenance
	if in.MaintenanceID != "" {
		maintenance = in.MaintenanceID
	}
	ready, err := gamblerMaintenance(ctx, tx, maintenance)
	if err != nil {
		return m, err
	}
	if err = lockIdentity(ctx, tx, "gambler-campaign", GamblerCampaignID); err != nil {
		return m, err
	}
	c, err := readGambler(ctx, tx, req.Target.ID, lock)
	if err != nil {
		return m, err
	}
	if c.Version != req.Target.ExpectedVersion {
		return m, ErrOpsPreviewStale
	}
	allowed := map[string][]string{"GAMBLER_SNAPSHOT_PREPARE": {"DRAFT"}, "GAMBLER_MEDALS_GRANT": {"SNAPSHOT_READY", "GRANTED"}, "GAMBLER_RESET_PREPARE": {"GRANTED"}, "GAMBLER_RESET_START": {"RESET_READY"}, "ECONOMY_POLICY_ACTIVATE": {"COMPLETED"}, "GAMBLER_REAUTHORIZE": {"SNAPSHOT_PREPARING", "SNAPSHOT_READY", "RESET_PREPARING", "RESET_READY", "RESET_RUNNING", "COMPLETED"}}
	if !slices.Contains(allowed[h.action], c.Phase) {
		return m, ErrOpsConflict
	}
	facts, err := gamblerDrain(ctx, tx)
	if err != nil {
		return m, err
	}
	if !ready {
		facts = append(facts, "REQUIRED_MAINTENANCE_NOT_ACTIVE")
	}
	if !c.NativeUnchanged {
		facts = append(facts, "NATIVE_CHECKPOINT_CHANGED")
	}
	if (h.action == "GAMBLER_RESET_START" || h.action == "ECONOMY_POLICY_ACTIVATE") && ready && len(facts) == 0 {
		if _, e := h.service.assets.native.ReadNativeQuota(ctx, c.actor); e != nil {
			facts = append(facts, "NATIVE_OBSERVER_UNAVAILABLE")
		}
		if err = h.service.verifyReset(ctx, tx, c, h.action == "ECONOMY_POLICY_ACTIVATE"); err != nil {
			if ctx.Err() != nil {
				return m, ctx.Err()
			}
			facts = append(facts, "RESET_CHECKPOINT_CHANGED_OR_UNAVAILABLE")
		}
	}
	if c.reverifyPending && h.action != "GAMBLER_REAUTHORIZE" {
		facts = append(facts, "CHECKPOINT_REVERIFICATION_PENDING")
	}
	proposed := map[string]any{"action": h.action, "reserve_target_units": strconv.FormatInt(gamblerResetReserve, 10), "available_chips_target_units": "0", "native": "UNCHANGED", "input": in}
	current := rawOpsValue(map[string]any{"campaign": c.GamblerCampaignView, "accepted_operation_id": c.operation, "accepted_by": strconv.FormatInt(c.actor, 10), "accepted_epoch": strconv.FormatInt(c.epoch, 10)})
	m = OpsPreparedMaterial{TargetVersion: c.Version, TargetLocator: c.ID, Impact: OpsImpact{CurrentState: current, ProposedChange: rawOpsValue(proposed), Before: current, BlockingFacts: facts, ContinuingAcceptedWork: []string{"已受理的结算、退款与离桌继续；Native 只读接口保持可达"}, RelatedIDs: []string{c.ID}, UnavailableMeasurements: []string{}}}
	if c.ResetCutoffAt != nil {
		var affected, increase, decrease, chips string
		err = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE (reset_before->>'reserve_units')::numeric<>$2 OR (reset_before->>'available_chips_units')::numeric<>0)::text,
 coalesce(sum(greatest($2-(reset_before->>'reserve_units')::numeric,0)),0)::text,
 coalesce(sum(greatest((reset_before->>'reserve_units')::numeric-$2,0)),0)::text,
 coalesce(sum((reset_before->>'available_chips_units')::numeric),0)::text FROM economy.gambler_campaign_accounts WHERE campaign_id=$1 AND reset_member AND reset_before IS NOT NULL`, c.ID, gamblerResetReserve).Scan(&affected, &increase, &decrease, &chips)
		if err != nil {
			return m, err
		}
		m.Impact.Delta = rawOpsValue(map[string]string{"affected_users": affected, "reserve_increase_units": increase, "reserve_decrease_units": decrease, "chips_decrease_units": chips, "native_delta_units": "0"})
	}
	return m, nil
}
func (h gamblerOperation) Execute(ctx context.Context, tx pgx.Tx, actor OpsPrincipal, op OpsOperation, m OpsPreparedMaterial) (OpsExecutionResult, error) {
	if len(m.Impact.BlockingFacts) > 0 {
		return OpsExecutionResult{}, ErrGamblerBlocked
	}
	var in gamblerInput
	if decodeOpsInput(op.inputPayload, &in) != nil {
		return OpsExecutionResult{}, ErrOpsInvalid
	}
	c, err := readGambler(ctx, tx, op.Target.ID, true)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	switch h.action {
	case "GAMBLER_SNAPSHOT_PREPARE":
		_, err = tx.Exec(ctx, `INSERT INTO economy.gambler_campaigns(campaign_id,version,phase,maintenance_id,accepted_by,accepted_epoch,accepted_operation_id,checkpoint) VALUES($1,2,'SNAPSHOT_PREPARING',$2,$3,$4,$5,$6)`, c.ID, in.MaintenanceID, actor.UserID, actor.Epoch, op.OperationID, rawOpsValue(in))
	case "GAMBLER_MEDALS_GRANT":
		if c.Phase != "GRANTED" {
			_, err = tx.Exec(ctx, `INSERT INTO identity.account_badges(newapi_user_id,badge_code,campaign_id) SELECT newapi_user_id,$2,campaign_id FROM economy.gambler_campaign_accounts WHERE campaign_id=$1 AND eligible ON CONFLICT DO NOTHING`, c.ID, GamblerBadgeCode)
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE economy.gambler_campaigns SET phase='GRANTED',granted_at=clock_timestamp(),version=version+1,updated_at=clock_timestamp() WHERE campaign_id=$1`, c.ID)
			}
		}
	case "GAMBLER_RESET_PREPARE":
		_, err = tx.Exec(ctx, `UPDATE economy.gambler_campaigns SET phase='RESET_PREPARING',maintenance_id=$2,accepted_by=$3,accepted_epoch=$4,accepted_operation_id=$5,checkpoint=$6,version=version+1,updated_at=clock_timestamp() WHERE campaign_id=$1`, c.ID, in.MaintenanceID, actor.UserID, actor.Epoch, op.OperationID, rawOpsValue(in))
	case "GAMBLER_RESET_START":
		_, err = tx.Exec(ctx, `UPDATE economy.gambler_campaigns SET phase='RESET_RUNNING',reset_started_at=clock_timestamp(),accepted_by=$2,accepted_epoch=$3,accepted_operation_id=$4,version=version+1,updated_at=clock_timestamp() WHERE campaign_id=$1`, c.ID, actor.UserID, actor.Epoch, op.OperationID)
	case "GAMBLER_REAUTHORIZE":
		_, err = tx.Exec(ctx, `UPDATE economy.gambler_campaigns SET maintenance_id=$2,accepted_by=$3,accepted_epoch=$4,accepted_operation_id=$5,checkpoint=checkpoint||$6::jsonb,reverify_pending=(phase<>'SNAPSHOT_READY'),verification_phase='',verification_cursor=0,blocking_facts='[]',version=version+1,updated_at=clock_timestamp() WHERE campaign_id=$1`, c.ID, in.MaintenanceID, actor.UserID, actor.Epoch, op.OperationID, rawOpsValue(in))
	case "ECONOMY_POLICY_ACTIVATE":
		if c.CompletedCount != c.TargetCount || !c.NativeUnchanged {
			return OpsExecutionResult{}, ErrGamblerBlocked
		}
		if err = h.service.verifyReset(ctx, tx, c, true); err != nil {
			return OpsExecutionResult{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE economy.policy_runtime SET active_version=$1 WHERE singleton AND (active_version IS NULL OR active_version=$1)`, EconomicPolicyVersion); err == nil {
			_, err = tx.Exec(ctx, `UPDATE economy.gambler_campaigns SET policy_activated_at=coalesce(policy_activated_at,clock_timestamp()),accepted_by=$2,accepted_epoch=$3,accepted_operation_id=$4,version=version+1,updated_at=clock_timestamp() WHERE campaign_id=$1`, c.ID, actor.UserID, actor.Epoch, op.OperationID)
		}
	}
	if err != nil {
		return OpsExecutionResult{}, err
	}
	after, err := readGambler(ctx, tx, c.ID, false)
	return OpsExecutionResult{BeforeSnapshot: m.Impact.Before, AfterSnapshot: rawOpsValue(after.GamblerCampaignView), Result: rawOpsValue(after.GamblerCampaignView), RelatedBusinessID: c.ID}, err
}
func sameGamblerAssets(a, b UnifiedAssets) bool {
	return a.UserID == b.UserID && a.ActiveQuotaUnits == b.ActiveQuotaUnits && a.ReserveUnits == b.ReserveUnits && a.AvailableChipsUnits == b.AvailableChipsUnits && a.PokerStackUnits == b.PokerStackUnits && a.PokerPotUnits == b.PokerPotUnits && a.QuotaTransferInFlightUnits == b.QuotaTransferInFlightUnits && a.SinglePlayerInFlightUnits == b.SinglePlayerInFlightUnits && a.RouletteEscrowUnits == b.RouletteEscrowUnits && a.TotalUnits == b.TotalUnits
}

// Each verification transaction observes at most limit frozen accounts. A
// persisted cursor survives process restarts; continuous maintenance + drained
// Native admission fence the completed observations between batches.
func (s *GamblerCampaignService) verifyCheckpointBatch(ctx context.Context, tx pgx.Tx, c gamblerRow, limit int, resume bool) (bool, error) {
	phase := c.Phase
	if resume {
		phase = "RESUME:" + phase
	}
	cursor := c.verificationCursor
	if c.verificationPhase != phase {
		cursor = 0
	}
	snapshot := c.Phase == "SNAPSHOT_PREPARING" || c.Phase == "SNAPSHOT_READY" || c.Phase == "GRANTED"
	rows, err := tx.Query(ctx, `SELECT newapi_user_id,CASE WHEN $2 THEN snapshot_assets WHEN reset_state='COMPLETED' THEN reset_after ELSE reset_before END,
 CASE WHEN $2 THEN snapshot_assets ELSE reset_before END FROM economy.gambler_campaign_accounts WHERE campaign_id=$1 AND newapi_user_id>$3
 AND CASE WHEN $2 THEN snapshot_member AND snapshot_assets IS NOT NULL ELSE reset_member AND reset_before IS NOT NULL END ORDER BY newapi_user_id LIMIT $4`, c.ID, snapshot, cursor, limit)
	if err != nil {
		return false, err
	}
	type saved struct {
		user             int64
		expected, before []byte
	}
	all := []saved{}
	for rows.Next() {
		var a saved
		if err = rows.Scan(&a.user, &a.expected, &a.before); err != nil {
			break
		}
		all = append(all, a)
	}
	rows.Close()
	if err != nil {
		return false, err
	}
	if rows.Err() != nil {
		return false, rows.Err()
	}
	for _, a := range all {
		var expected, before UnifiedAssets
		if json.Unmarshal(a.expected, &expected) != nil || json.Unmarshal(a.before, &before) != nil {
			return false, ErrGamblerBlocked
		}
		current, e := ReadUnifiedAssetsInTx(ctx, tx, s.assets.native, a.user)
		if e != nil {
			return false, e
		}
		if current.ActiveQuotaUnits != before.ActiveQuotaUnits {
			return false, errGamblerNativeChanged
		}
		if !sameGamblerAssets(expected, current) {
			return false, ErrGamblerBlocked
		}
		cursor = a.user
	}
	var remaining bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM economy.gambler_campaign_accounts WHERE campaign_id=$1 AND newapi_user_id>$3 AND CASE WHEN $2 THEN snapshot_member AND snapshot_assets IS NOT NULL ELSE reset_member AND reset_before IS NOT NULL END)`, c.ID, snapshot, cursor).Scan(&remaining); err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE economy.gambler_campaigns SET verification_phase=$2,verification_cursor=$3,reverify_pending=CASE WHEN $4 THEN false ELSE reverify_pending END,blocking_facts='[]',version=version+1,updated_at=clock_timestamp() WHERE campaign_id=$1`, c.ID, phase, cursor, resume && !remaining)
	return !remaining, err
}

func gamblerIdle(a UnifiedAssets) bool {
	return a.PokerStackUnits == 0 && a.PokerPotUnits == 0 && a.QuotaTransferInFlightUnits == 0 && a.SinglePlayerInFlightUnits == 0 && a.RouletteEscrowUnits == 0
}

// Native is checked in the resumable sealed pass and immediately before and
// after each user's reset. Here only local frozen balances are checked in SQL;
// no unbounded network scan is permitted on an Ops confirmation request.
func (s *GamblerCampaignService) verifyReset(ctx context.Context, tx pgx.Tx, c gamblerRow, completed bool) error {
	if c.reverifyPending {
		return ErrGamblerBlocked
	}
	var bad bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM economy.gambler_campaign_accounts a WHERE campaign_id=$1 AND reset_member AND
 (reset_before IS NULL OR ($2 AND reset_state<>'COMPLETED') OR reset_state NOT IN('CHECKED','COMPLETED') OR
 (SELECT count(*) FROM economy.wallet_balances w WHERE w.newapi_user_id=a.newapi_user_id)<>2 OR
 EXISTS(SELECT 1 FROM economy.wallet_balances w WHERE w.newapi_user_id=a.newapi_user_id AND w.balance_units::numeric <>
 (CASE WHEN a.reset_state='COMPLETED' THEN a.reset_after ELSE a.reset_before END ->> CASE WHEN w.asset_type='RESERVE_API_CREDIT' THEN 'reserve_units' ELSE 'available_chips_units' END)::numeric)))`, c.ID, completed).Scan(&bad)
	if err != nil {
		return err
	}
	if bad {
		return ErrGamblerBlocked
	}
	return nil
}

// Publication is retried after the existing rankings maintenance gate opens.
// The durable public pointer resolves a lost Build/COMMIT response; no second
// campaign snapshot is built when that pointer is already newer than activation.
func (s *GamblerCampaignService) RefreshRankings(ctx context.Context, build func(context.Context) (string, error)) (changed bool, err error) {
	if s == nil || build == nil {
		return false, ErrOpsUnavailable
	}
	err = s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if e := RequireNoMaintenance(ctx, tx, "RANKINGS_PUBLISHING"); e != nil {
			if errors.Is(e, ErrMaintenanceActive) {
				return nil
			}
			return e
		}
		if e := lockIdentity(ctx, tx, "gambler-campaign", GamblerCampaignID); e != nil {
			return e
		}
		c, e := readGambler(ctx, tx, GamblerCampaignID, true)
		if e != nil {
			return e
		}
		if c.Phase != "COMPLETED" || c.PolicyActivatedAt == nil || c.RankingsPublishedAt != nil {
			return nil
		}
		published := func() (bool, error) {
			var ready bool
			e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rankings.published_pointers p JOIN rankings.snapshots s USING(snapshot_id) WHERE p.domain='ASSETS' AND p.metric='TOTAL_ASSETS' AND p.period='CURRENT' AND s.status='READY' AND s.source_checked_at>=$1)`, c.PolicyActivatedAt).Scan(&ready)
			return ready, e
		}
		ready, e := published()
		if e != nil {
			return e
		}
		if !ready {
			if _, e = build(ctx); e != nil {
				return e
			}
			ready, e = published()
			if e != nil {
				return e
			}
			if !ready {
				return ErrGamblerBlocked
			}
		}
		_, e = tx.Exec(ctx, `UPDATE economy.gambler_campaigns SET rankings_published_at=clock_timestamp(),version=version+1,updated_at=clock_timestamp() WHERE campaign_id=$1`, c.ID)
		changed = e == nil
		return e
	})
	return changed, err
}
func gamblerCohortHash(ctx context.Context, tx pgx.Tx, id string, reset bool) (string, error) {
	var ids string
	err := tx.QueryRow(ctx, `SELECT coalesce(string_agg(newapi_user_id::text,',' ORDER BY newapi_user_id),'') FROM economy.gambler_campaign_accounts WHERE campaign_id=$1 AND CASE WHEN $2 THEN reset_member ELSE snapshot_member END`, id, reset).Scan(&ids)
	sum := sha256.Sum256([]byte(ids))
	return hex.EncodeToString(sum[:]), err
}
func gamblerBlock(ctx context.Context, tx pgx.Tx, id string, facts []string, nativeChanged bool) error {
	_, err := tx.Exec(ctx, `UPDATE economy.gambler_campaigns SET blocking_facts=$2,native_unchanged=native_unchanged AND NOT $3,version=version+1,updated_at=clock_timestamp() WHERE campaign_id=$1 AND (blocking_facts<>$2::jsonb OR ($3 AND native_unchanged))`, id, rawOpsValue(facts), nativeChanged)
	return err
}

// Each batch is one durable transaction. A lost response or process restart
// resumes the original campaign; completed accounts have no second ledger legs.
func (s *GamblerCampaignService) RunBatch(ctx context.Context, id string, limit int) (progressed bool, err error) {
	if s == nil || id != GamblerCampaignID || limit < 1 || limit > 50 {
		return false, ErrOpsInvalid
	}
	err = s.store.WithTx(ctx, func(tx pgx.Tx) error {
		prior, e := readGambler(ctx, tx, id, false)
		if e != nil {
			return e
		}
		if !prior.reverifyPending && !slices.Contains([]string{"SNAPSHOT_PREPARING", "RESET_PREPARING", "RESET_RUNNING"}, prior.Phase) {
			return nil
		}
		actor, e := requireOpsPermission(ctx, tx, prior.actor, prior.epoch, "economy.adjust", true)
		if e != nil || actor.Role != "SUPER_ADMIN" {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return gamblerBlock(ctx, tx, id, []string{"ACCEPTED_AUTHORITY_CHANGED"}, false)
		}
		ready, e := gamblerMaintenance(ctx, tx, prior.maintenance)
		if e != nil {
			return e
		}
		if e = lockIdentity(ctx, tx, "gambler-campaign", GamblerCampaignID); e != nil {
			return e
		}
		c, e := readGambler(ctx, tx, id, true)
		if e != nil {
			return e
		}
		if c.Phase != prior.Phase || c.operation != prior.operation {
			return nil
		}
		var accepted bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ops.admin_operations WHERE operation_id=$1 AND state='SUCCEEDED' AND newapi_user_id=$2 AND actor_authz_epoch_snapshot=$3)`, c.operation, c.actor, c.epoch).Scan(&accepted); e != nil {
			return e
		}
		if !accepted {
			return gamblerBlock(ctx, tx, id, []string{"ACCEPTED_OPERATION_UNRESOLVED"}, false)
		}
		facts, e := gamblerDrain(ctx, tx)
		if e != nil {
			return e
		}
		if !ready {
			facts = append(facts, "REQUIRED_MAINTENANCE_NOT_ACTIVE")
		}
		if !c.NativeUnchanged {
			facts = append(facts, "NATIVE_CHECKPOINT_CHANGED")
		}
		if len(facts) > 0 {
			return gamblerBlock(ctx, tx, id, facts, false)
		}
		if c.reverifyPending {
			_, e = s.verifyCheckpointBatch(ctx, tx, c, limit, true)
			if e != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return gamblerBlock(ctx, tx, id, []string{"RESUME_CHECKPOINT_CHANGED_OR_UNAVAILABLE"}, errors.Is(e, errGamblerNativeChanged))
			}
			progressed = true
			return nil
		}

		reset := c.Phase != "SNAPSHOT_PREPARING"
		if (!reset && c.CutoffAt == nil) || (reset && c.ResetCutoffAt == nil) {
			column, member := "cutoff_at", "snapshot_member"
			if reset {
				column, member = "reset_cutoff_at", "reset_member"
			}
			if _, e = tx.Exec(ctx, `UPDATE economy.gambler_campaigns SET `+column+`=clock_timestamp(),version=version+1,updated_at=clock_timestamp() WHERE campaign_id=$1`, id); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO economy.gambler_campaign_accounts(campaign_id,newapi_user_id,`+member+`) SELECT $1,newapi_user_id,true FROM identity.account_refs WHERE created_at<=(SELECT `+column+` FROM economy.gambler_campaigns WHERE campaign_id=$1) ON CONFLICT(campaign_id,newapi_user_id) DO UPDATE SET `+member+`=true`, id); e != nil {
				return e
			}
			hash, e := gamblerCohortHash(ctx, tx, id, reset)
			if e != nil {
				return e
			}
			hashColumn := "snapshot_target_hash"
			if reset {
				hashColumn = "reset_target_hash"
			}
			if _, e = tx.Exec(ctx, `UPDATE economy.gambler_campaigns SET `+hashColumn+`=$2,version=version+1,updated_at=clock_timestamp() WHERE campaign_id=$1`, id, hash); e != nil {
				return e
			}
			c, e = readGambler(ctx, tx, id, false)
			if e != nil {
				return e
			}
		}
		hash, e := gamblerCohortHash(ctx, tx, id, reset)
		if e != nil {
			return e
		}
		wantHash := c.snapshotHash
		if reset {
			wantHash = c.resetHash
		}
		var expectedIDs string
		cutoff := c.CutoffAt
		if reset {
			cutoff = c.ResetCutoffAt
		}
		if e = tx.QueryRow(ctx, `SELECT coalesce(string_agg(newapi_user_id::text,',' ORDER BY newapi_user_id),'') FROM identity.account_refs WHERE created_at<=$1`, cutoff).Scan(&expectedIDs); e != nil {
			return e
		}
		expectedHash := sha256.Sum256([]byte(expectedIDs))
		if hash != wantHash || hash != hex.EncodeToString(expectedHash[:]) {
			return gamblerBlock(ctx, tx, id, []string{"COHORT_DIGEST_CHANGED"}, false)
		}
		condition := "snapshot_member AND snapshot_assets IS NULL"
		if c.Phase == "RESET_PREPARING" {
			condition = "reset_member AND reset_state='PENDING'"
		}
		if c.Phase == "RESET_RUNNING" {
			condition = "reset_member AND reset_state='CHECKED'"
		}
		rows, e := tx.Query(ctx, `SELECT newapi_user_id FROM economy.gambler_campaign_accounts WHERE campaign_id=$1 AND `+condition+` ORDER BY newapi_user_id LIMIT $2`, id, limit)
		if e != nil {
			return e
		}
		var users []int64
		for rows.Next() {
			var u int64
			if e = rows.Scan(&u); e != nil {
				break
			}
			users = append(users, u)
		}
		rows.Close()
		if e != nil {
			return e
		}
		if rows.Err() != nil {
			return rows.Err()
		}
		for _, u := range users {
			// Savepoint keeps a failed Native observation or partially attempted reset
			// from committing any effects while retaining an explicit blocking fact.
			sub, e := tx.Begin(ctx)
			if e != nil {
				return e
			}
			e = s.processAccount(ctx, sub, c, u)
			if e != nil {
				_ = sub.Rollback(ctx)
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return gamblerBlock(ctx, tx, id, []string{fmt.Sprintf("ACCOUNT_%d_UNRESOLVED", u)}, errors.Is(e, errGamblerNativeChanged))
			}
			if e = sub.Commit(ctx); e != nil {
				return e
			}
			progressed = true
		}
		var remaining int64
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM economy.gambler_campaign_accounts WHERE campaign_id=$1 AND `+condition, id).Scan(&remaining); e != nil {
			return e
		}
		if remaining == 0 && len(users) == 0 {
			var done bool
			if done, e = s.verifyCheckpointBatch(ctx, tx, c, limit, false); e != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return gamblerBlock(ctx, tx, id, []string{"FINAL_CHECKPOINT_CHANGED_OR_UNAVAILABLE"}, errors.Is(e, errGamblerNativeChanged))
			}
			progressed = true
			if !done {
				return nil
			}
			phase, stamp := "SNAPSHOT_READY", "snapshot_sealed_at"
			if c.Phase == "RESET_PREPARING" {
				phase, stamp = "RESET_READY", "reset_prepared_at"
			}
			if c.Phase == "RESET_RUNNING" {
				phase, stamp = "COMPLETED", "completed_at"
			}
			_, e = tx.Exec(ctx, `UPDATE economy.gambler_campaigns SET phase=$2,`+stamp+`=clock_timestamp(),blocking_facts='[]',version=version+1,updated_at=clock_timestamp() WHERE campaign_id=$1`, id, phase)
			progressed = true
			return e
		}
		if e = gamblerBlock(ctx, tx, id, []string{}, false); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE economy.gambler_campaigns SET version=version+1,updated_at=clock_timestamp() WHERE campaign_id=$1`, id)
		return e
	})
	return progressed, err
}

var errGamblerNativeChanged = errors.New("native checkpoint changed")

func (s *GamblerCampaignService) processAccount(ctx context.Context, tx pgx.Tx, c gamblerRow, user int64) error {
	if err := LockEconomyUsersInTx(ctx, tx, user); err != nil {
		return err
	}
	// Some provisional accounts have no wallet rows yet; zero initialization is
	// structural, not a grant, and includes those accounts in the fixed cohort.
	if _, err := tx.Exec(ctx, `INSERT INTO economy.wallet_balances(newapi_user_id,asset_type) VALUES($1,'RESERVE_API_CREDIT'),($1,'AVAILABLE_CHIPS') ON CONFLICT DO NOTHING`, user); err != nil {
		return err
	}
	current, err := ReadUnifiedAssetsInTx(ctx, tx, s.assets.native, user)
	if err != nil {
		return err
	}
	if !gamblerIdle(current) {
		return ErrGamblerBlocked
	}
	raw := rawOpsValue(current)
	switch c.Phase {
	case "SNAPSHOT_PREPARING":
		digest := sha256.Sum256(raw)
		_, err = tx.Exec(ctx, `UPDATE economy.gambler_campaign_accounts SET snapshot_assets=$3,snapshot_digest=$4,snapshot_at=clock_timestamp(),eligible=$5 WHERE campaign_id=$1 AND newapi_user_id=$2`, c.ID, user, raw, hex.EncodeToString(digest[:]), current.TotalUnits > gamblerThreshold)
	case "RESET_PREPARING":
		_, err = tx.Exec(ctx, `UPDATE economy.gambler_campaign_accounts SET reset_before=$3,reset_state='CHECKED' WHERE campaign_id=$1 AND newapi_user_id=$2`, c.ID, user, raw)
	case "RESET_RUNNING":
		var stored []byte
		if err = tx.QueryRow(ctx, `SELECT reset_before FROM economy.gambler_campaign_accounts WHERE campaign_id=$1 AND newapi_user_id=$2 AND reset_state='CHECKED' FOR UPDATE`, c.ID, user).Scan(&stored); err != nil {
			return err
		}
		var before UnifiedAssets
		if json.Unmarshal(stored, &before) != nil {
			return ErrGamblerBlocked
		}
		if before.ActiveQuotaUnits != current.ActiveQuotaUnits {
			return errGamblerNativeChanged
		}
		if !sameGamblerAssets(before, current) {
			return ErrGamblerBlocked
		}
		entries := []LedgerEntry{}
		for _, a := range []struct {
			asset Asset
			delta int64
		}{{ReserveAPICredit, gamblerResetReserve - current.ReserveUnits}, {AvailableChips, -current.AvailableChipsUnits}} {
			if a.delta == 0 {
				continue
			}
			key := fmt.Sprintf("gambler-reset:%s:%d:%s", c.ID, user, a.asset)
			entry, e := applyInTx(ctx, tx, Mutation{UserID: user, Asset: a.asset, DeltaUnits: a.delta, BizType: "GAMBLER_BALANCE_RESET", BizID: key, EntryType: "CAMPAIGN_RESET", IdempotencyKey: key})
			if e != nil {
				return e
			}
			entries = append(entries, entry)
		}
		after, e := ReadUnifiedAssetsInTx(ctx, tx, s.assets.native, user)
		if e != nil {
			return e
		}
		if after.ActiveQuotaUnits != before.ActiveQuotaUnits {
			return errGamblerNativeChanged
		}
		if after.ReserveUnits != gamblerResetReserve || after.AvailableChipsUnits != 0 || !gamblerIdle(after) {
			return ErrGamblerBlocked
		}
		receipt := rawOpsValue(map[string]any{"campaign_id": c.ID, "user_id": strconv.FormatInt(user, 10), "entries": entries, "native_unchanged": true, "no_balance_change": len(entries) == 0})
		_, err = tx.Exec(ctx, `UPDATE economy.gambler_campaign_accounts SET reset_after=$3,reset_receipt=$4,reset_state='COMPLETED',reset_at=clock_timestamp() WHERE campaign_id=$1 AND newapi_user_id=$2`, c.ID, user, rawOpsValue(after), receipt)
	default:
		return ErrOpsConflict
	}
	return err
}
