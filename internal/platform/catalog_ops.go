package platform

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

// Base prices use credits before the account group ratio, not the public
// default-group quote. Convert only in SQL so fractional prices stay exact.
type CatalogPriceSettings struct {
	Input          string  `json:"input"`
	Output         string  `json:"output"`
	CacheRead      string  `json:"cache_read"`
	CacheWrite     string  `json:"cache_write"`
	CacheWrite1h   string  `json:"cache_write_1h"`
	PerRequest     *string `json:"per_request"`
	PerSecond      *string `json:"per_second"`
	DefaultSeconds int     `json:"default_seconds"`
}
type CatalogSettings struct {
	ModelID     string                `json:"model_id"`
	Metadata    CatalogMetadata       `json:"metadata"`
	Public      bool                  `json:"public"`
	Recommended bool                  `json:"recommended"`
	SortOrder   int                   `json:"sort_order"`
	Price       *CatalogPriceSettings `json:"price_settings"`
}
type CatalogPublicationSettings struct {
	ModelIDs []string `json:"model_ids"`
	Public   bool     `json:"public"`
}
type CatalogPricesSettings struct {
	ModelIDs []string              `json:"model_ids"`
	Price    *CatalogPriceSettings `json:"price_settings"`
}
type CatalogOpsDetail struct {
	Item  CatalogModel          `json:"item"`
	Price *CatalogPriceSettings `json:"price_settings"`
}

