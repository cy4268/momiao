package platform

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrCatalogInvalid       = errors.New("CATALOG_INVALID_REQUEST")
	ErrCatalogForbidden     = errors.New("MODELS_FORBIDDEN")
	ErrCatalogNotFound      = errors.New("MODEL_NOT_FOUND")
	ErrCatalogConflict      = errors.New("MODEL_VERSION_CONFLICT")
	ErrCatalogConfirmation  = errors.New("MODEL_CONFIRMATION_REQUIRED")
	ErrCatalogOperation     = errors.New("MODEL_OPERATION_CONFLICT")
	ErrCatalogIncomplete    = errors.New("MODEL_METADATA_INCOMPLETE")
	ErrCatalogSourceChanged = errors.New("CATALOG_SOURCE_CHANGED")
)

type CatalogPolicy struct{ StaleAfter, DisableAfter time.Duration }

func (p CatalogPolicy) valid() bool {
	return p.StaleAfter >= time.Minute && p.DisableAfter > p.StaleAfter && p.DisableAfter <= 24*time.Hour
}

type CatalogFreshness struct {
	State               string     `json:"state"`
	LastObservedAt      *time.Time `json:"last_observed_at"`
	LastVerifiedAt      *time.Time `json:"last_verified_at"`
	StaleAfterSeconds   int64      `json:"stale_after_seconds"`
	DisableAfterSeconds int64      `json:"disable_after_seconds"`
}

func catalogFreshness(status CatalogSyncStatus, policy CatalogPolicy, now time.Time) CatalogFreshness {
	result := CatalogFreshness{State: "NEVER_SYNCED", LastObservedAt: status.LastObservedAt, LastVerifiedAt: status.LastVerifiedAt, StaleAfterSeconds: int64(policy.StaleAfter / time.Second), DisableAfterSeconds: int64(policy.DisableAfter / time.Second)}
	if status.LastVerifiedAt != nil {
		result.State = "CURRENT"
		age := now.Sub(*status.LastVerifiedAt)
		if age >= policy.DisableAfter {
			result.State = "EXPIRED"
		} else if age >= policy.StaleAfter {
			result.State = "STALE"
		}
	}
	return result
}

type CatalogMetadata struct {
	DisplayName        string   `json:"display_name"`
	Family             string   `json:"family"`
	Summary            string   `json:"summary"`
	ContextLength      *int64   `json:"context_length,string"`
	Subtitle           string   `json:"subtitle"`
	Tags               []string `json:"tags"`
	UseCases           []string `json:"use_cases"`
	SpecialPricingNote string   `json:"special_pricing_note"`
	AssetID            string   `json:"asset_id"`
}
type CatalogPublicPrice struct {
	Mode       string             `json:"mode"`
	Configured bool               `json:"configured"`
	Status     string             `json:"status"`
	Reason     string             `json:"reason,omitempty"`
	Dimensions []CatalogDimension `json:"dimensions"`
	Unquoted   []string           `json:"unquoted_dimensions"`
}

func catalogPublicPrice(p CatalogPrice) CatalogPublicPrice {
	return CatalogPublicPrice{Mode: p.Mode, Configured: p.Configured, Status: p.Status, Reason: p.Reason, Dimensions: append([]CatalogDimension{}, p.Dimensions...), Unquoted: append([]string{}, p.Unquoted...)}
}

