// Copyright QuantumNous and contributors. AGPL-3.0-or-later.
// Adapted from official new-api@f1164142 controller/oauth.go:
// ten-minute state -> code exchange -> provider user -> one-use state.
// Storage is the existing Redis session store, not GORM auth flows or native.
package identity

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

const discordFlowTTL = 10 * time.Minute

type discordFlow struct {
	BrowserKey string `json:"browser_key"`
	Purpose    string `json:"purpose"`
}

func discordFlowKey(state string) string { return sessionKey(state) + ":discord" }

// Checking and deletion are atomic. A different browser cannot consume a flow.
const consumeBoundToken = `
if redis.call('GET',KEYS[1]) ~= ARGV[1] then return 0 end
redis.call('DEL',KEYS[1])
return 1
`

func (h *HTTP) discordStart(w http.ResponseWriter, r *http.Request, browser Session) error {
	purpose := "LOGIN"
	switch r.URL.Path {
	case "/api/v1/auth/discord/password-set/start":
		purpose = "PASSWORD_SET"
	case "/api/v1/auth/discord/password-reset/start":
		purpose = "PASSWORD_RESET"
	}
	if purpose == "LOGIN" && browser.UserID != 0 {
		return Fault{409, "AUTH_CONFLICT"}
	}
	if purpose != "LOGIN" && browser.UserID == 0 {
		return ErrSessionExpired
	}
	if h.Discord.ClientID == "" || h.Discord.ClientSecret == "" {
		return Fault{503, "DISCORD_CONFIG_ERROR"}
	}
	state := sessionToken()
	raw, err := json.Marshal(discordFlow{BrowserKey: sessionKey(browser.Token), Purpose: purpose})
	if err != nil {
		return err
	}
	if err := h.Sessions.Redis.Set(r.Context(), discordFlowKey(state), raw, discordFlowTTL).Err(); err != nil {
		return err
	}
	writeSuccess(w, map[string]any{"authorization_url": h.Discord.AuthorizationURL(state)})
	return nil
}

func (h *HTTP) discordCallback(w http.ResponseWriter, r *http.Request, browser Session) error {
	if h.Discord.ClientID == "" || h.Discord.ClientSecret == "" {
		return Fault{503, "DISCORD_CONFIG_ERROR"}
	}
	var body struct {
		State string `json:"state"`
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		return Fault{400, "AUTH_INPUT_INVALID"}
	}
	key := discordFlowKey(body.State)
	raw, err := h.Sessions.Redis.Get(r.Context(), key).Result()
	if errors.Is(err, redis.Nil) {
		return Fault{403, "AUTH_STATE_INVALID"}
	}
	if err != nil {
		return err
	}
	var flow discordFlow
	if err := json.Unmarshal([]byte(raw), &flow); err != nil {
		return err
	}
	if flow.BrowserKey != sessionKey(browser.Token) || (flow.Purpose == "LOGIN") != (browser.UserID == 0) {
		return Fault{403, "AUTH_STATE_INVALID"}
	}
	consume := func() error {
		consumed, err := h.Sessions.Redis.Eval(r.Context(), consumeBoundToken, []string{key}, raw).Int()
		if err != nil {
			return err
		}
		if consumed != 1 {
			return Fault{403, "AUTH_STATE_INVALID"}
		}
		return nil
	}
	if body.Error != "" {
		if err := consume(); err != nil {
			return err
		}
		return Fault{403, "DISCORD_DENIED"}
	}
	if body.Code == "" {
		return Fault{400, "AUTH_INPUT_INVALID"}
	}
	token, err := h.Discord.ExchangeToken(r.Context(), body.Code)
	if err != nil {
		return Fault{502, "DISCORD_UNAVAILABLE"}
	}
	profile, err := h.Discord.GetUserInfo(r.Context(), token)
	if err != nil {
		return Fault{502, "DISCORD_UNAVAILABLE"}
	}
	if err := consume(); err != nil {
		return err
	}
	if flow.Purpose != "LOGIN" {
		proof, err := h.Store.DiscordPasswordProof(r.Context(), h.Sessions, browser, profile.ID, flow.Purpose)
		if err != nil {
			return err
		}
		writeSuccess(w, proof)
		return nil
	}
	value, challenge, err := h.Store.DiscordLogin(r.Context(), h.Sessions, browser, profile.ID)
	if err != nil {
		return err
	}
	writeLogin(w, browser, value, challenge)
	return nil
}
