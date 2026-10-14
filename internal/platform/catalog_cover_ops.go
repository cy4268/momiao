package platform

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func catalogCoverUploadInput(c CatalogCoverUploadCommand, image CatalogCoverUploadImage) (string, error) {
	if err := ValidateCatalogCoverUpload(c); err != nil {
		return "", err
	}
	key, err := CatalogCoverObjectKey(c.Family, image.SHA256, image.Extension)
	if err != nil || image.ContentType != map[string]string{"png": "image/png", "jpg": "image/jpeg", "webp": "image/webp"}[image.Extension] || image.Size <= 0 || image.Size > 8*1024*1024 || image.Width <= 0 || image.Height <= 0 || image.Width > 8192 || image.Height > 8192 || int64(image.Width)*int64(image.Height) > 16777216 {
		return "", ErrCatalogInvalid
	}
	return key, nil
}

func (s *Store) RegisterCatalogCoverUpload(ctx context.Context, user int64, c CatalogCoverUploadCommand, image CatalogCoverUploadImage) (CatalogCoverUploadResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CatalogCoverUploadResult{}, err
	}
	defer rollback(tx)
	result, err := registerCatalogCoverUpload(ctx, tx, user, c, image)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func registerCatalogCoverUpload(ctx context.Context, tx pgx.Tx, user int64, c CatalogCoverUploadCommand, image CatalogCoverUploadImage) (CatalogCoverUploadResult, error) {
	var result CatalogCoverUploadResult
	key, err := catalogCoverUploadInput(c, image)
	if err != nil {
		return result, err
	}
	if err = catalogAdmin(ctx, tx, user, true); err != nil {
		return result, err
	}
	if err = lockCatalog(ctx, tx); err != nil {
		return result, err
	}
	err = tx.QueryRow(ctx, `SELECT asset_id::text,object_key,alt,width,height,rights_status,rights_note FROM catalog.family_cover_assets WHERE family=$1 AND sha256=$2`, c.Family, image.SHA256).Scan(&result.AssetID, &result.Image.Src, &result.Image.Alt, &result.Image.Width, &result.Image.Height, &result.RightsStatus, &result.RightsNote)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `INSERT INTO catalog.family_cover_assets(asset_id,family,sha256,object_key,content_type,size_bytes,width,height,alt,rights_status,rights_note,created_by) VALUES(gen_random_uuid(),$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING asset_id::text`, c.Family, image.SHA256, key, image.ContentType, image.Size, image.Width, image.Height, c.Alt, c.RightsStatus, c.RightsNote, user).Scan(&result.AssetID)
		result.Image = CatalogCoverImage{Src: key, Alt: c.Alt, Width: image.Width, Height: image.Height}
		result.RightsStatus = c.RightsStatus
		result.RightsNote = c.RightsNote
	} else if err == nil {
		result.Reused = true
	}
	if err != nil {
		return result, err
	}
	result.Image.AssetID = result.AssetID
	result.Image.Src = "/" + result.Image.Src
	result.Image.FocalPoint = [2]float64{0.5, 0.5}
	if !result.Reused {
		_, err = tx.Exec(ctx, `INSERT INTO ops.audit_log(actor_user_id,action,target_type,target,before_state,after_state) VALUES($1,'MODEL_COVER_UPLOAD','MODEL_FAMILY',$2,'null'::jsonb,$3)`, user, c.Family, result)
	}
	return result, err
}
func (s *Store) SaveCatalogFamilyCover(ctx context.Context, user int64, c CatalogFamilyCoverSettings) (CatalogFamilyCover, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CatalogFamilyCover{}, err
	}
	defer rollback(tx)
	result, err := saveCatalogFamilyCover(ctx, tx, user, c)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func saveCatalogFamilyCover(ctx context.Context, tx pgx.Tx, user int64, c CatalogFamilyCoverSettings) (CatalogFamilyCover, error) {
	var result CatalogFamilyCover
	if !catalogChoice(catalogFamilies, c.Family) || c.AssetID != "" && !announcementUUID(c.AssetID) {
		return result, ErrCatalogInvalid
	}
	if err := catalogAdmin(ctx, tx, user, true); err != nil {
		return result, err
	}
	if err := lockCatalog(ctx, tx); err != nil {
		return result, err
	}
	if c.AssetID != "" {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM catalog.family_cover_assets WHERE family=$1 AND asset_id=$2)`, c.Family, c.AssetID).Scan(&exists); err != nil {
			return result, err
		}
		if !exists {
			return result, ErrCatalogInvalid
		}
	}
	covers, err := catalogFamilyCoverMap(ctx, tx)
	if err != nil {
		return result, err
	}
	before := covers[c.Family]
	_, err = tx.Exec(ctx, `UPDATE catalog.family_covers SET asset_id=NULLIF($2,'')::uuid,version=version+1,updated_at=clock_timestamp() WHERE family=$1`, c.Family, c.AssetID)
	if err != nil {
		return result, err
	}
	covers, err = catalogFamilyCoverMap(ctx, tx)
	if err != nil {
		return result, err
	}
	result = covers[c.Family]
	_, err = tx.Exec(ctx, `INSERT INTO ops.audit_log(actor_user_id,action,target_type,target,before_state,after_state) VALUES($1,'MODEL_COVER_SAVE','MODEL_FAMILY',$2,$3,$4)`, user, c.Family, before, result)
	return result, err
}
