package platform

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

func catalogCoverAuthority(ctx context.Context, tx pgx.Tx, userID, epoch int64, permission string) error {
	if err := lockCatalog(ctx, tx); err != nil {
		return err
	}
	p, err := catalogAuthority(ctx, tx, userID, true)
	if err != nil {
		return err
	}
	if !slices.Contains(p.Permissions, permission) {
		return ErrCatalogForbidden
	}
	if p.Epoch != epoch {
		return ErrAnnouncementStale
	}
	return nil
}
func catalogCoverReplay(ctx context.Context, tx pgx.Tx, userID int64, id, action, hash string, result any) (bool, error) {
	if !announcementUUID(id) {
		return false, ErrCatalogInvalid
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,714622314783630005))", id); err != nil {
		return false, err
	}
	var actor int64
	var oldHash, oldAction string
	var data []byte
	err := tx.QueryRow(ctx, `SELECT COALESCE(newapi_user_id,0),request_hash,action,result FROM ops.admin_operations WHERE operation_id=$1`, id).Scan(&actor, &oldHash, &oldAction, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if actor != userID || hash != oldHash || action != oldAction {
		return false, ErrCatalogOperation
	}
	return true, json.Unmarshal(data, result)
}
func catalogCoverUploadInput(c CatalogCoverUploadCommand, image CatalogCoverUploadImage) (string, string, error) {
	if err := ValidateCatalogCoverUpload(c); err != nil {
		return "", "", err
	}
	key, err := CatalogCoverObjectKey(c.Family, image.SHA256, image.Extension)
	if err != nil || image.ContentType != map[string]string{"png": "image/png", "jpg": "image/jpeg", "webp": "image/webp"}[image.Extension] || image.Size <= 0 || image.Size > 8*1024*1024 || image.Width <= 0 || image.Height <= 0 || image.Width > 8192 || image.Height > 8192 || int64(image.Width)*int64(image.Height) > 16777216 {
		return "", "", ErrCatalogInvalid
	}
	return key, announcementHash(struct {
		Command CatalogCoverUploadCommand
		Image   CatalogCoverUploadImage
	}{c, image}), nil
}

