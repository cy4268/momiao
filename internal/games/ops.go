package games

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

var opsGameSlugs = map[string]string{
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
	ConfigVersionID    string             `json:"config_version_id"`
	VersionNumber      int64              `json:"version_number,string"`
	Status             string             `json:"status"`
	ConfigSchemaVersion string            `json:"config_schema_version"`
	RulesetVersion     string             `json:"ruleset_version"`
	AlgorithmVersion   string             `json:"algorithm_version"`
	ConfigHash         string             `json:"config_hash"`
	CreatedAt          time.Time          `json:"created_at"`
	ValidatedAt        *time.Time         `json:"validated_at,omitempty"`
	PreviewedAt        *time.Time         `json:"previewed_at,omitempty"`
	ActivatedAt        *time.Time         `json:"activated_at,omitempty"`
	EditableConfig     *OpsEditableConfig `json:"editable_config,omitempty"`
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
	GameSlug              string            `json:"game_slug"`
	Title                 string            `json:"title"`
	SortOrder             int64             `json:"sort_order,string"`
	Version               int64             `json:"version,string"`
	PublicationState      string            `json:"publication_state"`
	ConfiguredRuntimeState string           `json:"configured_runtime_state"`
	ImplementationKey     string            `json:"implementation_key"`
	ActiveConfig          *OpsConfigVersion `json:"active_config"`
	Rounds24h             int64             `json:"rounds_24h,string"`
	NeedsReviewRounds     int64             `json:"needs_review_rounds,string"`
}

type OpsOverview struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Games       []OpsGameSummary `json:"games"`
}

