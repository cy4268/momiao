package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCatalogPublicFilterSortAndPriceUnits(t *testing.T) {
	s, p, _ := catalogOpsFixture(t)
	ctx := context.Background()
	prefix := "filter-" + announcementID(t)
	ids := []string{prefix + "/a", prefix + "/b", prefix + "/c", prefix + "/d", prefix + "/e", prefix + "/hidden", prefix + "/pending"}
	source := catalogStoreSnapshot(t, ids...)
	for i := range source.Models {
		model := &source.Models[i]
		switch model.ModelID {
		case ids[0]:
			zero := "0"
			model.Price.Dimensions[0].Amount = &zero
		case ids[2]:
			two := "2"
			model.Price.Dimensions[0].Amount = &two
		case ids[3]:
			value, group := "0.001", "1"
			model.Price = CatalogPrice{Mode: "per_request", Configured: true, Status: "conditional", GroupMultiplier: &group, Dimensions: []CatalogDimension{{Kind: "text_request_base", Unit: "API_Credit_per_request", Amount: &value, Source: "native_effective", Condition: catalogDimensionConditions["text_request_base"], Support: "not_asserted"}}, Unquoted: catalogUnquoted}
		case ids[4]:
			model.Price = CatalogPrice{Mode: "tiered_expr", Configured: true, Status: "unquotable", Reason: "expression_requires_usage", Dimensions: []CatalogDimension{}, Unquoted: catalogUnquoted}
		}
	}
	source = catalogReparseSnapshot(t, source)
	if _, err := s.SyncCatalog(ctx, func(context.Context) (NativeCatalog, error) { return source, nil }); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids[:6] {
		_, model, err := s.OpsCatalogModel(ctx, p.UserID, id, catalogTestPolicy)
		if err != nil {
			t.Fatal(err)
		}
		metadata := catalogEditableMetadata()
		metadata.DisplayName = []string{"Alpha", "Beta", "Charlie", "Delta", "Echo", "Hidden"}[i] + " " + prefix
		metadata.Tags = []string{}
		metadata.UseCases = []string{}
		metadata.SpecialPricingNote = "合成审核说明：以原生实际请求计价，当前显示的是条件报价或待核对说明。"
		if i == 1 {
			contextLength := int64(128000)
			metadata.ContextLength = &contextLength
			metadata.Family = "claude"
			metadata.Tags = []string{"coding"}
			metadata.UseCases = []string{"analysis"}
		}
		if i == 2 {
			contextLength := int64(64000)
			metadata.ContextLength = &contextLength
			metadata.Tags = []string{"writing"}
			metadata.UseCases = []string{"writing"}
		}
		c := catalogCommand(t, s, p, model, "SAVE")
		c.Metadata = &metadata
		c.Recommended = i == 1 || i == 2
		if i == 1 {
			c.SortOrder = 2
		}
		catalogConfirm(t, s, p, c)
		_, model, _ = s.OpsCatalogModel(ctx, p.UserID, id, catalogTestPolicy)
		catalogConfirm(t, s, p, catalogCommand(t, s, p, model, "PUBLISH"))
		if i == 5 {
			_, model, _ = s.OpsCatalogModel(ctx, p.UserID, id, catalogTestPolicy)
			catalogConfirm(t, s, p, catalogCommand(t, s, p, model, "HIDE"))
		}
	}
	read := func(filter CatalogFilter) CatalogPage {
		t.Helper()
		filter.Search = prefix
		page, err := s.PublicCatalog(ctx, filter, catalogTestPolicy)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	page := read(CatalogFilter{})
	if page.Total != 5 || len(page.Items) != 5 || page.Items[0].ModelID != ids[2] || page.Items[1].ModelID != ids[1] {
		t.Fatal("published-only recommended projection wrong")
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), "group_multiplier") || strings.Contains(string(raw), ids[5]) || strings.Contains(string(raw), ids[6]) {
		t.Fatal("public DTO leaked internal pricing or unpublished identity")
	}
	prices := read(CatalogFilter{Sort: "price", PriceDimension: "input"})
	for i, id := range ids[:5] {
		if prices.Items[i].ModelID != id {
			t.Fatal("exact same-unit price sort wrong", i, prices.Items[i].ModelID)
		}
	}
	min, max := "0.00000000000000000001", "1"
	filtered := read(CatalogFilter{PriceDimension: "input", MinPrice: &min, MaxPrice: &max})
	if filtered.Total != 1 || filtered.Items[0].ModelID != ids[1] {
		t.Fatal("tiny positive, zero or unknown price range wrong")
	}
	request := read(CatalogFilter{Sort: "price", PriceDimension: "text_request_base"})
	if request.Items[0].ModelID != ids[3] {
		t.Fatal("request price was mixed with token tariffs")
	}
	contexts := read(CatalogFilter{Sort: "context"})
	if contexts.Items[0].ModelID != ids[1] || contexts.Items[1].ModelID != ids[2] {
		t.Fatal("context sort lost null-last semantics")
	}
	floor := int64(64000)
	if got := read(CatalogFilter{MinContext: &floor}); got.Total != 2 {
		t.Fatal("context filter wrong")
	}
	if got := read(CatalogFilter{UnknownContext: true}); got.Total != 3 {
		t.Fatal("unknown context became numeric zero")
	}
	for _, filter := range []CatalogFilter{{Tag: "coding"}, {UseCase: "analysis"}, {Family: "claude"}} {
		if got := read(filter); got.Total != 1 || got.Items[0].ModelID != ids[1] {
			t.Fatal("controlled attribute filter wrong")
		}
	}
	if got := read(CatalogFilter{RecommendedOnly: true}); got.Total != 2 {
		t.Fatal("recommendation filter wrong")
	}
	if got := read(CatalogFilter{Sort: "name", Offset: 1, Limit: 2}); got.Total != 5 || len(got.Items) != 2 || got.Items[0].ModelID != ids[1] {
		t.Fatal("filtered pagination wrong")
	}
	if got := read(CatalogFilter{Offset: 100, Limit: 2}); got.Total != 5 || len(got.Items) != 0 {
		t.Fatal("past-end page lost total")
	}
	if got, err := s.PublicCatalog(ctx, CatalogFilter{Search: "bEtA " + prefix}, catalogTestPolicy); err != nil || got.Total != 1 || got.Items[0].ModelID != ids[1] {
		t.Fatal("case-insensitive display name search wrong", err)
	}
	if got, err := s.PublicCatalog(ctx, CatalogFilter{Search: prefix + "/d"}, catalogTestPolicy); err != nil || got.Total != 1 {
		t.Fatal("opaque ID search wrong", err)
	}
	for _, filter := range []CatalogFilter{{Sort: "price"}, {MinPrice: &max}, {PriceDimension: "total"}, {PriceDimension: "input", MinPrice: &max, MaxPrice: &min}, {Tag: "private-tag"}, {UnknownContext: true, MinContext: &floor}, {Limit: 101}} {
		if _, err := s.PublicCatalog(ctx, filter, catalogTestPolicy); !errors.Is(err, ErrCatalogInvalid) {
			t.Fatal("ambiguous price/unit or unknown filter accepted", err)
		}
	}
	ops, err := s.OpsCatalog(ctx, p.UserID, CatalogOpsFilter{Search: prefix, State: "PENDING_METADATA"}, catalogTestPolicy)
	if err != nil || ops.Total != 1 || ops.Items[0].ModelID != ids[6] {
		t.Fatal("Ops pending list missing", err)
	}
}
func TestCatalogPublishedUnavailableAndStaleRetainDetails(t *testing.T) {
	s, p, model := catalogOpsFixture(t)
	ctx := context.Background()
	metadata := catalogEditableMetadata()
	save := catalogCommand(t, s, p, model, "SAVE")
	save.Metadata = &metadata
	catalogConfirm(t, s, p, save)
	_, model, _ = s.OpsCatalogModel(ctx, p.UserID, model.ModelID, catalogTestPolicy)
	catalogConfirm(t, s, p, catalogCommand(t, s, p, model, "PUBLISH"))
	if _, err := s.pool.Exec(ctx, `UPDATE catalog.model_sync_state SET last_verified_at=clock_timestamp()-interval '11 minutes' WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	stale, err := s.PublicCatalogModel(ctx, model.ModelID, catalogTestPolicy)
	if err != nil || stale.Freshness.State != "STALE" || !stale.CanUse {
		t.Fatal("warning threshold incorrectly removed detail or CTA", err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE catalog.model_sync_state SET last_verified_at=clock_timestamp()-interval '31 minutes' WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	expired, err := s.PublicCatalogModel(ctx, model.ModelID, catalogTestPolicy)
	if err != nil || expired.Freshness.State != "EXPIRED" || expired.CanUse || len(expired.Price.Dimensions) == 0 {
		t.Fatal("expiry did not retain last price and disable use", err)
	}
	empty := catalogStoreSnapshot(t)
	if _, err = s.SyncCatalog(ctx, func(context.Context) (NativeCatalog, error) { return empty, nil }); err != nil {
		t.Fatal(err)
	}
	missing, err := s.PublicCatalogModel(ctx, model.ModelID, catalogTestPolicy)
	if err != nil || missing.AvailabilityState != "NOT_OBSERVED" || missing.CanUse || missing.PublicationState != "PUBLISHED" {
		t.Fatal("absence became retirement or a working CTA", err)
	}
	page, err := s.PublicCatalog(ctx, CatalogFilter{Search: model.ModelID, Availability: "NOT_OBSERVED"}, catalogTestPolicy)
	if err != nil || page.Total != 1 {
		t.Fatal("public unavailable filter wrong", err)
	}
	source := catalogStoreSnapshot(t, model.ModelID)
	source.Models[0].Price = CatalogPrice{Mode: "tiered_expr", Configured: true, Status: "unquotable", Reason: "expression_requires_usage", Dimensions: []CatalogDimension{}, Unquoted: catalogUnquoted}
	source = catalogReparseSnapshot(t, source)
	if _, err = s.SyncCatalog(ctx, func(context.Context) (NativeCatalog, error) { return source, nil }); err != nil {
		t.Fatal(err)
	}
	unquoted, err := s.PublicCatalogModel(ctx, model.ModelID, catalogTestPolicy)
	if err != nil || unquoted.Price.Status != "unquotable" || unquoted.CanUse {
		t.Fatal("price mode change hid published detail or bypassed review", err)
	}
}
