package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/cy4268/momiao/internal/platform"
)

type accessDeclaration struct {
	Version                int               `json:"version"`
	Environment            string            `json:"environment"`
	Origin                 string            `json:"origin"`
	EvidenceRef            string            `json:"evidence_ref"`
	MigrationApplicability string            `json:"migration_applicability"`
	Resources              map[string]string `json:"resources"`
}

func loadAccessDeclaration(path, origin string) (*accessDeclaration, error) {
	invalid := errors.New("invalid access-gate deployment declaration")
	if !filepath.IsAbs(path) {
		return nil, invalid
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, invalid
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 || (runtime.GOOS != "windows" && info.Mode().Perm()&0022 != 0) {
		return nil, invalid
	}
	raw, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(raw) > 16384 {
		return nil, invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if !uniqueAnnouncementJSON(decoder, 0) {
		return nil, invalid
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, invalid
	}
	var d accessDeclaration
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&d) != nil || d.Version != 1 || d.Origin != origin || origin == "" || strings.TrimSpace(d.EvidenceRef) == "" || len(d.EvidenceRef) > 256 || (d.Environment != "DEVELOPMENT" && d.Environment != "STAGING" && d.Environment != "PRODUCTION") {
		return nil, invalid
	}
	if d.MigrationApplicability != "NO_MIGRATION_APPLICABLE" && d.MigrationApplicability != "PERSISTED_COMPLETED_NOTICE" && d.MigrationApplicability != "UNVERIFIED" {
		return nil, invalid
	}
	for domain, state := range d.Resources {
		switch domain {
		case "ACCOUNT", "API", "COMMUNITY", "OPERATIONS", "ASSETS", "EXPERIENCE":
		default:
			return nil, invalid
		}
		if state != "AVAILABLE" && state != "MAINTENANCE" && state != "UNAVAILABLE" && state != "UNVERIFIED" {
			return nil, invalid
		}
	}
	return &d, nil
}

type accessGateStore interface {
	ReadProfile(context.Context, int64) (platform.Profile, error)
	ReadMigrationNotice(context.Context, int64, bool) (platform.MigrationNotice, error)
	AcknowledgeMigrationNotice(context.Context, int64, int64) (platform.MigrationNotice, error)
	AnnouncementAuthority(context.Context, int64) (platform.AnnouncementPrincipal, error)
	CatalogAuthority(context.Context, int64) (platform.AnnouncementPrincipal, error)
}
type accessGateView struct {
	UserID int64                     `json:"user_id,string"`
	Route  string                    `json:"route"`
	Stage  string                    `json:"stage"`
	Notice *platform.MigrationNotice `json:"migration_notice,omitempty"`
}

// Same navigation-only boundary as normalizeRouteIntent in post-auth-intent.ts.
// Extend these explicit route policies when a new implemented route is added;
// never accept a URL, write body, arbitrary query or credential as an intent.
func gateRouteDomain(route string) string {
	if strings.Contains(route, "#") {
		return ""
	}
	if path, raw, found := strings.Cut(route, "?"); found {
		if raw == "" || path != "/api/access" && path != "/keys" {
			return ""
		}
		query, err := url.ParseQuery(raw)
		if err != nil || len(query["model_id"]) != 1 || !platform.ValidCatalogModelID(query.Get("model_id")) {
			return ""
		}
		for key, values := range query {
			if key != "model_id" && (key != "intent" || path != "/api/access" || len(values) != 1 || values[0] != "use") {
				return ""
			}
		}
		return "API"
	}
	switch route {
	case "/dashboard":
		return "COMMUNITY"
	case "/me", "/account", "/master-profile":
		return "ACCOUNT"
	case "/models", "/api/access", "/keys", "/logs", "/playground":
		return "API"
	case "/wallet", "/wallet/activate", "/rewards":
		return "ASSETS"
	case "/ops/announcements", "/ops/models", "/admin/channels":
		return "OPERATIONS"
	case "/games/dice":
		return "EXPERIENCE"
	}
	return ""
}
func newAccessGateHandler(origin string, store accessGateStore, declaration *accessDeclaration, transport http.RoundTripper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gate := r.URL.Path == "/platform/v1/access-gate"
		ack := r.URL.Path == "/platform/v1/migration-notice/acknowledge"
		if !gate && !ack && r.URL.Path != "/platform/v1/migration-notice" {
			walletError(w, 404, "NOT_FOUND")
			return
		}
		method := "GET"
		if ack {
			method = "POST"
		}
		if r.Method != method {
			w.Header().Set("Allow", method)
			walletError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		route := ""
		if gate {
			q, queryErr := url.ParseQuery(r.URL.RawQuery)
			route = q.Get("route")
			if queryErr != nil || len(q) != 1 || len(q["route"]) != 1 || gateRouteDomain(route) == "" {
				walletError(w, 400, "INVALID_RETURN_INTENT")
				return
			}
		} else if r.URL.RawQuery != "" || r.URL.ForceQuery {
			walletError(w, 400, "INVALID_REQUEST")
			return
		}
		if !nativeself.SessionCredential(r) {
			walletError(w, 401, "AUTH_UNAUTHORIZED")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		account, status := readNativeSelf(r, transport)
		if status != 0 {
			walletError(w, status, "AUTH_UNAVAILABLE")
			return
		}
		view := accessGateView{UserID: account.ID, Route: route}
		send := func(stage string) { view.Stage = stage; walletSuccess(w, view) }
		if account.Username == "" {
			walletError(w, 503, "ACCOUNT_STATUS_UNVERIFIED")
			return
		}
		if *account.Status != 1 && *account.Status != 2 {
			walletError(w, 503, "ACCOUNT_STATUS_UNVERIFIED")
			return
		}
		if *account.Status != 1 {
			if gate {
				send("ACCOUNT_RESTRICTED")
			} else {
				walletError(w, 403, "ACCOUNT_RESTRICTED")
			}
			return
		}
		if store == nil {
			walletError(w, 503, "ACCESS_GATE_UNVERIFIED")
			return
		}
		profile, err := store.ReadProfile(ctx, account.ID)
		if err != nil {
			walletError(w, 503, "MASTER_STATUS_UNVERIFIED")
			return
		}
		if profile.Status != "COMPLETE" {
			if gate {
				send("MASTER_REQUIRED")
			} else {
				walletError(w, 409, "MASTER_INITIALIZATION_REQUIRED")
			}
			return
		}
		none := declaration != nil && declaration.MigrationApplicability == "NO_MIGRATION_APPLICABLE"
		if ack {
			if origins := r.Header.Values("Origin"); len(origins) != 1 || origins[0] != origin {
				walletError(w, 403, "ORIGIN_REJECTED")
				return
			}
			contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || contentType != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
				walletError(w, 415, "INVALID_CONTENT_TYPE")
				return
			}
			fields, err := decodeStringFields(r.Body, "version")
			version, parseErr := strconv.ParseInt(fields["version"], 10, 64)
			if err != nil || len(fields) != 1 || parseErr != nil || version <= 0 || strconv.FormatInt(version, 10) != fields["version"] {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
			result, err := store.AcknowledgeMigrationNotice(ctx, account.ID, version)
			if errors.Is(err, platform.ErrMigrationNoticeStale) {
				walletError(w, 409, "MIGRATION_NOTICE_VERSION_STALE")
				return
			}
			if err != nil {
				walletError(w, 503, "MIGRATION_NOTICE_UNVERIFIED")
				return
			}
			walletSuccess(w, result)
			return
		}
		notice, err := store.ReadMigrationNotice(ctx, account.ID, none)
		if err != nil {
			walletError(w, 503, "MIGRATION_NOTICE_UNVERIFIED")
			return
		}
		if !gate {
			walletSuccess(w, notice)
			return
		}
		if notice.State == "UNVERIFIED" {
			send("MIGRATION_UNVERIFIED")
			return
		}
		if notice.State == "REQUIRED" {
			view.Notice = &notice
			send("MIGRATION_REQUIRED")
			return
		}
		if notice.State != "NOT_REQUIRED" && notice.State != "ACKNOWLEDGED" {
			send("MIGRATION_UNVERIFIED")
			return
		}
		if route == "/ops/announcements" {
			_, err = store.AnnouncementAuthority(ctx, account.ID)
			if errors.Is(err, platform.ErrAnnouncementForbidden) {
				send("ROLE_DENIED")
				return
			}
			if err != nil {
				send("ROLE_UNVERIFIED")
				return
			}
		}
		if route == "/ops/models" {
			_, err = store.CatalogAuthority(ctx, account.ID)
			if errors.Is(err, platform.ErrCatalogForbidden) {
				send("ROLE_DENIED")
				return
			}
			if err != nil {
				send("ROLE_UNVERIFIED")
				return
			}
		}
		if route == "/admin/channels" {
			if account.Role == nil {
				send("ROLE_UNVERIFIED")
				return
			}
			if *account.Role < 10 {
				send("ROLE_DENIED")
				return
			}
		}
		resource := ""
		if declaration != nil {
			resource = declaration.Resources[gateRouteDomain(route)]
		}
		switch resource {
		case "AVAILABLE":
			send("READY")
		case "MAINTENANCE":
			send("MAINTENANCE")
		case "UNAVAILABLE":
			send("RESOURCE_UNAVAILABLE")
		default:
			send("RESOURCE_UNVERIFIED")
		}
	})
}
