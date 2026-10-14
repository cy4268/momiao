package main

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
)

type catalogStore interface {
	PersonalCatalog(context.Context, int64, string) (platform.CatalogPersonalPrice, error)
	CatalogFamilyCovers(context.Context, int64) (platform.CatalogFamilyCoverPage, error)
	RegisterCatalogCoverUpload(context.Context, int64, platform.CatalogCoverUploadCommand, platform.CatalogCoverUploadImage) (platform.CatalogCoverUploadResult, error)
	SaveCatalogFamilyCover(context.Context, int64, platform.CatalogFamilyCoverSettings) (platform.CatalogFamilyCover, error)
	PublicCatalog(context.Context, platform.CatalogFilter) (platform.CatalogPage, error)
	PublicCatalogModel(context.Context, string) (platform.CatalogModel, error)
	OpsCatalog(context.Context, int64, platform.CatalogOpsFilter) (platform.CatalogOpsPage, error)
	OpsCatalogModel(context.Context, int64, string) (platform.CatalogOpsDetail, error)
	SaveCatalog(context.Context, int64, platform.CatalogSettings) (platform.CatalogOpsDetail, error)
	SaveCatalogPublication(context.Context, int64, platform.CatalogPublicationSettings) error
	SaveCatalogPrices(context.Context, int64, platform.CatalogPricesSettings) error
}

func catalogBrowserRoute(escaped string) bool {
	if !strings.HasPrefix(escaped, "/models/") {
		return false
	}
	id := strings.TrimPrefix(escaped, "/models/")
	switch id {
	case "~Lg":
		id = "."
	case "~Li4":
		id = ".."
	default:
		var err error
		id, err = url.PathUnescape(id)
		if err != nil {
			return false
		}
	}
	return platform.ValidCatalogModelID(id)
}

func catalogAPIRoute(path string) bool {
	return path == "/platform/v1/models" || strings.HasPrefix(path, "/platform/v1/models/") || path == "/platform/v1/ops/models" || strings.HasPrefix(path, "/platform/v1/ops/models/")
}

