package poker

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/cy4268/momiao/internal/poker/fairness"
	"github.com/jackc/pgx/v5"
)

var ErrMaintenance = errors.New("MAINTENANCE_ACTIVE")
var ErrRulesetIncomplete = errors.New("POKER_RULESET_INCOMPLETE")

const approvedRulesetVersion = "poker-cash-v1-20260906"

var supportedRuleset = LobbyRuleset{approvedRulesetVersion, "NO_ANTE", "WAIT_FOR_BB", engine.ButtonSelectionVersion, engine.EvaluatorVersion, "poker-pot-after-call-whole-chip-v1", fairness.AlgorithmVersion, "poker-deal-v1"}

type admissionFacts struct {
	now     time.Time
	ruleset LobbyRuleset
	ready   bool
	scopes  []string
}

func readAdmissionFacts(ctx context.Context, tx pgx.Tx, version string) (admissionFacts, error) {
	f := admissionFacts{scopes: []string{}}
	var enabled, global, poker bool
	err := tx.QueryRow(ctx, `SELECT transaction_timestamp(),coalesce(r.version,''),coalesce(r.ante_posting_mode,''),coalesce(r.entry_mode,''),coalesce(r.initial_button_version,''),coalesce(r.evaluator_version,''),coalesce(r.shortcut_version,''),coalesce(r.algorithm_version,''),coalesce(r.deal_version,''),coalesce(r.active,false),
 ops.is_maintenance_scope_active('CHALDEA_USER_WRITES'),ops.is_maintenance_scope_active('POKER_NEW_TABLES_NEW_HANDS')
 FROM (SELECT 1) anchor LEFT JOIN poker.ruleset_versions r ON r.version=$1`, version).Scan(&f.now, &f.ruleset.Version, &f.ruleset.AntePostingMode, &f.ruleset.EntryMode, &f.ruleset.InitialButtonVersion, &f.ruleset.EvaluatorVersion, &f.ruleset.ShortcutVersion, &f.ruleset.AlgorithmVersion, &f.ruleset.DealVersion, &enabled, &global, &poker)
	f.ready = enabled && f.ruleset == supportedRuleset
	if global {
		f.scopes = append(f.scopes, "CHALDEA_USER_WRITES")
	}
	if poker {
		f.scopes = append(f.scopes, "POKER_NEW_TABLES_NEW_HANDS")
	}
	return f, err
}

func newWorkEligible(f admissionFacts, profile, active bool) bool {
	return f.ready && len(f.scopes) == 0 && profile && !active
}

// A separate statement after the shared lock observes an activation that won
// the race. The lock is held through the caller's existing business commit.
// user == 0 gates a new hand, never already-COMMITTED work or settlement.
func requireAdmission(ctx context.Context, tx pgx.Tx, table *tableRow, user int64, preset string) error {
	if _, err := tx.Exec(ctx, "SELECT ops.lock_poker_admission_scopes()"); err != nil {
		return err
	}
	version := approvedRulesetVersion
	if table != nil {
		version = table.Ruleset
		preset = table.Preset
	}
	facts, err := readAdmissionFacts(ctx, tx, version)
	if err != nil {
		return err
	}
	if len(facts.scopes) > 0 {
		return ErrMaintenance
	}
	if !facts.ready {
		return ErrRulesetIncomplete
	}
	presets, err := readLobbyPresets(ctx, tx)
	if err != nil {
		return err
	}
	if len(presets) == 0 {
		return ErrRulesetIncomplete
	}
	found := false
	for _, p := range presets {
		found = found || p.ID == preset
	}
	if !found {
		return ErrInvalid
	}
	if user == 0 {
		return nil
	}
	v, err := readLobbyIdentity(ctx, tx, user)
	if err != nil {
		return err
	}
	var active bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM poker.sessions WHERE newapi_user_id=$1 AND state<>'SETTLED')", user).Scan(&active); err != nil {
		return err
	}
	if !newWorkEligible(facts, v.ProfileComplete, active) || table == nil && v.OwnedOpenTableID != nil || table != nil && !tableJoinable(table.State, table.AccessMode, table.Accepting, table.NeedsReview) {
		return ErrDenied
	}
	return nil
}
func tableJoinable(state, access string, accepting, needsReview bool) bool {
	return accepting && !needsReview && (access == "PUBLIC" || access == "PASSWORD") && state != "CLOSED" && state != "CLOSING" && state != "RECOVERING"
}
func readLobbyIdentity(ctx context.Context, tx pgx.Tx, user int64) (LobbyViewer, error) {
	v := LobbyViewer{UserID: decimal(user), PokerInPlayUnits: "0"}
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM identity.master_profiles WHERE newapi_user_id=$1 AND profile_version>0),
 (SELECT table_id::text FROM poker.tables WHERE owner_newapi_user_id=$1 AND lifecycle_state<>'CLOSED')`, user).Scan(&v.ProfileComplete, &v.OwnedOpenTableID)
	return v, err
}
func lobbyBlinds(sb, bb int64, minimum, maximum int) (LobbyBlinds, error) {
	if !units(sb) || sb <= 0 || bb <= 0 || bb > math.MaxInt64/100 || bb != 2*sb || minimum != 40 || maximum != 100 {
		return LobbyBlinds{}, ErrRulesetIncomplete
	}
	return LobbyBlinds{decimal(sb), decimal(bb), "0", decimal(bb * int64(minimum)), decimal(bb * int64(maximum))}, nil
}
func readLobbyPresets(ctx context.Context, tx pgx.Tx) ([]LobbyPreset, error) {
	rows, err := tx.Query(ctx, `SELECT version,small_blind_units,big_blind_units,minimum_buyin_bb,maximum_buyin_bb FROM poker.blind_preset_versions ORDER BY big_blind_units,version LIMIT 101`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LobbyPreset{}
	for rows.Next() {
		var p LobbyPreset
		var sb, bb int64
		var minimum, maximum int
		if err = rows.Scan(&p.ID, &sb, &bb, &minimum, &maximum); err != nil {
			return nil, err
		}
		if p.LobbyBlinds, err = lobbyBlinds(sb, bb, minimum, maximum); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if len(out) > 100 {
		return nil, ErrRulesetIncomplete
	}
	return out, rows.Err()
}
