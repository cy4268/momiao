package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/cy4268/momiao/internal/platform"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"
)

type profileStore interface {
	ReadProfile(context.Context, int64) (platform.Profile, error)
	InitializeProfile(context.Context, int64, int64, string, string) (platform.Profile, error)
	UpdateProfile(context.Context, int64, platform.ProfilePatch) (platform.Profile, error)
}

func newProfileHandler(origin string, store profileStore, transport http.RoundTripper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			walletError(w, 503, "PROFILE_UNAVAILABLE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		user, status := verifyWalletUser(r, transport)
		if status != 0 {
			code := "AUTH_UNAVAILABLE"
			if status == 401 {
				code = "AUTH_UNAUTHORIZED"
			} else if status == 403 {
				code = "AUTH_FORBIDDEN"
			}
			walletError(w, status, code)
			return
		}
		initialize := r.URL.Path == "/platform/v1/master-profile/initialize"
		if !initialize && r.URL.Path != "/platform/v1/master-profile" {
			walletError(w, 404, "NOT_FOUND")
			return
		}
		allowed := "GET, PATCH"
		if initialize {
			allowed = "POST"
		}
		if (initialize && r.Method != http.MethodPost) || (!initialize && r.Method != http.MethodGet && r.Method != http.MethodPatch) {
			w.Header().Set("Allow", allowed)
			walletError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			walletError(w, 400, "INVALID_REQUEST")
			return
		}
		var p platform.Profile
		var err error
		if r.Method == http.MethodGet {
			p, err = store.ReadProfile(ctx, user)
		} else {
			if origins := r.Header.Values("Origin"); len(origins) != 1 || origins[0] != origin {
				walletError(w, 403, "ORIGIN_REJECTED")
				return
			}
			ct, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if e != nil || ct != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
				walletError(w, 415, "INVALID_CONTENT_TYPE")
				return
			}
			patch, e := decodeProfileRequest(r.Body, initialize)
			if e != nil {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
			if initialize {
				p, err = store.InitializeProfile(ctx, user, patch.ExpectedVersion, *patch.DisplayName, *patch.AvatarID)
			} else {
				p, err = store.UpdateProfile(ctx, user, patch)
			}
		}
		if err != nil {
			writeProfileError(w, err)
			return
		}
		walletSuccess(w, p)
	})
}

// Decode tokens rather than a struct or map so duplicates, nulls and wrong JSON
// types are rejected. Check raw UTF-8 before encoding/json can replace bytes.
func decodeProfileRequest(body io.Reader, initialize bool) (platform.ProfilePatch, error) {
	invalid := platform.ErrInvalidProfile
	raw, err := io.ReadAll(io.LimitReader(body, 8193))
	if err != nil || len(raw) > 8192 || !utf8.Valid(raw) {
		return platform.ProfilePatch{}, invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return platform.ProfilePatch{}, invalid
	}
	values := map[string]string{}
	for decoder.More() {
		token, err = decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || (key != "expected_version" && key != "display_name" && key != "avatar_id") {
			return platform.ProfilePatch{}, invalid
		}
		if _, exists := values[key]; exists {
			return platform.ProfilePatch{}, invalid
		}
		token, err = decoder.Token()
		value, ok := token.(string)
		if err != nil || !ok {
			return platform.ProfilePatch{}, invalid
		}
		values[key] = value
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') {
		return platform.ProfilePatch{}, invalid
	}
	if _, err = decoder.Token(); err != io.EOF {
		return platform.ProfilePatch{}, invalid
	}
	version, err := decimalInt(values["expected_version"])
	if err != nil || strconv.FormatInt(version, 10) != values["expected_version"] || (initialize && version != 0) || (!initialize && version < 1) {
		return platform.ProfilePatch{}, invalid
	}
	patch := platform.ProfilePatch{ExpectedVersion: version}
	if value, ok := values["display_name"]; ok {
		patch.DisplayName = &value
	}
	if value, ok := values["avatar_id"]; ok {
		patch.AvatarID = &value
	}
	if (initialize && (patch.DisplayName == nil || patch.AvatarID == nil)) || (!initialize && patch.DisplayName == nil && patch.AvatarID == nil) {
		return platform.ProfilePatch{}, invalid
	}
	return patch, nil
}

func writeProfileError(w http.ResponseWriter, err error) {
	status, code := 503, "PROFILE_UNAVAILABLE"
	switch {
	case errors.Is(err, platform.ErrNicknameTaken):
		status, code = 409, "NICKNAME_TAKEN"
	case errors.Is(err, platform.ErrStaleProfileVersion):
		status, code = 409, "STALE_RESOURCE_VERSION"
	case errors.Is(err, platform.ErrRenameCooldown):
		status, code = 409, "RENAME_COOLDOWN"
	case errors.Is(err, platform.ErrNicknameReserved):
		status, code = 403, "NICKNAME_RESERVED"
	case errors.Is(err, platform.ErrInvalidNickname):
		status, code = 400, "INVALID_NICKNAME"
	case errors.Is(err, platform.ErrInvalidAvatar):
		status, code = 400, "INVALID_AVATAR"
	case errors.Is(err, platform.ErrInvalidProfile):
		status, code = 400, "INVALID_REQUEST"
	}
	walletError(w, status, code)
}
