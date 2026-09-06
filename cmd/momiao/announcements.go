package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/cy4268/momiao/internal/nativeself"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/platform"
)

type announcementStore interface {
	AnnouncementAuthority(context.Context, int64) (platform.AnnouncementPrincipal, error)
	OpsAnnouncements(context.Context, int64) (platform.AnnouncementPrincipal, []platform.OpsAnnouncement, error)
	OpsAnnouncement(context.Context, int64, string) (platform.AnnouncementPrincipal, platform.OpsAnnouncement, error)
	PrepareAnnouncement(context.Context, int64, platform.AnnouncementCommand) (platform.AnnouncementPreview, error)
	ExecuteAnnouncement(context.Context, int64, platform.AnnouncementCommand, string, bool) (platform.AnnouncementResult, error)
	PublicAnnouncements(context.Context, int64, platform.AnnouncementFilter) (platform.AnnouncementPage, error)
	PublicAnnouncement(context.Context, int64, string, bool) (platform.Announcement, error)
	ReadAnnouncement(context.Context, int64, string, int64) (time.Time, error)
}

var announcementIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func announcementBrowserRoute(path string) bool {
	return path == "/announcements" || path == "/ops/announcements" || strings.HasPrefix(path, "/announcements/") && announcementIDPattern.MatchString(strings.TrimPrefix(path, "/announcements/"))
}

