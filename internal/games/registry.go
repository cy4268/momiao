package games

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/cy4268/momiao/internal/games/blackjack"
	"github.com/cy4268/momiao/internal/games/slot"
	"github.com/jackc/pgx/v5"
)

// The database references these compiled implementations; it never supplies code.
func loadConfigVersion(slug, version string, canonical []byte) (Config, error) {
	switch slug {
	case "dice":
		var payload struct {
			TripleRule string `json:"triple_rule"`
		}
		if json.Unmarshal(canonical, &payload) != nil {
			return Config{}, ErrInvalidConfig
		}
		if payload.TripleRule == "PUSH" {
			return DiceV2(version)
		}
		return DiceV1(version)
	case "slot":
		var payload struct {
			PaytableVersion string `json:"paytable_version"`
		}
		if json.Unmarshal(canonical, &payload) != nil {
			return Config{}, ErrInvalidConfig
		}
		if payload.PaytableVersion == slot.FairPaytableVersion {
			return SlotV2(version)
		}
		if payload.PaytableVersion == slot.FrequentPaytableVersion {
			return SlotV3(version)
		}
		return SlotV1(version)
	case "blackjack":
		var payload struct {
			FairReturnVersion string `json:"fair_return_version"`
		}
		if json.Unmarshal(canonical, &payload) != nil {
			return Config{}, ErrInvalidConfig
		}
		if payload.FairReturnVersion == blackjack.FairReturnVersion {
			return BlackjackV2(version)
		}
		return BlackjackV1(version)
	case "scratch", "summon":
		var payload struct {
			Prizes       []Prize `json:"prizes"`
			PrizeVersion string  `json:"prize_table_version"`
		}
		if json.Unmarshal(canonical, &payload) != nil {
			return Config{}, ErrInvalidConfig
		}
		if slug == "scratch" {
			return NewScratchConfig(version, payload.PrizeVersion, payload.Prizes)
		}
		return NewSummonConfig(version, payload.PrizeVersion, payload.Prizes)
	default:
		return Config{}, ErrUnavailable
	}
}
func configByID(ctx context.Context, tx pgx.Tx, slug, version string) (Config, error) {
	var schema, ruleset, algorithm string
	var canonical, hash []byte
	err := tx.QueryRow(ctx, `SELECT config_schema_version,ruleset_version,algorithm_version,canonical_payload,config_hash FROM games.game_config_versions WHERE config_version_id=$1 AND game_slug=$2`, version, slug).Scan(&schema, &ruleset, &algorithm, &canonical, &hash)
	if err != nil {
		return Config{}, err
	}
	c, err := loadConfigVersion(slug, version, canonical)
	if err != nil {
		return Config{}, err
	}
	binding := c.Binding()
	if schema != binding.SchemaVersion || ruleset != binding.RulesetVersion || algorithm != binding.AlgorithmVersion || !bytes.Equal(c.CanonicalJSON(), canonical) || !bytes.Equal(binding.Hash[:], hash) {
		return Config{}, ErrConfigMismatch
	}
	return c, nil
}
func summary(c Config) *ConfigSummary {
	b := c.Binding()
	return &ConfigSummary{Version: b.Version, Hash: hex.EncodeToString(b.Hash[:]), Schema: b.SchemaVersion, Ruleset: b.RulesetVersion, Algorithm: b.AlgorithmVersion, Prizes: c.Prizes(), Statistics: mathSummary(c), Validation: json.RawMessage(extraValidationSummary(c))}
}
func expectedResources(c Config) []byte {
	b := c.Binding()
	resources := map[string]string{}
	if b.PrizeTableVersion != "" {
		resources["prize_table_version"] = b.PrizeTableVersion
	}
	if b.PoolID != "" {
		resources["pool_id"] = b.PoolID
	}
	if b.Game == "slot" {
		resources["reel_strip_version"] = "slot-strips-v1"
		resources["payline_version"] = "slot-paylines-v1"
		resources["paytable_version"] = "slot-paytable-v1"
		if b.RulesetVersion == "slot-rules-v2" {
			resources["paytable_version"] = slot.FairPaytableVersion
		}
		if b.RulesetVersion == slot.FrequentRulesetVersion {
			resources["reel_strip_version"] = slot.FrequentReelStripVersion
			resources["paytable_version"] = slot.FrequentPaytableVersion
		}
	}
	if b.Game == "blackjack" {
		resources["shuffle_algorithm_version"] = "blackjack-fy-v1"
		if b.RulesetVersion == blackjack.FairRulesetVersion {
			resources["fair_return_version"] = blackjack.FairReturnVersion
		}
	}
	data, _ := json.Marshal(resources)
	return data
}

