package platform

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

var catalogTestPolicy = CatalogPolicy{StaleAfter: 10 * time.Minute, DisableAfter: 30 * time.Minute}

func catalogOpsFixture(t *testing.T) (*Store, AnnouncementPrincipal, CatalogModel) {
	t.Helper()
	s := catalogTestStore(t)
	ctx := context.Background()
	user := time.Now().UnixMicro()
	if err := s.seedAnnouncementPrincipal(ctx, user, "SUPER_ADMIN", false); err != nil {
		t.Fatal(err)
	}
	p, err := s.CatalogAuthority(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	id := "ops/星/" + announcementID(t)
	snapshot := catalogStoreSnapshot(t, id)
	if _, err = s.SyncCatalog(ctx, func(context.Context) (NativeCatalog, error) { return snapshot, nil }); err != nil {
		t.Fatal(err)
	}
	_, model, err := s.OpsCatalogModel(ctx, user, id, catalogTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	return s, p, model
}
func catalogEditableMetadata() CatalogMetadata {
	return CatalogMetadata{DisplayName: "Synthetic Astra", Family: "other", Summary: "Synthetic reviewed description for catalog acceptance.", Subtitle: "Synthetic model", Tags: []string{"coding"}, UseCases: []string{"coding"}}
}
func catalogCommand(t *testing.T, s *Store, p AnnouncementPrincipal, model CatalogModel, action string) CatalogCommand {
	t.Helper()
	status, err := s.CatalogSyncStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return CatalogCommand{OperationID: announcementID(t), Epoch: p.Epoch, Action: action, ModelID: model.ModelID, ExpectedVersion: model.Version, ExpectedCatalogVersion: status.Version, Reason: "Synthetic review"}
}
func catalogConfirm(t *testing.T, s *Store, p AnnouncementPrincipal, c CatalogCommand) CatalogResult {
	t.Helper()
	preview, err := s.PrepareCatalog(context.Background(), p.UserID, c, nil, catalogTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.ExecuteCatalog(context.Background(), p.UserID, c, preview.ID, true, nil, catalogTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestCatalogOpsPublicationLifecycleAndIdentityHistory(t *testing.T) {
	s, p, model := catalogOpsFixture(t)
	ctx := context.Background()
	if _, err := s.PublicCatalogModel(ctx, model.ModelID, catalogTestPolicy); !errors.Is(err, ErrCatalogNotFound) {
		t.Fatal("pending model leaked", err)
	}
	publish := catalogCommand(t, s, p, model, "PUBLISH")
	if _, err := s.PrepareCatalog(ctx, p.UserID, publish, nil, catalogTestPolicy); !errors.Is(err, ErrCatalogIncomplete) {
		t.Fatal("incomplete metadata allowed publication", err)
	}
	metadata := catalogEditableMetadata()
	save := catalogCommand(t, s, p, model, "SAVE")
	save.Metadata = &metadata
	save.Recommended = true
	save.SortOrder = 2
	if _, err := s.ExecuteCatalog(ctx, p.UserID, save, "", false, nil, catalogTestPolicy); !errors.Is(err, ErrCatalogConfirmation) {
		t.Fatal("write bypassed server preview", err)
	}
	saved := catalogConfirm(t, s, p, save)
	if saved.Version != model.Version+1 {
		t.Fatal("save version missing")
	}
	_, model, _ = s.OpsCatalogModel(ctx, p.UserID, model.ModelID, catalogTestPolicy)
	if model.Metadata.DisplayName != "Synthetic Astra" || model.Metadata.ContextLength != nil || model.PublicationState != "PENDING_METADATA" {
		t.Fatal("metadata save invented context or publication")
	}
	publish = catalogCommand(t, s, p, model, "PUBLISH")
	published := catalogConfirm(t, s, p, publish)
	visible, err := s.PublicCatalogModel(ctx, model.ModelID, catalogTestPolicy)
	if err != nil || visible.PublicationState != "PUBLISHED" || !visible.CanUse || !visible.Recommended {
		t.Fatal("published model not discoverable", err)
	}
	if visible.FamilyCover == nil {
		t.Fatal("new public model omitted family cover")
	}
	before := *visible.FamilyCover
	upload := CatalogCoverUploadCommand{OperationID: announcementID(t), Epoch: p.Epoch, Family: "other", Alt: "Test cover", RightsStatus: "ORIGINAL_GENERATED", RightsNote: "Generated acceptance image", Reason: "Review cover"}
	image := CatalogCoverUploadImage{SHA256: strings.Repeat("a", 64), ContentType: "image/png", Extension: "png", Size: 100, Width: 10, Height: 10}
	candidate, err := s.RegisterCatalogCoverUpload(ctx, p.UserID, upload, image)
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := s.PublicCatalogModel(ctx, model.ModelID, catalogTestPolicy)
	if err != nil || !reflect.DeepEqual(unchanged.FamilyCover, &before) {
		t.Fatal("upload published without confirmation", err)
	}
	upload.OperationID = announcementID(t)
	upload.Alt = "Duplicate must retain original caption"
	duplicate, err := s.RegisterCatalogCoverUpload(ctx, p.UserID, upload, image)
	if err != nil || !duplicate.Reused || duplicate.AssetID != candidate.AssetID || duplicate.Image.Alt != candidate.Image.Alt {
		t.Fatal("dedup changed immutable metadata", err)
	}
	image.Width++
	if _, err = s.RegisterCatalogCoverUpload(ctx, p.UserID, upload, image); !errors.Is(err, ErrCatalogOperation) {
		t.Fatal("upload operation rebound", err)
	}
	image.Width--
	set := CatalogFamilyCoverCommand{OperationID: announcementID(t), Epoch: p.Epoch, Action: "SET", Family: "other", AssetID: candidate.AssetID, ExpectedVersion: before.Version, Reason: "Publish family cover"}
	previewCover, err := s.PrepareCatalogFamilyCover(ctx, p.UserID, set)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteCatalogFamilyCover(ctx, p.UserID, set, previewCover.ID, false); !errors.Is(err, ErrCatalogConfirmation) {
		t.Fatal("cover bypassed confirmation", err)
	}
	after, err := s.ExecuteCatalogFamilyCover(ctx, p.UserID, set, previewCover.ID, true)
	if err != nil || after.Cover.Version != before.Version+1 {
		t.Fatal("cover version did not advance", err)
	}
	detail, err := s.PublicCatalogModel(ctx, model.ModelID, catalogTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.PublicCatalog(ctx, CatalogFilter{Search: model.ModelID}, catalogTestPolicy)
	if err != nil || len(list.Items) != 1 || detail.FamilyCover.Image == nil || !reflect.DeepEqual(detail.FamilyCover, list.Items[0].FamilyCover) || detail.FamilyCover.Image.AssetID != candidate.AssetID {
		t.Fatal("public cover diverged", err)
	}
	coverReplay, err := s.ExecuteCatalogFamilyCover(ctx, p.UserID, set, "", false)
	if err != nil || !reflect.DeepEqual(coverReplay, after) {
		t.Fatal("cover receipt changed", err)
	}
	stale := set
	stale.OperationID = announcementID(t)
	if _, err = s.PrepareCatalogFamilyCover(ctx, p.UserID, stale); !errors.Is(err, ErrCatalogConflict) {
		t.Fatal("stale cover version accepted", err)
	}
	changed := set
	changed.Reason = "Rebound"
	if _, err = s.ExecuteCatalogFamilyCover(ctx, p.UserID, changed, previewCover.ID, true); !errors.Is(err, ErrCatalogOperation) {
		t.Fatal("cover operation rebound", err)
	}
	stale.Epoch--
	if _, err = s.ExecuteCatalogFamilyCover(ctx, p.UserID, stale, previewCover.ID, true); !errors.Is(err, ErrAnnouncementStale) {
		t.Fatal("stale epoch accepted", err)
	}
	page, err := s.CatalogFamilyCovers(ctx, p.UserID)
	if err != nil {
		t.Fatal(err)
	}
	for _, cover := range page.Items {
		if cover.Family == "gpt" {
			wrong := set
			wrong.OperationID = announcementID(t)
			wrong.Family = "gpt"
			wrong.ExpectedVersion = cover.Version
			if _, err = s.PrepareCatalogFamilyCover(ctx, p.UserID, wrong); !errors.Is(err, ErrCatalogInvalid) {
				t.Fatal("cross-family asset accepted", err)
			}
		}
	}
	reset := set
	reset.OperationID = announcementID(t)
	reset.Action = "RESET"
	reset.AssetID = ""
	reset.ExpectedVersion = after.Cover.Version
	resetPreview, err := s.PrepareCatalogFamilyCover(ctx, p.UserID, reset)
	if err != nil {
		t.Fatal(err)
	}
	resetResult, err := s.ExecuteCatalogFamilyCover(ctx, p.UserID, reset, resetPreview.ID, true)
	if err != nil || resetResult.Cover.Version != after.Cover.Version+1 || !reflect.DeepEqual(resetResult.Cover.Image, resetResult.Cover.DefaultImage) {
		t.Fatal("reset lost version or default", err)
	}
	coverReplay, err = s.ExecuteCatalogFamilyCover(ctx, p.UserID, set, "", false)
	if err != nil || !reflect.DeepEqual(coverReplay, after) {
		t.Fatal("old confirmed receipt lost after reset", err)
	}
	replay, err := s.ExecuteCatalog(ctx, p.UserID, publish, "", false, nil, catalogTestPolicy)
	if err != nil || replay != published {
		t.Fatal("confirmed operation receipt did not replay", err)
	}
	publish.Reason = "Changed semantics"
	if _, err = s.ExecuteCatalog(ctx, p.UserID, publish, "", false, nil, catalogTestPolicy); !errors.Is(err, ErrCatalogOperation) {
		t.Fatal("operation ID rebound to another request", err)
	}
	metadata.DisplayName = "Synthetic Astra Revised"
	rename := catalogCommand(t, s, p, visible, "SAVE")
	rename.Metadata = &metadata
	catalogConfirm(t, s, p, rename)
	var versions, open int
	var previous string
	if err = s.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE effective_until IS NULL) FROM catalog.historical_model_identity WHERE model_id=$1`, model.ModelID).Scan(&versions, &open); err != nil {
		t.Fatal(err)
	}
	if err = s.pool.QueryRow(ctx, `SELECT display_name_snapshot FROM catalog.historical_model_identity WHERE model_id=$1 AND effective_until IS NOT NULL ORDER BY effective_from DESC LIMIT 1`, model.ModelID).Scan(&previous); err != nil {
		t.Fatal(err)
	}
	if versions != 3 || open != 1 || previous != "Synthetic Astra" {
		t.Fatal("rename overwrote historical identity", versions, open, previous)
	}
	_, model, _ = s.OpsCatalogModel(ctx, p.UserID, model.ModelID, catalogTestPolicy)
	hidden := catalogConfirm(t, s, p, catalogCommand(t, s, p, model, "HIDE"))
	if hidden.PublicationState != "HIDDEN" {
		t.Fatal("hide failed")
	}
	if _, err = s.PublicCatalogModel(ctx, model.ModelID, catalogTestPolicy); !errors.Is(err, ErrCatalogNotFound) {
		t.Fatal("hidden model leaked", err)
	}
	_, model, _ = s.OpsCatalogModel(ctx, p.UserID, model.ModelID, catalogTestPolicy)
	retired := catalogConfirm(t, s, p, catalogCommand(t, s, p, model, "RETIRE"))
	if retired.PublicationState != "RETIRED" {
		t.Fatal("retire failed")
	}
	if _, err = s.PublicCatalogModel(ctx, model.ModelID, catalogTestPolicy); !errors.Is(err, ErrCatalogNotFound) {
		t.Fatal("retired model leaked", err)
	}
	if _, err = s.PublicCatalogModel(ctx, "not-a-real-published-model", catalogTestPolicy); !errors.Is(err, ErrCatalogNotFound) {
		t.Fatal("unpublished identities have inconsistent not-found")
	}
}
func TestCatalogOpsRolesScopeEpochAndPreviewBinding(t *testing.T) {
	s, p, model := catalogOpsFixture(t)
	ctx := context.Background()
	nativeAdminOnly := p.UserID + 1
	if _, err := s.CatalogAuthority(ctx, nativeAdminOnly); !errors.Is(err, ErrCatalogForbidden) {
		t.Fatal("native role implicitly authorized platform Ops", err)
	}
	operator := p.UserID + 2
	if err := s.seedAnnouncementPrincipal(ctx, operator, "OPERATOR", true); err != nil {
		t.Fatal(err)
	} // ANNOUNCEMENTS only
	if _, err := s.CatalogAuthority(ctx, operator); !errors.Is(err, ErrCatalogForbidden) {
		t.Fatal("announcements scope authorized Models", err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO ops.admin_principal_scopes(admin_principal_id,scope) SELECT admin_principal_id,'MODELS' FROM ops.admin_principals WHERE newapi_user_id=$1`, operator); err != nil {
		t.Fatal(err)
	}
	op, err := s.CatalogAuthority(ctx, operator)
	if err != nil {
		t.Fatal(err)
	}
	c := catalogCommand(t, s, op, model, "SAVE")
	metadata := catalogEditableMetadata()
	c.Metadata = &metadata
	preview, err := s.PrepareCatalog(ctx, op.UserID, c, nil, catalogTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	other := c
	other.Epoch = p.Epoch
	if _, err = s.ExecuteCatalog(ctx, p.UserID, other, preview.ID, true, nil, catalogTestPolicy); !errors.Is(err, ErrCatalogConfirmation) {
		t.Fatal("cross-account preview reused", err)
	}
	stale := c
	stale.Epoch--
	if _, err = s.ExecuteCatalog(ctx, op.UserID, stale, preview.ID, true, nil, catalogTestPolicy); !errors.Is(err, ErrAnnouncementStale) {
		t.Fatal("stale security epoch accepted", err)
	}
	if _, err = s.pool.Exec(ctx, `DELETE FROM ops.admin_principal_scopes WHERE admin_principal_id=(SELECT admin_principal_id FROM ops.admin_principals WHERE newapi_user_id=$1) AND scope='MODELS'`, op.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteCatalog(ctx, op.UserID, c, preview.ID, true, nil, catalogTestPolicy); !errors.Is(err, ErrCatalogForbidden) {
		t.Fatal("revoked scope still authorized preview", err)
	}
	auditor := p.UserID + 3
	if err = s.seedAnnouncementPrincipal(ctx, auditor, "AUDITOR", false); err != nil {
		t.Fatal(err)
	}
	audit, err := s.CatalogAuthority(ctx, auditor)
	if err != nil || len(audit.Permissions) != 1 || audit.Permissions[0] != "models.read" {
		t.Fatal("auditor read authority wrong", err)
	}
	c.Epoch = audit.Epoch
	if _, err = s.PrepareCatalog(ctx, auditor, c, nil, catalogTestPolicy); !errors.Is(err, ErrCatalogForbidden) {
		t.Fatal("auditor edited catalog", err)
	}
}
func TestCatalogMetadataRejectsUnsafeValuesAndRequiresSpecialPriceExplanation(t *testing.T) {
	s, p, model := catalogOpsFixture(t)
	ctx := context.Background()
	for name, modify := range map[string]func(*CatalogMetadata){
		"html":             func(m *CatalogMetadata) { m.Summary = "<script>private()</script>" },
		"url":              func(m *CatalogMetadata) { m.AssetID = "https://unreviewed.invalid/image.webp" },
		"unapproved_asset": func(m *CatalogMetadata) { m.AssetID = "selected-not-production-ready" },
		"unknown_tag":      func(m *CatalogMetadata) { m.Tags = []string{"arbitrary_html_tag"} },
		"duplicate_tag":    func(m *CatalogMetadata) { m.Tags = []string{"coding", "coding"} },
		"unknown_use":      func(m *CatalogMetadata) { m.UseCases = []string{"unreviewed_use"} },
		"fake_context":     func(m *CatalogMetadata) { v := int64(-1); m.ContextLength = &v },
		"oversize_name":    func(m *CatalogMetadata) { m.DisplayName = strings.Repeat("x", 121) },
	} {
		t.Run(name, func(t *testing.T) {
			metadata := catalogEditableMetadata()
			modify(&metadata)
			c := catalogCommand(t, s, p, model, "SAVE")
			c.Metadata = &metadata
			if _, err := s.PrepareCatalog(ctx, p.UserID, c, nil, catalogTestPolicy); !errors.Is(err, ErrCatalogInvalid) {
				t.Fatal("unsafe metadata accepted", err)
			}
		})
	}
	// Source price changes remain separate from editorial identity and publication.
	snapshot := catalogStoreSnapshot(t, model.ModelID)
	snapshot.Models[0].Price = CatalogPrice{Mode: "tiered_expr", Configured: true, Status: "unquotable", Reason: "expression_requires_usage", Dimensions: []CatalogDimension{}, Unquoted: append([]string{}, catalogUnquoted...)}
	// Re-enter the actual reader boundary after constructing a synthetic source.
	snapshot = catalogReparseSnapshot(t, snapshot)
	if _, err := s.SyncCatalog(ctx, func(context.Context) (NativeCatalog, error) { return snapshot, nil }); err != nil {
		t.Fatal(err)
	}
	metadata := catalogEditableMetadata()
	c := catalogCommand(t, s, p, model, "SAVE")
	c.Metadata = &metadata
	catalogConfirm(t, s, p, c)
	_, model, _ = s.OpsCatalogModel(ctx, p.UserID, model.ModelID, catalogTestPolicy)
	if _, err := s.PrepareCatalog(ctx, p.UserID, catalogCommand(t, s, p, model, "PUBLISH"), nil, catalogTestPolicy); !errors.Is(err, ErrCatalogIncomplete) {
		t.Fatal("unquotable model published without explanation", err)
	}
	metadata.SpecialPricingNote = "按实际请求内容使用原生表达式计价，当前不提供固定报价，请在调用前核对。"
	c = catalogCommand(t, s, p, model, "SAVE")
	c.Metadata = &metadata
	catalogConfirm(t, s, p, c)
	_, model, _ = s.OpsCatalogModel(ctx, p.UserID, model.ModelID, catalogTestPolicy)
	catalogConfirm(t, s, p, catalogCommand(t, s, p, model, "PUBLISH"))
	visible, err := s.PublicCatalogModel(ctx, model.ModelID, catalogTestPolicy)
	if err != nil || visible.Price.Status != "unquotable" || len(visible.Price.Dimensions) != 0 || visible.Metadata.SpecialPricingNote == "" {
		t.Fatal("unquotable price replaced by synthetic zero", err)
	}
}
