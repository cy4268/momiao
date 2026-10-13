package platform

import (
	"context"
	"encoding/json"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type CatalogDimension struct {
	Kind      string  `json:"kind"`
	Unit      string  `json:"unit"`
	Amount    *string `json:"amount"`
	Source    string  `json:"source"`
	Condition string  `json:"condition"`
	Support   string  `json:"support"`
}
type CatalogEndpoint struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Method string `json:"method"`
}

var catalogDimensionConditions = map[string]string{
	"input": "uncached_plain_text_tokens", "output": "plain_text_output_tokens",
	"cache_read":        "reported_billable_cache_read_tokens",
	"cache_write":       "reported_billable_generic_cache_write_tokens",
	"cache_write_5m":    "reported_anthropic_5m_cache_write_tokens",
	"cache_write_1h":    "reported_anthropic_1h_cache_write_tokens",
	"text_request_base": "plain_text_request_without_extra_multipliers_or_tool_fees",
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

// Local prices use the same final units and one group ratio
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
	if _, err = loadCatalogModel(ctx, tx, model, true); err != nil {
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
