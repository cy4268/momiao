package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const NativeCatalogMaxBytes = 2 << 20
const NativeCatalogSchema = "momiao.native-catalog.v1"

var ErrCatalogSource = errors.New("CATALOG_SOURCE_INVALID")

type CatalogDimension struct {
	Kind      string  `json:"kind"`
	Unit      string  `json:"unit"`
	Amount    *string `json:"amount"`
	Source    string  `json:"source"`
	Condition string  `json:"condition"`
	Support   string  `json:"support"`
}
type CatalogPrice struct {
	Mode            string             `json:"mode"`
	Configured      bool               `json:"configured"`
	Status          string             `json:"status"`
	Reason          string             `json:"reason,omitempty"`
	GroupMultiplier *string            `json:"group_multiplier"`
	Dimensions      []CatalogDimension `json:"dimensions"`
	Unquoted        []string           `json:"unquoted_dimensions"`
}
type CatalogEndpoint struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Method string `json:"method"`
}
type NativeCatalogModel struct {
	ModelID              string            `json:"model_id"`
	EnabledConfiguration bool              `json:"enabled_configuration"`
	NativeCatalogVisible bool              `json:"native_catalog_visible"`
	EndpointStatus       string            `json:"endpoint_status"`
	Endpoints            []CatalogEndpoint `json:"endpoints"`
	Price                CatalogPrice      `json:"price"`
}
type nativeCatalogData struct {
	Schema           string               `json:"schema"`
	Basis            string               `json:"basis"`
	BillingAuthority string               `json:"billing_authority"`
	Notices          []string             `json:"notices"`
	Models           []NativeCatalogModel `json:"models"`
}

// NativeCatalog is a validated server-side snapshot, never a public vendor object.
type NativeCatalog struct {
	validated  bool
	Hash       string
	ObservedAt time.Time
	VerifiedAt time.Time
	Models     []NativeCatalogModel
}

var catalogNotices = []string{
	"configuration_not_call_health", "absence_is_not_retirement", "unpublished_models_require_platform_review",
	"extra_fees_and_integer_quota_rounding_not_included", "API_Credit_is_native_USD_denominated_accounting_unit_not_currency_conversion",
}
var catalogUnquoted = []string{"image", "audio", "tools", "request_adjustments"}
var catalogEndpointPaths = map[string]string{
	"openai": "/v1/chat/completions", "openai-response": "/v1/responses", "anthropic": "/v1/messages", "image-generation": "/v1/images/generations",
}
var catalogDimensionConditions = map[string]string{
	"input": "uncached_plain_text_tokens", "output": "plain_text_output_tokens",
	"cache_read":        "only_if_native_reports_billable_cache_read_tokens",
	"cache_write":       "only_if_native_reports_billable_generic_cache_write_tokens",
	"cache_write_5m":    "only_if_native_reports_anthropic_5m_cache_write_tokens",
	"cache_write_1h":    "only_if_native_reports_anthropic_1h_cache_write_tokens",
	"text_request_base": "plain_text_request_without_extra_multipliers_or_tool_fees",
}
var catalogPriceReasons = []string{
	"expression_missing", "expression_too_large", "expression_invalid", "expression_requires_usage",
	"unsupported_billing_mode", "unsupported_quota_unit", "price_not_configured", "group_price_not_configured",
	"invalid_numeric_configuration", "unsupported_billing_surface", "native_catalog_hidden",
}

func ValidCatalogModelID(id string) bool {
	return len(id) > 0 && len(id) <= 255 && utf8.ValidString(id) && strings.TrimSpace(id) == id && !strings.ContainsFunc(id, unicode.IsControl)
}

// Bounds are representation limits, not a spending or currency conversion rule.
var catalogDecimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)