type OpsGameDetail struct {
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
	dbSlug, ok := opsDBSlug(apiSlug)
	return dbSlug, ok && dbSlug != "texas-holdem"
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
	if result.Status == "DRAFT" && (dbSlug == "scratch" || dbSlug == "summon") {
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
	 WHERE game_slug IN('dice','scratch','summon','slot','blackjack','texas-holdem')`
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

func readOpsValidationArtifacts(ctx context.Context, tx pgx.Tx, dbSlug string) ([]OpsValidationArtifact, error) {
	rows, err := tx.Query(ctx, `SELECT validation_artifact_id::text,config_version_id::text,artifact_type,
	 implementation_key,ruleset_version,algorithm_version,config_hash,validator_version,validation_build,
	 result_summary,artifact_sha256,status,generated_at,verified_at
	 FROM games.game_validation_artifacts WHERE game_slug=$1 ORDER BY generated_at DESC,validation_artifact_id DESC LIMIT 100`, dbSlug)
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

func (s *Service) ReadOpsGame(ctx context.Context, slug string) (OpsGameDetail, error) {
	result := OpsGameDetail{ConfigVersions: []OpsConfigVersion{}, ValidationArtifacts: []OpsValidationArtifact{}, BaselineLocked: true}
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
		idRows, err := tx.Query(ctx, `SELECT config_version_id::text FROM games.game_config_versions
		 WHERE game_slug=$1 ORDER BY version_number DESC LIMIT 100`, dbSlug)
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
		result.ValidationArtifacts, err = readOpsValidationArtifacts(ctx, tx, dbSlug)
		return err
	})
	result.GeneratedAt = result.GeneratedAt.UTC()
	return result, err
}

const (
	gameMetadataUpdate   = "GAME_METADATA_UPDATE"
	gamePublicationSet   = "GAME_PUBLICATION_SET"
	gameRuntimeSet       = "GAME_RUNTIME_SET"
	gameConfigClone      = "GAME_CONFIG_CLONE_DRAFT"
	gameConfigSave       = "GAME_CONFIG_DRAFT_SAVE"
	gameConfigValidate   = "GAME_CONFIG_VALIDATE"
	gameConfigPreview    = "GAME_CONFIG_PREVIEW"
	gameConfigActivate   = "GAME_CONFIG_ACTIVATE"
)

type gameOpsHandler struct {
	service *Service
	kind    string
}

func gameBinding(service *Service, descriptor platform.OpsOperationDescriptor, kind string) platform.OpsOperationBinding {
	var handler platform.OpsOperationHandler
	if service != nil && service.store != nil {
		handler = gameOpsHandler{service: service, kind: kind}
	}
	return platform.OpsOperationBinding{Descriptor: descriptor, Handler: handler}
}

func gameDescriptor(operation string, risk platform.OpsRisk, permission, target, input string,
	roles []string) platform.OpsOperationDescriptor {
	result := platform.OpsOperationDescriptor{OperationType: operation, Risk: risk,
		RequiredPermission: permission, AllowedRoles: roles, TargetType: target,
		InputSchemaVersion: input, ImpactSchemaVersion: "games-impact.v1",
		ExecutionMode: platform.OpsSameDatabase}
	switch risk {
	case platform.OpsRiskRoutine:
		result.ConfirmationMode = platform.OpsConfirmNone
	case platform.OpsRiskImpactful:
		result.ConfirmationMode = platform.OpsConfirmExplicit
	case platform.OpsRiskCritical:
		result.ConfirmationMode = platform.OpsConfirmTyped
		result.RequiresReason, result.RequiresFreshAuth = true, true
	}
	return result
}

func OpsBindings(service *Service) []platform.OpsOperationBinding {
	operator := []string{"SUPER_ADMIN", "OPERATOR"}
	return []platform.OpsOperationBinding{
		gameBinding(service, gameDescriptor(gameMetadataUpdate, platform.OpsRiskRoutine, "games.metadata.write", "game", "game-metadata.v1", operator), gameMetadataUpdate),
		gameBinding(service, gameDescriptor(gamePublicationSet, platform.OpsRiskImpactful, "games.metadata.write", "game", "game-publication.v1", operator), gamePublicationSet),
		gameBinding(service, gameDescriptor(gameRuntimeSet, platform.OpsRiskImpactful, "games.runtime.write", "game", "game-runtime.v1", operator), gameRuntimeSet),
		gameBinding(service, gameDescriptor(gameConfigClone, platform.OpsRiskRoutine, "games.config.draft", "game_config", "game-config-reference.v1", operator), gameConfigClone),
		gameBinding(service, gameDescriptor(gameConfigSave, platform.OpsRiskRoutine, "games.config.draft", "game_config", "game-config-prizes.v1", operator), gameConfigSave),
		gameBinding(service, gameDescriptor(gameConfigValidate, platform.OpsRiskRoutine, "games.config.validate", "game_config", "game-config-reference.v1", operator), gameConfigValidate),
		gameBinding(service, gameDescriptor(gameConfigPreview, platform.OpsRiskRoutine, "games.config.validate", "game_config", "game-config-reference.v1", operator), gameConfigPreview),
		gameBinding(service, gameDescriptor(gameConfigActivate, platform.OpsRiskCritical, "games.config.activate", "game_config", "game-config-reference.v1", []string{"SUPER_ADMIN"}), gameConfigActivate),
	}
}

type gameMetadataInput struct {
	Title     string `json:"title"`
	SortOrder string `json:"sort_order"`
}

type gamePublicationInput struct {
	PublicationState string `json:"publication_state"`
}

type gameRuntimeInput struct {
	ConfiguredRuntimeState string `json:"configured_runtime_state"`
}

type gameConfigReferenceInput struct {
	GameSlug string `json:"game_slug"`
}

type gameConfigSaveInput struct {
	GameSlug string            `json:"game_slug"`
	Config   OpsEditableConfig `json:"config"`
}

func decodeGameOpsInput(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return platform.ErrOpsInvalid
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return platform.ErrOpsInvalid
	}
	return nil
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

func (h gameOpsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var value any
	switch h.kind {
	case gameMetadataUpdate:
		var input gameMetadataInput
		if err := decodeGameOpsInput(raw, &input); err != nil {
			return nil, err
		}
		input.Title = strings.TrimSpace(input.Title)
		if !validGameTitle(input.Title) {
			return nil, platform.ErrOpsInvalid
		}
		if _, ok := canonicalUnsigned(input.SortOrder, 999999999); !ok {
			return nil, platform.ErrOpsInvalid
		}
		value = input
	case gamePublicationSet:
		var input gamePublicationInput
		if err := decodeGameOpsInput(raw, &input); err != nil {
			return nil, err
		}
		if !opsStringIn(input.PublicationState, "DRAFT", "PUBLISHED", "COMING_SOON", "RETIRED") {
			return nil, platform.ErrOpsInvalid
		}
		value = input
	case gameRuntimeSet:
		var input gameRuntimeInput
		if err := decodeGameOpsInput(raw, &input); err != nil {
			return nil, err
		}
		if !opsStringIn(input.ConfiguredRuntimeState, "AVAILABLE", "MAINTENANCE", "UNAVAILABLE") {
			return nil, platform.ErrOpsInvalid
		}
		value = input
	case gameConfigClone, gameConfigValidate, gameConfigPreview, gameConfigActivate:
		var input gameConfigReferenceInput
		if err := decodeGameOpsInput(raw, &input); err != nil {
			return nil, err
		}
		if _, ok := directOpsDBSlug(input.GameSlug); !ok {
			return nil, platform.ErrOpsInvalid
		}
		value = input
	case gameConfigSave:
		var input gameConfigSaveInput
		if err := decodeGameOpsInput(raw, &input); err != nil {
			return nil, err
		}
		if _, _, err := normalizeEditableConfig(input.GameSlug, input.Config); err != nil {
			return nil, err
		}
		value = input
	default:
		return nil, platform.ErrOpsInvalid
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return canonical, nil
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
	ID                  string
	DBSlug              string
	APISlug             string
	Version             int64
	Status              string
	Schema              string
	Ruleset             string
	Algorithm           string
	Canonical           []byte
	Hash                []byte
	Resources           []byte
	CreatedAt           time.Time
	ValidatedAt         *time.Time
	PreviewedAt         *time.Time
	ActivatedAt         *time.Time
	SupersededAt        *time.Time
	Config              Config
	Registry            opsRegistryState
}

type opsConfigState struct {
	ConfigVersionID   string  `json:"config_version_id"`
	GameSlug           string  `json:"game_slug"`
	VersionNumber     string  `json:"version_number"`
	Status             string  `json:"status"`
	ConfigSchemaVersion string `json:"config_schema_version"`
	RulesetVersion    string  `json:"ruleset_version"`
	AlgorithmVersion  string  `json:"algorithm_version"`
	ConfigHash         string  `json:"config_hash"`
	Active             bool    `json:"active"`
	RegistryVersion    string  `json:"registry_version"`
	ActiveConfigID     *string `json:"active_config_version_id"`
}

func (record opsConfigRecord) state() opsConfigState {
	return opsConfigState{ConfigVersionID: record.ID, GameSlug: record.APISlug,
		VersionNumber: strconv.FormatInt(record.Version, 10), Status: record.Status,
		ConfigSchemaVersion: record.Schema, RulesetVersion: record.Ruleset,
		AlgorithmVersion: record.Algorithm, ConfigHash: hex.EncodeToString(record.Hash),
		Active: record.Registry.ActiveConfigVersionID != nil && *record.Registry.ActiveConfigVersionID == record.ID,
		RegistryVersion: record.Registry.Version, ActiveConfigID: record.Registry.ActiveConfigVersionID}
}

func loadOpsConfigRecord(ctx context.Context, tx pgx.Tx, apiSlug, id string, lock bool) (opsConfigRecord, error) {
	var result opsConfigRecord
	dbSlug, ok := directOpsDBSlug(apiSlug)
	if !ok {
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

func gameOpsAffectedOne() *int64 {
	value := int64(1)
	return &value
}

func gameOpsImpact(current, proposed any, related, continuing []string) platform.OpsImpact {
	return platform.OpsImpact{CurrentState: gameOpsRaw(current), ProposedChange: gameOpsRaw(proposed),
		AffectedItems: gameOpsAffectedOne(), BlockingFacts: []string{},
		ContinuingAcceptedWork: continuing, RelatedIDs: related, UnavailableMeasurements: []string{}}
}

func parseGameReference(raw json.RawMessage) (gameConfigReferenceInput, error) {
	var input gameConfigReferenceInput
	if err := decodeGameOpsInput(raw, &input); err != nil {
		return input, err
	}
	if _, ok := directOpsDBSlug(input.GameSlug); !ok {
		return input, platform.ErrOpsInvalid
	}
	return input, nil
}

type opsValidationEvidence struct {
	ArtifactType     string
	ValidatorVersion string
	ValidationBuild  string
	Summary          json.RawMessage
	ArtifactHash     [sha256.Size]byte
}

func validationEvidence(config Config) (opsValidationEvidence, error) {
	binding := config.Binding()
	evidence := opsValidationEvidence{ArtifactType: "EXACT_MATH", ValidatorVersion: validationVersion,
		ValidationBuild: "direct-games-v1"}
	if external, ok := extraValidationSummaries[binding.Game]; ok {
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

func proposedRegistry(current opsRegistryState, input any) (opsRegistryState, error) {
	next := current
	version, err := strconv.ParseInt(current.Version, 10, 64)
	if err != nil || version <= 0 || version == math.MaxInt64 {
		return next, platform.ErrOpsConflict
	}
	switch value := input.(type) {
	case gameMetadataInput:
		if current.Title == value.Title && current.SortOrder == value.SortOrder {
			return next, platform.ErrOpsConflict
		}
		next.Title, next.SortOrder = value.Title, value.SortOrder
	case gamePublicationInput:
		if current.PublicationState == value.PublicationState {
			return next, platform.ErrOpsConflict
		}
		next.PublicationState = value.PublicationState
	case gameRuntimeInput:
		if current.ConfiguredRuntimeState == value.ConfiguredRuntimeState {
			return next, platform.ErrOpsConflict
		}
		next.ConfiguredRuntimeState = value.ConfiguredRuntimeState
	default:
		return next, platform.ErrOpsInvalid
	}
	next.Version = strconv.FormatInt(version+1, 10)
	return next, nil
}

func (h gameOpsHandler) Prepare(ctx context.Context, tx pgx.Tx, _ platform.OpsPrincipal,
	request platform.OpsPrepareRequest, lock bool) (platform.OpsPreparedMaterial, error) {
	if h.service == nil || h.service.store == nil || ctx == nil || tx == nil || request.OperationType != h.kind {
		return platform.OpsPreparedMaterial{}, platform.ErrOpsUnavailable
	}
	switch h.kind {
	case gameMetadataUpdate, gamePublicationSet, gameRuntimeSet:
		current, err := loadOpsRegistryState(ctx, tx, request.Target.ID, lock)
		if err != nil {
			return platform.OpsPreparedMaterial{}, err
		}
		if current.Version != request.Target.ExpectedVersion {
			return platform.OpsPreparedMaterial{}, platform.ErrOpsPreviewStale
		}
		var input any
		switch h.kind {
		case gameMetadataUpdate:
			var value gameMetadataInput
			if err = decodeGameOpsInput(request.Input, &value); err != nil {
				return platform.OpsPreparedMaterial{}, err
			}
			input = value
		case gamePublicationSet:
			var value gamePublicationInput
			if err = decodeGameOpsInput(request.Input, &value); err != nil {
				return platform.OpsPreparedMaterial{}, err
			}
			input = value
		case gameRuntimeSet:
			var value gameRuntimeInput
			if err = decodeGameOpsInput(request.Input, &value); err != nil {
				return platform.OpsPreparedMaterial{}, err
			}
			input = value
		}
		next, err := proposedRegistry(current, input)
		if err != nil {
			return platform.OpsPreparedMaterial{}, err
		}
		continuing := []string{}
		if h.kind == gamePublicationSet || h.kind == gameRuntimeSet {
			continuing = []string{"accepted rounds and settlements retain their bound configuration"}
		}
		impact := gameOpsImpact(current, next, []string{current.APISlug}, continuing)
		return platform.OpsPreparedMaterial{Impact: impact, TargetVersion: current.Version,
			TargetLocator: current.DBSlug}, nil
	}

	var apiSlug string
	if h.kind == gameConfigSave {
		var input gameConfigSaveInput
		if err := decodeGameOpsInput(request.Input, &input); err != nil {
			return platform.OpsPreparedMaterial{}, err
		}
		apiSlug = input.GameSlug
	} else {
		input, err := parseGameReference(request.Input)
		if err != nil {
			return platform.OpsPreparedMaterial{}, err
		}
		apiSlug = input.GameSlug
	}
	record, err := loadOpsConfigRecord(ctx, tx, apiSlug, request.Target.ID, lock)
	if err != nil {
		return platform.OpsPreparedMaterial{}, err
	}
	version := strconv.FormatInt(record.Version, 10)
	if version != request.Target.ExpectedVersion {
		return platform.OpsPreparedMaterial{}, platform.ErrOpsPreviewStale
	}
	currentState := record.state()
	related := []string{record.ID, record.APISlug}
	continuing := []string{}
	var proposed any

	switch h.kind {
	case gameConfigClone:
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM games.game_config_versions
		 WHERE config_version_id=$1)`, request.OperationID).Scan(&exists); err != nil {
			return platform.OpsPreparedMaterial{}, err
		}
		if exists {
			return platform.OpsPreparedMaterial{}, platform.ErrOpsConflict
		}
		nextVersion, versionErr := nextOpsConfigVersion(ctx, tx, record.DBSlug)
		if versionErr != nil {
			return platform.OpsPreparedMaterial{}, versionErr
		}
		cloned, cloneErr := loadConfigVersion(record.DBSlug, request.OperationID, record.Canonical)
		if cloneErr != nil {
			return platform.OpsPreparedMaterial{}, platform.ErrOpsConflict
		}
		binding := cloned.Binding()
		newRecord := opsConfigRecord{ID: request.OperationID, DBSlug: record.DBSlug, APISlug: record.APISlug,
			Version: nextVersion, Status: "DRAFT", Schema: binding.SchemaVersion,
			Ruleset: binding.RulesetVersion, Algorithm: binding.AlgorithmVersion,
			Canonical: cloned.CanonicalJSON(), Hash: append([]byte{}, binding.Hash[:]...),
			Resources: expectedResources(cloned), Config: cloned, Registry: record.Registry}
		proposed = map[string]any{"source": currentState, "draft": newRecord.state()}
		related = append(related, request.OperationID)
	case gameConfigSave:
		if record.Status != "DRAFT" {
			return platform.OpsPreparedMaterial{}, platform.ErrOpsConflict
		}
		var input gameConfigSaveInput
		if err = decodeGameOpsInput(request.Input, &input); err != nil {
			return platform.OpsPreparedMaterial{}, err
		}
		updated, updateErr := configFromEditable(record.ID, record.APISlug, input.Config)
		if updateErr != nil {
			return platform.OpsPreparedMaterial{}, updateErr
		}
		binding := updated.Binding()
		if bytes.Equal(binding.Hash[:], record.Hash) {
			return platform.OpsPreparedMaterial{}, platform.ErrOpsConflict
		}
		next := record
		next.Schema, next.Ruleset, next.Algorithm = binding.SchemaVersion, binding.RulesetVersion, binding.AlgorithmVersion
		next.Canonical, next.Hash, next.Resources, next.Config = updated.CanonicalJSON(),
			append([]byte{}, binding.Hash[:]...), expectedResources(updated), updated
		proposed = next.state()
	case gameConfigValidate:
		if record.Status != "DRAFT" {
			return platform.OpsPreparedMaterial{}, platform.ErrOpsConflict
		}
		var artifactExists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM games.game_validation_artifacts
		 WHERE config_version_id=$1 OR validation_artifact_id=$2)`, record.ID, request.OperationID).Scan(&artifactExists); err != nil {
			return platform.OpsPreparedMaterial{}, err
		}
		if artifactExists {
			return platform.OpsPreparedMaterial{}, platform.ErrOpsConflict
		}
		evidence, evidenceErr := validationEvidence(record.Config)
		if evidenceErr != nil {
			return platform.OpsPreparedMaterial{}, platform.ErrOpsConflict
		}
		next := record
		next.Status = "VALIDATED"
		proposed = map[string]any{"config": next.state(), "validation_artifact": map[string]any{
			"validation_artifact_id": request.OperationID, "artifact_type": evidence.ArtifactType,
			"artifact_sha256": hex.EncodeToString(evidence.ArtifactHash[:]), "status": "VERIFIED"}}
		related = append(related, request.OperationID)
	case gameConfigPreview:
		if record.Status != "VALIDATED" {
			return platform.OpsPreparedMaterial{}, platform.ErrOpsConflict
		}
		if err = verifyOpsValidationArtifact(ctx, tx, record); err != nil {
			return platform.OpsPreparedMaterial{}, err
		}
		next := record
		next.Status = "PREVIEWED"
		proposed = next.state()
	case gameConfigActivate:
		if record.Status != "PREVIEWED" || record.Registry.ImplementationKey != "direct."+record.DBSlug+".v1" {
			return platform.OpsPreparedMaterial{}, platform.ErrOpsConflict
		}
		if err = verifyOpsValidationArtifact(ctx, tx, record); err != nil {
			return platform.OpsPreparedMaterial{}, err
		}
		registryVersion, parseErr := strconv.ParseInt(record.Registry.Version, 10, 64)
		if parseErr != nil || registryVersion <= 0 || registryVersion == math.MaxInt64 {
			return platform.OpsPreparedMaterial{}, platform.ErrOpsConflict
		}
		next := record
		next.Status = "ACTIVE"
		next.Registry.Version = strconv.FormatInt(registryVersion+1, 10)
		next.Registry.ActiveConfigVersionID = &next.ID
		proposed = next.state()
		if record.Registry.ActiveConfigVersionID != nil {
			related = append(related, *record.Registry.ActiveConfigVersionID)
		}
		continuing = []string{"accepted rounds and settlements retain their original configuration binding"}
	default:
		return platform.OpsPreparedMaterial{}, platform.ErrOpsInvalid
	}
	impact := gameOpsImpact(currentState, proposed, related, continuing)
	return platform.OpsPreparedMaterial{Impact: impact, TargetVersion: version,
		TargetLocator: record.DBSlug}, nil
}

func requireOneUpdated(tag interface{ RowsAffected() int64 }) error {
	if tag.RowsAffected() != 1 {
		return platform.ErrOpsPreviewStale
	}
	return nil
}

func (h gameOpsHandler) Execute(ctx context.Context, tx pgx.Tx, actor platform.OpsPrincipal,
	operation platform.OpsOperation, material platform.OpsPreparedMaterial) (platform.OpsExecutionResult, error) {
	if h.service == nil || h.service.store == nil || ctx == nil || tx == nil || operation.OperationType != h.kind {
		return platform.OpsExecutionResult{}, platform.ErrOpsUnavailable
	}
	canonicalInput := operation.CanonicalInput()
	switch h.kind {
	case gameMetadataUpdate, gamePublicationSet, gameRuntimeSet:
		dbSlug, ok := opsDBSlug(operation.Target.ID)
		if !ok || dbSlug != material.TargetLocator {
			return platform.OpsExecutionResult{}, platform.ErrOpsInvalid
		}
		expected, err := strconv.ParseInt(operation.Target.ExpectedVersion, 10, 64)
		if err != nil || expected <= 0 {
			return platform.OpsExecutionResult{}, platform.ErrOpsInvalid
		}
		var tag interface{ RowsAffected() int64 }
		switch h.kind {
		case gameMetadataUpdate:
			var input gameMetadataInput
			if err = decodeGameOpsInput(canonicalInput, &input); err != nil {
				return platform.OpsExecutionResult{}, err
			}
			var sortOrder int64
			sortOrder, ok = canonicalUnsigned(input.SortOrder, 999999999)
			if !ok {
				return platform.OpsExecutionResult{}, platform.ErrOpsInvalid
			}
			commandTag, updateErr := tx.Exec(ctx, `UPDATE games.game_registry SET title=$2,sort_order=$3,
			 version=version+1,updated_at=clock_timestamp() WHERE game_slug=$1 AND version=$4`,
				dbSlug, input.Title, sortOrder, expected)
			if updateErr != nil {
				return platform.OpsExecutionResult{}, updateErr
			}
			tag = commandTag
		case gamePublicationSet:
			var input gamePublicationInput
			if err = decodeGameOpsInput(canonicalInput, &input); err != nil {
				return platform.OpsExecutionResult{}, err
			}
			commandTag, updateErr := tx.Exec(ctx, `UPDATE games.game_registry SET publication_state=$2,
			 version=version+1,updated_at=clock_timestamp() WHERE game_slug=$1 AND version=$3`,
				dbSlug, input.PublicationState, expected)
			if updateErr != nil {
				return platform.OpsExecutionResult{}, updateErr
			}
			tag = commandTag
		case gameRuntimeSet:
			var input gameRuntimeInput
			if err = decodeGameOpsInput(canonicalInput, &input); err != nil {
				return platform.OpsExecutionResult{}, err
			}
			commandTag, updateErr := tx.Exec(ctx, `UPDATE games.game_registry SET configured_runtime_state=$2,
			 version=version+1,updated_at=clock_timestamp() WHERE game_slug=$1 AND version=$3`,
				dbSlug, input.ConfiguredRuntimeState, expected)
			if updateErr != nil {
				return platform.OpsExecutionResult{}, updateErr
			}
			tag = commandTag
		}
		if err = requireOneUpdated(tag); err != nil {
			return platform.OpsExecutionResult{}, err
		}
		return gameOpsExecution(material, map[string]any{"game": material.Impact.ProposedChange}, operation.Target.ID), nil
	}

	var apiSlug string
	if h.kind == gameConfigSave {
		var input gameConfigSaveInput
		if err := decodeGameOpsInput(canonicalInput, &input); err != nil {
			return platform.OpsExecutionResult{}, err
		}
		apiSlug = input.GameSlug
	} else {
		input, err := parseGameReference(canonicalInput)
		if err != nil {
			return platform.OpsExecutionResult{}, err
		}
		apiSlug = input.GameSlug
	}
	record, err := loadOpsConfigRecord(ctx, tx, apiSlug, operation.Target.ID, false)
	if err != nil {
		return platform.OpsExecutionResult{}, err
	}
	if record.DBSlug != material.TargetLocator || strconv.FormatInt(record.Version, 10) != operation.Target.ExpectedVersion {
		return platform.OpsExecutionResult{}, platform.ErrOpsPreviewStale
	}

	switch h.kind {
	case gameConfigClone:
		nextVersion, versionErr := nextOpsConfigVersion(ctx, tx, record.DBSlug)
		if versionErr != nil {
			return platform.OpsExecutionResult{}, versionErr
		}
		cloned, cloneErr := loadConfigVersion(record.DBSlug, operation.OperationID, record.Canonical)
		if cloneErr != nil {
			return platform.OpsExecutionResult{}, platform.ErrOpsConflict
		}
		binding := cloned.Binding()
		canonical := cloned.CanonicalJSON()
		resources := expectedResources(cloned)
		_, err = tx.Exec(ctx, `INSERT INTO games.game_config_versions(
		 config_version_id,game_slug,parent_version_id,version_number,status,created_by,
		 config_schema_version,ruleset_version,algorithm_version,config_payload,canonical_payload,
		 config_hash,resource_versions) VALUES($1,$2,$3,$4,'DRAFT',$5,$6,$7,$8,$9::jsonb,$10,$11,$12::jsonb)`,
			operation.OperationID, record.DBSlug, record.ID, nextVersion, actor.UserID,
			binding.SchemaVersion, binding.RulesetVersion, binding.AlgorithmVersion,
			json.RawMessage(canonical), canonical, binding.Hash[:], json.RawMessage(resources))
		if err != nil {
			return platform.OpsExecutionResult{}, err
		}
		return gameOpsExecution(material, map[string]any{"config": material.Impact.ProposedChange}, operation.OperationID), nil
	case gameConfigSave:
		if record.Status != "DRAFT" {
			return platform.OpsExecutionResult{}, platform.ErrOpsPreviewStale
		}
		var input gameConfigSaveInput
		if err = decodeGameOpsInput(canonicalInput, &input); err != nil {
			return platform.OpsExecutionResult{}, err
		}
		updated, updateErr := configFromEditable(record.ID, record.APISlug, input.Config)
		if updateErr != nil {
			return platform.OpsExecutionResult{}, updateErr
		}
		binding := updated.Binding()
		canonical := updated.CanonicalJSON()
		resources := expectedResources(updated)
		tag, updateErr := tx.Exec(ctx, `UPDATE games.game_config_versions SET config_schema_version=$2,
		 ruleset_version=$3,algorithm_version=$4,config_payload=$5::jsonb,canonical_payload=$6,
		 config_hash=$7,resource_versions=$8::jsonb WHERE config_version_id=$1 AND status='DRAFT'`,
			record.ID, binding.SchemaVersion, binding.RulesetVersion, binding.AlgorithmVersion,
			json.RawMessage(canonical), canonical, binding.Hash[:], json.RawMessage(resources))
		if updateErr != nil {
			return platform.OpsExecutionResult{}, updateErr
		}
		if err = requireOneUpdated(tag); err != nil {
			return platform.OpsExecutionResult{}, err
		}
		return gameOpsExecution(material, map[string]any{"config": material.Impact.ProposedChange}, record.ID), nil
	case gameConfigValidate:
		if record.Status != "DRAFT" {
			return platform.OpsExecutionResult{}, platform.ErrOpsPreviewStale
		}
		evidence, evidenceErr := validationEvidence(record.Config)
		if evidenceErr != nil {
			return platform.OpsExecutionResult{}, platform.ErrOpsConflict
		}
		_, err = tx.Exec(ctx, `INSERT INTO games.game_validation_artifacts(
		 validation_artifact_id,game_slug,artifact_type,implementation_key,ruleset_version,
		 algorithm_version,config_version_id,config_hash,validator_version,validation_build,
		 result_summary,artifact_sha256,status,verified_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,'VERIFIED',clock_timestamp())`,
			operation.OperationID, record.DBSlug, evidence.ArtifactType, record.Registry.ImplementationKey,
			record.Ruleset, record.Algorithm, record.ID, record.Hash, evidence.ValidatorVersion,
			evidence.ValidationBuild, evidence.Summary, evidence.ArtifactHash[:])
		if err != nil {
			return platform.OpsExecutionResult{}, err
		}
		tag, updateErr := tx.Exec(ctx, `UPDATE games.game_config_versions SET status='VALIDATED',
		 validated_at=clock_timestamp() WHERE config_version_id=$1 AND status='DRAFT'`, record.ID)
		if updateErr != nil {
			return platform.OpsExecutionResult{}, updateErr
		}
		if err = requireOneUpdated(tag); err != nil {
			return platform.OpsExecutionResult{}, err
		}
		return gameOpsExecution(material, map[string]any{"config": material.Impact.ProposedChange}, record.ID), nil
	case gameConfigPreview:
		if record.Status != "VALIDATED" {
			return platform.OpsExecutionResult{}, platform.ErrOpsPreviewStale
		}
		if err = verifyOpsValidationArtifact(ctx, tx, record); err != nil {
			return platform.OpsExecutionResult{}, err
		}
		tag, updateErr := tx.Exec(ctx, `UPDATE games.game_config_versions SET status='PREVIEWED',
		 previewed_at=clock_timestamp() WHERE config_version_id=$1 AND status='VALIDATED'`, record.ID)
		if updateErr != nil {
			return platform.OpsExecutionResult{}, updateErr
		}
		if err = requireOneUpdated(tag); err != nil {
			return platform.OpsExecutionResult{}, err
		}
		return gameOpsExecution(material, map[string]any{"config": material.Impact.ProposedChange}, record.ID), nil
	case gameConfigActivate:
		if record.Status != "PREVIEWED" {
			return platform.OpsExecutionResult{}, platform.ErrOpsPreviewStale
		}
		if err = verifyOpsValidationArtifact(ctx, tx, record); err != nil {
			return platform.OpsExecutionResult{}, err
		}
		var now time.Time
		if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return platform.OpsExecutionResult{}, err
		}
		if record.Registry.ActiveConfigVersionID != nil {
			if *record.Registry.ActiveConfigVersionID == record.ID {
				return platform.OpsExecutionResult{}, platform.ErrOpsConflict
			}
			tag, updateErr := tx.Exec(ctx, `UPDATE games.game_config_versions SET status='SUPERSEDED',
			 superseded_at=$2 WHERE config_version_id=$1 AND status='ACTIVE'`,
				*record.Registry.ActiveConfigVersionID, now)
			if updateErr != nil {
				return platform.OpsExecutionResult{}, updateErr
			}
			if err = requireOneUpdated(tag); err != nil {
				return platform.OpsExecutionResult{}, err
			}
		}
		tag, updateErr := tx.Exec(ctx, `UPDATE games.game_config_versions SET status='ACTIVE',
		 activated_at=$2 WHERE config_version_id=$1 AND status='PREVIEWED'`, record.ID, now)
		if updateErr != nil {
			return platform.OpsExecutionResult{}, updateErr
		}
		if err = requireOneUpdated(tag); err != nil {
			return platform.OpsExecutionResult{}, err
		}
		registryVersion, parseErr := strconv.ParseInt(record.Registry.Version, 10, 64)
		if parseErr != nil || registryVersion <= 0 {
			return platform.OpsExecutionResult{}, platform.ErrOpsConflict
		}
		tag, updateErr = tx.Exec(ctx, `UPDATE games.game_registry SET active_config_version_id=$2,
		 version=version+1,updated_at=$3 WHERE game_slug=$1 AND version=$4`,
			record.DBSlug, record.ID, now, registryVersion)
		if updateErr != nil {
			return platform.OpsExecutionResult{}, updateErr
		}
		if err = requireOneUpdated(tag); err != nil {
			return platform.OpsExecutionResult{}, err
		}
		return gameOpsExecution(material, map[string]any{"config": material.Impact.ProposedChange}, record.ID), nil
	default:
		return platform.OpsExecutionResult{}, platform.ErrOpsInvalid
	}
}

func gameOpsExecution(material platform.OpsPreparedMaterial, result any, related string) platform.OpsExecutionResult {
	return platform.OpsExecutionResult{BeforeSnapshot: material.Impact.CurrentState,
		AfterSnapshot: material.Impact.ProposedChange, Result: gameOpsRaw(result), RelatedBusinessID: related}
}