// Read an already committed receipt before any network write. Locks are released
// before returning; a new upload is still reauthorized after its successful PUT.
func (s *Store) CatalogCoverUploadReceipt(parent context.Context, userID int64, c CatalogCoverUploadCommand, image CatalogCoverUploadImage) (*CatalogCoverUploadResult, error) {
	_, hash, err := catalogCoverUploadInput(c, image)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	if err = catalogCoverAuthority(ctx, tx, userID, c.Epoch, "models.write"); err != nil {
		return nil, err
	}
	var result CatalogCoverUploadResult
	found, err := catalogCoverReplay(ctx, tx, userID, c.OperationID, "MODEL_COVER_UPLOAD", hash, &result)
	if err != nil || !found {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Store) RegisterCatalogCoverUpload(parent context.Context, userID int64, c CatalogCoverUploadCommand, image CatalogCoverUploadImage) (CatalogCoverUploadResult, error) {
	var result CatalogCoverUploadResult
	key, hash, err := catalogCoverUploadInput(c, image)
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer rollback(tx)
	if err = catalogCoverAuthority(ctx, tx, userID, c.Epoch, "models.write"); err != nil {
		return result, err
	}
	replay, err := catalogCoverReplay(ctx, tx, userID, c.OperationID, "MODEL_COVER_UPLOAD", hash, &result)
	if err != nil {
		return result, err
	}
	if replay {
		return result, tx.Commit(ctx)
	}
	result.OperationID = c.OperationID
	err = tx.QueryRow(ctx, `SELECT asset_id::text,object_key,alt,width,height,rights_status,rights_note FROM catalog.family_cover_assets WHERE family=$1 AND sha256=$2`, c.Family, image.SHA256).Scan(&result.AssetID, &result.Image.Src, &result.Image.Alt, &result.Image.Width, &result.Image.Height, &result.RightsStatus, &result.RightsNote)
	if errors.Is(err, pgx.ErrNoRows) {
		result.AssetID, err = uuidV7()
		if err != nil {
			return result, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO catalog.family_cover_assets(asset_id,family,sha256,object_key,content_type,size_bytes,width,height,alt,rights_status,rights_note,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, result.AssetID, c.Family, image.SHA256, key, image.ContentType, image.Size, image.Width, image.Height, c.Alt, c.RightsStatus, c.RightsNote, userID)
		result.Image = CatalogCoverImage{Src: key, Alt: c.Alt, Width: image.Width, Height: image.Height}
		result.RightsStatus = c.RightsStatus
		result.RightsNote = c.RightsNote
	} else if err == nil {
		result.Reused = true
	}
	if err != nil {
		return CatalogCoverUploadResult{}, err
	}
	result.Image.AssetID = result.AssetID
	result.Image.Src = "/" + result.Image.Src
	result.Image.FocalPoint = [2]float64{0.5, 0.5}
	details := map[string]any{"family": c.Family, "sha256": image.SHA256, "reason": c.Reason, "authz_epoch": c.Epoch}
	if _, err = tx.Exec(ctx, `INSERT INTO ops.admin_operations(operation_id,actor_kind,newapi_user_id,action,request_hash,details,result) VALUES($1,'ADMIN',$2,'MODEL_COVER_UPLOAD',$3,$4,$5)`, c.OperationID, userID, hash, details, result); err != nil {
		return CatalogCoverUploadResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CatalogCoverUploadResult{}, err
	}
	return result, nil
}

func validateCatalogFamilyCover(ctx context.Context, tx pgx.Tx, c CatalogFamilyCoverCommand) (CatalogFamilyCover, CatalogFamilyCover, error) {
	var before, after CatalogFamilyCover
	if !announcementUUID(c.OperationID) || !catalogChoice(catalogFamilies, c.Family) || !slices.Contains([]string{"SET", "RESET"}, c.Action) || c.ExpectedVersion <= 0 || c.Reason == "" || !catalogPlainText(c.Reason, 500, true) || (c.Action == "SET" && !announcementUUID(c.AssetID)) || (c.Action == "RESET" && c.AssetID != "") {
		return before, after, ErrCatalogInvalid
	}
	covers, err := catalogFamilyCoverMap(ctx, tx)
	if err != nil {
		return before, after, err
	}
	before = covers[c.Family]
	after = before
	if before.Version != c.ExpectedVersion {
		return before, after, ErrCatalogConflict
	}
	after.Version++
	after.Image = after.DefaultImage
	if c.Action == "SET" {
		image := CatalogCoverImage{AssetID: c.AssetID, FocalPoint: [2]float64{0.5, 0.5}}
		err = tx.QueryRow(ctx, `SELECT object_key,alt,width,height FROM catalog.family_cover_assets WHERE family=$1 AND asset_id=$2`, c.Family, c.AssetID).Scan(&image.Src, &image.Alt, &image.Width, &image.Height)
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrCatalogInvalid
		}
		if err != nil {
			return before, after, err
		}
		image.Src = "/" + image.Src
		after.Image = &image
	}
	return before, after, nil
}
func (s *Store) PrepareCatalogFamilyCover(parent context.Context, userID int64, c CatalogFamilyCoverCommand) (CatalogFamilyCoverPreview, error) {
	var result CatalogFamilyCoverPreview
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer rollback(tx)
	if err = catalogCoverAuthority(ctx, tx, userID, c.Epoch, "models.publish"); err != nil {
		return result, err
	}
	result.Before, result.After, err = validateCatalogFamilyCover(ctx, tx, c)
	if err != nil {
		return result, err
	}
	result.ID, err = uuidV7()
	if err != nil {
		return result, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO ops.model_previews(preview_id,newapi_user_id,authz_epoch,command_hash,impact,expires_at) VALUES($1,$2,$3,$4,$5,clock_timestamp()+interval '10 minutes') RETURNING expires_at`, result.ID, userID, c.Epoch, announcementHash(c), result).Scan(&result.ExpiresAt)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func (s *Store) ExecuteCatalogFamilyCover(parent context.Context, userID int64, c CatalogFamilyCoverCommand, previewID string, confirmed bool) (CatalogFamilyCoverResult, error) {
	var result CatalogFamilyCoverResult
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer rollback(tx)
	if err = catalogCoverAuthority(ctx, tx, userID, c.Epoch, "models.publish"); err != nil {
		return result, err
	}
	hash := announcementHash(c)
	replay, err := catalogCoverReplay(ctx, tx, userID, c.OperationID, "MODEL_COVER_"+c.Action, hash, &result)
	if err != nil {
		return result, err
	}
	if replay {
		return result, tx.Commit(ctx)
	}
	before, after, err := validateCatalogFamilyCover(ctx, tx, c)
	if err != nil {
		return result, err
	}
	if !confirmed || !announcementUUID(previewID) {
		return result, ErrCatalogConfirmation
	}
	var exists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ops.model_previews WHERE preview_id=$1 AND newapi_user_id=$2 AND authz_epoch=$3 AND command_hash=$4 AND expires_at>clock_timestamp())`, previewID, userID, c.Epoch, hash).Scan(&exists)
	if err != nil {
		return result, err
	}
	if !exists {
		return result, ErrCatalogConfirmation
	}
	tag, err := tx.Exec(ctx, `UPDATE catalog.family_covers SET asset_id=NULLIF($2,'')::uuid,version=version+1,updated_at=clock_timestamp() WHERE family=$1 AND version=$3`, c.Family, c.AssetID, c.ExpectedVersion)
	if err != nil {
		return result, err
	}
	if tag.RowsAffected() != 1 {
		return result, ErrCatalogConflict
	}
	result = CatalogFamilyCoverResult{OperationID: c.OperationID, Cover: after}
	details := map[string]any{"family": c.Family, "before": before, "after": after, "reason": c.Reason, "authz_epoch": c.Epoch, "preview_id": previewID}
	if _, err = tx.Exec(ctx, `INSERT INTO ops.admin_operations(operation_id,actor_kind,newapi_user_id,action,request_hash,details,result) VALUES($1,'ADMIN',$2,$3,$4,$5,$6)`, c.OperationID, userID, "MODEL_COVER_"+c.Action, hash, details, result); err != nil {
		return CatalogFamilyCoverResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CatalogFamilyCoverResult{}, err
	}
	return result, nil
}
