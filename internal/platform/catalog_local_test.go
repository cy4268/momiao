package platform

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Uses imported dev data. Price/group/channel edits stay in one rolled-back transaction.
func TestLocalCatalog(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires the existing dev database")
	}
	dsn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read dev DSN")
	}
	s, err := Open(t.Context(), strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("open dev database")
	}
	defer s.Close()
	ctx := t.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	price := func(model string) json.RawMessage {
		t.Helper()
		value, err := readRelayPrice(ctx, tx, model, "default")
		if err != nil {
			t.Fatal("read current price", err)
		}
		return value
	}
	if _, err = tx.Exec(ctx, `UPDATE relay.prices SET input=250000,output=0 WHERE model='gpt-5.6-sol'`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE relay.groups SET ratio=2 WHERE name='default'`); err != nil {
		t.Fatal(err)
	}
	read := func(id string) CatalogModel {
		t.Helper()
		m, err := loadCatalogModel(ctx, tx, id, false)
		if err != nil {
			t.Fatal("local catalog read", id, err)
		}
		return m
	}
	amount := func(m CatalogModel, kind string) string {
		t.Helper()
		for _, d := range m.Price.Dimensions {
			if d.Kind == kind && d.Amount != nil {
				return *d.Amount
			}
		}
		t.Fatal("missing local price dimension", kind)
		return ""
	}
	m := read("gpt-5.6-sol")
	if CompareCatalogDecimal(amount(m, "input"), "1") != 0 || CompareCatalogDecimal(amount(m, "output"), "0") != 0 {
		t.Fatal("catalog did not read live relay price/group", amount(m, "input"), amount(m, "output"))
	}
	if m.AvailabilityState != "CONFIGURED" || len(m.Endpoints) != 3 || m.Freshness.State != "CURRENT" {
		t.Fatal("local channel configuration", m.AvailabilityState, len(m.Endpoints), m.Freshness.State)
	}
	var cost int64
	if err = tx.QueryRow(ctx, relayCostSQL, price("gpt-5.6-sol"), 1000000, 0, 0, 0, 0, nil).Scan(&cost); err != nil || cost != UnitsPerCredit {
		t.Fatal("display/settlement units diverged", cost, err)
	}
	image := read("grok-imagine-image-2.0")
	if image.Price.Mode != "per_request" || CompareCatalogDecimal(amount(image, "text_request_base"), "0.08") != 0 {
		t.Fatal("local request price", image.Price.Mode)
	}
	video := read("grok-imagine-video")
	videoPrice, _ := json.Marshal(video.Price)
	if !strings.Contains(string(videoPrice), `"default_seconds":8`) || video.Price.Mode != "per_second" || CompareCatalogDecimal(amount(video, "second"), "0.14") != 0 {
		t.Fatal("local duration price", video.Price.Mode)
	}
	var unseen string
	if err = tx.QueryRow(ctx, `SELECT model FROM relay.channels,unnest(models) model WHERE NOT EXISTS(SELECT 1 FROM catalog.model_catalog_metadata WHERE model_id=model) ORDER BY model LIMIT 1`).Scan(&unseen); err != nil {
		t.Fatal("existing channel-only model", err)
	}
	unreviewed := read(unseen)
	if unreviewed.PublicationState != "PENDING_METADATA" || unreviewed.CanUse {
		t.Fatal("channel-only model was published automatically")
	}
	if _, err = loadCatalogModel(ctx, tx, unseen, true); err != ErrCatalogNotFound {
		t.Fatal("channel-only model exposed publicly", err)
	}
	unpriced := read("workbuddy/glm-5.3")
	if unpriced.Price.Configured || unpriced.Price.Status != "unquotable" || len(unpriced.Price.Dimensions) != 0 {
		t.Fatal("missing price became free", unpriced.Price)
	}
	unpriced.PublicationState = "PUBLISHED"
	unpriced.Metadata = CatalogMetadata{DisplayName: "Local model", Family: "glm", Summary: "Local price required", Tags: []string{}, UseCases: []string{}, SpecialPricingNote: "This note does not replace a configured local price."}
	if catalogCanUse(unpriced) {
		t.Fatal("editorial pricing note bypassed local price requirement")
	}
	if _, err = tx.Exec(ctx, `UPDATE relay.channels SET enabled=false WHERE 'gpt-5.6-sol'=ANY(models)`); err != nil {
		t.Fatal(err)
	}
	if disabled := read("gpt-5.6-sol"); disabled.AvailabilityState != "DISABLED" || len(disabled.Endpoints) != 0 || disabled.CanUse {
		t.Fatal("disabled channel still usable")
	}
	if _, err = tx.Exec(ctx, `UPDATE relay.channels SET models=array_remove(models,'gpt-5.6-sol') WHERE 'gpt-5.6-sol'=ANY(models)`); err != nil {
		t.Fatal(err)
	}
	missing := read("gpt-5.6-sol")
	if missing.AvailabilityState != "NO_CHANNEL" || missing.CanUse || missing.ModelID != "gpt-5.6-sol" {
		t.Fatal("missing channel lost metadata or stayed usable")
	}
	raw, _ := json.Marshal(m)
	for _, secret := range []string{"api_key", "base_url", "group_multiplier", "native_effective"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("catalog leaked channel/internal price field", secret)
		}
	}

	// Reuse the imported channel-only model, existing users and existing cover asset.
	// No new account or synthetic data fixture is needed for direct Ops writes.
	var admin, regular int64
	if err = tx.QueryRow(ctx, `SELECT min(id) FILTER(WHERE role>=10),min(id) FILTER(WHERE role<10) FROM identity.users WHERE status=1 AND deleted_at IS NULL`).Scan(&admin, &regular); err != nil {
		t.Fatal(err)
	}
	var audits int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM ops.audit_log`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	v := CatalogSettings{ModelID: unseen, Metadata: CatalogMetadata{DisplayName: unseen, Family: "other", Summary: "本地渠道模型", Tags: []string{}, UseCases: []string{}}, Public: true, Recommended: true, SortOrder: 7,
		Price: &CatalogPriceSettings{Input: "0.5", Output: "0", CacheRead: "0", CacheWrite: "0", CacheWrite1h: "0"}}
	if _, err = saveCatalog(ctx, tx, regular, v); err != ErrCatalogForbidden {
		t.Fatal("ordinary user saved catalog", err)
	}
	saved, err := saveCatalog(ctx, tx, admin, v)
	if err != nil {
		t.Fatal("direct model save", err)
	}
	if saved.Item.PublicationState != "PUBLISHED" || saved.Item.MetadataVersion != 1 || !saved.Item.Recommended || saved.Item.SortOrder != 7 || CompareCatalogDecimal(saved.Price.Input, "0.5") != 0 || CompareCatalogDecimal(amount(saved.Item, "input"), "1") != 0 {
		t.Fatal("direct save did not preserve base/group price", saved)
	}
	if err = tx.QueryRow(ctx, relayCostSQL, price(unseen), 1000000, 0, 0, 0, 0, nil).Scan(&cost); err != nil || cost != 500000 {
		t.Fatal("new price not used by next calculation", cost, err)
	}
	if _, err = loadCatalogModel(ctx, tx, unseen, true); err != nil {
		t.Fatal("direct publication not visible", err)
	}
	v.Metadata.DisplayName = "已更新本地模型"
	v.Price.Input = "0.000000000000000002"
	requestPrice := "0.25"
	v.Price.PerRequest = &requestPrice
	saved, err = saveCatalog(ctx, tx, admin, v)
	if err != nil {
		t.Fatal(err)
	}
	if CompareCatalogDecimal(saved.Price.Input, v.Price.Input) != 0 || saved.Item.Price.Mode != "per_request" {
		t.Fatal("exact tiny price/request mode lost")
	}
	if err = tx.QueryRow(ctx, relayCostSQL, price(unseen), 1, 0, 0, 0, 0, nil).Scan(&cost); err != nil || cost != 250000 {
		t.Fatal("request price not used", cost, err)
	}
	v.Price.PerRequest = nil
	second := "0.1"
	v.Price.PerSecond = &second
	v.Price.DefaultSeconds = 8
	saved, err = saveCatalog(ctx, tx, admin, v)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Item.Price.Mode != "per_second" || saved.Item.Price.DefaultSeconds != 8 {
		t.Fatal("duration settings lost")
	}
	if err = tx.QueryRow(ctx, relayCostSQL, price(unseen), 1, 0, 0, 0, 0, nil).Scan(&cost); err != nil || cost != 800000 {
		t.Fatal("duration price not used", cost, err)
	}
	var history int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM catalog.historical_model_identity WHERE model_id=$1`, unseen).Scan(&history); err != nil || history != 2 || saved.Item.MetadataVersion != 2 {
		t.Fatal("metadata history not preserved", history, saved.Item.MetadataVersion, err)
	}
	var publishedID string
	if err = tx.QueryRow(ctx, `SELECT model_id FROM catalog.model_catalog_publication WHERE publication_state='PUBLISHED' AND model_id<>$1 ORDER BY model_id LIMIT 1`, unseen).Scan(&publishedID); err != nil {
		t.Fatal(err)
	}
	ids := []string{unseen, publishedID}
	if err = saveCatalogPublicationBatch(ctx, tx, admin, CatalogPublicationSettings{ModelIDs: ids}); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if _, err = loadCatalogModel(ctx, tx, id, true); err != ErrCatalogNotFound {
			t.Fatal("bulk hide not immediate", id, err)
		}
	}
	if err = saveCatalogPublicationBatch(ctx, tx, admin, CatalogPublicationSettings{ModelIDs: ids, Public: true}); err != nil {
		t.Fatal(err)
	}
	// An invalid member rolls back the whole batch, including earlier valid members.
	nested, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = saveCatalogPublicationBatch(ctx, nested, admin, CatalogPublicationSettings{ModelIDs: []string{unseen, "zzzz/absent-model"}}); err != ErrCatalogNotFound {
		t.Fatal("invalid batch target", err)
	}
	if err = nested.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = loadCatalogModel(ctx, tx, unseen, true); err != nil {
		t.Fatal("batch left partial visibility", err)
	}
	// Price-only batches preserve metadata and visibility, and share single-save units.
	beforePriceBatch := read(unseen)
	prices := CatalogPricesSettings{ModelIDs: ids, Price: &CatalogPriceSettings{Input: "0.125", Output: "0", CacheRead: "0", CacheWrite: "0", CacheWrite1h: "0"}}
	if err = saveCatalogPrices(ctx, tx, regular, prices); err != ErrCatalogForbidden {
		t.Fatal("ordinary user saved batch prices", err)
	}
	if err = saveCatalogPrices(ctx, tx, admin, prices); err != nil {
		t.Fatal("batch prices", err)
	}
	for _, id := range ids {
		p, err := readCatalogPrice(ctx, tx, id)
		if err != nil || p == nil || CompareCatalogDecimal(p.Input, "0.125") != 0 || p.PerSecond != nil || p.PerRequest != nil {
			t.Fatal("batch base price/mode", id, p, err)
		}
	}
	if next := read(unseen); next.MetadataVersion != beforePriceBatch.MetadataVersion || next.PublicationState != beforePriceBatch.PublicationState || next.Recommended != beforePriceBatch.Recommended || next.SortOrder != beforePriceBatch.SortOrder {
		t.Fatal("price-only batch changed editorial settings")
	}
	if err = tx.QueryRow(ctx, relayCostSQL, price(unseen), 1000000, 0, 0, 0, 0, nil).Scan(&cost); err != nil || cost != 125000 {
		t.Fatal("batch price/group applied more than once", cost, err)
	}
	nested, err = tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	prices.ModelIDs = []string{unseen, "zzzz/absent-model"}
	prices.Price.Input = "999"
	if err = saveCatalogPrices(ctx, nested, admin, prices); err != ErrCatalogNotFound {
		t.Fatal("invalid batch price target", err)
	}
	if err = nested.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if p, err := readCatalogPrice(ctx, tx, unseen); err != nil || CompareCatalogDecimal(p.Input, "0.125") != 0 {
		t.Fatal("failed batch left partial price", p, err)
	}
	var c CatalogCoverUploadCommand
	var file CatalogCoverUploadImage
	var assetID string
	if err = tx.QueryRow(ctx, `SELECT asset_id::text,family,alt,rights_status,rights_note,sha256,content_type,size_bytes,width,height FROM catalog.family_cover_assets ORDER BY created_at LIMIT 1`).Scan(&assetID, &c.Family, &c.Alt, &c.RightsStatus, &c.RightsNote, &file.SHA256, &file.ContentType, &file.Size, &file.Width, &file.Height); err != nil {
		t.Fatal("existing cover", err)
	}
	file.Extension = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}[file.ContentType]
	reused, err := registerCatalogCoverUpload(ctx, tx, admin, c, file)
	if err != nil || !reused.Reused || reused.AssetID != assetID {
		t.Fatal("cover content hash reuse", err)
	}
	cover, err := saveCatalogFamilyCover(ctx, tx, admin, CatalogFamilyCoverSettings{Family: c.Family, AssetID: assetID})
	if err != nil || cover.Image.AssetID != assetID {
		t.Fatal("direct cover binding", err)
	}
	cover, err = saveCatalogFamilyCover(ctx, tx, admin, CatalogFamilyCoverSettings{Family: c.Family})
	if err != nil || cover.Image.AssetID != cover.DefaultImage.AssetID {
		t.Fatal("cover default restore", err)
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM ops.audit_log`).Scan(&count); err != nil || count != audits+8 {
		t.Fatal("one audit per direct save; reused upload/no failed batch audit", count-audits, err)
	}
	t.Log("direct saves: base 0.5/group2 -> cost500000; request250000; 8-second800000; exact tiny decimal; new metadata/history; batch hide/publish; failed batch atomic; existing cover reuse/bind/reset; batch price-only save/rollback, 8 audits in rollback transaction")
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	unpricedPage, err := s.OpsCatalog(ctx, admin, CatalogOpsFilter{State: "UNPRICED"})
	if err != nil || unpricedPage.Limit != 10 || unpricedPage.Total == 0 || len(unpricedPage.Items) > 10 {
		t.Fatal("missing-price filter/default pagination", unpricedPage.Total, unpricedPage.Limit, err)
	}
	for _, item := range unpricedPage.Items {
		if item.Price.Configured {
			t.Fatal("priced model included in missing-price filter", item.ModelID)
		}
	}
	page, err := s.PublicCatalog(ctx, CatalogFilter{PriceDimension: "input", Sort: "price", Limit: 100})
	if err != nil || page.Total != 9 || len(page.Items) != 9 || page.Freshness.State != "CURRENT" {
		t.Fatal("local published catalog", page.Total, page.Freshness.State, err)
	}
	var previous string
	for _, item := range page.Items {
		for _, d := range item.Price.Dimensions {
			if d.Kind == "input" {
				if previous != "" && CompareCatalogDecimal(previous, *d.Amount) > 0 {
					t.Fatal("local price sort")
				}
				previous = *d.Amount
			}
		}
	}
	t.Log("live local prices/channels, credit units, publication boundary, missing price, disabled/missing channels and public sort verified; transaction rolled back")
}