func ValidCatalogDecimal(value string) bool {
	if len(value) > 1400 || !catalogDecimalPattern.MatchString(value) {
		return false
	}
	amount, ok := new(big.Rat).SetString(value)
	return ok && amount.Sign() >= 0 && amount.Cmp(big.NewRat(1_000_000_000_000, 1)) <= 0
}
func CompareCatalogDecimal(a, b string) int {
	// Call only after ValidCatalogDecimal; no float conversion or display rounding.
	x, _ := new(big.Rat).SetString(a)
	y, _ := new(big.Rat).SetString(b)
	return x.Cmp(y)
}
func validCatalogPrice(p CatalogPrice, personal bool) bool {
	if !slices.Contains([]string{"ratio", "per_request", "tiered_expr", "unknown"}, p.Mode) || !slices.Equal(p.Unquoted, catalogUnquoted) {
		return false
	}
	if p.GroupMultiplier != nil && !ValidCatalogDecimal(*p.GroupMultiplier) {
		return false
	}
	if personal && p.GroupMultiplier != nil {
		return false
	}
	if p.Status == "unquotable" {
		return len(p.Dimensions) == 0 && slices.Contains(catalogPriceReasons, p.Reason)
	}
	if !p.Configured || !personal && p.GroupMultiplier == nil || p.Reason != "" {
		return false
	}
	var kinds []string
	switch p.Status {
	case "reference":
		if p.Mode != "ratio" {
			return false
		}
		kinds = []string{"input", "output", "cache_read", "cache_write"}
		if len(p.Dimensions) == 5 {
			kinds = []string{"input", "output", "cache_read", "cache_write_5m", "cache_write_1h"}
		}
	case "conditional":
		if p.Mode != "per_request" {
			return false
		}
		kinds = []string{"text_request_base"}
	default:
		return false
	}
	if len(p.Dimensions) != len(kinds) {
		return false
	}
	for i, d := range p.Dimensions {
		unit := "API_Credit_per_1M_tokens"
		if d.Kind == "text_request_base" {
			unit = "API_Credit_per_request"
		}
		if d.Kind != kinds[i] || d.Unit != unit || d.Amount == nil || !ValidCatalogDecimal(*d.Amount) || d.Condition != catalogDimensionConditions[d.Kind] || d.Support != "not_asserted" {
			return false
		}
		if d.Source != "native_effective" && !(strings.HasPrefix(d.Kind, "cache_") && d.Source == "native_default") {
			return false
		}
	}
	return true
}
func validCatalogObserved(value string, now time.Time) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, value)
	return t, err == nil && strings.HasSuffix(value, "Z") && !t.After(now.Add(time.Minute)) && !t.Before(now.Add(-5*time.Minute))
}
func ParseNativeCatalog(raw []byte, now time.Time) (NativeCatalog, error) {
	var e struct {
		Success    bool            `json:"success"`
		Complete   bool            `json:"complete"`
		ObservedAt string          `json:"observed_at"`
		Hash       string          `json:"content_hash"`
		Data       json.RawMessage `json:"data"`
	}
	fail := func() (NativeCatalog, error) { return NativeCatalog{}, ErrCatalogSource }
	if !decodeCatalogJSON(raw, &e) || !e.Success || !e.Complete {
		return fail()
	}
	observed, ok := validCatalogObserved(e.ObservedAt, now)
	if !ok {
		return fail()
	}
	var compact bytes.Buffer
	if json.Compact(&compact, e.Data) != nil {
		return fail()
	}
	hash := sha256.Sum256(compact.Bytes())
	if e.Hash != "sha256:"+hex.EncodeToString(hash[:]) {
		return fail()
	}
	var d nativeCatalogData
	if !decodeCatalogJSON(e.Data, &d) || d.Schema != NativeCatalogSchema || d.Basis != "public_default_reference" || d.BillingAuthority != "native_settlement" || !slices.Equal(d.Notices, catalogNotices) || len(d.Models) > 1000 {
		return fail()
	}
	previous := ""
	for _, m := range d.Models {
		if m.ModelID <= previous || !validNativeCatalogModel(m) {
			return fail()
		}
		previous = m.ModelID
	}
	return NativeCatalog{validated: true, Hash: e.Hash, ObservedAt: observed, VerifiedAt: now.UTC(), Models: d.Models}, nil
}
func validNativeCatalogModel(m NativeCatalogModel) bool {
	if !ValidCatalogModelID(m.ModelID) || !m.EnabledConfiguration || len(m.Endpoints) > 4 || !validCatalogPrice(m.Price, false) {
		return false
	}
	if len(m.Endpoints) == 0 {
		if m.EndpointStatus != "unverified" {
			return false
		}
	} else if m.EndpointStatus != "configured_subset_not_health" && m.EndpointStatus != "partial_configured_subset_not_health" {
		return false
	}
	previousEndpoint := ""
	for _, ep := range m.Endpoints {
		if ep.Kind <= previousEndpoint || catalogEndpointPaths[ep.Kind] != ep.Path || ep.Method != "POST" || ep.Path == "" {
			return false
		}
		previousEndpoint = ep.Kind
	}
	if !m.NativeCatalogVisible && (m.Price.Status != "unquotable" || m.Price.Reason != "native_catalog_hidden") {
		return false
	}
	return true
}

