package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/platform"
	_ "golang.org/x/image/webp"
)

const catalogCoversPath = "/platform/v1/ops/models/family-covers"
const catalogCoverMaxBytes = 8 * 1024 * 1024

type catalogCoverFile struct {
	Bytes []byte
	Image platform.CatalogCoverUploadImage
}

func parseCatalogCoverUpload(w http.ResponseWriter, r *http.Request) (platform.CatalogCoverUploadCommand, catalogCoverFile, error) {
	var c platform.CatalogCoverUploadCommand
	var f catalogCoverFile
	bad := func() (platform.CatalogCoverUploadCommand, catalogCoverFile, error) {
		return c, catalogCoverFile{}, platform.ErrCatalogInvalid
	}
	if len(r.Header.Values("Content-Type")) != 1 || r.ContentLength > catalogCoverMaxBytes+64*1024 {
		return bad()
	}
	r.Body = http.MaxBytesReader(w, r.Body, catalogCoverMaxBytes+64*1024)
	reader, err := r.MultipartReader()
	if err != nil {
		return bad()
	}
	seen := map[string]bool{}
	declared := ""
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return bad()
		}
		name := part.FormName()
		if seen[name] || (name != "metadata" && name != "file") || part.Header.Get("Content-Transfer-Encoding") != "" {
			return bad()
		}
		seen[name] = true
		limit := int64(16 * 1024)
		if name == "file" {
			limit = catalogCoverMaxBytes
		}
		data, err := io.ReadAll(io.LimitReader(part, limit+1))
		if err != nil || int64(len(data)) > limit {
			return bad()
		}
		if name == "metadata" {
			if part.FileName() != "" || !decodeAnnouncementBody(bytes.NewReader(data), &c) {
				return bad()
			}
		} else {
			if part.FileName() == "" || len(part.Header.Values("Content-Type")) > 1 {
				return bad()
			}
			declared = part.Header.Get("Content-Type")
			f.Bytes = data
		}
		_ = part.Close()
	}
	if !seen["metadata"] || !seen["file"] || platform.ValidateCatalogCoverUpload(c) != nil {
		return bad()
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(f.Bytes))
	if err != nil || !slices.Contains([]string{"png", "jpeg", "webp"}, format) || config.Width <= 0 || config.Height <= 0 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > 16777216 {
		return bad()
	}
	if !catalogStaticRaster(f.Bytes, format) {
		return bad()
	}
	actual := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "webp": "image/webp"}[format]
	if declared != "" {
		media, _, err := mime.ParseMediaType(declared)
		if err != nil || (media != "application/octet-stream" && media != actual) {
			return bad()
		}
	}
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(f.Bytes))
	if err != nil || decodedFormat != format || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return bad()
	}
	hash := sha256.Sum256(f.Bytes)
	ext := format
	if ext == "jpeg" {
		ext = "jpg"
	}
	f.Image = platform.CatalogCoverUploadImage{SHA256: hex.EncodeToString(hash[:]), Extension: ext, ContentType: actual, Size: int64(len(f.Bytes)), Width: config.Width, Height: config.Height}
	return c, f, nil
}

// Check the entire bounded container, not a substring inside compressed pixels.
func catalogStaticRaster(b []byte, format string) bool {
	switch format {
	case "png":
		if len(b) < 8 || string(b[:8]) != "\x89PNG\r\n\x1a\n" {
			return false
		}
		for pos := 8; pos < len(b); {
			if len(b)-pos < 12 {
				return false
			}
			n := uint64(binary.BigEndian.Uint32(b[pos : pos+4]))
			if n > uint64(len(b)-pos-12) {
				return false
			}
			kind := string(b[pos+4 : pos+8])
			if kind == "acTL" || kind == "fcTL" || kind == "fdAT" {
				return false
			}
			pos += 12 + int(n)
			if kind == "IEND" {
				return n == 0 && pos == len(b)
			}
		}
		return false
	case "webp":
		if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WEBP" || uint64(binary.LittleEndian.Uint32(b[4:8]))+8 != uint64(len(b)) {
			return false
		}
		for pos := 12; pos < len(b); {
			if len(b)-pos < 8 {
				return false
			}
			n := uint64(binary.LittleEndian.Uint32(b[pos+4 : pos+8]))
			padded := n + (n % 2)
			if padded > uint64(len(b)-pos-8) {
				return false
			}
			kind := string(b[pos : pos+4])
			if kind == "ANIM" || kind == "ANMF" || (kind == "VP8X" && (n != 10 || b[pos+8]&2 != 0)) {
				return false
			}
			pos += 8 + int(padded)
		}
		return true
	case "jpeg":
		return len(b) >= 4 && b[0] == 0xff && b[1] == 0xd8 && b[len(b)-2] == 0xff && b[len(b)-1] == 0xd9
	}
	return false
}

