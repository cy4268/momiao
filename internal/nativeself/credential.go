// Package nativeself holds the existing native dashboard credential-kind gate
// and fixed Unix transport. It does not verify signatures or grant identity.
package nativeself

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func decimalInt(v string) (int64, error) {
	if v == "" || strings.IndexFunc(v, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseInt(v, 10, 64)
}
func authHeader(r *http.Request, key string, max int, required bool) (string, bool) {
	v := r.Header.Values(key)
	if len(v) == 0 {
		return "", !required
	}
	if len(v) != 1 || len(v[0]) > max || v[0] == "" {
		return "", false
	}
	for _, c := range v[0] {
		if c < 33 || c > 126 {
			if key == "Authorization" && c == ' ' {
				continue
			}
			return "", false
		}
	}
	return v[0], true
}

// Credential-kind gate only; these unverified claims NEVER grant identity.
// Fixed native service/auth_token.go ParseDashboardAccessToken recognizes this
// issuer/audience/use and never falls back to PAT. The unchanged full token then
// goes to the fixed Unix /api/user/self for signature, expiry and live-session checks.
func SessionCredential(r *http.Request) bool {
	auth, ok := authHeader(r, "Authorization", 8192, true)
	if !ok {
		return false
	}
	bearer := strings.Split(auth, " ")
	if len(bearer) != 2 || !strings.EqualFold(bearer[0], "Bearer") {
		return false
	}
	parts := strings.Split(bearer[1], ".")
	if len(parts) != 3 {
		return false
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) == 0 {
		return false
	}
	for _, raw := range [][]byte{header, payload} {
		if !utf8.Valid(raw) {
			return false
		}
		d := json.NewDecoder(bytes.NewReader(raw))
		if !UniqueJSON(d, 0) {
			return false
		}
		if _, err = d.Token(); err != io.EOF {
			return false
		}
	}
	var h map[string]json.RawMessage
	if json.Unmarshal(header, &h) != nil {
		return false
	}
	var algorithm string
	// Native JWT parsing indexes the exact lowercase map key, unlike struct decoding.
	if json.Unmarshal(h["alg"], &algorithm) != nil || algorithm != "HS256" {
		return false
	}
	// Match the native typed claim grammar so malformed registered/custom fields
	// cannot evade its internal-credential classifier and reach opaque PAT lookup.
	var c struct {
		Issuer    string          `json:"iss"`
		Audience  json.RawMessage `json:"aud"`
		Use       string          `json:"token_use"`
		Subject   string          `json:"sub"`
		Session   string          `json:"sid"`
		UV        int64           `json:"uv"`
		SV        int64           `json:"sv"`
		Expires   int64           `json:"exp"`
		Issued    int64           `json:"iat"`
		NotBefore int64           `json:"nbf"`
		JTI       string          `json:"jti"`
		Method    string          `json:"method"`
		Scopes    []string        `json:"scopes"`
	}
	if json.Unmarshal(payload, &c) != nil || c.Issuer != "new-api" || c.Use != "access" || c.Expires <= 0 || c.Issued <= 0 || c.NotBefore <= 0 || c.JTI == "" || c.UV <= 0 || c.SV <= 0 {
		return false
	}
	var audiences []string
	if json.Unmarshal(c.Audience, &audiences) != nil {
		var audience string
		if json.Unmarshal(c.Audience, &audience) != nil {
			return false
		}
		audiences = []string{audience}
	}
	audienceOK := false
	for _, audience := range audiences {
		if audience == "new-api-dashboard" {
			audienceOK = true
		}
	}
	user, ok := authHeader(r, "New-Api-User", 19, true)
	if !ok || c.Subject != user {
		return false
	}
	id, err := decimalInt(user)
	if err != nil || id <= 0 {
		return false
	}
	session, ok := authHeader(r, "X-Auth-Session", 512, true)
	return audienceOK && ok && c.Session != "" && c.Session == session
}
func UniqueJSON(d *json.Decoder, depth int) bool {
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
			if !UniqueJSON(d, depth+1) {
				return false
			}
		}
		token, err = d.Token()
		return err == nil && token == json.Delim('}')
	}
	if delimiter == '[' {
		for d.More() {
			if !UniqueJSON(d, depth+1) {
				return false
			}
		}
		token, err = d.Token()
		return err == nil && token == json.Delim(']')
	}
	return false
}
func NewTransport(socket string) *http.Transport {
	dialer := &net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socket)
		},
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       60 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}