type NativePersonalQuote struct {
	Candidate            int           `json:"candidate"`
	EnabledConfiguration bool          `json:"enabled_configuration"`
	NativeCatalogVisible bool          `json:"native_catalog_visible"`
	Price                *CatalogPrice `json:"price"`
	Reason               string        `json:"reason,omitempty"`
}
type NativePersonalCatalog struct {
	Success          bool                  `json:"success"`
	Schema           string                `json:"schema"`
	ObservedAt       string                `json:"observed_at"`
	ModelID          string                `json:"model_id"`
	Basis            string                `json:"basis"`
	BillingAuthority string                `json:"billing_authority"`
	Quotes           []NativePersonalQuote `json:"quotes"`
}

func ParseNativePersonalCatalog(raw []byte, modelID string, now time.Time) (NativePersonalCatalog, error) {
	var result NativePersonalCatalog
	fail := func() (NativePersonalCatalog, error) { return NativePersonalCatalog{}, ErrCatalogSource }
	if !ValidCatalogModelID(modelID) || !decodeCatalogJSON(raw, &result) || !result.Success || result.Schema != NativeCatalogSchema || result.ModelID != modelID || result.BillingAuthority != "native_settlement" {
		return fail()
	}
	if _, ok := validCatalogObserved(result.ObservedAt, now); !ok {
		return fail()
	}
	if len(result.Quotes) < 1 || len(result.Quotes) > 32 {
		return fail()
	}
	switch result.Basis {
	case "current_user_group_reference_not_token_selection":
		if len(result.Quotes) != 1 {
			return fail()
		}
	case "eligible_auto_candidates_not_selected":
	default:
		return fail()
	}
	for i, q := range result.Quotes {
		if q.Candidate != i+1 {
			return fail()
		}
		if q.Price == nil {
			if q.EnabledConfiguration || q.NativeCatalogVisible || q.Reason != "model_not_enabled_in_candidate" {
				return fail()
			}
		} else if !q.EnabledConfiguration || q.Reason != "" || !validCatalogPrice(*q.Price, true) || !q.NativeCatalogVisible && (q.Price.Status != "unquotable" || q.Price.Reason != "native_catalog_hidden") {
			return fail()
		}
	}
	return result, nil
}