func newCatalogHandler(cfg config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parent := r.Context()
		ctx, cancel := context.WithTimeout(parent, 8*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		ops := strings.HasPrefix(r.URL.Path, "/platform/v1/ops/models")
		base := "/platform/v1/models"
		if ops {
			base = "/platform/v1/ops/models"
		}
		tail := strings.TrimPrefix(r.URL.Path, base)
		personal := !ops && tail == "/personal-price"
		var user int64
		if personal || ops {
			account, ok := identity.UserFromContext(ctx)
			if !ok {
				walletError(w, 401, "AUTH_UNAUTHORIZED")
				return
			}
			user = account.ID
		}
		if ops {
			account, _ := identity.UserFromContext(ctx)
			if account.Role < 10 {
				walletError(w, 403, "MODELS_FORBIDDEN")
				return
			}
		}
		if !ops && r.Method != "GET" {
			w.Header().Set("Allow", "GET")
			walletError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		if !ops && tail == "/access-config" {
			if _, err := catalogQuery(r.URL); err != nil || r.URL.RawQuery != "" || r.URL.ForceQuery {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
			walletSuccess(w, map[string]any{"api_base_url": cfg.APIBaseURL})
			return
		}
		if cfg.catalog == nil {
			walletError(w, 503, "CATALOG_UNAVAILABLE")
			return
		}
		if ops && (tail == "/family-covers" || strings.HasPrefix(tail, "/family-covers/")) {
			if tail == "/family-covers/upload" && r.Method == "POST" {
				r = r.WithContext(parent)
			}
			serveCatalogFamilyCovers(w, r, cfg, user)
			return
		}
		var data any
		var err error
		if r.Method == "PUT" && ops {
			if origins := r.Header.Values("Origin"); len(origins) != 1 || origins[0] != cfg.PublicOrigin {
				walletError(w, 403, "ORIGIN_REJECTED")
				return
			}
			media, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if mediaErr != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
				walletError(w, 415, "INVALID_CONTENT_TYPE")
				return
			}
			if r.URL.RawQuery != "" || r.URL.ForceQuery {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
			switch tail {
			case "":
				var body platform.CatalogSettings
				if !decodeAnnouncementBody(r.Body, &body) {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
				data, err = cfg.catalog.SaveCatalog(ctx, user, body)
			case "/prices":
				var body platform.CatalogPricesSettings
				if !decodeAnnouncementBody(r.Body, &body) {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
				err = cfg.catalog.SaveCatalogPrices(ctx, user, body)
				data = map[string]bool{"saved": err == nil}
			case "/publication":
				var body platform.CatalogPublicationSettings
				if !decodeAnnouncementBody(r.Body, &body) {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
				err = cfg.catalog.SaveCatalogPublication(ctx, user, body)
				data = map[string]bool{"saved": err == nil}
			default:
				walletError(w, 404, "NOT_FOUND")
				return
			}
		} else {
			if r.Method != "GET" {
				walletError(w, 404, "NOT_FOUND")
				return
			}
			var q url.Values
			q, err = catalogQuery(r.URL)
			if err != nil {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
			switch {
			case tail == "":
				if ops {
					var f platform.CatalogOpsFilter
					f, err = catalogOpsFilter(q)
					if err == nil {
						data, err = cfg.catalog.OpsCatalog(ctx, user, f)
					}
				} else {
					var f platform.CatalogFilter
					f, err = catalogPublicFilter(q)
					if err == nil {
						data, err = cfg.catalog.PublicCatalog(ctx, f)
					}
				}
			case tail == "/detail" || personal:
				if len(q) != 1 || !platform.ValidCatalogModelID(q.Get("model_id")) {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
				id := q.Get("model_id")
				if ops {
					data, err = cfg.catalog.OpsCatalogModel(ctx, user, id)
				} else if personal {
					data, err = cfg.catalog.PersonalCatalog(ctx, user, id)
				} else {
					var item platform.CatalogModel
					item, err = cfg.catalog.PublicCatalogModel(ctx, id)
					if err == nil {
						var vocabulary platform.CatalogVocabulary
						vocabulary, err = platform.CatalogChoices()
						data = map[string]any{"item": item, "vocabulary": vocabulary, "api_base_url": cfg.APIBaseURL}
					}
				}
			default:
				walletError(w, 404, "NOT_FOUND")
				return
			}
		}
		if err != nil {
			writeCatalogError(w, err)
			return
		}
		walletSuccess(w, data)
	})
}

func catalogQuery(u *url.URL) (url.Values, error) {
	if len(u.RawQuery) > 8192 {
		return nil, platform.ErrCatalogInvalid
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, platform.ErrCatalogInvalid
	}
	for _, v := range q {
		if len(v) != 1 || v[0] == "" {
			return nil, platform.ErrCatalogInvalid
		}
	}
	return q, nil
}
func catalogPagination(q url.Values) (int, int, error) {
	offset, limit := 0, 0
	for key, target := range map[string]*int{"offset": &offset, "limit": &limit} {
		if value, ok := q[key]; ok {
			n, err := decimalInt(value[0])
			if err != nil || n > 1000000 || key == "limit" && (n < 1 || n > 100) {
				return 0, 0, platform.ErrCatalogInvalid
			}
			*target = int(n)
		}
	}
	return offset, limit, nil
}
func catalogOpsFilter(q url.Values) (platform.CatalogOpsFilter, error) {
	var f platform.CatalogOpsFilter
	for key := range q {
		if key != "q" && key != "state" && key != "offset" && key != "limit" {
			return f, platform.ErrCatalogInvalid
		}
	}
	var err error
	f.Offset, f.Limit, err = catalogPagination(q)
	f.Search = q.Get("q")
	f.State = q.Get("state")
	return f, err
}
func catalogPublicFilter(q url.Values) (platform.CatalogFilter, error) {
	var f platform.CatalogFilter
	allowed := map[string]bool{"q": true, "availability": true, "family": true, "tag": true, "use_case": true, "recommended": true, "unknown_context": true, "min_context": true, "price_dimension": true, "min_price": true, "max_price": true, "sort": true, "offset": true, "limit": true, "group_by": true}
	for key := range q {
		if !allowed[key] {
			return f, platform.ErrCatalogInvalid
		}
	}
	var err error
	f.Offset, f.Limit, err = catalogPagination(q)
	if err != nil {
		return f, err
	}
	f.Search = q.Get("q")
	f.Availability = q.Get("availability")
	f.Family = q.Get("family")
	f.Tag = q.Get("tag")
	f.UseCase = q.Get("use_case")
	f.Sort = q.Get("sort")
	f.PriceDimension = q.Get("price_dimension")
	if value, ok := q["group_by"]; ok {
		if value[0] != "family" {
			return f, platform.ErrCatalogInvalid
		}
		f.GroupBy = "family"
	}
	for key, target := range map[string]*bool{"recommended": &f.RecommendedOnly, "unknown_context": &f.UnknownContext} {
		if value, ok := q[key]; ok {
			if value[0] != "true" {
				return f, platform.ErrCatalogInvalid
			}
			*target = true
		}
	}
	if value, ok := q["min_context"]; ok {
		n, e := decimalInt(value[0])
		if e != nil || n < 1 || n > 9007199254740991 {
			return f, platform.ErrCatalogInvalid
		}
		f.MinContext = &n
	}
	for key, target := range map[string]**string{"min_price": &f.MinPrice, "max_price": &f.MaxPrice} {
		if value, ok := q[key]; ok {
			if !platform.ValidCatalogDecimal(value[0]) {
				return f, platform.ErrCatalogInvalid
			}
			v := value[0]
			*target = &v
		}
	}
	return f, nil
}

func writeCatalogError(w http.ResponseWriter, err error) {
	status, code := 503, "CATALOG_UNAVAILABLE"
	switch {
	case errors.Is(err, platform.ErrCatalogInvalid):
		status, code = 400, platform.ErrCatalogInvalid.Error()
	case errors.Is(err, platform.ErrCatalogIncomplete):
		status, code = 400, platform.ErrCatalogIncomplete.Error()
	case errors.Is(err, platform.ErrCatalogForbidden):
		status, code = 403, platform.ErrCatalogForbidden.Error()
	case errors.Is(err, platform.ErrCatalogNotFound):
		status, code = 404, platform.ErrCatalogNotFound.Error()
	}
	walletError(w, status, code)
}
