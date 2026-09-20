package roulette

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"github.com/cy4268/momiao/internal/games/fairness"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

const devilCanonical = `{"algorithm_version":"momiao-devil-rng-v1","game":"devil-roulette","game_seconds":1800,"rake_bps":0,"ruleset_version":"momiao-devil-rules-v1","turn_seconds":62,"upstream_commit":"6bfbda58b19e045062f4f5939bb9223d810eddb3","wait_seconds":300}`
const pressureCanonical = `{"algorithm_version":"momiao-pressure-rng-v1","game":"pressure-roulette","game_seconds":1800,"rake_bps":0,"ruleset_version":"momiao-pressure-rules-v1","turn_seconds":[45,25,10],"upstream_commit":"6bfbda58b19e045062f4f5939bb9223d810eddb3","vote_seconds":35,"wait_seconds":300}`
const policyCanonical = `{"input_step_units":500000,"maximum_mode":"NONE","minimum_wager_units":5000000,"quick_amount_units":[5000000,50000000,250000000,500000000],"version_number":1}`
const rulesArtifact = `{"conservation":"funded=refunds+payouts+escrow","rake_bps":0,"validation":"COMPILED_RULESET","version":1}`

type Config struct {
	ID, Game, Schema, Ruleset, Algorithm string
	Canonical                            []byte
	Hash                                 [32]byte
}

func IsGame(game string) bool { return game == "devil-roulette" || game == "pressure-roulette" }
func implementation(game string) string {
	if game == "devil-roulette" {
		return "roulette.devil.v1"
	}
	if game == "pressure-roulette" {
		return "roulette.pressure.v1"
	}
	return ""
}
func ParseConfig(game, id string, canonical []byte) (Config, error) {
	var want, rules, algorithm string
	switch game {
	case "devil-roulette":
		want = devilCanonical
		rules = "momiao-devil-rules-v1"
		algorithm = "momiao-devil-rng-v1"
	case "pressure-roulette":
		want = pressureCanonical
		rules = "momiao-pressure-rules-v1"
		algorithm = "momiao-pressure-rng-v1"
	default:
		return Config{}, ErrUnavailable
	}
	if !ValidRoundID(id) || !bytes.Equal([]byte(want), canonical) {
		return Config{}, ErrUnavailable
	}
	return Config{id, game, "momiao-roulette-config-v1", rules, algorithm, bytes.Clone(canonical), sha256.Sum256(canonical)}, nil
}
func ReadConfig(ctx context.Context, tx pgx.Tx, game, id string) (Config, error) {
	var schema, rules, algorithm, status string
	var canonical, hash []byte
	e := tx.QueryRow(ctx, `SELECT config_schema_version,ruleset_version,algorithm_version,canonical_payload,config_hash,status FROM games.game_config_versions WHERE config_version_id=$1 AND game_slug=$2`, id, game).Scan(&schema, &rules, &algorithm, &canonical, &hash, &status)
	if e != nil {
		return Config{}, e
	}
	c, e := ParseConfig(game, id, canonical)
	if e != nil || schema != c.Schema || rules != c.Ruleset || algorithm != c.Algorithm || !bytes.Equal(hash, c.Hash[:]) || status == "DRAFT" {
		return Config{}, ErrUnavailable
	}
	var kind, impl, arules, aalg, validator, build, astatus string
	var ch, proof, summary []byte
	e = tx.QueryRow(ctx, `SELECT artifact_type,implementation_key,ruleset_version,algorithm_version,validator_version,validation_build,status,config_hash,artifact_sha256,result_summary FROM games.game_validation_artifacts WHERE config_version_id=$1 AND game_slug=$2 AND verified_at IS NOT NULL`, id, game).Scan(&kind, &impl, &arules, &aalg, &validator, &build, &astatus, &ch, &proof, &summary)
	if e != nil {
		return Config{}, e
	}
	var value any
	if json.Unmarshal(summary, &value) != nil {
		return Config{}, ErrUnavailable
	}
	normalized, _ := json.Marshal(value)
	ah := sha256.Sum256([]byte(rulesArtifact))
	if kind != "MULTIPLAYER_RULESET" || impl != implementation(game) || arules != rules || aalg != algorithm || validator != "momiao-roulette-validate-v1" || build != "momiao-roulette-v1" || astatus != "VERIFIED" || !bytes.Equal(ch, c.Hash[:]) || !bytes.Equal(proof, ah[:]) || !bytes.Equal(normalized, []byte(rulesArtifact)) {
		return Config{}, ErrUnavailable
	}
	return c, nil
}

type CatalogStatus struct{ Slug, Title, Implementation, State string }
type policy struct {
	ID            string
	Hash          [32]byte
	Minimum, Step int64
}