// encoding/json accepts case aliases, duplicate names, missing bools and null
// slices. This bounded shape pass rejects those ambiguities at the v1 boundary.
func decodeCatalogJSON(raw []byte, target any) bool {
	if len(raw) == 0 || len(raw) > NativeCatalogMaxBytes || !utf8.Valid(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if !catalogJSONShape(d, reflect.TypeOf(target).Elem(), 0) {
		return false
	}
	if _, err := d.Token(); err != io.EOF {
		return false
	}
	return json.Unmarshal(raw, target) == nil
}

var catalogRawType = reflect.TypeOf(json.RawMessage{})

func catalogJSONShape(d *json.Decoder, t reflect.Type, depth int) bool {
	if depth > 12 {
		return false
	}
	if t == catalogRawType {
		t = nil
	}
	token, err := d.Token()
	if err != nil {
		return false
	}
	if t != nil && t.Kind() == reflect.Pointer {
		if token == nil {
			return true
		}
		t = t.Elem()
	}
	delim, container := token.(json.Delim)
	if !container {
		if t == nil {
			return true
		}
		switch t.Kind() {
		case reflect.Bool:
			_, ok := token.(bool)
			return ok
		case reflect.String:
			_, ok := token.(string)
			return ok
		case reflect.Int:
			_, ok := token.(json.Number)
			return ok
		default:
			return false
		}
	}
	if delim == '{' {
		if t != nil && t.Kind() != reflect.Struct {
			return false
		}
		fields := map[string]reflect.StructField{}
		required := map[string]bool{}
		if t != nil {
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				tag := strings.Split(f.Tag.Get("json"), ",")
				if tag[0] != "-" {
					fields[tag[0]] = f
					if len(tag) == 1 {
						required[tag[0]] = true
					}
				}
			}
		}
		seen := map[string]bool{}
		for d.More() {
			keyToken, e := d.Token()
			key, ok := keyToken.(string)
			if e != nil || !ok || seen[key] {
				return false
			}
			seen[key] = true
			var fieldType reflect.Type
			if t != nil {
				f, found := fields[key]
				if !found {
					return false
				}
				fieldType = f.Type
				delete(required, key)
			}
			if !catalogJSONShape(d, fieldType, depth+1) {
				return false
			}
		}
		end, e := d.Token()
		return e == nil && end == json.Delim('}') && len(required) == 0
	}
	if delim == '[' {
		if t != nil && t.Kind() != reflect.Slice {
			return false
		}
		var elem reflect.Type
		if t != nil {
			elem = t.Elem()
		}
		for d.More() {
			if !catalogJSONShape(d, elem, depth+1) {
				return false
			}
		}
		end, e := d.Token()
		return e == nil && end == json.Delim(']')
	}
	return false
}

// Display final local prices in API Credit, using the same one-time group factor
// as relayCostSQL. PostgreSQL numeric keeps zero and fractional prices exact.
const catalogLocalPriceJSON = `jsonb_build_object(
 'configured',rp.model IS NOT NULL AND g.name IS NOT NULL,
 'input',(rp.input*g.ratio/500000)::text,'output',(rp.output*g.ratio/500000)::text,
 'cache_read',(rp.cache_read*g.ratio/500000)::text,'cache_write_5m',(rp.cache_write*g.ratio/500000)::text,
 'cache_write_1h',(rp.cache_write_1h*g.ratio/500000)::text,
 'text_request_base',(rp.per_request*g.ratio/500000)::text,'second',(rp.per_second*g.ratio/500000)::text,
 'default_seconds',coalesce(rp.default_seconds,0))`

func localCatalogFreshness() CatalogFreshness {
	now := time.Now().UTC()
	return CatalogFreshness{State: "CURRENT", LastObservedAt: &now, LastVerifiedAt: &now}
}