type runtimeConfig struct {
	Entry  CatalogEntry
	Config Config
	Policy Policy
}

func resolveRuntime(ctx context.Context, tx pgx.Tx, slug string, lock bool) (runtimeConfig, error) {
	var result runtimeConfig
	var maintenance bool
	var policyID string
	suffix := ""
	if lock {
		suffix = " FOR SHARE"
	}
	err := tx.QueryRow(ctx, `SELECT maintenance,active_wager_policy_version_id::text FROM games.runtime_gate WHERE singleton`+suffix).Scan(&maintenance, &policyID)
	if err != nil {
		return result, err
	}
	var publication, runtimeState string
	var active *string
	err = tx.QueryRow(ctx, `SELECT game_slug,title,implementation_key,publication_state,configured_runtime_state,active_config_version_id::text FROM games.game_registry WHERE game_slug=$1`+suffix, slug).Scan(&result.Entry.Slug, &result.Entry.Title, &result.Entry.Implementation, &publication, &runtimeState, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	result.Entry.State = "TEMPORARILY_UNAVAILABLE"
	if publication == "COMING_SOON" {
		result.Entry.State = "COMING_SOON"
		return result, nil
	}
	if publication == "RETIRED" {
		result.Entry.State = "RETIRED"
		return result, nil
	}
	if publication != "PUBLISHED" {
		return result, ErrNotFound
	}
	if maintenance || runtimeState == "MAINTENANCE" {
		result.Entry.State = "MAINTENANCE"
	}
	if runtimeState != "AVAILABLE" && runtimeState != "MAINTENANCE" {
		return result, nil
	}
	if result.Entry.Implementation != "direct."+slug+".v1" || active == nil {
		return result, nil
	}
	var activeStatus string
	if err = tx.QueryRow(ctx, `SELECT status FROM games.game_config_versions WHERE config_version_id=$1 AND game_slug=$2`+suffix, *active, slug).Scan(&activeStatus); err != nil {
		return result, err
	}
	if activeStatus != "ACTIVE" {
		result.Entry.State = "TEMPORARILY_UNAVAILABLE"
		return result, nil
	}
	c, err := configByID(ctx, tx, slug, *active)
	if errors.Is(err, ErrUnavailable) || errors.Is(err, ErrInvalidConfig) || errors.Is(err, ErrConfigMismatch) {
		result.Entry.State = "TEMPORARILY_UNAVAILABLE"
		return result, nil
	}
	if err != nil {
		return result, err
	}
	b := c.Binding()
	m := mathSummary(c)
	canonicalSummary, _ := json.Marshal(m)
	expectedValidator, expectedBuild := validationVersion, "direct-games-v1"
	if external := extraValidationSummary(c); external != "" {
		canonicalSummary = []byte(external)
		var metadata struct {
			Validator string `json:"validator_version"`
			Build     string `json:"validation_build"`
		}
		if json.Unmarshal(canonicalSummary, &metadata) != nil {
			return result, ErrInvalidConfig
		}
		expectedValidator, expectedBuild = metadata.Validator, metadata.Build
	}
	artifactHash := sha256.Sum256(canonicalSummary)
	var status, impl, ruleset, algorithm, validator, build string
	var configHash, proof []byte
	var raw []byte
	var resources []byte
	err = tx.QueryRow(ctx, `SELECT a.status,a.implementation_key,a.ruleset_version,a.algorithm_version,a.validator_version,a.validation_build,a.config_hash,a.result_summary,a.artifact_sha256,c.resource_versions FROM games.game_validation_artifacts a JOIN games.game_config_versions c ON c.config_version_id=a.config_version_id WHERE a.config_version_id=$1 AND a.game_slug=$2 AND a.verified_at IS NOT NULL`, *active, slug).Scan(&status, &impl, &ruleset, &algorithm, &validator, &build, &configHash, &raw, &proof, &resources)
	if errors.Is(err, pgx.ErrNoRows) {
		result.Entry.State = "TEMPORARILY_UNAVAILABLE"
		return result, nil
	}
	if err != nil {
		return result, err
	}
	var resMap, expectedMap map[string]string
	var actualSummary any
	if json.Unmarshal(raw, &actualSummary) != nil || json.Unmarshal(resources, &resMap) != nil || json.Unmarshal(expectedResources(c), &expectedMap) != nil {
		return result, ErrInvalidConfig
	}
	resActual, _ := json.Marshal(resMap)
	resExpected, _ := json.Marshal(expectedMap)
	actualCanonical, _ := json.Marshal(actualSummary)
	if status != "VERIFIED" || impl != result.Entry.Implementation || ruleset != b.RulesetVersion || algorithm != b.AlgorithmVersion || validator != expectedValidator || build != expectedBuild || !bytes.Equal(configHash, b.Hash[:]) || !bytes.Equal(actualCanonical, canonicalSummary) || !bytes.Equal(proof, artifactHash[:]) || !bytes.Equal(resActual, resExpected) {
		result.Entry.State = "TEMPORARILY_UNAVAILABLE"
		return result, nil
	}
	var quick []int64
	var hash []byte
	p := Policy{}
	err = tx.QueryRow(ctx, `SELECT wager_policy_version_id::text,version_number,minimum_wager_units,maximum_mode,input_step_units,quick_amount_units,policy_hash FROM games.wager_policy_versions WHERE wager_policy_version_id=$1`, policyID).Scan(&p.ID, &p.Version, &p.Minimum, &p.MaximumMode, &p.Step, &quick, &hash)
	if err != nil {
		return result, err
	}
	expectedPolicy := sha256.Sum256([]byte(policyCanonical))
	if p.Version != 1 || p.Minimum != 5000000 || p.Step != 500000 || p.MaximumMode != "NONE" || !bytes.Equal(hash, expectedPolicy[:]) {
		result.Entry.State = "TEMPORARILY_UNAVAILABLE"
		return result, nil
	}
	p.Hash = hex.EncodeToString(hash)
	p.Quick = []string{"5000000", "50000000", "250000000", "500000000"}
	result.Config = c
	result.Policy = p
	result.Entry.Config = summary(c)
	if !maintenance && runtimeState == "AVAILABLE" {
		result.Entry.State = "PLAY"
	}
	return result, nil
}

const policyCanonical = `{"input_step_units":500000,"maximum_mode":"NONE","minimum_wager_units":5000000,"quick_amount_units":[5000000,50000000,250000000,500000000],"version_number":1}`

type PokerCatalogRuntime func(context.Context) (string, error)

func pokerCatalogEntry(ctx context.Context, tx pgx.Tx, readPoker PokerCatalogRuntime) (CatalogEntry, error) {
	entry := CatalogEntry{State: "TEMPORARILY_UNAVAILABLE"}
	var publication, configured string
	err := tx.QueryRow(ctx, `SELECT game_slug,title,implementation_key,publication_state,configured_runtime_state FROM games.game_registry WHERE game_slug='texas-holdem'`).Scan(&entry.Slug, &entry.Title, &entry.Implementation, &publication, &configured)
	if err != nil {
		return entry, err
	}
	if publication == "COMING_SOON" || publication == "RETIRED" {
		entry.State = publication
		return entry, nil
	}
	if publication != "PUBLISHED" {
		return entry, ErrNotFound
	}
	if configured != "AVAILABLE" || entry.Implementation != "poker.texas-holdem.v1" || readPoker == nil {
		return entry, nil
	}
	// Public discovery only: Poker owns its readiness and maintenance scopes.
	state, err := readPoker(ctx)
	if err == nil && (state == "PLAY" || state == "MAINTENANCE") {
		entry.State = state
	}
	return entry, nil
}

func (s *Service) Catalog(ctx context.Context, readPoker PokerCatalogRuntime) ([]CatalogEntry, error) {
	items := []CatalogEntry{}
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT game_slug FROM games.game_registry WHERE publication_state<>'DRAFT' ORDER BY sort_order,game_slug`)
		if err != nil {
			return err
		}
		slugs := []string{}
		for rows.Next() {
			var slug string
			if err = rows.Scan(&slug); err != nil {
				rows.Close()
				return err
			}
			slugs = append(slugs, slug)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, slug := range slugs {
			if slug == "texas-holdem" {
				entry, err := pokerCatalogEntry(ctx, tx, readPoker)
				if err != nil {
					return err
				}
				items = append(items, entry)
				continue
			}
			result, err := resolveRuntime(ctx, tx, slug, false)
			if err != nil {
				return err
			}
			items = append(items, result.Entry)
		}
		return nil
	})
	return items, err
}