func newAnnouncementHandler(origin string, store announcementStore, transport http.RoundTripper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		ops := strings.HasPrefix(r.URL.Path, "/platform/v1/ops/announcements")
		base := "/platform/v1/announcements"
		if ops {
			base = "/platform/v1/ops/announcements"
		}
		tail := strings.TrimPrefix(r.URL.Path, base)
		reads := strings.HasSuffix(tail, "/reads")
		var user int64
		hasAuth := r.Header.Get("Authorization") != "" || len(r.Header.Values("Authorization")) > 0 || len(r.Header.Values("New-Api-User")) > 0 || len(r.Header.Values("X-Auth-Session")) > 0
		if ops || reads || hasAuth || tail == "/current-post-login-popup" {
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
		if store == nil {
			walletError(w, 503, "ANNOUNCEMENTS_UNAVAILABLE")
			return
		}
		if ops {
			if _, err := store.AnnouncementAuthority(ctx, user); err != nil {
				writeAnnouncementError(w, err)
				return
			}
		}
		if r.Method != "GET" && r.Method != "POST" {
			w.Header().Set("Allow", "GET, POST")
			walletError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		if r.Method == "POST" {
			if origins := r.Header.Values("Origin"); len(origins) != 1 || origins[0] != origin {
				walletError(w, 403, "ORIGIN_REJECTED")
				return
			}
			ct, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || ct != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
				walletError(w, 415, "INVALID_CONTENT_TYPE")
				return
			}
			if r.URL.RawQuery != "" || r.URL.ForceQuery {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
		}
		var data any
		var err error
		if ops {
			if r.URL.RawQuery != "" || r.URL.ForceQuery {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
			switch {
			case r.Method == "GET" && tail == "":
				var p platform.AnnouncementPrincipal
				var items []platform.OpsAnnouncement
				p, items, err = store.OpsAnnouncements(ctx, user)
				data = map[string]any{"principal": p, "items": items}
			case r.Method == "GET" && announcementIDPattern.MatchString(strings.TrimPrefix(tail, "/")):
				var p platform.AnnouncementPrincipal
				var item platform.OpsAnnouncement
				p, item, err = store.OpsAnnouncement(ctx, user, strings.TrimPrefix(tail, "/"))
				data = map[string]any{"principal": p, "item": item}
			case r.Method == "POST" && tail == "/render-preview":
				var body struct {
					Content platform.AnnouncementContent `json:"content"`
				}
				if !decodeAnnouncementBody(r.Body, &body) {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
				data, err = platform.RenderAnnouncement(body.Content.Markdown)
			case r.Method == "POST" && tail == "/prepare":
				var body struct {
					Command platform.AnnouncementCommand `json:"command"`
				}
				if !decodeAnnouncementBody(r.Body, &body) {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
				data, err = store.PrepareAnnouncement(ctx, user, body.Command)
			case r.Method == "POST" && tail == "/execute":
				var body struct {
					Command   platform.AnnouncementCommand `json:"command"`
					PreviewID string                       `json:"preview_id"`
					Confirmed bool                         `json:"confirmed"`
				}
				if !decodeAnnouncementBody(r.Body, &body) {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
				data, err = store.ExecuteAnnouncement(ctx, user, body.Command, body.PreviewID, body.Confirmed)
			default:
				walletError(w, 404, "NOT_FOUND")
				return
			}
		} else {
			switch {
			case r.Method == "POST" && reads && announcementIDPattern.MatchString(strings.TrimSuffix(strings.TrimPrefix(tail, "/"), "/reads")):
				var body struct {
					Revision int64 `json:"notification_revision"`
				}
				if !decodeAnnouncementBody(r.Body, &body) {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
				var at time.Time
				at, err = store.ReadAnnouncement(ctx, user, strings.TrimSuffix(strings.TrimPrefix(tail, "/"), "/reads"), body.Revision)
				data = map[string]any{"notification_revision": body.Revision, "read_at": at}
			case r.Method == "GET":
				f, e := announcementQuery(r.URL)
				if e != nil {
					walletError(w, 400, "INVALID_REQUEST")
					return
				}
				switch {
				case tail == "":
					data, err = store.PublicAnnouncements(ctx, user, f)
				case announcementIDPattern.MatchString(strings.TrimPrefix(tail, "/")):
					if len(r.URL.Query()) > 0 && (len(r.URL.Query()) != 1 || r.URL.Query().Get("archive") == "") {
						walletError(w, 400, "INVALID_REQUEST")
						return
					}
					data, err = store.PublicAnnouncement(ctx, user, strings.TrimPrefix(tail, "/"), f.Archive)
				case tail == "/current-entry-popup" || tail == "/current-home-banner" || tail == "/current-post-login-popup" || tail == "/dashboard-summary":
					if r.URL.RawQuery != "" {
						walletError(w, 400, "INVALID_REQUEST")
						return
					}
					f.Placement = map[string]string{"/current-entry-popup": "ENTRY_POPUP", "/current-home-banner": "PUBLIC_HOME_BANNER", "/current-post-login-popup": "POST_LOGIN_POPUP", "/dashboard-summary": "DASHBOARD_SUMMARY"}[tail]
					if tail == "/current-entry-popup" && user != 0 {
						data = map[string]any{"item": nil}
						break
					}
					f.Limit = 50
					var page platform.AnnouncementPage
					page, err = store.PublicAnnouncements(ctx, user, f)
					if tail == "/current-post-login-popup" {
						data = map[string]any{"candidates": page.Items, "has_more": page.HasMore}
					} else if tail == "/dashboard-summary" {
						data = page
					} else {
						if len(page.Items) > 1 {
							walletError(w, 503, "ANNOUNCEMENTS_UNAVAILABLE")
							return
						}
						var item *platform.Announcement
						if len(page.Items) == 1 {
							item = &page.Items[0]
						}
						data = map[string]any{"item": item}
					}
				default:
					walletError(w, 404, "NOT_FOUND")
					return
				}
			default:
				walletError(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
		}
		if err != nil {
			writeAnnouncementError(w, err)
			return
		}
		walletSuccess(w, data)
	})
}

func announcementSessionCredential(r *http.Request) bool { return nativeself.SessionCredential(r) }
func writeAnnouncementError(w http.ResponseWriter, err error) {
	status, code := 503, "ANNOUNCEMENTS_UNAVAILABLE"
	switch {
	case errors.Is(err, platform.ErrAnnouncementInvalid):
		status, code = 400, err.Error()
	case errors.Is(err, platform.ErrAnnouncementForbidden), errors.Is(err, platform.ErrAnnouncementStale):
		status, code = 403, err.Error()
	case errors.Is(err, platform.ErrAnnouncementNotFound):
		status, code = 404, err.Error()
	case errors.Is(err, platform.ErrAnnouncementConflict), errors.Is(err, platform.ErrAnnouncementWindow), errors.Is(err, platform.ErrAnnouncementConfirmation), errors.Is(err, platform.ErrAnnouncementOperation):
		status, code = 409, err.Error()
	}
	walletError(w, status, code)
}
func decodeAnnouncementBody(reader io.Reader, target any) bool {
	raw, err := io.ReadAll(io.LimitReader(reader, 65537))
	if err != nil || len(raw) > 65536 || !utf8.Valid(raw) {
		return false
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return false
	}
	// Reject duplicate names recursively before decoding the strict typed request.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if !uniqueAnnouncementJSON(decoder, 0) {
		return false
	}
	if _, err = decoder.Token(); err != io.EOF {
		return false
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil
}
func uniqueAnnouncementJSON(d *json.Decoder, depth int) bool {
	if depth > 12 {
		return false
	}
	token, err := d.Token()
	if err != nil {
		return false
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return true
	}
	if delimiter == '{' {
		seen := map[string]bool{}
		for d.More() {
			token, err = d.Token()
			key, ok := token.(string)
			if err != nil || !ok || seen[key] {
				return false
			}
			seen[key] = true
			if !uniqueAnnouncementJSON(d, depth+1) {
				return false
			}
		}
		token, err = d.Token()
		return err == nil && token == json.Delim('}')
	}
	if delimiter == '[' {
		for d.More() {
			if !uniqueAnnouncementJSON(d, depth+1) {
				return false
			}
		}
		token, err = d.Token()
		return err == nil && token == json.Delim(']')
	}
	return false
}
func announcementQuery(u *url.URL) (platform.AnnouncementFilter, error) {
	f := platform.AnnouncementFilter{Limit: 20}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return f, err
	}
	for key, values := range q {
		if len(values) != 1 || !strings.Contains("|type|search|date_from|date_to|archive|offset|limit|", "|"+key+"|") {
			return f, platform.ErrAnnouncementInvalid
		}
	}
	f.Type = q.Get("type")
	f.Search = q.Get("search")
	if v, ok := q["archive"]; ok {
		if v[0] != "true" && v[0] != "false" {
			return f, platform.ErrAnnouncementInvalid
		}
		f.Archive = v[0] == "true"
	}
	for _, key := range []string{"offset", "limit"} {
		if v, ok := q[key]; ok {
			n, e := strconv.Atoi(v[0])
			if e != nil || n < 0 || key == "limit" && (n < 1 || n > 50) || key == "offset" && n > 10000 {
				return f, platform.ErrAnnouncementInvalid
			}
			if key == "limit" {
				f.Limit = n
			} else {
				f.Offset = n
			}
		}
	}
	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	for _, key := range []string{"date_from", "date_to"} {
		if v, ok := q[key]; ok {
			d, e := time.ParseInLocation("2006-01-02", v[0], shanghai)
			if e != nil {
				return f, platform.ErrAnnouncementInvalid
			}
			if key == "date_from" {
				f.DateFrom = &d
			} else {
				d = d.AddDate(0, 0, 1)
				f.DateTo = &d
			}
		}
	}
	if f.DateFrom != nil && f.DateTo != nil && !f.DateTo.After(*f.DateFrom) {
		return f, platform.ErrAnnouncementInvalid
	}
	return f, nil
}

func runAnnouncementWorker(ctx context.Context, store *platform.Store) {
	for {
		if ctx.Err() != nil {
			return
		}
		call, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, _ = store.RunAnnouncementJobs(call)
		cancel()
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
