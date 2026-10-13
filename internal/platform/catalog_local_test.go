package platform

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
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
	if _, err = tx.Exec(ctx, `UPDATE relay.prices SET input=250000,output=0 WHERE model='gpt-5.6-sol'`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE relay.groups SET ratio=2 WHERE name='default'`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	fresh := CatalogFreshness{State: "CURRENT", LastObservedAt: &now, LastVerifiedAt: &now}
	read := func(id string) CatalogModel {
		t.Helper()
		m, err := loadCatalogModel(ctx, tx, id, false, fresh)
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
	if err = tx.QueryRow(ctx, relayCostSQL, "gpt-5.6-sol", "default", 1000000, 0, 0, 0, 0, nil).Scan(&cost); err != nil || cost != UnitsPerCredit {
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
	if _, err = loadCatalogModel(ctx, tx, unseen, true, fresh); err != ErrCatalogNotFound {
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
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := s.PublicCatalog(ctx, CatalogFilter{PriceDimension: "input", Sort: "price", Limit: 100}, CatalogPolicy{StaleAfter: 10 * time.Minute, DisableAfter: 30 * time.Minute})
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
