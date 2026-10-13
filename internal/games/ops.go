package games

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/roulette"
	"github.com/jackc/pgx/v5"
)

var opsGameSlugs = map[string]string{
	"devil-roulette": "devil-roulette", "pressure-roulette": "pressure-roulette",
	"dice": "dice", "scratch": "scratch", "summon": "summon",
	"slot": "slot", "blackjack": "blackjack", "poker": "texas-holdem",
}

type OpsEditablePrize struct {
	Tier       string `json:"tier"`
	Multiplier string `json:"multiplier"`
	Weight     string `json:"weight"`
}

type OpsEditableConfig struct {
	Type              string             `json:"type"`
	PrizeTableVersion string             `json:"prize_table_version"`
	Prizes            []OpsEditablePrize `json:"prizes"`
}

type OpsConfigVersion struct {
	ConfigVersionID     string             `json:"config_version_id"`
	VersionNumber       int64              `json:"version_number,string"`
	Status              string             `json:"status"`
	ConfigSchemaVersion string             `json:"config_schema_version"`
	RulesetVersion      string             `json:"ruleset_version"`
	AlgorithmVersion    string             `json:"algorithm_version"`
	ConfigHash          string             `json:"config_hash"`
	CreatedAt           time.Time          `json:"created_at"`
	ValidatedAt         *time.Time         `json:"validated_at,omitempty"`
	PreviewedAt         *time.Time         `json:"previewed_at,omitempty"`
	ActivatedAt         *time.Time         `json:"activated_at,omitempty"`
	EditableConfig      *OpsEditableConfig `json:"editable_config,omitempty"`
}

type OpsValidationArtifact struct {
	ValidationArtifactID string          `json:"validation_artifact_id"`
	ConfigVersionID      string          `json:"config_version_id"`
	ArtifactType         string          `json:"artifact_type"`
	ImplementationKey    string          `json:"implementation_key"`
	RulesetVersion       string          `json:"ruleset_version"`
	AlgorithmVersion     string          `json:"algorithm_version"`
	ConfigHash           string          `json:"config_hash"`
	ValidatorVersion     string          `json:"validator_version"`
	ValidationBuild      string          `json:"validation_build"`
	ResultSummary        json.RawMessage `json:"result_summary"`
	ArtifactSHA256       string          `json:"artifact_sha256"`
	Status               string          `json:"status"`
	GeneratedAt          time.Time       `json:"generated_at"`
	VerifiedAt           *time.Time      `json:"verified_at,omitempty"`
}

type OpsGameSummary struct {
	GameSlug               string            `json:"game_slug"`
	Title                  string            `json:"title"`
	SortOrder              int64             `json:"sort_order,string"`
	Version                int64             `json:"version,string"`
	PublicationState       string            `json:"publication_state"`
	ConfiguredRuntimeState string            `json:"configured_runtime_state"`
	ImplementationKey      string            `json:"implementation_key"`
	ActiveConfig           *OpsConfigVersion `json:"active_config"`
	Rounds24h              int64             `json:"rounds_24h,string"`
	NeedsReviewRounds      int64             `json:"needs_review_rounds,string"`
}

type OpsOverview struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Games       []OpsGameSummary `json:"games"`
}

type OpsGameDetail struct {
	Page                int                     `json:"page"`
	PageSize            int                     `json:"page_size"`
	ConfigTotal         int                     `json:"config_total"`
	Roulette            *roulette.OpsStats      `json:"roulette,omitempty"`
	GeneratedAt         time.Time               `json:"generated_at"`
	Game                OpsGameSummary          `json:"game"`
	ConfigVersions      []OpsConfigVersion      `json:"config_versions"`
	ValidationArtifacts []OpsValidationArtifact `json:"validation_artifacts"`
	BaselineLocked      bool                    `json:"baseline_locked"`
}

type opsGameRow struct {
	view     OpsGameSummary
	dbSlug   string
	activeID *string
}

func opsAPISlug(dbSlug string) string {
	if dbSlug == "texas-holdem" {
		return "poker"
	}
	return dbSlug
}

func opsDBSlug(apiSlug string) (string, bool) {
	dbSlug, ok := opsGameSlugs[apiSlug]
	return dbSlug, ok
}

func directOpsDBSlug(apiSlug string) (string, bool) {
	switch apiSlug {
	case "dice", "scratch", "summon", "slot", "blackjack":
		return apiSlug, true
	default:
		return "", false
	}
}

func utcPointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}

