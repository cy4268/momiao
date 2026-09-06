package platform

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

type CatalogCommand struct {
	OperationID            string           `json:"operation_id"`
	Epoch                  int64            `json:"authz_epoch"`
	Action                 string           `json:"action"`
	ModelID                string           `json:"model_id,omitempty"`
	ExpectedVersion        int64            `json:"expected_version,string"`
	ExpectedCatalogVersion int64            `json:"expected_catalog_version,string"`
	Metadata               *CatalogMetadata `json:"metadata,omitempty"`
	Recommended            bool             `json:"recommended"`
	SortOrder              int              `json:"sort_order"`
	Reason                 string           `json:"reason"`
}
type CatalogResult struct {
	OperationID      string             `json:"operation_id"`
	ModelID          string             `json:"model_id,omitempty"`
	Version          int64              `json:"version,string"`
	MetadataVersion  int64              `json:"metadata_version,string"`
	PublicationState string             `json:"publication_state,omitempty"`
	Sync             *CatalogSyncResult `json:"sync,omitempty"`
}
type CatalogImpact struct {
	Action           string        `json:"action"`
	Before           *CatalogModel `json:"before,omitempty"`
	After            *CatalogModel `json:"after,omitempty"`
	CatalogVersion   int64         `json:"catalog_version,string"`
	SourceHash       string        `json:"source_hash,omitempty"`
	ObservedCount    int           `json:"observed_count"`
	NewModels        int           `json:"new_models"`
	MissingPublished int           `json:"missing_published"`
	Effect           string        `json:"effect"`
}
type CatalogPreview struct {
	ID        string        `json:"preview_id"`
	Impact    CatalogImpact `json:"impact"`
	ExpiresAt time.Time     `json:"expires_at"`
}

