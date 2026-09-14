package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/cy4268/momiao/internal/bffauth"
	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/cy4268/momiao/internal/session"
)

func sessionAPIRoute(name string) bool {
	return name == "/api/v1/session" || strings.HasPrefix(name, "/api/v1/session/") ||
		name == "/api/v1/auth" || strings.HasPrefix(name, "/api/v1/auth/") ||
		name == "/api/v1/account" || strings.HasPrefix(name, "/api/v1/account/")
}

func legacyNativeBrowserAuthRoute(name string) bool {
	return name == "/api/user/login" || name == "/api/user/login/2fa" ||
		name == "/api/user/auth/refresh" || name == "/api/user/auth/logout" ||
		name == "/api/momiao/auth" || strings.HasPrefix(name, "/api/momiao/auth/") ||
		name == "/api/momiao/account" || strings.HasPrefix(name, "/api/momiao/account/")
}

func sessionEnvelope(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Success bool `json:"success"`
		Data    any  `json:"data"`
	}{true, data})
}

func sessionError(w http.ResponseWriter, err error) {
	status, code, clear := http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", false
	var bffFault bffauth.Fault
	var platformFault session.Fault
	if errors.As(err, &bffFault) {
		status, code, clear = bffFault.Status, bffFault.Code, bffFault.ClearCookie
	} else if errors.As(err, &platformFault) {
		status, code, clear = platformFault.Status, platformFault.Code, platformFault.ClearCookie
	}
	if clear {
		bffauth.ClearCookie(w)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}{Success: false, Error: struct {
		Code string `json:"code"`
	}{Code: code}})
}

func sessionRequestSafe(r *http.Request) bool {
	if r == nil || r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength > 8192 {
		return false
	}
	for _, name := range []string{"Authorization", "New-Api-User", "X-Auth-Session"} {
		if len(r.Header.Values(name)) != 0 {
			return false
		}
	}
	for _, header := range r.Header.Values("Cookie") {
		cookies, err := http.ParseCookie(header)
		if err != nil {
			return false
		}
		for _, cookie := range cookies {
			if cookie.Name == "new_api_refresh" || cookie.Name == "__Host-momiao_oauth" {
				return false
			}
		}
	}
	return true
}

func decodeSessionBody(r *http.Request, out any, allowed ...string) bool {
	if r == nil || len(r.Header.Values("Content-Type")) != 1 {
		return false
	}
	media, parameters, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(parameters) != 0 {
		return false
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 8193))
	if err != nil || len(raw) == 0 || len(raw) > 8192 {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if !nativeself.UniqueJSON(decoder, 0) {
		return false
	}
	if _, err = decoder.Token(); err != io.EOF {
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return false
	}
	keys := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		keys[key] = true
	}
	for key := range object {
		if !keys[key] {
			return false
		}
	}
	return json.Unmarshal(raw, out) == nil
}

func validLoginText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value
}

