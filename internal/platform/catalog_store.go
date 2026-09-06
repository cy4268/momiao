package platform

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func catalogAuthority(ctx context.Context, q announcementQuerier, userID int64, lock bool) (AnnouncementPrincipal, error) {
	p, err := opsDomainAuthority(ctx, q, userID, lock, "MODELS", "models")
	if errors.Is(err, ErrAnnouncementForbidden) {
		err = ErrCatalogForbidden
	}
	return p, err
}
func (s *Store) CatalogAuthority(ctx context.Context, userID int64) (AnnouncementPrincipal, error) {
	return catalogAuthority(ctx, s.pool, userID, false)
}

const catalogModelColumns = `m.model_id,m.display_name,m.family,COALESCE(m.summary,''),m.context_length,m.metadata,m.metadata_version,
 p.publication_state,p.recommended,p.sort_order,p.version,p.published_at,p.retired_at,greatest(m.updated_at,p.updated_at),
 a.availability_state,a.observed_at,a.last_seen_at,a.source_facts`
const catalogModelTables = ` FROM catalog.model_catalog_metadata m JOIN catalog.model_catalog_publication p USING(model_id) JOIN catalog.model_availability_mappings a USING(model_id) `

func scanCatalogModel(row pgx.Row, fresh CatalogFreshness) (CatalogModel, error) {
	var m CatalogModel
	var metadata, facts []byte
	m.Metadata.Tags = []string{}
	m.Metadata.UseCases = []string{}
	err := row.Scan(&m.ModelID, &m.Metadata.DisplayName, &m.Metadata.Family, &m.Metadata.Summary, &m.Metadata.ContextLength, &metadata, &m.MetadataVersion, &m.PublicationState, &m.Recommended, &m.SortOrder, &m.Version, &m.PublishedAt, &m.RetiredAt, &m.UpdatedAt, &m.AvailabilityState, &m.SourceObservedAt, &m.LastSeenAt, &facts)
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
	var source NativeCatalogModel
	if !decodeCatalogJSON(facts, &source) || source.ModelID != m.ModelID || !validNativeCatalogModel(source) {
		return m, ErrCatalogSource
	}
	m.EndpointStatus = source.EndpointStatus
	m.Endpoints = source.Endpoints
	m.Price = catalogPublicPrice(source.Price)
	m.Freshness = fresh
	m.CanUse = catalogCanUse(m)
	return m, nil
}
func loadCatalogModel(ctx context.Context, q announcementQuerier, id string, public bool, fresh CatalogFreshness) (CatalogModel, error) {
	if !ValidCatalogModelID(id) {
		return CatalogModel{}, ErrCatalogNotFound
	}
	query := "SELECT " + catalogModelColumns + catalogModelTables + " WHERE m.model_id=$1"
	if public {
		query += " AND p.publication_state='PUBLISHED'"
	}
	m, err := scanCatalogModel(q.QueryRow(ctx, query, id), fresh)
	if err == nil && public && (!validCatalogMetadata(m.Metadata) || m.Metadata.DisplayName == "" || m.Metadata.Family == "" || m.Metadata.Summary == "") {
		return CatalogModel{}, ErrCatalogIncomplete
	}
	return m, err
}
func (s *Store) PublicCatalogModel(ctx context.Context, id string, policy CatalogPolicy) (CatalogModel, error) {
	if !policy.valid() {
		return CatalogModel{}, ErrCatalogInvalid
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return CatalogModel{}, err
	}
	defer rollback(tx)
	status, err := catalogSyncStatus(ctx, tx)
	if err != nil {
		return CatalogModel{}, err
	}
	m, err := loadCatalogModel(ctx, tx, id, true, catalogFreshness(status, policy, time.Now()))
	if err != nil {
		return m, err
	}
	return m, tx.Commit(ctx)
}
func (s *Store) OpsCatalogModel(ctx context.Context, userID int64, id string, policy CatalogPolicy) (AnnouncementPrincipal, CatalogModel, error) {
	var p AnnouncementPrincipal
	var m CatalogModel
	if !policy.valid() {
		return p, m, ErrCatalogInvalid
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return p, m, err
	}
	defer rollback(tx)
	p, err = catalogAuthority(ctx, tx, userID, false)
	if err != nil {
		return p, m, err
	}
	status, err := catalogSyncStatus(ctx, tx)
	if err != nil {
		return p, m, err
	}
	m, err = loadCatalogModel(ctx, tx, id, false, catalogFreshness(status, policy, time.Now()))
	if err != nil {
		return p, m, err
	}
	return p, m, tx.Commit(ctx)
}