func catalogLocalPrice(raw []byte) (CatalogPublicPrice, error) {
	var prices struct {
		Configured     bool    `json:"configured"`
		Input          *string `json:"input"`
		Output         *string `json:"output"`
		CacheRead      *string `json:"cache_read"`
		CacheWrite     *string `json:"cache_write_5m"`
		CacheWrite1h   *string `json:"cache_write_1h"`
		Request        *string `json:"text_request_base"`
		Second         *string `json:"second"`
		DefaultSeconds int     `json:"default_seconds"`
	}
	if err := json.Unmarshal(raw, &prices); err != nil {
		return CatalogPublicPrice{}, err
	}
	p := CatalogPublicPrice{Mode: "unknown", Status: "unquotable", Reason: "price_not_configured", Dimensions: []CatalogDimension{}, Unquoted: []string{}}
	if !prices.Configured {
		return p, nil
	}
	p.Configured, p.Mode, p.Status, p.Reason = true, "ratio", "reference", ""
	add := func(kind, unit, condition string, amount *string) {
		p.Dimensions = append(p.Dimensions, CatalogDimension{Kind: kind, Unit: unit, Condition: condition, Amount: amount, Source: "local_price", Support: "configured"})
	}
	switch {
	case prices.Request != nil:
		p.Mode, p.Status = "per_request", "conditional"
		add("text_request_base", "API_Credit_per_request", "per_request", prices.Request)
	case prices.Second != nil:
		p.Mode, p.Status, p.DefaultSeconds = "per_second", "conditional", prices.DefaultSeconds
		add("second", "API_Credit_per_second", "request_duration_seconds", prices.Second)
	default:
		add("input", "API_Credit_per_1M_tokens", "uncached_plain_text_tokens", prices.Input)
		add("output", "API_Credit_per_1M_tokens", "plain_text_output_tokens", prices.Output)
		add("cache_read", "API_Credit_per_1M_tokens", "reported_cache_read_tokens", prices.CacheRead)
		add("cache_write_5m", "API_Credit_per_1M_tokens", "reported_cache_write_5m_tokens", prices.CacheWrite)
		add("cache_write_1h", "API_Credit_per_1M_tokens", "reported_cache_write_1h_tokens", prices.CacheWrite1h)
	}
	return p, nil
}

type CatalogPersonalQuote struct {
	Candidate int                 `json:"candidate"`
	Enabled   bool                `json:"enabled_configuration"`
	Price     *CatalogPublicPrice `json:"price"`
	Reason    string              `json:"reason,omitempty"`
}
type CatalogPersonalPrice struct {
	ModelID    string                 `json:"model_id"`
	ObservedAt time.Time              `json:"observed_at"`
	Basis      string                 `json:"basis"`
	Quotes     []CatalogPersonalQuote `json:"quotes"`
}

func (s *Store) PersonalCatalog(ctx context.Context, userID int64, model string) (CatalogPersonalPrice, error) {
	result := CatalogPersonalPrice{ModelID: model, ObservedAt: time.Now().UTC(), Basis: "current_user_group_reference_not_token_selection", Quotes: []CatalogPersonalQuote{}}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer rollback(tx)
	if _, err = loadCatalogModel(ctx, tx, model, true, localCatalogFreshness()); err != nil {
		return result, err
	}
	var group string
	if err = tx.QueryRow(ctx, `SELECT group_name FROM identity.users WHERE id=$1 AND status=1 AND deleted_at IS NULL`, userID).Scan(&group); err != nil {
		return result, err
	}
	quote := CatalogPersonalQuote{Candidate: 1}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM relay.channels WHERE enabled AND $1=ANY(models) AND $2=ANY(groups))`, model, group).Scan(&quote.Enabled); err != nil {
		return result, err
	}
	if quote.Enabled {
		var raw []byte
		if err = tx.QueryRow(ctx, `SELECT `+catalogLocalPriceJSON+` FROM (SELECT $1::text model_id) m LEFT JOIN relay.prices rp ON rp.model=m.model_id LEFT JOIN relay.groups g ON g.name=$2`, model, group).Scan(&raw); err != nil {
			return result, err
		}
		price, err := catalogLocalPrice(raw)
		if err != nil {
			return result, err
		}
		quote.Price = &price
	} else {
		quote.Reason = "model_not_enabled_in_candidate"
	}
	result.Quotes = append(result.Quotes, quote)
	return result, tx.Commit(ctx)
}
