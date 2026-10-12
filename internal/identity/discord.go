// Copyright QuantumNous and contributors. AGPL-3.0-or-later.
// Adapted from official new-api@f1164142 oauth/discord.go. The OAuth exchange
// and profile fields are unchanged; config is explicit rather than global,
// and there is no Gin/provider registry or native authentication bridge.
package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Discord struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

type DiscordUser struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"global_name"`
}

func (d Discord) AuthorizationURL(state string) string {
	query := url.Values{
		"client_id": {d.ClientID}, "redirect_uri": {d.RedirectURI},
		"response_type": {"code"}, "scope": {"identify"}, "state": {state},
	}
	return "https://discord.com/oauth2/authorize?" + query.Encode()
}

func (d Discord) ExchangeToken(ctx context.Context, code string) (string, error) {
	if code == "" {
		return "", ErrCredentials
	}
	values := url.Values{}
	values.Set("client_id", d.ClientID)
	values.Set("client_secret", d.ClientSecret)
	values.Set("code", code)
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", d.RedirectURI)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://discord.com/api/v10/oauth2/token", strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Discord token exchange: HTTP %d", res.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err = json.NewDecoder(res.Body).Decode(&token); err != nil {
		return "", err
	}
	if token.AccessToken == "" {
		return "", ErrCredentials
	}
	return token.AccessToken, nil
}

func (d Discord) GetUserInfo(ctx context.Context, token string) (DiscordUser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://discord.com/api/v10/users/@me", nil)
	if err != nil {
		return DiscordUser{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return DiscordUser{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return DiscordUser{}, fmt.Errorf("Discord user info: HTTP %d", res.StatusCode)
	}
	var user DiscordUser
	if err = json.NewDecoder(res.Body).Decode(&user); err != nil {
		return DiscordUser{}, err
	}
	if user.ID == "" || user.Username == "" {
		return DiscordUser{}, ErrCredentials
	}
	return user, nil
}
