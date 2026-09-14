package bffauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/session"
)

const (
	CookieName      = "__Host-chaldea_session"
	preauthPrefix   = "chaldea:bff:preauth:"
	anonymousTTL    = 30 * time.Minute
	challengeTTL    = 5 * time.Minute
	committedTTL    = time.Minute
	maximumAttempts = 5
)

type preauthRecord struct {
	Schema             int    `json:"schema_version"`
	State              string `json:"state"`
	Hash               string `json:"session_id_hash"`
	CSRF               string `json:"csrf_token"`
	Created            int64  `json:"created_at_ms"`
	Expires            int64  `json:"expires_at_ms"`
	Purpose            string `json:"discord_purpose,omitempty"`
	AuthSurface        string `json:"auth_surface,omitempty"`
	OperationHash      string `json:"operation_hash,omitempty"`
	NativeStateHash    string `json:"native_state_hash,omitempty"`
	ChallengeHash      string `json:"challenge_hash,omitempty"`
	ChallengeExpires   int64  `json:"challenge_expires_at_ms,omitempty"`
	Attempts           int    `json:"attempts,omitempty"`
	NativeBrowserBox   string `json:"native_browser_box,omitempty"`
	NativeFlowBox      string `json:"native_flow_box,omitempty"`
	PlatformSessionBox string `json:"platform_session_box,omitempty"`
	PlatformCSRF       string `json:"platform_csrf,omitempty"`
	NativeHash         string `json:"native_session_id_hash,omitempty"`
	raw                string `json:"-"`
}

