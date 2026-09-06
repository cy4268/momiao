package platform

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type CatalogFilter struct {
	Search, Availability, Family, Tag, UseCase, PriceDimension, Sort string
	RecommendedOnly, UnknownContext                                  bool
	MinContext                                                       *int64
	MinPrice, MaxPrice                                               *string
	Offset, Limit                                                    int
}
type CatalogPage struct {
	Items          []CatalogModel    `json:"items"`
	Total          int               `json:"total"`
	Offset         int               `json:"offset"`
	Limit          int               `json:"limit"`
	Freshness      CatalogFreshness  `json:"freshness"`
	Vocabulary     CatalogVocabulary `json:"vocabulary"`
	PriceDimension string            `json:"price_dimension"`
	PriceUnit      string            `json:"price_unit"`
}
type CatalogOpsFilter struct {
	Search, State string
	Offset, Limit int
}
type CatalogOpsPage struct {
	Principal  AnnouncementPrincipal `json:"principal"`
	Items      []CatalogModel        `json:"items"`
	Total      int                   `json:"total"`
	Offset     int                   `json:"offset"`
	Limit      int                   `json:"limit"`
	Sync       CatalogSyncStatus     `json:"sync"`
	Freshness  CatalogFreshness      `json:"freshness"`
	Vocabulary CatalogVocabulary     `json:"vocabulary"`
}

func catalogSearchValid(search string) bool {
	return utf8.ValidString(search) && len(search) <= 512 && !strings.ContainsFunc(search, unicode.IsControl)
}
func catalogPriceUnit(dimension string) string {
	if dimension == "text_request_base" {
		return "API_Credit_per_request"
	}
	if _, ok := catalogDimensionConditions[dimension]; ok {
		return "API_Credit_per_1M_tokens"
	}
	return ""
}
func validateCatalogFilter(f *CatalogFilter) bool {
	if f.Limit == 0 {
		f.Limit = 24
	}
	if f.Sort == "" {
		f.Sort = "recommended"
	}
	f.Search = strings.TrimSpace(f.Search)
	if !catalogSearchValid(f.Search) || f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 1000000 || !slices.Contains([]string{"recommended", "name", "context", "price"}, f.Sort) || !slices.Contains([]string{"", "CONFIGURED", "NATIVE_HIDDEN", "NOT_OBSERVED"}, f.Availability) {
		return false
	}
	if f.Family != "" && !catalogChoice(catalogFamilies, f.Family) || f.Tag != "" && !catalogChoice(catalogTags, f.Tag) || f.UseCase != "" && !catalogChoice(catalogUseCases, f.UseCase) {
		return false
	}
	if f.MinContext != nil && (*f.MinContext <= 0 || *f.MinContext > 9007199254740991 || f.UnknownContext) {
		return false
	}
	if f.PriceDimension != "" && catalogPriceUnit(f.PriceDimension) == "" {
		return false
	}
	if (f.Sort == "price" || f.MinPrice != nil || f.MaxPrice != nil) && f.PriceDimension == "" {
		return false
	}
	if f.MinPrice != nil && !ValidCatalogDecimal(*f.MinPrice) || f.MaxPrice != nil && !ValidCatalogDecimal(*f.MaxPrice) {
		return false
	}
	return f.MinPrice == nil || f.MaxPrice == nil || CompareCatalogDecimal(*f.MinPrice, *f.MaxPrice) <= 0
}

const catalogPublicSelection = catalogModelTables + `
 LEFT JOIN LATERAL (
  SELECT (dimension->>'amount')::numeric amount FROM jsonb_array_elements(a.source_facts->'price'->'dimensions') d(dimension)
  WHERE dimension->>'kind'=$1 AND dimension->>'unit'=$2 AND jsonb_typeof(dimension->'amount')='string'
 ) price ON true
 WHERE p.publication_state='PUBLISHED'
 AND ($3='' OR strpos(lower(m.display_name),lower($3))>0 OR strpos(lower(m.model_id),lower($3))>0)
 AND ($4='' OR a.availability_state=$4)
 AND (NOT $5 OR p.recommended)
 AND ($6='' OR m.family=$6)
 AND ($7='' OR COALESCE(m.metadata->'tags','[]'::jsonb) ? $7)
 AND ($8='' OR COALESCE(m.metadata->'use_cases','[]'::jsonb) ? $8)
 AND ($9::bigint IS NULL OR m.context_length>=$9)
 AND (NOT $10 OR m.context_length IS NULL)
 AND ($11::text IS NULL OR price.amount>=$11::numeric)
 AND ($12::text IS NULL OR price.amount<=$12::numeric) `