func readPolicy(ctx context.Context, tx pgx.Tx, id string) (policy, error) {
	p := policy{ID: id}
	var version int64
	var mode string
	var quick []int64
	var hash []byte
	e := tx.QueryRow(ctx, `SELECT version_number,minimum_wager_units,input_step_units,maximum_mode,quick_amount_units,policy_hash FROM games.wager_policy_versions WHERE wager_policy_version_id=$1`, id).Scan(&version, &p.Minimum, &p.Step, &mode, &quick, &hash)
	if e != nil {
		return p, e
	}
	p.Hash = sha256.Sum256([]byte(policyCanonical))
	if version != 1 || p.Minimum != 5000000 || p.Step != 500000 || mode != "NONE" || !slices.Equal(quick, []int64{5000000, 50000000, 250000000, 500000000}) || !bytes.Equal(p.Hash[:], hash) {
		return p, ErrUnavailable
	}
	return p, nil
}
func resolve(ctx context.Context, tx pgx.Tx, game string, lock bool) (CatalogStatus, Config, policy, error) {
	entry := CatalogStatus{Slug: game, State: "TEMPORARILY_UNAVAILABLE"}
	var config Config
	var p policy
	if !IsGame(game) {
		return entry, config, p, ErrNotFound
	}
	suffix := ""
	if lock {
		suffix = " FOR SHARE"
		if e := platform.RequireNoMaintenance(ctx, tx, "CHALDEA_USER_WRITES", "DIRECT_PLAY_NEW_ROUNDS"); e != nil {
			if errors.Is(e, platform.ErrMaintenanceActive) {
				e = ErrUnavailable
			}
			return entry, config, p, e
		}
	}
	var maintenance bool
	var policyID, pub, runtime string
	var active *string
	e := tx.QueryRow(ctx, `SELECT maintenance,active_wager_policy_version_id::text FROM games.runtime_gate WHERE singleton`+suffix).Scan(&maintenance, &policyID)
	if e != nil {
		return entry, config, p, e
	}
	e = tx.QueryRow(ctx, `SELECT title,implementation_key,publication_state,configured_runtime_state,active_config_version_id::text FROM games.game_registry WHERE game_slug=$1`+suffix, game).Scan(&entry.Title, &entry.Implementation, &pub, &runtime, &active)
	if e != nil {
		return entry, config, p, e
	}
	if pub == "COMING_SOON" || pub == "RETIRED" {
		entry.State = pub
		return entry, config, p, nil
	}
	if pub != "PUBLISHED" {
		return entry, config, p, ErrNotFound
	}
	if active == nil || entry.Implementation != implementation(game) {
		return entry, config, p, nil
	}
	var status string
	e = tx.QueryRow(ctx, `SELECT status FROM games.game_config_versions WHERE config_version_id=$1`, *active).Scan(&status)
	if e != nil {
		return entry, config, p, e
	}
	if status != "ACTIVE" {
		return entry, config, p, nil
	}
	config, e = ReadConfig(ctx, tx, game, *active)
	if e != nil {
		return entry, config, p, e
	}
	p, e = readPolicy(ctx, tx, policyID)
	if e != nil {
		return entry, config, p, e
	}
	if maintenance || runtime == "MAINTENANCE" {
		entry.State = "MAINTENANCE"
	} else if runtime == "AVAILABLE" {
		entry.State = "PLAY"
	}
	return entry, config, p, nil
}
func CatalogInTx(ctx context.Context, tx pgx.Tx, game string) (CatalogStatus, error) {
	v, _, _, e := resolve(ctx, tx, game, false)
	if errors.Is(e, ErrUnavailable) {
		e = nil
	}
	return v, e
}
func binding(c Config, p policy) Binding {
	return Binding{c.ID, hex.EncodeToString(c.Hash[:]), p.ID, hex.EncodeToString(p.Hash[:]), c.Ruleset, c.Algorithm, fairness.StreamVersion}
}

// READY uses the archived binding; a newly active config never rewrites a room.
func readyGate(ctx context.Context, tx pgx.Tx, game string) error {
	if e := platform.RequireNoMaintenance(ctx, tx, "CHALDEA_USER_WRITES", "DIRECT_PLAY_NEW_ROUNDS"); e != nil {
		return ErrUnavailable
	}
	var maintenance bool
	var pub, runtime, impl string
	e := tx.QueryRow(ctx, `SELECT maintenance FROM games.runtime_gate WHERE singleton FOR SHARE`).Scan(&maintenance)
	if e != nil {
		return e
	}
	e = tx.QueryRow(ctx, `SELECT publication_state,configured_runtime_state,implementation_key FROM games.game_registry WHERE game_slug=$1 FOR SHARE`, game).Scan(&pub, &runtime, &impl)
	if e != nil {
		return e
	}
	if maintenance || pub != "PUBLISHED" || runtime != "AVAILABLE" || impl != implementation(game) {
		return ErrUnavailable
	}
	return nil
}
