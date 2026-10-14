package platform

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

func catalogAdmin(ctx context.Context, q announcementQuerier, user int64, lock bool) error {
	query := `SELECT role FROM identity.users WHERE id=$1 AND status=1 AND deleted_at IS NULL`
	if lock {
		query += " FOR SHARE"
	}
	var role int
	err := q.QueryRow(ctx, query, user).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && role < 10 {
		return ErrCatalogForbidden
	}
	return err
}

// Channels are live configuration; editorial metadata never needs a sync to exist.
const catalogModelColumns = `m.model_id,m.display_name,m.family,COALESCE(m.summary,''),m.context_length,m.metadata,m.metadata_version,
 COALESCE(p.publication_state,'PENDING_METADATA'),COALESCE(p.recommended,false),COALESCE(p.sort_order,0),COALESCE(p.version,0),p.published_at,p.retired_at,greatest(m.updated_at,p.updated_at,a.observed_at),
 a.availability_state,a.observed_at,a.observed_at,a.supported,` + catalogLocalPriceJSON
const catalogModelTables = ` FROM (
 SELECT coalesce(meta.model_id,channels.model_id) model_id,coalesce(meta.display_name,'') display_name,coalesce(meta.family,'') family,
 meta.summary,meta.context_length,coalesce(meta.metadata,'{}'::jsonb) metadata,coalesce(meta.metadata_version,0) metadata_version,coalesce(meta.updated_at,now()) updated_at
 FROM catalog.model_catalog_metadata meta FULL JOIN (SELECT DISTINCT unnest(models) model_id FROM relay.channels) channels USING(model_id)
 ) m LEFT JOIN catalog.model_catalog_publication p USING(model_id)
 LEFT JOIN LATERAL (
  SELECT CASE WHEN count(*)=0 THEN 'NO_CHANNEL' WHEN bool_or(c.enabled) THEN 'CONFIGURED' ELSE 'DISABLED' END availability_state,
  coalesce(max(c.updated_at),m.updated_at) observed_at,coalesce(bool_or(c.enabled AND c.type IN (1,14,24,41,43)),false) supported
  FROM relay.channels c WHERE m.model_id=ANY(c.models)
 ) a ON true
 LEFT JOIN relay.prices rp ON rp.model=m.model_id LEFT JOIN relay.groups g ON g.name='default' `

func scanCatalogModel(row pgx.Row) (CatalogModel, error) {
	var m CatalogModel
	var metadata, price []byte
	var supported bool
	m.Metadata.Tags = []string{}
	m.Metadata.UseCases = []string{}
	err := row.Scan(&m.ModelID, &m.Metadata.DisplayName, &m.Metadata.Family, &m.Metadata.Summary, &m.Metadata.ContextLength, &metadata, &m.MetadataVersion, &m.PublicationState, &m.Recommended, &m.SortOrder, &m.Version, &m.PublishedAt, &m.RetiredAt, &m.UpdatedAt, &m.AvailabilityState, &m.SourceObservedAt, &m.LastSeenAt, &supported, &price)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrCatalogNotFound
	}
	if err != nil {
		return m, err
	}
	extras := catalogMetadataExtras{Tags: []string{}, UseCases: []string{}}
	if json.Unmarshal(metadata, &extras) != nil {
		return m, ErrCatalogInvalid
	}
	m.Metadata.Subtitle = extras.Subtitle
	m.Metadata.Tags = extras.Tags
	m.Metadata.UseCases = extras.UseCases
	m.Metadata.SpecialPricingNote = extras.SpecialPricingNote
	m.Metadata.AssetID = extras.AssetID
	m.Price, err = catalogLocalPrice(price)
	if err != nil {
		return m, err
	}
	m.EndpointStatus = "unverified"
	m.Endpoints = []CatalogEndpoint{}
	if supported {
		m.EndpointStatus = "configured_subset_not_health"
		m.Endpoints = []CatalogEndpoint{{Kind: "anthropic", Path: "/v1/messages", Method: "POST"}, {Kind: "openai", Path: "/v1/chat/completions", Method: "POST"}, {Kind: "openai-response", Path: "/v1/responses", Method: "POST"}}
	}
	m.Freshness = localCatalogFreshness()
	m.SourceObservedAt, m.LastSeenAt = *m.Freshness.LastVerifiedAt, *m.Freshness.LastVerifiedAt
	m.CanUse = catalogCanUse(m)
	return m, nil
}
func loadCatalogModel(ctx context.Context, q announcementQuerier, id string, public bool) (CatalogModel, error) {
	if !ValidCatalogModelID(id) {
		return CatalogModel{}, ErrCatalogNotFound
	}
	query := "SELECT " + catalogModelColumns + catalogModelTables + " WHERE m.model_id=$1"
	if public {
		query += " AND p.publication_state='PUBLISHED'"
	}
	m, err := scanCatalogModel(q.QueryRow(ctx, query, id))
	if err == nil && public && (!validCatalogMetadata(m.Metadata) || m.Metadata.DisplayName == "" || m.Metadata.Family == "" || m.Metadata.Summary == "") {
		return CatalogModel{}, ErrCatalogIncomplete
	}
	return m, err
}
func (s *Store) PublicCatalogModel(ctx context.Context, id string) (CatalogModel, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return CatalogModel{}, err
	}
	defer rollback(tx)
	m, err := loadCatalogModel(ctx, tx, id, true)
	if err != nil {
		return m, err
	}
	models := []CatalogModel{m}
	if err = attachCatalogCovers(ctx, tx, models); err != nil {
		return m, err
	}
	return models[0], tx.Commit(ctx)
}
func (s *Store) OpsCatalogModel(ctx context.Context, userID int64, id string) (CatalogOpsDetail, error) {
	var result CatalogOpsDetail
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer rollback(tx)
	if err = catalogAdmin(ctx, tx, userID, false); err != nil {
		return result, err
	}
	result.Item, err = loadCatalogModel(ctx, tx, id, false)
	if err != nil {
		return result, err
	}
	result.Price, err = readCatalogPrice(ctx, tx, id)
	if err != nil {
		return result, err
	}
	models := []CatalogModel{result.Item}
	if err = attachCatalogCovers(ctx, tx, models); err != nil {
		return result, err
	}
	result.Item = models[0]
	return result, tx.Commit(ctx)
}