func (s *Store) PublicCatalog(ctx context.Context, filter CatalogFilter, policy CatalogPolicy) (CatalogPage, error) {
	page := CatalogPage{Items: []CatalogModel{}}
	if !policy.valid() || !validateCatalogFilter(&filter) {
		return page, ErrCatalogInvalid
	}
	var err error
	page.Vocabulary, err = catalogVocabulary()
	if err != nil {
		return page, err
	}
	page.Offset = filter.Offset
	page.Limit = filter.Limit
	page.PriceDimension = filter.PriceDimension
	page.PriceUnit = catalogPriceUnit(filter.PriceDimension)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return page, err
	}
	defer rollback(tx)
	status, err := catalogSyncStatus(ctx, tx)
	if err != nil {
		return page, err
	}
	page.Freshness = catalogFreshness(status, policy, time.Now())
	args := []any{filter.PriceDimension, page.PriceUnit, filter.Search, filter.Availability, filter.RecommendedOnly, filter.Family, filter.Tag, filter.UseCase, filter.MinContext, filter.UnknownContext, filter.MinPrice, filter.MaxPrice}
	if err = tx.QueryRow(ctx, "SELECT count(*)"+catalogPublicSelection, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	order := map[string]string{"recommended": `p.recommended DESC,p.sort_order ASC,lower(m.display_name) COLLATE "C",m.model_id COLLATE "C"`, "name": `lower(m.display_name) COLLATE "C",m.model_id COLLATE "C"`, "context": `m.context_length DESC NULLS LAST,lower(m.display_name) COLLATE "C",m.model_id COLLATE "C"`, "price": `price.amount ASC NULLS LAST,lower(m.display_name) COLLATE "C",m.model_id COLLATE "C"`}[filter.Sort]
	rows, err := tx.Query(ctx, "SELECT "+catalogModelColumns+catalogPublicSelection+" ORDER BY "+order+" LIMIT $13 OFFSET $14", append(args, filter.Limit, filter.Offset)...)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		model, e := scanCatalogModel(rows, page.Freshness)
		if e != nil {
			rows.Close()
			return page, e
		}
		if !validCatalogMetadata(model.Metadata) || model.Metadata.DisplayName == "" || model.Metadata.Family == "" || model.Metadata.Summary == "" {
			rows.Close()
			return page, ErrCatalogIncomplete
		}
		page.Items = append(page.Items, model)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return page, err
	}
	return page, tx.Commit(ctx)
}
func (s *Store) OpsCatalog(ctx context.Context, userID int64, filter CatalogOpsFilter, policy CatalogPolicy) (CatalogOpsPage, error) {
	page := CatalogOpsPage{Items: []CatalogModel{}}
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	filter.Search = strings.TrimSpace(filter.Search)
	if !policy.valid() || !catalogSearchValid(filter.Search) || filter.Limit < 1 || filter.Limit > 100 || filter.Offset < 0 || filter.Offset > 1000000 || !slices.Contains([]string{"", "PENDING_METADATA", "PUBLISHED", "HIDDEN", "RETIRED"}, filter.State) {
		return page, ErrCatalogInvalid
	}
	var err error
	page.Vocabulary, err = catalogVocabulary()
	if err != nil {
		return page, err
	}
	page.Offset = filter.Offset
	page.Limit = filter.Limit
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return page, err
	}
	defer rollback(tx)
	page.Principal, err = catalogAuthority(ctx, tx, userID, false)
	if err != nil {
		return page, err
	}
	page.Sync, err = catalogSyncStatus(ctx, tx)
	if err != nil {
		return page, err
	}
	page.Freshness = catalogFreshness(page.Sync, policy, time.Now())
	selection := catalogModelTables + ` WHERE ($1='' OR p.publication_state=$1) AND ($2='' OR strpos(lower(m.display_name),lower($2))>0 OR strpos(lower(m.model_id),lower($2))>0)`
	if err = tx.QueryRow(ctx, "SELECT count(*)"+selection, filter.State, filter.Search).Scan(&page.Total); err != nil {
		return page, err
	}
	rows, err := tx.Query(ctx, "SELECT "+catalogModelColumns+selection+` ORDER BY (p.publication_state='PENDING_METADATA') DESC,greatest(m.updated_at,p.updated_at) DESC,m.model_id COLLATE "C" LIMIT $3 OFFSET $4`, filter.State, filter.Search, filter.Limit, filter.Offset)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		model, e := scanCatalogModel(rows, page.Freshness)
		if e != nil {
			rows.Close()
			return page, e
		}
		page.Items = append(page.Items, model)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return page, err
	}
	return page, tx.Commit(ctx)
}