func readOpsConfig(ctx context.Context, tx pgx.Tx, dbSlug, id string) (OpsConfigVersion, error) {
	var result OpsConfigVersion
	var canonical, storedHash []byte
	err := tx.QueryRow(ctx, `SELECT config_version_id::text,version_number,status,config_schema_version,
	 ruleset_version,algorithm_version,canonical_payload,config_hash,created_at,validated_at,previewed_at,activated_at
	 FROM games.game_config_versions WHERE game_slug=$1 AND config_version_id=$2`, dbSlug, id).Scan(
		&result.ConfigVersionID, &result.VersionNumber, &result.Status, &result.ConfigSchemaVersion,
		&result.RulesetVersion, &result.AlgorithmVersion, &canonical, &storedHash, &result.CreatedAt,
		&result.ValidatedAt, &result.PreviewedAt, &result.ActivatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	if roulette.IsGame(dbSlug) {
		c, e := roulette.ReadConfig(ctx, tx, dbSlug, id)
		if e != nil {
			return result, e
		}
		result.ConfigHash = hex.EncodeToString(c.Hash[:])
		return result, nil
	}
	config, err := loadConfigVersion(dbSlug, result.ConfigVersionID, canonical)
	if err != nil {
		return result, ErrInvalidConfig
	}
	binding := config.Binding()
	if binding.SchemaVersion != result.ConfigSchemaVersion || binding.RulesetVersion != result.RulesetVersion ||
		binding.AlgorithmVersion != result.AlgorithmVersion || !bytes.Equal(config.CanonicalJSON(), canonical) ||
		!bytes.Equal(binding.Hash[:], storedHash) {
		return result, ErrConfigMismatch
	}
	result.ConfigHash = hex.EncodeToString(storedHash)
	result.CreatedAt = result.CreatedAt.UTC()
	result.ValidatedAt = utcPointer(result.ValidatedAt)
	result.PreviewedAt = utcPointer(result.PreviewedAt)
	result.ActivatedAt = utcPointer(result.ActivatedAt)
	if dbSlug == "scratch" || dbSlug == "summon" {
		editable := editableOpsConfig(config)
		result.EditableConfig = &editable
	}
	return result, nil
}

func editableOpsConfig(config Config) OpsEditableConfig {
	binding := config.Binding()
	typeName := strings.ToUpper(binding.Game) + "_V1"
	result := OpsEditableConfig{Type: typeName, PrizeTableVersion: binding.PrizeTableVersion, Prizes: []OpsEditablePrize{}}
	for _, prize := range config.Prizes() {
		result.Prizes = append(result.Prizes, OpsEditablePrize{Tier: prize.Tier,
			Multiplier: strconv.FormatInt(prize.Multiplier, 10), Weight: strconv.FormatInt(prize.Weight, 10)})
	}
	return result
}

func readOpsActivity(ctx context.Context, tx pgx.Tx) (map[string][2]int64, error) {
	rows, err := tx.Query(ctx, `SELECT game_slug,rounds_24h,needs_review_rounds FROM games.ops_activity_read()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string][2]int64{}
	for rows.Next() {
		var slug string
		var recent, review int64
		if err = rows.Scan(&slug, &recent, &review); err != nil {
			return nil, err
		}
		if recent < 0 || review < 0 {
			return nil, ErrUnavailable
		}
		result[slug] = [2]int64{recent, review}
	}
	return result, rows.Err()
}

func readOpsGameRows(ctx context.Context, tx pgx.Tx, only string) ([]opsGameRow, error) {
	query := `SELECT game_slug,title,sort_order,version,publication_state,configured_runtime_state,
	 implementation_key,active_config_version_id::text FROM games.game_registry
	 WHERE game_slug IN('dice','scratch','summon','slot','blackjack','texas-holdem','devil-roulette','pressure-roulette')`
	args := []any{}
	if only != "" {
		query += ` AND game_slug=$1`
		args = append(args, only)
	}
	query += ` ORDER BY sort_order,game_slug`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	result := []opsGameRow{}
	for rows.Next() {
		var item opsGameRow
		if err = rows.Scan(&item.dbSlug, &item.view.Title, &item.view.SortOrder, &item.view.Version,
			&item.view.PublicationState, &item.view.ConfiguredRuntimeState, &item.view.ImplementationKey,
			&item.activeID); err != nil {
			rows.Close()
			return nil, err
		}
		item.view.GameSlug = opsAPISlug(item.dbSlug)
		result = append(result, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return result, nil
}

func completeOpsGameRows(ctx context.Context, tx pgx.Tx, rows []opsGameRow) ([]opsGameRow, error) {
	activity, err := readOpsActivity(ctx, tx)
	if err != nil {
		return nil, err
	}
	for index := range rows {
		counts := activity[rows[index].dbSlug]
		rows[index].view.Rounds24h, rows[index].view.NeedsReviewRounds = counts[0], counts[1]
		if rows[index].activeID != nil {
			config, loadErr := readOpsConfig(ctx, tx, rows[index].dbSlug, *rows[index].activeID)
			if loadErr != nil {
				return nil, loadErr
			}
			if config.Status != "ACTIVE" {
				return nil, ErrConfigMismatch
			}
			rows[index].view.ActiveConfig = &config
		}
	}
	return rows, nil
}

func (s *Service) ReadOps(ctx context.Context) (OpsOverview, error) {
	result := OpsOverview{Games: []OpsGameSummary{}}
	if s == nil || s.store == nil || ctx == nil {
		return result, ErrUnavailable
	}
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&result.GeneratedAt); err != nil {
			return err
		}
		rows, err := readOpsGameRows(ctx, tx, "")
		if err != nil {
			return err
		}
		rows, err = completeOpsGameRows(ctx, tx, rows)
		if err != nil {
			return err
		}
		for _, item := range rows {
			result.Games = append(result.Games, item.view)
		}
		return nil
	})
	result.GeneratedAt = result.GeneratedAt.UTC()
	return result, err
}

func readOpsValidationArtifacts(ctx context.Context, tx pgx.Tx, dbSlug string, ids []string) ([]OpsValidationArtifact, error) {
	rows, err := tx.Query(ctx, `SELECT validation_artifact_id::text,config_version_id::text,artifact_type,
	 implementation_key,ruleset_version,algorithm_version,config_hash,validator_version,validation_build,
	 result_summary,artifact_sha256,status,generated_at,verified_at
	 FROM games.game_validation_artifacts WHERE game_slug=$1 AND config_version_id=ANY($2::uuid[]) ORDER BY generated_at DESC,validation_artifact_id DESC`, dbSlug, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []OpsValidationArtifact{}
	for rows.Next() {
		var item OpsValidationArtifact
		var configHash, artifactHash, summary []byte
		if err = rows.Scan(&item.ValidationArtifactID, &item.ConfigVersionID, &item.ArtifactType,
			&item.ImplementationKey, &item.RulesetVersion, &item.AlgorithmVersion, &configHash,
			&item.ValidatorVersion, &item.ValidationBuild, &summary, &artifactHash, &item.Status,
			&item.GeneratedAt, &item.VerifiedAt); err != nil {
			return nil, err
		}
		if len(configHash) != sha256.Size || len(artifactHash) != sha256.Size || !json.Valid(summary) {
			return nil, ErrInvalidConfig
		}
		item.ConfigHash, item.ArtifactSHA256 = hex.EncodeToString(configHash), hex.EncodeToString(artifactHash)
		item.ResultSummary = append(json.RawMessage{}, summary...)
		item.GeneratedAt = item.GeneratedAt.UTC()
		item.VerifiedAt = utcPointer(item.VerifiedAt)
		result = append(result, item)
	}
	return result, rows.Err()
}

type OpsConfigFilter struct {
	Page     int
	Status   string
	From, To *time.Time
}

func (s *Service) ReadOpsGame(ctx context.Context, slug string, filter OpsConfigFilter) (OpsGameDetail, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	result := OpsGameDetail{Page: filter.Page, PageSize: 20, ConfigVersions: []OpsConfigVersion{}, ValidationArtifacts: []OpsValidationArtifact{}, BaselineLocked: true}
	dbSlug, ok := opsDBSlug(slug)
	if !ok {
		return result, ErrNotFound
	}
	if s == nil || s.store == nil || ctx == nil {
		return result, ErrUnavailable
	}
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&result.GeneratedAt); err != nil {
			return err
		}
		rows, err := readOpsGameRows(ctx, tx, dbSlug)
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			return ErrNotFound
		}
		rows, err = completeOpsGameRows(ctx, tx, rows)
		if err != nil {
			return err
		}
		result.Game = rows[0].view
		const filterSQL = ` FROM games.game_config_versions WHERE game_slug=$1 AND ($2='' OR status=$2)
         AND ($3::timestamptz IS NULL OR created_at>=$3) AND ($4::timestamptz IS NULL OR created_at<$4)`
		args := []any{dbSlug, filter.Status, filter.From, filter.To}
		if err = tx.QueryRow(ctx, `SELECT count(*)`+filterSQL, args...).Scan(&result.ConfigTotal); err != nil {
			return err
		}
		idRows, err := tx.Query(ctx, `SELECT config_version_id::text`+filterSQL+` ORDER BY version_number DESC LIMIT 20 OFFSET $5`, append(args, (filter.Page-1)*20)...)

		if err != nil {
			return err
		}
		ids := []string{}
		for idRows.Next() {
			var id string
			if err = idRows.Scan(&id); err != nil {
				idRows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = idRows.Err()
		idRows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			config, loadErr := readOpsConfig(ctx, tx, dbSlug, id)
			if loadErr != nil {
				return loadErr
			}
			result.ConfigVersions = append(result.ConfigVersions, config)
		}
		if roulette.IsGame(dbSlug) {
			stats, e := roulette.OpsStatsInTx(ctx, tx, dbSlug)
			if e != nil {
				return e
			}
			result.Roulette = &stats
		}
		result.ValidationArtifacts, err = readOpsValidationArtifacts(ctx, tx, dbSlug, ids)
		return err
	})
	result.GeneratedAt = result.GeneratedAt.UTC()
	return result, err
}

func validGameTitle(value string) bool {
	if !utf8.ValidString(value) || value == "" || utf8.RuneCountInString(value) > 100 {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func canonicalUnsigned(value string, maximum int64) (int64, bool) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil && parsed >= 0 && parsed <= maximum && strconv.FormatInt(parsed, 10) == value
}

func normalizeEditableConfig(slug string, input OpsEditableConfig) (OpsEditableConfig, []Prize, error) {
	dbSlug, ok := directOpsDBSlug(slug)
	if !ok || dbSlug != "scratch" && dbSlug != "summon" || !validVersion(input.PrizeTableVersion) {
		return input, nil, platform.ErrOpsInvalid
	}
	expectedType, expected := "SCRATCH_V1", scratchPrizesV1()
	if dbSlug == "summon" {
		expectedType, expected = "SUMMON_V1", summonPrizesV1()
	}
	if input.Type != expectedType || len(input.Prizes) != len(expected) {
		return input, nil, platform.ErrOpsInvalid
	}
	seen := map[string]bool{}
	prizes := make([]Prize, 0, len(input.Prizes))
	for _, item := range input.Prizes {
		multiplier, multiplierOK := canonicalUnsigned(item.Multiplier, math.MaxInt64)
		weight, weightOK := canonicalUnsigned(item.Weight, prizeWeightTotal)
		if !multiplierOK || !weightOK || seen[item.Tier] {
			return input, nil, platform.ErrOpsInvalid
		}
		matched := false
		for _, baseline := range expected {
			if item.Tier == baseline.Tier && multiplier == baseline.Multiplier {
				matched = true
				break
			}
		}
		if !matched {
			return input, nil, platform.ErrOpsInvalid
		}
		seen[item.Tier] = true
		prizes = append(prizes, Prize{Tier: item.Tier, Multiplier: multiplier, Weight: weight})
	}
	return input, prizes, nil
}

func opsStringIn(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

type opsRegistryState struct {
	APISlug                string  `json:"game_slug"`
	DBSlug                 string  `json:"-"`
	Title                  string  `json:"title"`
	SortOrder              string  `json:"sort_order"`
	Version                string  `json:"version"`
	PublicationState       string  `json:"publication_state"`
	ConfiguredRuntimeState string  `json:"configured_runtime_state"`
	ImplementationKey      string  `json:"implementation_key"`
	ActiveConfigVersionID  *string `json:"active_config_version_id"`
}

func loadOpsRegistryState(ctx context.Context, tx pgx.Tx, apiSlug string, lock bool) (opsRegistryState, error) {
	var result opsRegistryState
	dbSlug, ok := opsDBSlug(apiSlug)
	if !ok {
		return result, platform.ErrOpsInvalid
	}
	query := `SELECT game_slug,title,sort_order::text,version::text,publication_state,
	 configured_runtime_state,implementation_key,active_config_version_id::text
	 FROM games.game_registry WHERE game_slug=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	err := tx.QueryRow(ctx, query, dbSlug).Scan(&result.DBSlug, &result.Title, &result.SortOrder,
		&result.Version, &result.PublicationState, &result.ConfiguredRuntimeState,
		&result.ImplementationKey, &result.ActiveConfigVersionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, platform.ErrOpsNotFound
	}
	if err != nil {
		return result, err
	}
	result.APISlug = opsAPISlug(result.DBSlug)
	if result.APISlug != apiSlug {
		return result, platform.ErrOpsInvalid
	}
	return result, nil
}

type opsConfigRecord struct {
	ID           string
	DBSlug       string
	APISlug      string
	Version      int64
	Status       string
	Schema       string
	Ruleset      string
	Algorithm    string
	Canonical    []byte
	Hash         []byte
	Resources    []byte
	CreatedAt    time.Time
	ValidatedAt  *time.Time
	PreviewedAt  *time.Time
	ActivatedAt  *time.Time
	SupersededAt *time.Time
	Config       Config
	Registry     opsRegistryState
}

type opsConfigState struct {
	EditableConfig      *OpsEditableConfig `json:"editable_config,omitempty"`
	ConfigVersionID     string             `json:"config_version_id"`
	GameSlug            string             `json:"game_slug"`
	VersionNumber       string             `json:"version_number"`
	Status              string             `json:"status"`
	ConfigSchemaVersion string             `json:"config_schema_version"`
	RulesetVersion      string             `json:"ruleset_version"`
	AlgorithmVersion    string             `json:"algorithm_version"`
	ConfigHash          string             `json:"config_hash"`
	Active              bool               `json:"active"`
	RegistryVersion     string             `json:"registry_version"`
	ActiveConfigID      *string            `json:"active_config_version_id"`
}

func (record opsConfigRecord) state() opsConfigState {
	var editable *OpsEditableConfig
	if record.DBSlug == "scratch" || record.DBSlug == "summon" {
		value := editableOpsConfig(record.Config)
		editable = &value
	}
	return opsConfigState{EditableConfig: editable, ConfigVersionID: record.ID, GameSlug: record.APISlug,
		VersionNumber: strconv.FormatInt(record.Version, 10), Status: record.Status,
		ConfigSchemaVersion: record.Schema, RulesetVersion: record.Ruleset,
		AlgorithmVersion: record.Algorithm, ConfigHash: hex.EncodeToString(record.Hash),
		Active:          record.Registry.ActiveConfigVersionID != nil && *record.Registry.ActiveConfigVersionID == record.ID,
		RegistryVersion: record.Registry.Version, ActiveConfigID: record.Registry.ActiveConfigVersionID}
}

func loadOpsConfigRecord(ctx context.Context, tx pgx.Tx, apiSlug, id string, lock bool) (opsConfigRecord, error) {
	var result opsConfigRecord
	dbSlug, ok := opsDBSlug(apiSlug)
	if !ok || dbSlug == "texas-holdem" {
		return result, platform.ErrOpsInvalid
	}
	registry, err := loadOpsRegistryState(ctx, tx, apiSlug, lock)
	if err != nil {
		return result, err
	}
	query := `SELECT config_version_id::text,game_slug,version_number,status,config_schema_version,
	 ruleset_version,algorithm_version,canonical_payload,config_hash,resource_versions,
	 created_at,validated_at,previewed_at,activated_at,superseded_at
	 FROM games.game_config_versions WHERE game_slug=$1 AND config_version_id=$2`
	if lock {
		query += ` FOR UPDATE`
	}
	err = tx.QueryRow(ctx, query, dbSlug, id).Scan(&result.ID, &result.DBSlug, &result.Version,
		&result.Status, &result.Schema, &result.Ruleset, &result.Algorithm, &result.Canonical,
		&result.Hash, &result.Resources, &result.CreatedAt, &result.ValidatedAt,
		&result.PreviewedAt, &result.ActivatedAt, &result.SupersededAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, platform.ErrOpsNotFound
	}
	if err != nil {
		return result, err
	}
	result.APISlug, result.Registry = apiSlug, registry
	if roulette.IsGame(dbSlug) {
		_, err = roulette.ReadConfig(ctx, tx, dbSlug, id)
		return result, err
	}
	result.Config, err = loadConfigVersion(dbSlug, result.ID, result.Canonical)
	if err != nil {
		return result, platform.ErrOpsConflict
	}
	binding := result.Config.Binding()
	if binding.Game != dbSlug || binding.Version != result.ID || binding.SchemaVersion != result.Schema ||
		binding.RulesetVersion != result.Ruleset || binding.AlgorithmVersion != result.Algorithm ||
		!bytes.Equal(result.Config.CanonicalJSON(), result.Canonical) || !bytes.Equal(binding.Hash[:], result.Hash) {
		return result, platform.ErrOpsConflict
	}
	expectedResourcesRaw := expectedResources(result.Config)
	actualResources, canonicalErr := canonicalJSONObject(result.Resources)
	if canonicalErr != nil || !bytes.Equal(actualResources, expectedResourcesRaw) {
		return result, platform.ErrOpsConflict
	}
	return result, nil
}

func canonicalJSONObject(raw []byte) ([]byte, error) {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return nil, ErrInvalidConfig
	}
	return json.Marshal(value)
}

func gameOpsRaw(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

type opsValidationEvidence struct {
	ArtifactType     string
	ValidatorVersion string
	ValidationBuild  string
	Summary          json.RawMessage
	ArtifactHash     [sha256.Size]byte
}

func validationEvidence(config Config) (opsValidationEvidence, error) {
	evidence := opsValidationEvidence{ArtifactType: "EXACT_MATH", ValidatorVersion: validationVersion,
		ValidationBuild: "direct-games-v1"}
	if external := extraValidationSummary(config); external != "" {
		evidence.Summary = json.RawMessage(external)
		var metadata struct {
			ArtifactType     string `json:"artifact_type"`
			ValidatorVersion string `json:"validator_version"`
			ValidationBuild  string `json:"validation_build"`
		}
		if json.Unmarshal(evidence.Summary, &metadata) != nil || metadata.ArtifactType == "" ||
			metadata.ValidatorVersion == "" || metadata.ValidationBuild == "" {
			return evidence, ErrInvalidConfig
		}
		evidence.ArtifactType, evidence.ValidatorVersion, evidence.ValidationBuild =
			metadata.ArtifactType, metadata.ValidatorVersion, metadata.ValidationBuild
	} else {
		statistics, err := config.Statistics()
		if err != nil || statistics.RTP == nil {
			return evidence, ErrInvalidConfig
		}
		evidence.Summary = gameOpsRaw(mathSummary(config))
	}
	var decoded any
	if json.Unmarshal(evidence.Summary, &decoded) != nil {
		return evidence, ErrInvalidConfig
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return evidence, err
	}
	evidence.Summary = canonical
	evidence.ArtifactHash = sha256.Sum256(canonical)
	return evidence, nil
}

func verifyOpsValidationArtifact(ctx context.Context, tx pgx.Tx, record opsConfigRecord) error {
	if roulette.IsGame(record.DBSlug) {
		_, err := roulette.ReadConfig(ctx, tx, record.DBSlug, record.ID)
		return err
	}
	expected, err := validationEvidence(record.Config)
	if err != nil {
		return platform.ErrOpsConflict
	}
	var artifactType, implementation, ruleset, algorithm, validator, build, status string
	var configHash, summary, artifactHash []byte
	var verifiedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT artifact_type,implementation_key,ruleset_version,algorithm_version,
	 config_hash,validator_version,validation_build,result_summary,artifact_sha256,status,verified_at
	 FROM games.game_validation_artifacts WHERE config_version_id=$1`, record.ID).Scan(
		&artifactType, &implementation, &ruleset, &algorithm, &configHash, &validator, &build,
		&summary, &artifactHash, &status, &verifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.ErrOpsConflict
	}
	if err != nil {
		return err
	}
	var decoded any
	if json.Unmarshal(summary, &decoded) != nil {
		return platform.ErrOpsConflict
	}
	canonicalSummary, err := json.Marshal(decoded)
	if err != nil {
		return err
	}
	if artifactType != expected.ArtifactType || implementation != record.Registry.ImplementationKey ||
		ruleset != record.Ruleset || algorithm != record.Algorithm ||
		validator != expected.ValidatorVersion || build != expected.ValidationBuild ||
		status != "VERIFIED" || verifiedAt == nil || !bytes.Equal(configHash, record.Hash) ||
		!bytes.Equal(canonicalSummary, expected.Summary) || !bytes.Equal(artifactHash, expected.ArtifactHash[:]) {
		return platform.ErrOpsConflict
	}
	return nil
}

func configFromEditable(targetID, slug string, editable OpsEditableConfig) (Config, error) {
	_, prizes, err := normalizeEditableConfig(slug, editable)
	if err != nil {
		return Config{}, err
	}
	if slug == "scratch" {
		config, buildErr := NewScratchConfig(targetID, editable.PrizeTableVersion, prizes)
		if buildErr != nil {
			return Config{}, platform.ErrOpsInvalid
		}
		return config, nil
	}
	config, buildErr := NewSummonConfig(targetID, editable.PrizeTableVersion, prizes)
	if buildErr != nil {
		return Config{}, platform.ErrOpsInvalid
	}
	return config, nil
}

func nextOpsConfigVersion(ctx context.Context, tx pgx.Tx, dbSlug string) (int64, error) {
	var maximum int64
	if err := tx.QueryRow(ctx, `SELECT coalesce(max(version_number),0) FROM games.game_config_versions
	 WHERE game_slug=$1`, dbSlug).Scan(&maximum); err != nil {
		return 0, err
	}
	if maximum < 0 || maximum == math.MaxInt64 {
		return 0, platform.ErrOpsConflict
	}
	return maximum + 1, nil
}

// Registry versions and immutable config history remain business data; clients
// no longer carry approval receipts or expected versions.
type OpsGameInput struct {
	Title                  string `json:"title"`
	SortOrder              string `json:"sort_order"`
	PublicationState       string `json:"publication_state"`
	ConfiguredRuntimeState string `json:"configured_runtime_state"`
}

func (s *Service) SaveOpsGame(ctx context.Context, actor int64, slug string, input OpsGameInput) (opsRegistryState, error) {
	var result opsRegistryState
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		result, err = s.saveOpsGame(ctx, tx, actor, slug, input)
		return err
	})
	return result, err
}
func (s *Service) saveOpsGame(ctx context.Context, tx pgx.Tx, actor int64, slug string, input OpsGameInput) (opsRegistryState, error) {
	var result opsRegistryState
	if err := platform.RequireLocalOpsAdmin(ctx, tx, actor); err != nil {
		return result, err
	}
	input.Title = strings.TrimSpace(input.Title)
	order, ok := canonicalUnsigned(input.SortOrder, 999999999)
	if !validGameTitle(input.Title) || !ok || !opsStringIn(input.PublicationState, "DRAFT", "PUBLISHED", "COMING_SOON", "RETIRED") || !opsStringIn(input.ConfiguredRuntimeState, "AVAILABLE", "MAINTENANCE", "UNAVAILABLE") {
		return result, platform.ErrOpsInvalid
	}
	before, err := loadOpsRegistryState(ctx, tx, slug, true)
	if err != nil {
		return result, err
	}
	_, err = tx.Exec(ctx, `UPDATE games.game_registry SET title=$2,sort_order=$3,publication_state=$4,
 configured_runtime_state=$5,version=version+1,updated_at=clock_timestamp() WHERE game_slug=$1`, before.DBSlug, input.Title, order, input.PublicationState, input.ConfiguredRuntimeState)
	if err != nil {
		return result, err
	}
	result, err = loadOpsRegistryState(ctx, tx, slug, false)
	if err != nil {
		return result, err
	}
	return result, platform.RecordOpsChange(ctx, tx, actor, "GAME_SETTINGS_SAVE", "game", slug, before, result)
}

func (s *Service) ChangeOpsConfig(ctx context.Context, actor int64, slug, id, action string, input OpsEditableConfig) (OpsConfigVersion, error) {
	var result OpsConfigVersion
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		result, err = s.changeOpsConfig(ctx, tx, actor, slug, id, action, input)
		return err
	})
	return result, err
}
func (s *Service) changeOpsConfig(ctx context.Context, tx pgx.Tx, actor int64, slug, id, action string, input OpsEditableConfig) (OpsConfigVersion, error) {
	var result OpsConfigVersion
	if err := platform.RequireLocalOpsAdmin(ctx, tx, actor); err != nil {
		return result, err
	}
	if _, err := uuidBytes(id); err != nil {
		return result, platform.ErrOpsInvalid
	}
	if roulette.IsGame(slug) && action != "activate" {
		return result, platform.ErrOpsInvalid
	}
	record, err := loadOpsConfigRecord(ctx, tx, slug, id, true)
	if err != nil {
		return result, err
	}
	target := id
	before := record.state()
	switch action {
	case "clone":
		target, err = newUUID()
		if err != nil {
			return result, err
		}
		version, e := nextOpsConfigVersion(ctx, tx, record.DBSlug)
		if e != nil {
			return result, e
		}
		config, e := loadConfigVersion(record.DBSlug, target, record.Canonical)
		if e != nil {
			return result, e
		}
		b := config.Binding()
		canonical := config.CanonicalJSON()
		_, err = tx.Exec(ctx, `INSERT INTO games.game_config_versions(config_version_id,game_slug,parent_version_id,version_number,status,created_by,
   config_schema_version,ruleset_version,algorithm_version,config_payload,canonical_payload,config_hash,resource_versions)
   VALUES($1,$2,$3,$4,'DRAFT',$5,$6,$7,$8,$9::jsonb,$10,$11,$12::jsonb)`, target, record.DBSlug, record.ID, version, actor, b.SchemaVersion, b.RulesetVersion, b.AlgorithmVersion, json.RawMessage(canonical), canonical, b.Hash[:], json.RawMessage(expectedResources(config)))
	case "save":
		if record.Status != "DRAFT" {
			return result, platform.ErrOpsConflict
		}
		config, e := configFromEditable(id, slug, input)
		if e != nil {
			return result, e
		}
		b := config.Binding()
		canonical := config.CanonicalJSON()
		_, err = tx.Exec(ctx, `UPDATE games.game_config_versions SET config_schema_version=$2,ruleset_version=$3,algorithm_version=$4,
   config_payload=$5::jsonb,canonical_payload=$6,config_hash=$7,resource_versions=$8::jsonb WHERE config_version_id=$1`, id, b.SchemaVersion, b.RulesetVersion, b.AlgorithmVersion, json.RawMessage(canonical), canonical, b.Hash[:], json.RawMessage(expectedResources(config)))
	case "validate":
		if record.Status != "DRAFT" {
			return result, platform.ErrOpsConflict
		}
		evidence, e := validationEvidence(record.Config)
		if e != nil {
			return result, e
		}
		artifact, e := newUUID()
		if e != nil {
			return result, e
		}
		_, err = tx.Exec(ctx, `INSERT INTO games.game_validation_artifacts(validation_artifact_id,game_slug,artifact_type,implementation_key,ruleset_version,
   algorithm_version,config_version_id,config_hash,validator_version,validation_build,result_summary,artifact_sha256,status,verified_at)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,'VERIFIED',clock_timestamp())`, artifact, record.DBSlug, evidence.ArtifactType, record.Registry.ImplementationKey, record.Ruleset, record.Algorithm, id, record.Hash, evidence.ValidatorVersion, evidence.ValidationBuild, evidence.Summary, evidence.ArtifactHash[:])
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE games.game_config_versions SET status='VALIDATED',validated_at=clock_timestamp() WHERE config_version_id=$1`, id)
		}
	case "activate":
		expectedImpl := "direct." + record.DBSlug + ".v1"
		if roulette.IsGame(slug) {
			expectedImpl = "roulette." + strings.TrimSuffix(slug, "-roulette") + ".v1"
		}
		if !opsStringIn(record.Status, "VALIDATED", "PREVIEWED") || record.Registry.ImplementationKey != expectedImpl {
			return result, platform.ErrOpsConflict
		}
		if err = verifyOpsValidationArtifact(ctx, tx, record); err != nil {
			return result, err
		}
		// Keep the existing database lifecycle; preview is folded into the direct activation.
		if err = activateOpsConfig(ctx, tx, record.DBSlug, id); err != nil {
			return result, err
		}
	default:
		return result, platform.ErrOpsInvalid
	}
	if err != nil {
		return result, err
	}
	result, err = readOpsConfig(ctx, tx, record.DBSlug, target)
	if err != nil {
		return result, err
	}
	return result, platform.RecordOpsChange(ctx, tx, actor, "GAME_CONFIG_"+strings.ToUpper(action), "game_config", target, before, result)
}
func activateOpsConfig(ctx context.Context, tx pgx.Tx, slug, id string) error {
	if _, err := tx.Exec(ctx, `UPDATE games.game_config_versions SET status='PREVIEWED',previewed_at=clock_timestamp() WHERE config_version_id=$1 AND status='VALIDATED'`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE games.game_config_versions SET status='SUPERSEDED',superseded_at=clock_timestamp() WHERE game_slug=$1 AND status='ACTIVE'`, slug); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE games.game_config_versions SET status='ACTIVE',activated_at=clock_timestamp() WHERE config_version_id=$1`, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE games.game_registry SET active_config_version_id=$2,version=version+1,updated_at=clock_timestamp() WHERE game_slug=$1`, slug, id)
	return err
}
func (s *Service) VoidOpsRound(ctx context.Context, actor int64, slug, id string) error {
	return s.store.WithTx(ctx, func(tx pgx.Tx) error { return roulette.SystemVoidInTx(ctx, tx, actor, slug, id) })
}