type CatalogModel struct {
	ModelID           string             `json:"model_id"`
	Metadata          CatalogMetadata    `json:"metadata"`
	PublicationState  string             `json:"publication_state"`
	Recommended       bool               `json:"recommended"`
	SortOrder         int                `json:"sort_order"`
	Version           int64              `json:"version,string"`
	MetadataVersion   int64              `json:"metadata_version,string"`
	PublishedAt       *time.Time         `json:"published_at"`
	RetiredAt         *time.Time         `json:"retired_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
	AvailabilityState string             `json:"availability_state"`
	SourceObservedAt  time.Time          `json:"source_observed_at"`
	LastSeenAt        time.Time          `json:"last_seen_at"`
	EndpointStatus    string             `json:"endpoint_status"`
	Endpoints         []CatalogEndpoint  `json:"endpoints"`
	Price             CatalogPublicPrice `json:"price"`
	CanUse            bool               `json:"can_use"`
	Freshness         CatalogFreshness   `json:"freshness"`
}
type CatalogChoice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
type CatalogVocabulary struct {
	Families []CatalogChoice `json:"families"`
	Tags     []CatalogChoice `json:"tags"`
	UseCases []CatalogChoice `json:"use_cases"`
	Assets   []CatalogAsset  `json:"assets"`
}

var catalogFamilies = []CatalogChoice{{"deepseek", "DeepSeek"}, {"gpt", "GPT"}, {"claude", "Claude"}, {"gemini", "Gemini"}, {"glm", "GLM"}, {"kimi", "Kimi"}, {"ernie", "ERNIE / 文心"}, {"qwen", "Qwen / 千问"}, {"grok", "Grok"}, {"other", "其他家族"}}
var catalogTags = []CatalogChoice{{"coding", "代码"}, {"reasoning", "推理"}, {"writing", "写作"}, {"multimodal", "多模态"}, {"lightweight", "轻量"}, {"general", "通用"}}
var catalogUseCases = []CatalogChoice{{"coding", "编程辅助"}, {"analysis", "资料分析"}, {"writing", "创作与写作"}, {"translation", "翻译"}, {"conversation", "日常对话"}}

type CatalogAsset struct {
	AssetID         string     `json:"asset_id"`
	Domain          string     `json:"domain"`
	Scene           string     `json:"scene"`
	Character       string     `json:"character"`
	Skin            string     `json:"skin"`
	Role            string     `json:"role"`
	Version         string     `json:"version"`
	Source          string     `json:"source"`
	Src             string     `json:"src"`
	Fallback        string     `json:"fallback"`
	FocalPoint      [2]float64 `json:"focal_point"`
	SafeArea        float64    `json:"safe_area"`
	Alpha           bool       `json:"alpha"`
	Status          string     `json:"status"`
	RightsStatus    string     `json:"rights_status"`
	RightsNote      string     `json:"rights_note"`
	ReviewDate      string     `json:"review_date"`
	PromptArchiveID string     `json:"prompt_archive_id"`
}

//go:embed catalog-personas.json
var catalogPersonaManifest []byte

func catalogVocabulary() (CatalogVocabulary, error) {
	var manifest struct {
		Schema string         `json:"schema"`
		Assets []CatalogAsset `json:"assets"`
	}
	decoder := json.NewDecoder(bytes.NewReader(catalogPersonaManifest))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&manifest) != nil || manifest.Schema != "momiao.model-personas.v1" || manifest.Assets == nil {
		return CatalogVocabulary{}, ErrCatalogInvalid
	}
	ids := map[string]bool{}
	for _, a := range manifest.Assets {
		if a.AssetID == "" || ids[a.AssetID] || a.Status != "PRODUCTION_READY" || a.Domain != "models" || a.Source == "" || a.RightsNote == "" || a.ReviewDate == "" || !a.Alpha || a.SafeArea < 0.05 || a.SafeArea > 0.5 || a.FocalPoint[0] < 0 || a.FocalPoint[0] > 1 || a.FocalPoint[1] < 0 || a.FocalPoint[1] > 1 || !slices.Contains([]string{"ORIGINAL_PLATFORM", "ORIGINAL_GENERATED", "LICENSED_OR_APPROVED"}, a.RightsStatus) {
			return CatalogVocabulary{}, ErrCatalogInvalid
		}
		for _, path := range []string{a.Src, a.Fallback} {
			if !strings.HasPrefix(path, "/assets/models/") || strings.ContainsAny(path, "?#\\") || strings.Contains(path, "..") || !(strings.HasSuffix(path, ".webp") || strings.HasSuffix(path, ".png")) {
				return CatalogVocabulary{}, ErrCatalogInvalid
			}
		}
		ids[a.AssetID] = true
	}
	return CatalogVocabulary{Families: catalogFamilies, Tags: catalogTags, UseCases: catalogUseCases, Assets: manifest.Assets}, nil
}

// CatalogChoices exposes only the code-owned vocabulary and approved asset manifest.
func CatalogChoices() (CatalogVocabulary, error) { return catalogVocabulary() }
func catalogChoice(choices []CatalogChoice, value string) bool {
	return slices.ContainsFunc(choices, func(c CatalogChoice) bool { return c.Value == value })
}
func catalogPlainText(value string, max int, multiline bool) bool {
	if !utf8.ValidString(value) || len([]rune(value)) > max || strings.TrimSpace(value) != value || strings.ContainsAny(value, "<>") {
		return false
	}
	if strings.ContainsFunc(value, func(r rune) bool { return unicode.IsControl(r) && !(multiline && r == '\n') }) {
		return false
	}
	lower := strings.ToLower(value)
	return !strings.Contains(lower, "https://") && !strings.Contains(lower, "http://") && !strings.Contains(lower, "data:") && !strings.Contains(lower, "javascript:")
}
func validCatalogMetadata(m CatalogMetadata) bool {
	if !catalogPlainText(m.DisplayName, 120, false) || !catalogPlainText(m.Summary, 2000, true) || !catalogPlainText(m.Subtitle, 160, false) || !catalogPlainText(m.SpecialPricingNote, 1000, true) || m.Family != "" && !catalogChoice(catalogFamilies, m.Family) {
		return false
	}
	if m.ContextLength != nil && (*m.ContextLength <= 0 || *m.ContextLength > 9007199254740991) {
		return false
	}
	if m.Tags == nil || m.UseCases == nil || len(m.Tags) > 6 || len(m.UseCases) > 5 {
		return false
	}
	for _, pair := range []struct {
		values  []string
		choices []CatalogChoice
	}{{m.Tags, catalogTags}, {m.UseCases, catalogUseCases}} {
		seen := map[string]bool{}
		for _, v := range pair.values {
			if seen[v] || !catalogChoice(pair.choices, v) {
				return false
			}
			seen[v] = true
		}
	}
	vocabulary, err := catalogVocabulary()
	return err == nil && (m.AssetID == "" || slices.ContainsFunc(vocabulary.Assets, func(a CatalogAsset) bool { return a.AssetID == m.AssetID }))
}
func catalogPublishable(model CatalogModel) bool {
	m := model.Metadata
	if !validCatalogMetadata(m) || m.DisplayName == "" || m.Summary == "" || m.Family == "" {
		return false
	}
	return model.Price.Status == "reference" || len([]rune(m.SpecialPricingNote)) >= 10
}
func catalogCanUse(model CatalogModel) bool {
	return model.PublicationState == "PUBLISHED" && model.AvailabilityState == "CONFIGURED" && len(model.Endpoints) > 0 && (model.Freshness.State == "CURRENT" || model.Freshness.State == "STALE") && catalogPublishable(model)
}