type PreauthView struct {
	State     string    `json:"authentication_state"`
	CSRFToken string    `json:"csrf_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type LoginChallenge struct {
	ID        string    `json:"challenge_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

func WriteCookie(w http.ResponseWriter, value string) {
	w.Header().Set("Cache-Control", "no-store")
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func ClearCookie(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func cookieID(r *http.Request) (string, error) {
	if r == nil {
		return "", errInput
	}
	value, count := "", 0
	for _, header := range r.Header.Values("Cookie") {
		for _, part := range strings.Split(header, ";") {
			name, v, ok := strings.Cut(strings.TrimSpace(part), "=")
			if ok && name == CookieName {
				count++
				value = v
			}
		}
	}
	if count == 0 {
		return "", errMissing
	}
	if count != 1 || !opaque(value) {
		return "", errUnauthorized
	}
	return value, nil
}

func validPreauth(r preauthRecord, hash string, now int64) bool {
	if r.Schema != 1 || r.Hash != hash || !digest(hash) || !opaque(r.CSRF) || r.Created <= 0 || r.Expires <= now || r.Created > now || r.Expires-r.Created != anonymousTTL.Milliseconds() || r.Attempts < 0 || r.Attempts > maximumAttempts {
		return false
	}
	emptyDiscord := r.Purpose == "" && r.AuthSurface == "" && r.NativeStateHash == "" && r.NativeBrowserBox == ""
	validDiscordSurface := r.AuthSurface == "" || r.AuthSurface == nativeUIAuthSurface
	emptyPlatform := r.PlatformSessionBox == "" && r.PlatformCSRF == "" && r.NativeHash == ""
	switch r.State {
	case "ANONYMOUS":
		return emptyDiscord && emptyPlatform && r.OperationHash == "" && r.ChallengeHash == "" && r.ChallengeExpires == 0 && r.NativeFlowBox == "" && r.Attempts == 0
	case "PASSWORD_PENDING":
		return emptyDiscord && emptyPlatform && digest(r.OperationHash) && r.ChallengeHash == "" && r.ChallengeExpires == 0 && r.NativeFlowBox == "" && r.Attempts == 0
	case "TWO_FA", "TWO_FA_PENDING":
		return emptyDiscord && emptyPlatform && digest(r.ChallengeHash) && r.ChallengeExpires > now && r.ChallengeExpires <= r.Expires && r.NativeFlowBox != "" && (r.State == "TWO_FA" && r.OperationHash == "" || r.State == "TWO_FA_PENDING" && digest(r.OperationHash))
	case "DISCORD_START_PENDING":
		return validAnonymousDiscordPurpose(r.Purpose) && validDiscordSurface && digest(r.OperationHash) && r.NativeStateHash == "" && r.NativeBrowserBox == "" && r.NativeFlowBox == "" && r.ChallengeHash == "" && r.ChallengeExpires == 0 && emptyPlatform && r.Attempts == 0
	case "DISCORD_CALLBACK", "DISCORD_CALLBACK_PENDING":
		return validAnonymousDiscordPurpose(r.Purpose) && validDiscordSurface && digest(r.NativeStateHash) && r.NativeBrowserBox != "" && r.NativeFlowBox == "" && r.ChallengeHash == "" && r.ChallengeExpires == 0 && emptyPlatform && r.Attempts == 0 && (r.State == "DISCORD_CALLBACK" && r.OperationHash == "" || r.State == "DISCORD_CALLBACK_PENDING" && digest(r.OperationHash))
	case "DISCORD_TWO_FA", "DISCORD_TWO_FA_PENDING":
		return validAnonymousDiscordPurpose(r.Purpose) && validDiscordSurface && digest(r.NativeStateHash) && r.NativeBrowserBox != "" && r.NativeFlowBox != "" && digest(r.ChallengeHash) && r.ChallengeExpires > now && r.ChallengeExpires <= r.Expires && emptyPlatform && (r.State == "DISCORD_TWO_FA" && r.OperationHash == "" || r.State == "DISCORD_TWO_FA_PENDING" && digest(r.OperationHash))
	case "COMMITTED":
		return emptyDiscord && r.OperationHash == "" && r.NativeFlowBox == "" && r.ChallengeHash == "" && r.ChallengeExpires == 0 && r.PlatformSessionBox != "" && opaque(r.PlatformCSRF) && digest(r.NativeHash) && r.Attempts == 0
	case "CANCELLED", "UNKNOWN":
		return emptyDiscord && emptyPlatform && r.OperationHash == "" && r.NativeFlowBox == "" && r.ChallengeHash == "" && r.ChallengeExpires == 0 && r.Attempts == 0
	}
	return false
}

func (s *Service) loadPreauth(ctx context.Context, sid string) (preauthRecord, error) {
	hash := hashText(sid)
	raw, now, ttl, err := s.readValue(ctx, preauthPrefix+hash)
	if err != nil {
		if errors.Is(err, errMissing) {
			return preauthRecord{}, errUnauthorized
		}
		return preauthRecord{}, err
	}
	var r preauthRecord
	if !strictJSON([]byte(raw), &r, 16384) || !validPreauth(r, hash, now) || ttl > r.Expires-now+1000 {
		return preauthRecord{}, errUnauthorized
	}
	r.raw = raw
	return r, nil
}

func (s *Service) NewAnonymous(ctx context.Context) (string, PreauthView, error) {
	if s == nil || ctx == nil {
		return "", PreauthView{}, errInput
	}
	for range 3 {
		sid, err := randomSecret()
		if err != nil {
			return "", PreauthView{}, err
		}
		csrf, err := randomSecret()
		if err != nil {
			return "", PreauthView{}, err
		}
		now := time.Now().UnixMilli()
		r := preauthRecord{Schema: 1, State: "ANONYMOUS", Hash: hashText(sid), CSRF: csrf, Created: now, Expires: now + anonymousTTL.Milliseconds()}
		raw, _ := json.Marshal(r)
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		created, e := s.redis.SetNX(c, preauthPrefix+r.Hash, raw, anonymousTTL).Result()
		cancel()
		if e != nil {
			return "", PreauthView{}, errUnavailable
		}
		if created {
			return sid, r.view(), nil
		}
	}
	return "", PreauthView{}, errUnavailable
}

func (r preauthRecord) view() PreauthView {
	state := r.State
	if state == "PASSWORD_PENDING" || state == "TWO_FA_PENDING" || state == "DISCORD_START_PENDING" || state == "DISCORD_CALLBACK_PENDING" || state == "DISCORD_TWO_FA_PENDING" {
		state = "AUTHENTICATION_PENDING"
	}
	return PreauthView{State: state, CSRFToken: r.CSRF, ExpiresAt: time.UnixMilli(r.Expires).UTC()}
}

func (s *Service) verifyPreauth(r *http.Request) (preauthRecord, string, error) {
	if r == nil || len(r.Header.Values("Origin")) != 1 || r.Header.Get("Origin") != s.origin || len(r.Header.Values("Sec-Fetch-Site")) != 1 || r.Header.Get("Sec-Fetch-Site") != "same-origin" || len(r.Header.Values("X-CSRF-Token")) != 1 {
		return preauthRecord{}, "", errCSRF
	}
	sid, err := cookieID(r)
	if err != nil {
		return preauthRecord{}, "", err
	}
	record, err := s.loadPreauth(r.Context(), sid)
	if err != nil {
		return preauthRecord{}, "", err
	}
	if record.CSRF != r.Header.Get("X-CSRF-Token") {
		return preauthRecord{}, "", errCSRF
	}
	return record, sid, nil
}

func (s *Service) begin(r *http.Request, challenge string) (preauthRecord, string, error) {
	record, _, err := s.verifyPreauth(r)
	if err != nil {
		return record, "", err
	}
	want, next := "ANONYMOUS", "PASSWORD_PENDING"
	if challenge != "" {
		want, next = "TWO_FA", "TWO_FA_PENDING"
	}
	if record.State != want || challenge != "" && (!opaque(challenge) || record.ChallengeHash != hashText(challenge)) {
		return record, "", errConflict
	}
	op, err := randomSecret()
	if err != nil {
		return record, "", err
	}
	old := record.raw
	record.State, record.OperationHash = next, hashText(op)
	raw, _ := json.Marshal(record)
	if err = s.casValue(r.Context(), preauthPrefix+record.Hash, old, string(raw), 0); err != nil {
		return record, "", err
	}
	record.raw = string(raw)
	return record, op, nil
}

func (s *Service) challenge(ctx context.Context, r preauthRecord, op, nativeFlow string, expires time.Time) (LoginChallenge, error) {
	if r.State != "PASSWORD_PENDING" || r.OperationHash != hashText(op) || nativeFlow == "" {
		return LoginChallenge{}, errConflict
	}
	client, err := randomSecret()
	if err != nil {
		return LoginChallenge{}, err
	}
	deadline := expires
	if cap := time.Now().Add(challengeTTL); deadline.After(cap) {
		deadline = cap
	}
	if deadline.UnixMilli() > r.Expires {
		deadline = time.UnixMilli(r.Expires)
	}
	if !deadline.After(time.Now()) {
		return LoginChallenge{}, errUnknown
	}
	box, err := s.sealJSON("flow:1:"+r.Hash+":"+hashText(client), struct {
		Token string `json:"token"`
	}{nativeFlow})
	if err != nil {
		return LoginChallenge{}, err
	}
	old := r.raw
	r.State, r.OperationHash, r.ChallengeHash, r.ChallengeExpires, r.NativeFlowBox, r.Attempts = "TWO_FA", "", hashText(client), deadline.UnixMilli(), box, 0
	raw, _ := json.Marshal(r)
	if err = s.casValue(ctx, preauthPrefix+r.Hash, old, string(raw), 0); err != nil {
		return LoginChallenge{}, err
	}
	return LoginChallenge{ID: client, ExpiresAt: deadline.UTC()}, nil
}

func (s *Service) nativeFlow(r preauthRecord) (string, error) {
	var value struct {
		Token string `json:"token"`
	}
	if s.openJSON("flow:1:"+r.Hash+":"+r.ChallengeHash, r.NativeFlowBox, &value) != nil || value.Token == "" {
		return "", errUnauthorized
	}
	return value.Token, nil
}

func (s *Service) resolve(ctx context.Context, r preauthRecord, op string, unknown bool) error {
	if r.OperationHash != hashText(op) {
		return errConflict
	}
	old, twoFA := r.raw, r.State == "TWO_FA_PENDING"
	r.OperationHash = ""
	if unknown {
		r.State, r.NativeFlowBox, r.ChallengeHash, r.PlatformSessionBox = "UNKNOWN", "", "", ""
	} else if twoFA && r.Attempts+1 < maximumAttempts {
		r.State, r.Attempts = "TWO_FA", r.Attempts+1
	} else if twoFA {
		r.State, r.NativeFlowBox, r.ChallengeHash = "CANCELLED", "", ""
	} else {
		r.State = "ANONYMOUS"
	}
	r.Purpose, r.AuthSurface, r.NativeStateHash, r.NativeBrowserBox = "", "", "", ""
	if r.State != "TWO_FA" {
		r.ChallengeExpires, r.Attempts = 0, 0
	}
	raw, _ := json.Marshal(r)
	return s.casValue(ctx, preauthPrefix+r.Hash, old, string(raw), 0)
}

type headerWriter struct{ header http.Header }

func (w *headerWriter) Header() http.Header       { return w.header }
func (*headerWriter) Write(p []byte) (int, error) { return len(p), nil }
func (*headerWriter) WriteHeader(int)             {}

func (s *Service) commit(ctx context.Context, r preauthRecord, op, nativeHash string, grant session.Grant) (string, error) {
	if r.OperationHash != hashText(op) || !digest(nativeHash) {
		return "", errConflict
	}
	w := &headerWriter{header: make(http.Header)}
	if err := s.sessions.WriteGrant(w, grant); err != nil {
		return "", errUnavailable
	}
	cookies := (&http.Response{Header: w.header}).Cookies()
	if len(cookies) != 1 || cookies[0].Name != CookieName || !opaque(cookies[0].Value) {
		return "", errUnavailable
	}
	box, err := s.sealJSON("upgrade:1:"+r.Hash, struct {
		SID string `json:"sid"`
	}{cookies[0].Value})
	if err != nil {
		return "", err
	}
	old := r.raw
	r.State, r.Purpose, r.AuthSurface, r.OperationHash, r.NativeStateHash, r.NativeBrowserBox, r.NativeFlowBox, r.ChallengeHash = "COMMITTED", "", "", "", "", "", "", ""
	r.ChallengeExpires, r.Attempts = 0, 0
	r.PlatformSessionBox, r.PlatformCSRF, r.NativeHash = box, grant.View().CSRFToken, nativeHash
	raw, _ := json.Marshal(r)
	ttl := min(committedTTL, time.Until(time.UnixMilli(r.Expires)))
	if err = s.casValue(ctx, preauthPrefix+r.Hash, old, string(raw), ttl); err != nil {
		return "", err
	}
	return cookies[0].Value, nil
}

func (s *Service) cancelPreauth(ctx context.Context, r preauthRecord) error {
	old := r.raw
	r.State, r.Purpose, r.AuthSurface, r.OperationHash, r.NativeStateHash, r.NativeBrowserBox, r.NativeFlowBox, r.ChallengeHash, r.PlatformSessionBox, r.PlatformCSRF, r.NativeHash = "CANCELLED", "", "", "", "", "", "", "", "", "", ""
	r.ChallengeExpires, r.Attempts = 0, 0
	raw, _ := json.Marshal(r)
	return s.casValue(ctx, preauthPrefix+r.Hash, old, string(raw), min(committedTTL, time.Until(time.UnixMilli(r.Expires))))
}