func catalogPermission(action string) string {
	switch action {
	case "SAVE", "SYNC":
		return "models.write"
	case "PUBLISH", "HIDE", "RETIRE":
		return "models.publish"
	default:
		return ""
	}
}
func validateCatalogCommand(ctx context.Context, tx pgx.Tx, p AnnouncementPrincipal, c CatalogCommand, policy CatalogPolicy) (CatalogModel, CatalogModel, CatalogSyncStatus, error) {
	var before, after CatalogModel
	var status CatalogSyncStatus
	fail := func(err error) (CatalogModel, CatalogModel, CatalogSyncStatus, error) {
		return before, after, status, err
	}
	if !policy.valid() || !announcementUUID(c.OperationID) || catalogPermission(c.Action) == "" || c.ExpectedCatalogVersion < 0 || !catalogPlainText(c.Reason, 500, true) || c.Reason == "" {
		return fail(ErrCatalogInvalid)
	}
	if !slices.Contains(p.Permissions, catalogPermission(c.Action)) {
		return fail(ErrCatalogForbidden)
	}
	if p.Epoch != c.Epoch {
		return fail(ErrAnnouncementStale)
	}
	var err error
	status, err = catalogSyncStatus(ctx, tx)
	if err != nil {
		return fail(err)
	}
	if status.Version != c.ExpectedCatalogVersion {
		return fail(ErrCatalogConflict)
	}
	if c.Action == "SYNC" {
		if c.ModelID != "" || c.ExpectedVersion != 0 || c.Metadata != nil || c.Recommended || c.SortOrder != 0 {
			return fail(ErrCatalogInvalid)
		}
		return before, after, status, nil
	}
	if !ValidCatalogModelID(c.ModelID) || c.ExpectedVersion <= 0 {
		return fail(ErrCatalogInvalid)
	}
	var locked string
	if err = tx.QueryRow(ctx, `SELECT model_id FROM catalog.model_catalog_metadata WHERE model_id=$1 FOR UPDATE`, c.ModelID).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrCatalogNotFound
		}
		return fail(err)
	}
	before, err = loadCatalogModel(ctx, tx, c.ModelID, false, catalogFreshness(status, policy, time.Now()))
	if err != nil {
		return fail(err)
	}
	if before.Version != c.ExpectedVersion {
		return fail(ErrCatalogConflict)
	}
	after = before
	after.Version++
	if c.Action != "SAVE" && (c.Metadata != nil || c.Recommended || c.SortOrder != 0) {
		return fail(ErrCatalogInvalid)
	}
	switch c.Action {
	case "SAVE":
		if c.Metadata == nil || !validCatalogMetadata(*c.Metadata) || c.SortOrder < 0 || c.SortOrder > 1000000 {
			return fail(ErrCatalogInvalid)
		}
		after.Metadata = *c.Metadata
		after.Recommended = c.Recommended
		after.SortOrder = c.SortOrder
		if announcementHash(before.Metadata) != announcementHash(after.Metadata) {
			after.MetadataVersion++
		}
		if before.PublicationState == "PUBLISHED" && !catalogPublishable(after) {
			return fail(ErrCatalogIncomplete)
		}
	case "PUBLISH":
		if before.PublicationState == "PUBLISHED" {
			return fail(ErrCatalogConflict)
		}
		if !catalogPublishable(after) {
			return fail(ErrCatalogIncomplete)
		}
		after.PublicationState = "PUBLISHED"
	case "HIDE":
		if before.PublicationState == "HIDDEN" || before.PublicationState == "RETIRED" {
			return fail(ErrCatalogConflict)
		}
		after.PublicationState = "HIDDEN"
	case "RETIRE":
		if before.PublicationState == "RETIRED" {
			return fail(ErrCatalogConflict)
		}
		after.PublicationState = "RETIRED"
	}
	after.CanUse = catalogCanUse(after)
	return before, after, status, nil
}
func (s *Store) PrepareCatalog(parent context.Context, userID int64, c CatalogCommand, read CatalogSource, policy CatalogPolicy) (CatalogPreview, error) {
	var result CatalogPreview
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer rollback(tx)
	if err = lockCatalog(ctx, tx); err != nil {
		return result, err
	}
	p, err := catalogAuthority(ctx, tx, userID, true)
	if err != nil {
		return result, err
	}
	before, after, status, err := validateCatalogCommand(ctx, tx, p, c, policy)
	if err != nil {
		return result, err
	}
	impact := CatalogImpact{Action: c.Action, CatalogVersion: status.Version}
	var sourceHash *string
	if c.Action == "SYNC" {
		source, err := readCatalogSource(ctx, read)
		if err != nil {
			return result, ErrCatalogSource
		}
		impact.SourceHash = source.Hash
		sourceHash = &impact.SourceHash
		impact.ObservedCount = len(source.Models)
		ids := make([]string, 0, len(source.Models))
		for _, m := range source.Models {
			ids = append(ids, m.ModelID)
		}
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM unnest($1::text[]) id WHERE NOT EXISTS(SELECT 1 FROM catalog.model_catalog_metadata m WHERE m.model_id=id)`, ids).Scan(&impact.NewModels); err != nil {
			return result, err
		}
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM catalog.model_catalog_publication WHERE publication_state='PUBLISHED' AND NOT(model_id=ANY($1::text[]))`, ids).Scan(&impact.MissingPublished); err != nil {
			return result, err
		}
		impact.Effect = "原子更新来源配置与参考价。新模型进入待完善；未观察到的已发布模型保留详情，并暂停接入操作。"
	} else {
		impact.Before = &before
		impact.After = &after
		switch c.Action {
		case "SAVE":
			impact.Effect = "保存展示元数据及推荐排序；已发布模型的公开信息同步更新，历史身份与审计保留。"
		case "PUBLISH":
			impact.Effect = "模型进入统一公开目录、详情、搜索、首页推荐与接入选择。使用操作仍受来源配置和时效约束。"
		case "HIDE":
			impact.Effect = "停止所有公开发现和详情访问，保留元数据、来源与身份历史。"
		case "RETIRE":
			impact.Effect = "记录人工退役并停止公开访问，保留全部历史；再次发布需要重新审核。"
		}
	}
	result.ID, err = uuidV7()
	if err != nil {
		return result, err
	}
	result.Impact = impact
	err = tx.QueryRow(ctx, `INSERT INTO ops.model_previews(preview_id,newapi_user_id,authz_epoch,command_hash,source_hash,impact,expires_at) VALUES($1,$2,$3,$4,$5,$6,clock_timestamp()+interval '10 minutes') RETURNING expires_at`, result.ID, userID, p.Epoch, announcementHash(c), sourceHash, impact).Scan(&result.ExpiresAt)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func (s *Store) ExecuteCatalog(parent context.Context, userID int64, c CatalogCommand, previewID string, confirmed bool, read CatalogSource, policy CatalogPolicy) (CatalogResult, error) {
	var result CatalogResult
	if !announcementUUID(c.OperationID) {
		return result, ErrCatalogInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer rollback(tx)
	if err = lockCatalog(ctx, tx); err != nil {
		return result, err
	}
	p, err := catalogAuthority(ctx, tx, userID, true)
	if err != nil {
		return result, err
	}
	if !slices.Contains(p.Permissions, catalogPermission(c.Action)) {
		return result, ErrCatalogForbidden
	}
	if p.Epoch != c.Epoch {
		return result, ErrAnnouncementStale
	}
	// Same global operation namespace/lock as announcements; a replay may carry
	// stale target versions, but never a different actor, domain or command.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,714622314783630005))", c.OperationID); err != nil {
		return result, err
	}
	hash := announcementHash(c)
	var oldUser int64
	var oldHash, oldAction string
	var oldResult []byte
	err = tx.QueryRow(ctx, `SELECT COALESCE(newapi_user_id,0),request_hash,action,result FROM ops.admin_operations WHERE operation_id=$1`, c.OperationID).Scan(&oldUser, &oldHash, &oldAction, &oldResult)
	if err == nil {
		if oldUser != userID || oldHash != hash || oldAction != "MODEL_"+c.Action {
			return result, ErrCatalogOperation
		}
		if err = json.Unmarshal(oldResult, &result); err != nil {
			return result, err
		}
		return result, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	before, after, _, err := validateCatalogCommand(ctx, tx, p, c, policy)
	if err != nil {
		return result, err
	}
	if !confirmed || !announcementUUID(previewID) {
		return result, ErrCatalogConfirmation
	}
	var previewSourceHash string
	err = tx.QueryRow(ctx, `SELECT COALESCE(source_hash,'') FROM ops.model_previews WHERE preview_id=$1 AND newapi_user_id=$2 AND authz_epoch=$3 AND command_hash=$4 AND expires_at>clock_timestamp()`, previewID, userID, p.Epoch, hash).Scan(&previewSourceHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrCatalogConfirmation
	}
	if err != nil {
		return result, err
	}
	result = CatalogResult{OperationID: c.OperationID, ModelID: c.ModelID, Version: after.Version, MetadataVersion: after.MetadataVersion, PublicationState: after.PublicationState}
	var modelID *string
	if c.Action == "SYNC" {
		source, readErr := readCatalogSource(ctx, read)
		if readErr == nil && source.Hash != previewSourceHash {
			return result, ErrCatalogSourceChanged
		}
		synced, err := syncCatalogInTx(ctx, tx, source, readErr, "OPS")
		if err != nil {
			return result, err
		}
		result.Sync = &synced
	} else {
		modelID = &c.ModelID
		if after.MetadataVersion != before.MetadataVersion {
			if err = saveCatalogMetadata(ctx, tx, userID, before, after); err != nil {
				return result, err
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE catalog.model_catalog_publication SET publication_state=$2,recommended=$3,sort_order=$4,version=version+1,
   published_at=CASE WHEN $5='PUBLISH' THEN clock_timestamp() ELSE published_at END,
   retired_at=CASE WHEN $2='RETIRED' THEN COALESCE(retired_at,clock_timestamp()) ELSE NULL END,updated_at=clock_timestamp()
   WHERE model_id=$1 AND version=$6`, c.ModelID, after.PublicationState, after.Recommended, after.SortOrder, c.Action, c.ExpectedVersion)
		if err != nil {
			return result, err
		}
		if tag.RowsAffected() != 1 {
			return result, ErrCatalogConflict
		}
	}
	details := map[string]any{"authz_epoch": c.Epoch, "preview_id": previewID, "reason": c.Reason, "previous_version": c.ExpectedVersion, "previous_catalog_version": c.ExpectedCatalogVersion, "previous_publication_state": before.PublicationState, "metadata_version": result.MetadataVersion}
	if _, err = tx.Exec(ctx, `INSERT INTO ops.admin_operations(operation_id,actor_kind,newapi_user_id,action,model_id,request_hash,details,result) VALUES($1,'ADMIN',$2,$3,$4,$5,$6,$7)`, c.OperationID, userID, "MODEL_"+c.Action, modelID, hash, details, result); err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CatalogResult{}, err
	}
	return result, nil
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
	tag, err := tx.Exec(ctx, `UPDATE catalog.model_catalog_metadata SET display_name=$2,family=$3,summary=NULLIF($4,''),context_length=$5,metadata=$6,metadata_version=metadata_version+1,updated_at=clock_timestamp() WHERE model_id=$1 AND metadata_version=$7`, after.ModelID, m.DisplayName, m.Family, m.Summary, m.ContextLength, extras, before.MetadataVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrCatalogConflict
	}
	if before.Metadata.DisplayName != m.DisplayName || before.Metadata.Family != m.Family {
		now := time.Now().UTC()
		tag, err = tx.Exec(ctx, `UPDATE catalog.historical_model_identity SET effective_until=$2 WHERE model_id=$1 AND effective_until IS NULL`, after.ModelID, now)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrCatalogConflict
		}
		id, err := uuidV7()
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO catalog.historical_model_identity(historical_identity_id,model_id,display_name_snapshot,family_snapshot,effective_from) VALUES($1,$2,$3,$4,$5)`, id, after.ModelID, m.DisplayName, m.Family, now); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO catalog.model_metadata_revisions(model_id,metadata_version,content,created_by) VALUES($1,$2,$3,$4)`, after.ModelID, after.MetadataVersion, m, user)
	return err
}
