package main

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/platform"
)

type catalogStore interface {
	CatalogAuthority(context.Context, int64) (platform.AnnouncementPrincipal, error)
	PublicCatalog(context.Context, platform.CatalogFilter, platform.CatalogPolicy) (platform.CatalogPage, error)
	PublicCatalogModel(context.Context, string, platform.CatalogPolicy) (platform.CatalogModel, error)
	OpsCatalog(context.Context, int64, platform.CatalogOpsFilter, platform.CatalogPolicy) (platform.CatalogOpsPage, error)
	OpsCatalogModel(context.Context, int64, string, platform.CatalogPolicy) (platform.AnnouncementPrincipal, platform.CatalogModel, error)
	PrepareCatalog(context.Context, int64, platform.CatalogCommand, platform.CatalogSource, platform.CatalogPolicy) (platform.CatalogPreview, error)
	ExecuteCatalog(context.Context, int64, platform.CatalogCommand, string, bool, platform.CatalogSource, platform.CatalogPolicy) (platform.CatalogResult, error)
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

func newCatalogHandler(cfg config, transport http.RoundTripper) http.Handler {
	policy := platform.CatalogPolicy{StaleAfter: cfg.CatalogStaleAfter, DisableAfter: cfg.CatalogDisableAfter}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
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
		if ops || personal {
			if !announcementSessionCredential(r) {
				walletError(w, 401, "AUTH_UNAUTHORIZED")
				return
			}
			id, status := verifyWalletUser(r, transport)
			if status != 0 {
				code := "AUTH_UNAVAILABLE"
				if status == 401 {
					code = "AUTH_UNAUTHORIZED"
				}
				if status == 403 {
					code = "AUTH_FORBIDDEN"
				}
				walletError(w, status, code)
				return
			}
			user = id
		}
		if r.Method != "GET" && !(ops && r.Method == "POST") {
			w.Header().Set("Allow", "GET")
			if ops {
				w.Header().Set("Allow", "GET, POST")
			}
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
		if ops {
			if _, err := cfg.catalog.CatalogAuthority(ctx, user); err != nil {
				writeCatalogError(w, err)
				return
			}
		}
		var data any
		var err error
		if r.Method == "POST" {
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
			case "/prepare":
				var body struct {
					Command platform.CatalogCommand `json:"command"`
				}
				if !decodeAnnouncementBody(r.Body, &body) {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
				data, err = cfg.catalog.PrepareCatalog(ctx, user, body.Command, cfg.catalogSource, policy)
			case "/execute":
				var body struct {
					Command   platform.CatalogCommand `json:"command"`
					PreviewID string                  `json:"preview_id"`
					Confirmed bool                    `json:"confirmed"`
				}
				if !decodeAnnouncementBody(r.Body, &body) {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
				data, err = cfg.catalog.ExecuteCatalog(ctx, user, body.Command, body.PreviewID, body.Confirmed, cfg.catalogSource, policy)
			default:
				walletError(w, 404, "NOT_FOUND")
				return
			}
		} else {
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
						data, err = cfg.catalog.OpsCatalog(ctx, user, f, policy)
					}
				} else {
					var f platform.CatalogFilter
					f, err = catalogPublicFilter(q)
					if err == nil {
						data, err = cfg.catalog.PublicCatalog(ctx, f, policy)
					}
				}
			case tail == "/detail" || personal:
				if len(q) != 1 || !platform.ValidCatalogModelID(q.Get("model_id")) {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
				id := q.Get("model_id")
				if ops {
					var p platform.AnnouncementPrincipal
					var item platform.CatalogModel
					p, item, err = cfg.catalog.OpsCatalogModel(ctx, user, id, policy)
					data = map[string]any{"principal": p, "item": item}
				} else {
					var item platform.CatalogModel
					item, err = cfg.catalog.PublicCatalogModel(ctx, id, policy)
					if err == nil && personal {
						data, err = readPersonalCatalog(r, transport, id)
						if err == nil {
							_, err = cfg.catalog.PublicCatalogModel(ctx, id, policy)
						}
					} else if err == nil {
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
	allowed := map[string]bool{"q": true, "availability": true, "family": true, "tag": true, "use_case": true, "recommended": true, "unknown_context": true, "min_context": true, "price_dimension": true, "min_price": true, "max_price": true, "sort": true, "offset": true, "limit": true}
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

var errCatalogPersonal = errors.New("PERSONAL_PRICE_UNAVAILABLE")

type catalogPersonalQuote struct {
	Candidate int                          `json:"candidate"`
	Enabled   bool                         `json:"enabled_configuration"`
	Visible   bool                         `json:"native_catalog_visible"`
	Price     *platform.CatalogPublicPrice `json:"price"`
	Reason    string                       `json:"reason,omitempty"`
}
type catalogPersonalPrice struct {
	ModelID    string                 `json:"model_id"`
	ObservedAt string                 `json:"observed_at"`
	Basis      string                 `json:"basis"`
	Quotes     []catalogPersonalQuote `json:"quotes"`
}

func readPersonalCatalog(r *http.Request, transport http.RoundTripper, id string) (catalogPersonalPrice, error) {
	var result catalogPersonalPrice
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://unix/api/momiao/catalog/prices?"+url.Values{"model_id": []string{id}}.Encode(), nil)
	req.Host = "localhost"
	for _, key := range []string{"Authorization", "New-Api-User", "X-Auth-Session"} {
		if value := r.Header.Get(key); value != "" {
			req.Header.Set(key, value)
		}
	}
	req.Header.Set("Accept", "application/json")
	response, err := transport.RoundTrip(req)
	if err != nil {
		return result, errCatalogPersonal
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return result, &catalogAuthError{status: response.StatusCode}
	}
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != 200 || err != nil || media != "application/json" {
		return result, errCatalogPersonal
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, platform.NativeCatalogMaxBytes+1))
	if err != nil {
		return result, errCatalogPersonal
	}
	parsed, err := platform.ParseNativePersonalCatalog(raw, id, time.Now())
	if err != nil {
		return result, errCatalogPersonal
	}
	result = catalogPersonalPrice{ModelID: parsed.ModelID, ObservedAt: parsed.ObservedAt, Basis: parsed.Basis, Quotes: []catalogPersonalQuote{}}
	for _, q := range parsed.Quotes {
		item := catalogPersonalQuote{Candidate: q.Candidate, Enabled: q.EnabledConfiguration, Visible: q.NativeCatalogVisible, Reason: q.Reason}
		if q.Price != nil {
			p := q.Price
			item.Price = &platform.CatalogPublicPrice{Mode: p.Mode, Configured: p.Configured, Status: p.Status, Reason: p.Reason, Dimensions: append([]platform.CatalogDimension{}, p.Dimensions...), Unquoted: append([]string{}, p.Unquoted...)}
		}
		result.Quotes = append(result.Quotes, item)
	}
	return result, nil
}

type catalogAuthError struct{ status int }

func (e *catalogAuthError) Error() string { return "AUTH_UNAVAILABLE_" + strconv.Itoa(e.status) }
func writeCatalogError(w http.ResponseWriter, err error) {
	var auth *catalogAuthError
	if errors.As(err, &auth) {
		code := "AUTH_UNAUTHORIZED"
		if auth.status == 403 {
			code = "AUTH_FORBIDDEN"
		}
		walletError(w, auth.status, code)
		return
	}
	status, code := 503, "CATALOG_UNAVAILABLE"
	switch {
	case errors.Is(err, platform.ErrCatalogInvalid):
		status, code = 400, platform.ErrCatalogInvalid.Error()
	case errors.Is(err, platform.ErrCatalogIncomplete):
		status, code = 400, platform.ErrCatalogIncomplete.Error()
	case errors.Is(err, platform.ErrCatalogConfirmation):
		status, code = 400, platform.ErrCatalogConfirmation.Error()
	case errors.Is(err, platform.ErrCatalogForbidden):
		status, code = 403, platform.ErrCatalogForbidden.Error()
	case errors.Is(err, platform.ErrCatalogNotFound):
		status, code = 404, platform.ErrCatalogNotFound.Error()
	case errors.Is(err, platform.ErrCatalogConflict):
		status, code = 409, platform.ErrCatalogConflict.Error()
	case errors.Is(err, platform.ErrCatalogOperation):
		status, code = 409, platform.ErrCatalogOperation.Error()
	case errors.Is(err, platform.ErrCatalogSourceChanged):
		status, code = 409, platform.ErrCatalogSourceChanged.Error()
	case errors.Is(err, platform.ErrAnnouncementStale):
		status, code = 409, platform.ErrAnnouncementStale.Error()
	case errors.Is(err, errCatalogPersonal):
		code = errCatalogPersonal.Error()
	}
	walletError(w, status, code)
}

// One serial store path is used by both scheduled recovery and explicit Ops sync.
func runCatalogWorker(ctx context.Context, interval time.Duration, sync func(context.Context) (platform.CatalogSyncResult, error)) {
	if interval <= 0 {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		_, _ = sync(ctx)
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