func serveCatalogFamilyCovers(w http.ResponseWriter, r *http.Request, cfg config, userID int64) {
	tail := strings.TrimPrefix(r.URL.Path, catalogCoversPath)
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		walletError(w, 400, "INVALID_REQUEST")
		return
	}
	p, err := cfg.catalog.CatalogAuthority(r.Context(), userID)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	var data any
	if r.Method == "GET" && tail == "" {
		var page platform.CatalogFamilyCoverPage
		page, err = cfg.catalog.CatalogFamilyCovers(r.Context(), userID)
		data = struct {
			platform.CatalogFamilyCoverPage
			UploadEnabled bool `json:"upload_enabled"`
		}{page, cfg.catalogAssets != nil}
	} else if r.Method == "POST" {
		if origins := r.Header.Values("Origin"); len(origins) != 1 || origins[0] != cfg.PublicOrigin {
			walletError(w, 403, "ORIGIN_REJECTED")
			return
		}
		permission := "models.publish"
		if tail == "/upload" {
			permission = "models.write"
		}
		if !slices.Contains(p.Permissions, permission) {
			writeCatalogError(w, platform.ErrCatalogForbidden)
			return
		}
		if tail == "/upload" {
			if cfg.catalogAssets == nil {
				walletError(w, 503, "CATALOG_ASSET_UPLOAD_DISABLED")
				return
			}
			// Only authenticated, authorized uploads replace the normal socket deadline.
			deadline := time.Now().Add(90 * time.Second)
			controller := http.NewResponseController(w)
			for _, e := range []error{controller.SetReadDeadline(deadline), controller.SetWriteDeadline(deadline)} {
				if e != nil && !errors.Is(e, http.ErrNotSupported) {
					walletError(w, 503, "CATALOG_UNAVAILABLE")
					return
				}
			}
			ctx, cancel := context.WithDeadline(r.Context(), deadline)
			defer cancel()
			r = r.WithContext(ctx)
			c, f, e := parseCatalogCoverUpload(w, r)
			if e != nil {
				writeCatalogError(w, e)
				return
			}
			if c.Epoch != p.Epoch {
				writeCatalogError(w, platform.ErrAnnouncementStale)
				return
			}
			if e = cfg.catalogAssets.Put(ctx, c.Family, f); e != nil {
				walletError(w, 503, "CATALOG_ASSET_UPLOAD_FAILED")
				return
			}
			data, err = cfg.catalog.RegisterCatalogCoverUpload(ctx, userID, c, f.Image)
		} else {
			media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if e != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
				walletError(w, 415, "INVALID_CONTENT_TYPE")
				return
			}
			switch tail {
			case "/prepare":
				var body struct {
					Command platform.CatalogFamilyCoverCommand `json:"command"`
				}
				if !decodeAnnouncementBody(r.Body, &body) {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
				data, err = cfg.catalog.PrepareCatalogFamilyCover(r.Context(), userID, body.Command)
			case "/execute":
				var body struct {
					Command   platform.CatalogFamilyCoverCommand `json:"command"`
					PreviewID string                             `json:"preview_id"`
					Confirmed bool                               `json:"confirmed"`
				}
				if !decodeAnnouncementBody(r.Body, &body) {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
				data, err = cfg.catalog.ExecuteCatalogFamilyCover(r.Context(), userID, body.Command, body.PreviewID, body.Confirmed)
			default:
				walletError(w, 404, "NOT_FOUND")
				return
			}
		}
	} else {
		walletError(w, 404, "NOT_FOUND")
		return
	}
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	walletSuccess(w, data)
}
