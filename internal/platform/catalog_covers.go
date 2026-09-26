package platform

import (
	"context"
	"encoding/hex"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type CatalogCoverImage struct {
	AssetID    string     `json:"asset_id"`
	Src        string     `json:"src"`
	Alt        string     `json:"alt"`
	Width      int        `json:"width"`
	Height     int        `json:"height"`
	FocalPoint [2]float64 `json:"focal_point"`
}
type CatalogFamilyCover struct {
	Family       string             `json:"family"`
	Version      int64              `json:"version,string"`
	Image        *CatalogCoverImage `json:"image"`
	DefaultImage *CatalogCoverImage `json:"default_image"`
}
type CatalogFamilyCoverPage struct {
	Principal AnnouncementPrincipal `json:"principal"`
	Items     []CatalogFamilyCover  `json:"items"`
}
type CatalogCoverUploadCommand struct {
	OperationID  string `json:"operation_id"`
	Epoch        int64  `json:"authz_epoch"`
	Family       string `json:"family"`
	Alt          string `json:"alt"`
	RightsStatus string `json:"rights_status"`
	RightsNote   string `json:"rights_note"`
	Reason       string `json:"reason"`
}
type CatalogCoverUploadImage struct {
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type"`
	Extension   string `json:"extension"`
	Size        int64  `json:"size"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
}
type CatalogCoverUploadResult struct {
	OperationID  string            `json:"operation_id"`
	AssetID      string            `json:"asset_id"`
	Image        CatalogCoverImage `json:"image"`
	RightsStatus string            `json:"rights_status"`
	RightsNote   string            `json:"rights_note"`
	Reused       bool              `json:"reused"`
}
type CatalogFamilyCoverCommand struct {
	OperationID     string `json:"operation_id"`
	Epoch           int64  `json:"authz_epoch"`
	Action          string `json:"action"`
	Family          string `json:"family"`
	AssetID         string `json:"asset_id"`
	ExpectedVersion int64  `json:"expected_version,string"`
	Reason          string `json:"reason"`
}
type CatalogFamilyCoverPreview struct {
	ID        string             `json:"preview_id"`
	Before    CatalogFamilyCover `json:"before"`
	After     CatalogFamilyCover `json:"after"`
	ExpiresAt time.Time          `json:"expires_at"`
}
type CatalogFamilyCoverResult struct {
	OperationID string             `json:"operation_id"`
	Cover       CatalogFamilyCover `json:"cover"`
}

func CatalogCoverObjectKey(family, hash, extension string) (string, error) {
	decoded, err := hex.DecodeString(hash)
	if !catalogChoice(catalogFamilies, family) || err != nil || len(decoded) != 32 || strings.ToLower(hash) != hash || !slices.Contains([]string{"png", "jpg", "webp"}, extension) {
		return "", ErrCatalogInvalid
	}
	return "assets/models/uploads/" + family + "/" + hash + "." + extension, nil
}

// Shared with the HTTP boundary: reject invalid metadata before writing an object.
func ValidateCatalogCoverUpload(c CatalogCoverUploadCommand) error {
	if !announcementUUID(c.OperationID) || c.Epoch <= 0 || !catalogChoice(catalogFamilies, c.Family) || c.Alt == "" || !catalogPlainText(c.Alt, 120, false) || c.RightsNote == "" || !catalogPlainText(c.RightsNote, 500, true) || c.Reason == "" || !catalogPlainText(c.Reason, 500, true) || !slices.Contains([]string{"ORIGINAL_PLATFORM", "ORIGINAL_GENERATED", "LICENSED_OR_APPROVED"}, c.RightsStatus) {
		return ErrCatalogInvalid
	}
	return nil
}

func catalogDefaultCover(family string, assets []CatalogAsset) *CatalogCoverImage {
	name := family
	if name == "ernie" {
		name = "wenxin"
	}
	for _, a := range assets {
		if a.AssetID == "persona_"+name+"_master" {
			return &CatalogCoverImage{AssetID: a.AssetID, Src: a.Src, Alt: a.Character, FocalPoint: a.FocalPoint}
		}
	}
	return nil
}

func catalogFamilyCoverMap(ctx context.Context, q announcementQuerier) (map[string]CatalogFamilyCover, error) {
	vocabulary, err := catalogVocabulary()
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT c.family,c.version,COALESCE(a.asset_id::text,''),COALESCE(a.object_key,''),COALESCE(a.alt,''),COALESCE(a.width,0),COALESCE(a.height,0) FROM catalog.family_covers c LEFT JOIN catalog.family_cover_assets a ON (a.family,a.asset_id)=(c.family,c.asset_id) ORDER BY c.family`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]CatalogFamilyCover, len(catalogFamilies))
	for rows.Next() {
		var c CatalogFamilyCover
		var image CatalogCoverImage
		if err = rows.Scan(&c.Family, &c.Version, &image.AssetID, &image.Src, &image.Alt, &image.Width, &image.Height); err != nil {
			return nil, err
		}
		c.DefaultImage = catalogDefaultCover(c.Family, vocabulary.Assets)
		c.Image = c.DefaultImage
		if image.AssetID != "" {
			image.Src = "/" + image.Src
			image.FocalPoint = [2]float64{0.5, 0.5}
			c.Image = &image
		}
		result[c.Family] = c
	}
	return result, rows.Err()
}
func attachCatalogCovers(ctx context.Context, q announcementQuerier, models []CatalogModel) error {
	if len(models) == 0 {
		return nil
	}
	covers, err := catalogFamilyCoverMap(ctx, q)
	if err != nil {
		return err
	}
	for i := range models {
		if c, ok := covers[models[i].Metadata.Family]; ok {
			models[i].FamilyCover = &c
		}
	}
	return nil
}
func (s *Store) CatalogFamilyCovers(ctx context.Context, userID int64) (CatalogFamilyCoverPage, error) {
	result := CatalogFamilyCoverPage{Items: []CatalogFamilyCover{}}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer rollback(tx)
	result.Principal, err = catalogAuthority(ctx, tx, userID, false)
	if err != nil {
		return result, err
	}
	covers, err := catalogFamilyCoverMap(ctx, tx)
	if err != nil {
		return result, err
	}
	for _, f := range catalogFamilies {
		if c, ok := covers[f.Value]; ok {
			result.Items = append(result.Items, c)
		}
	}
	return result, tx.Commit(ctx)
}