func readCatalogPrice(ctx context.Context, q announcementQuerier, id string) (*CatalogPriceSettings, error) {
	var p CatalogPriceSettings
	err := q.QueryRow(ctx, `SELECT (input/500000)::text,(output/500000)::text,(cache_read/500000)::text,(cache_write/500000)::text,(cache_write_1h/500000)::text,(per_request/500000)::text,(per_second/500000)::text,default_seconds FROM relay.prices WHERE model=$1`, id).Scan(&p.Input, &p.Output, &p.CacheRead, &p.CacheWrite, &p.CacheWrite1h, &p.PerRequest, &p.PerSecond, &p.DefaultSeconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}
func validCatalogPriceSettings(p CatalogPriceSettings) bool {
	for _, v := range []string{p.Input, p.Output, p.CacheRead, p.CacheWrite, p.CacheWrite1h} {
		if !ValidCatalogDecimal(v) {
			return false
		}
	}
	for _, v := range []*string{p.PerRequest, p.PerSecond} {
		if v != nil && !ValidCatalogDecimal(*v) {
			return false
		}
	}
	return !(p.PerRequest != nil && p.PerSecond != nil) && p.DefaultSeconds >= 0 && p.DefaultSeconds <= 2147483647
}
func (s *Store) SaveCatalog(ctx context.Context, user int64, value CatalogSettings) (CatalogOpsDetail, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CatalogOpsDetail{}, err
	}
	defer rollback(tx)
	result, err := saveCatalog(ctx, tx, user, value)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

// ponytail: rare model/cover writes share the existing catalog lock; use per-model
// locks only if admin editing becomes contended.
func lockCatalog(ctx context.Context, tx pgx.Tx) error {
	return lockIdentity(ctx, tx, "catalog", "sync-publication-v1")
}
func saveCatalog(ctx context.Context, tx pgx.Tx, user int64, v CatalogSettings) (CatalogOpsDetail, error) {
	var result CatalogOpsDetail
	if !ValidCatalogModelID(v.ModelID) || !validCatalogMetadata(v.Metadata) || v.SortOrder < 0 || v.SortOrder > 1000000 || v.Price != nil && !validCatalogPriceSettings(*v.Price) {
		return result, ErrCatalogInvalid
	}
	if err := catalogAdmin(ctx, tx, user, true); err != nil {
		return result, err
	}
	if err := lockCatalog(ctx, tx); err != nil {
		return result, err
	}
	before, err := loadCatalogModel(ctx, tx, v.ModelID, false)
	if err != nil {
		return result, err
	}
	oldPrice, err := readCatalogPrice(ctx, tx, v.ModelID)
	if err != nil {
		return result, err
	}
	after := before
	after.Metadata = v.Metadata
	if v.Public && !catalogPublishable(after) {
		return result, ErrCatalogIncomplete
	}
	if before.MetadataVersion == 0 || announcementHash(before.Metadata) != announcementHash(after.Metadata) {
		after.MetadataVersion = before.MetadataVersion + 1
		if err = saveCatalogMetadata(ctx, tx, user, before, after); err != nil {
			return result, err
		}
	}
	if v.Price != nil {
		err = saveCatalogPrice(ctx, tx, v.ModelID, *v.Price)
		if err != nil {
			return result, err
		}
	}
	if err = saveCatalogPublication(ctx, tx, v.ModelID, v.Public, v.Recommended, v.SortOrder); err != nil {
		return result, err
	}
	result.Item, err = loadCatalogModel(ctx, tx, v.ModelID, false)
	if err != nil {
		return result, err
	}
	result.Price, err = readCatalogPrice(ctx, tx, v.ModelID)
	if err != nil {
		return result, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO ops.audit_log(actor_user_id,action,target_type,target,before_state,after_state) VALUES($1,'MODEL_SAVE','MODEL',$2,$3,$4)`, user, v.ModelID, CatalogOpsDetail{Item: before, Price: oldPrice}, result)
	return result, err
}
func saveCatalogPrice(ctx context.Context, tx pgx.Tx, id string, p CatalogPriceSettings) error {
	_, err := tx.Exec(ctx, `INSERT INTO relay.prices(model,input,output,cache_read,cache_write,cache_write_1h,per_request,per_second,default_seconds)
   VALUES($1,$2::numeric*500000,$3::numeric*500000,$4::numeric*500000,$5::numeric*500000,$6::numeric*500000,$7::numeric*500000,$8::numeric*500000,$9)
   ON CONFLICT(model) DO UPDATE SET input=EXCLUDED.input,output=EXCLUDED.output,cache_read=EXCLUDED.cache_read,cache_write=EXCLUDED.cache_write,cache_write_1h=EXCLUDED.cache_write_1h,per_request=EXCLUDED.per_request,per_second=EXCLUDED.per_second,default_seconds=EXCLUDED.default_seconds`, id, p.Input, p.Output, p.CacheRead, p.CacheWrite, p.CacheWrite1h, p.PerRequest, p.PerSecond, p.DefaultSeconds)
	return err
}

func (s *Store) SaveCatalogPrices(ctx context.Context, user int64, v CatalogPricesSettings) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = saveCatalogPrices(ctx, tx, user, v); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func saveCatalogPrices(ctx context.Context, tx pgx.Tx, user int64, v CatalogPricesSettings) error {
	if len(v.ModelIDs) == 0 || len(v.ModelIDs) > 100 || v.Price == nil || !validCatalogPriceSettings(*v.Price) {
		return ErrCatalogInvalid
	}
	for _, id := range v.ModelIDs {
		if !ValidCatalogModelID(id) {
			return ErrCatalogInvalid
		}
	}
	if err := catalogAdmin(ctx, tx, user, true); err != nil {
		return err
	}
	if err := lockCatalog(ctx, tx); err != nil {
		return err
	}
	ids := slices.Clone(v.ModelIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	before := make(map[string]*CatalogPriceSettings, len(ids))
	for _, id := range ids {
		if _, err := loadCatalogModel(ctx, tx, id, false); err != nil {
			return err
		}
		price, err := readCatalogPrice(ctx, tx, id)
		if err != nil {
			return err
		}
		before[id] = price
		if err = saveCatalogPrice(ctx, tx, id, *v.Price); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `INSERT INTO ops.audit_log(actor_user_id,action,target_type,target,before_state,after_state) VALUES($1,'MODEL_PRICE_SAVE','MODEL','BATCH',$2,$3)`, user, before, CatalogPricesSettings{ModelIDs: ids, Price: v.Price})
	return err
}

func saveCatalogPublication(ctx context.Context, tx pgx.Tx, id string, public, recommended bool, order int) error {
	state := "HIDDEN"
	if public {
		state = "PUBLISHED"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO catalog.model_catalog_publication(model_id) VALUES($1) ON CONFLICT DO NOTHING`, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE catalog.model_catalog_publication SET publication_state=$2,recommended=$3,sort_order=$4,version=version+1,
 published_at=CASE WHEN $2='PUBLISHED' THEN coalesce(published_at,clock_timestamp()) ELSE published_at END,retired_at=NULL,updated_at=clock_timestamp() WHERE model_id=$1`, id, state, recommended, order)
	return err
}
func (s *Store) SaveCatalogPublication(ctx context.Context, user int64, v CatalogPublicationSettings) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = saveCatalogPublicationBatch(ctx, tx, user, v); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func saveCatalogPublicationBatch(ctx context.Context, tx pgx.Tx, user int64, v CatalogPublicationSettings) error {
	if len(v.ModelIDs) == 0 || len(v.ModelIDs) > 100 {
		return ErrCatalogInvalid
	}
	for _, id := range v.ModelIDs {
		if !ValidCatalogModelID(id) {
			return ErrCatalogInvalid
		}
	}
	if err := catalogAdmin(ctx, tx, user, true); err != nil {
		return err
	}
	if err := lockCatalog(ctx, tx); err != nil {
		return err
	}
	ids := slices.Clone(v.ModelIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	before := []CatalogModel{}
	after := []CatalogModel{}
	for _, id := range ids {
		m, err := loadCatalogModel(ctx, tx, id, false)
		if err != nil {
			return err
		}
		if v.Public && !catalogPublishable(m) {
			return ErrCatalogIncomplete
		}
		before = append(before, m)
		if m.MetadataVersion == 0 {
			next := m
			next.MetadataVersion = 1
			if err = saveCatalogMetadata(ctx, tx, user, m, next); err != nil {
				return err
			}
		}
		if err = saveCatalogPublication(ctx, tx, id, v.Public, m.Recommended, m.SortOrder); err != nil {
			return err
		}
		m, err = loadCatalogModel(ctx, tx, id, false)
		if err != nil {
			return err
		}
		after = append(after, m)
	}
	_, err := tx.Exec(ctx, `INSERT INTO ops.audit_log(actor_user_id,action,target_type,target,before_state,after_state) VALUES($1,'MODEL_PUBLICATION_SAVE','MODEL','BATCH',$2,$3)`, user, before, after)
	return err
}

type catalogMetadataExtras struct {
	Subtitle           string   `json:"subtitle"`
	Tags               []string `json:"tags"`
	UseCases           []string `json:"use_cases"`
	SpecialPricingNote string   `json:"special_pricing_note"`
	AssetID            string   `json:"asset_id"`
}

func saveCatalogMetadata(ctx context.Context, tx pgx.Tx, user int64, before, after CatalogModel) error {
	m := after.Metadata
	extras := catalogMetadataExtras{m.Subtitle, m.Tags, m.UseCases, m.SpecialPricingNote, m.AssetID}
	if _, err := tx.Exec(ctx, `INSERT INTO catalog.model_catalog_metadata(model_id) VALUES($1) ON CONFLICT DO NOTHING`, after.ModelID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE catalog.model_catalog_metadata SET display_name=$2,family=$3,summary=NULLIF($4,''),context_length=$5,metadata=$6,metadata_version=$7,updated_at=clock_timestamp() WHERE model_id=$1`, after.ModelID, m.DisplayName, m.Family, m.Summary, m.ContextLength, extras, after.MetadataVersion)
	if err != nil {
		return err
	}
	if before.MetadataVersion == 0 || before.Metadata.DisplayName != m.DisplayName || before.Metadata.Family != m.Family {
		now := time.Now().UTC()
		if before.MetadataVersion != 0 {
			if _, err = tx.Exec(ctx, `UPDATE catalog.historical_model_identity SET effective_until=$2 WHERE model_id=$1 AND effective_until IS NULL`, after.ModelID, now); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO catalog.historical_model_identity(historical_identity_id,model_id,display_name_snapshot,family_snapshot,effective_from) VALUES(gen_random_uuid(),$1,$2,$3,$4)`, after.ModelID, m.DisplayName, m.Family, now); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO catalog.model_metadata_revisions(model_id,metadata_version,content,created_by) VALUES($1,$2,$3,$4)`, after.ModelID, after.MetadataVersion, m, user)
	return err
}