func newSessionHandler(auth *bffauth.Service, sessions *session.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth == nil || sessions == nil {
			sessionError(w, bffauth.Fault{Status: 503, Code: "SESSION_UNAVAILABLE"})
			return
		}
		if !sessionRequestSafe(r) {
			sessionError(w, bffauth.Fault{Status: 400, Code: "AUTH_INPUT_INVALID"})
			return
		}
		switch r.URL.Path {
		case "/api/v1/session/bootstrap":
			if r.Method != http.MethodGet || r.ContentLength > 0 {
				w.Header().Set("Allow", "GET")
				sessionError(w, bffauth.Fault{Status: 405, Code: "METHOD_NOT_ALLOWED"})
				return
			}
			result, err := auth.Bootstrap(r)
			if err != nil {
				sessionError(w, err)
				return
			}
			if result.Cookie != "" {
				bffauth.WriteCookie(w, result.Cookie)
			}
			sessionEnvelope(w, http.StatusOK, result)
		case "/api/v1/auth/password/login":
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", "POST")
				sessionError(w, bffauth.Fault{Status: 405, Code: "METHOD_NOT_ALLOWED"})
				return
			}
			var body struct {
				Identifier string `json:"identifier"`
				Password   string `json:"password"`
				Turnstile  string `json:"turnstile_token,omitempty"`
			}
			if !decodeSessionBody(r, &body, "identifier", "password", "turnstile_token") || !validLoginText(body.Identifier, 128) || body.Password == "" || len(body.Password) > 1024 || len(body.Turnstile) > 4096 {
				sessionError(w, bffauth.Fault{Status: 400, Code: "AUTH_INPUT_INVALID"})
				return
			}
			result, err := auth.AuthenticatePassword(r, body.Identifier, body.Password, body.Turnstile)
			if err != nil {
				sessionError(w, err)
				return
			}
			if result.Grant != nil {
				if err = sessions.WriteGrant(w, *result.Grant); err != nil {
					sessionError(w, err)
					return
				}
			}
			sessionEnvelope(w, http.StatusOK, result)
		case "/api/v1/auth/password/2fa":
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", "POST")
				sessionError(w, bffauth.Fault{Status: 405, Code: "METHOD_NOT_ALLOWED"})
				return
			}
			var body struct {
				ChallengeID string `json:"challenge_id"`
				Code        string `json:"code"`
			}
			if !decodeSessionBody(r, &body, "challenge_id", "code") || !validLoginText(body.ChallengeID, 64) || !validLoginText(body.Code, 128) {
				sessionError(w, bffauth.Fault{Status: 400, Code: "AUTH_INPUT_INVALID"})
				return
			}
			result, err := auth.AuthenticateTwoFA(r, body.ChallengeID, body.Code)
			if err != nil {
				sessionError(w, err)
				return
			}
			if result.Grant == nil || sessions.WriteGrant(w, *result.Grant) != nil {
				sessionError(w, bffauth.Fault{Status: 503, Code: "AUTH_RESULT_UNKNOWN", ClearCookie: true})
				return
			}
			sessionEnvelope(w, http.StatusOK, result)
		case "/api/v1/auth/discord/login/start",
			"/api/v1/auth/discord/registration/start",
			"/api/v1/auth/native-ui/discord/login/start",
			"/api/v1/auth/native-ui/discord/registration/start",
			"/api/v1/auth/discord/fresh/start",
			"/api/v1/auth/discord/password-reset/start":
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", "POST")
				sessionError(w, bffauth.Fault{Status: 405, Code: "METHOD_NOT_ALLOWED"})
				return
			}
			var body struct{}
			if !decodeSessionBody(r, &body) {
				sessionError(w, bffauth.Fault{Status: 400, Code: "AUTH_INPUT_INVALID"})
				return
			}
			purpose := bffauth.DiscordPurposeLogin
			switch r.URL.Path {
			case "/api/v1/auth/discord/registration/start", "/api/v1/auth/native-ui/discord/registration/start":
				purpose = bffauth.DiscordPurposeRegistration
			case "/api/v1/auth/discord/fresh/start":
				purpose = bffauth.DiscordPurposeFresh
			case "/api/v1/auth/discord/password-reset/start":
				purpose = bffauth.DiscordPurposePasswordReset
			}
			var result bffauth.DiscordResult
			var err error
			if strings.HasPrefix(r.URL.Path, "/api/v1/auth/native-ui/") {
				result, err = auth.StartNativeUIDiscord(r, purpose)
			} else {
				result, err = auth.StartDiscord(r, purpose)
			}
			if err != nil {
				sessionError(w, err)
				return
			}
			sessionEnvelope(w, http.StatusOK, result)
		case "/api/v1/auth/discord/callback":
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", "POST")
				sessionError(w, bffauth.Fault{Status: 405, Code: "METHOD_NOT_ALLOWED"})
				return
			}
			var body bffauth.DiscordCallbackInput
			if !decodeSessionBody(r, &body, "code", "error", "error_description", "state") {
				sessionError(w, bffauth.Fault{Status: 400, Code: "AUTH_INPUT_INVALID"})
				return
			}
			result, err := auth.CompleteDiscord(r, body)
			if err != nil {
				sessionError(w, err)
				return
			}
			if result.Grant != nil {
				if err = sessions.WriteGrant(w, *result.Grant); err != nil {
					sessionError(w, bffauth.Fault{Status: 503, Code: "AUTH_RESULT_UNKNOWN", ClearCookie: true})
					return
				}
			}
			sessionEnvelope(w, http.StatusOK, result)
		case "/api/v1/auth/discord/2fa":
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", "POST")
				sessionError(w, bffauth.Fault{Status: 405, Code: "METHOD_NOT_ALLOWED"})
				return
			}
			var body struct {
				ChallengeID string `json:"challenge_id"`
				Code        string `json:"code"`
			}
			if !decodeSessionBody(r, &body, "challenge_id", "code") || !validLoginText(body.ChallengeID, 64) || !validLoginText(body.Code, 128) {
				sessionError(w, bffauth.Fault{Status: 400, Code: "AUTH_INPUT_INVALID"})
				return
			}
			result, err := auth.VerifyDiscordTwoFA(r, body.ChallengeID, body.Code)
			if err != nil {
				sessionError(w, err)
				return
			}
			if result.Grant != nil {
				if err = sessions.WriteGrant(w, *result.Grant); err != nil {
					sessionError(w, bffauth.Fault{Status: 503, Code: "AUTH_RESULT_UNKNOWN", ClearCookie: true})
					return
				}
			}
			sessionEnvelope(w, http.StatusOK, result)
		case "/api/v1/account":
			if r.Method != http.MethodGet || r.ContentLength > 0 {
				w.Header().Set("Allow", "GET")
				sessionError(w, bffauth.Fault{Status: 405, Code: "METHOD_NOT_ALLOWED"})
				return
			}
			result, err := auth.Account(r)
			if err != nil {
				sessionError(w, err)
				return
			}
			sessionEnvelope(w, http.StatusOK, result)
		case "/api/v1/account/password/set",
			"/api/v1/account/password/change",
			"/api/v1/account/password/reset":
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", "POST")
				sessionError(w, bffauth.Fault{Status: 405, Code: "METHOD_NOT_ALLOWED"})
				return
			}
			var body struct {
				OldPassword string `json:"old_password,omitempty"`
				Password    string `json:"password"`
			}
			mode := strings.TrimPrefix(r.URL.Path, "/api/v1/account/password/")
			allowed := []string{"password"}
			if mode == "change" {
				allowed = append(allowed, "old_password")
			}
			if !decodeSessionBody(r, &body, allowed...) || body.Password == "" || len(body.Password) > 1024 || mode == "change" && (body.OldPassword == "" || len(body.OldPassword) > 1024) {
				sessionError(w, bffauth.Fault{Status: 400, Code: "AUTH_INPUT_INVALID"})
				return
			}
			result, err := auth.AccountPassword(r, mode, body.Password, body.OldPassword)
			if err != nil {
				sessionError(w, err)
				return
			}
			if result.Grant == nil || sessions.WriteGrant(w, *result.Grant) != nil {
				sessionError(w, bffauth.Fault{Status: 503, Code: "AUTH_RESULT_UNKNOWN", ClearCookie: true})
				return
			}
			sessionEnvelope(w, http.StatusOK, result)
		case "/api/v1/auth/logout":
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", "POST")
				sessionError(w, bffauth.Fault{Status: 405, Code: "METHOD_NOT_ALLOWED"})
				return
			}
			var body struct{}
			if !decodeSessionBody(r, &body) {
				sessionError(w, bffauth.Fault{Status: 400, Code: "AUTH_INPUT_INVALID"})
				return
			}
			result, err := auth.Logout(r)
			if err != nil {
				sessionError(w, err)
				return
			}
			bffauth.ClearCookie(w)
			status := http.StatusAccepted
			if result.Confirmed() {
				status = http.StatusOK
			}
			sessionEnvelope(w, status, result)
		default:
			sessionError(w, bffauth.Fault{Status: 404, Code: "AUTH_ROUTE_NOT_FOUND"})
		}
	})
}
