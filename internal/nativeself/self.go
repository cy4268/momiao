package nativeself

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"
)

var ErrTarget = errors.New("BOOTSTRAP_TARGET_VERIFY_FAILED")

// Verify reads the live self row through the same native Unix boundary as the
// portal. The credential-kind gate is not authentication: only the native
// signature/session checks and returned live row establish the exact identity.
// Native role is intentionally not decoded, checked, or mapped to platform RBAC.
func Verify(ctx context.Context, transport http.RoundTripper, target int64, username, token, session string) error {
	if target <= 0 || username == "" || transport == nil {
		return ErrTarget
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/api/user/self", nil)
	req.Host = "localhost"
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("New-Api-User", strconv.FormatInt(target, 10))
	req.Header.Set("X-Auth-Session", session)
	if !SessionCredential(req) {
		return ErrTarget
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		return ErrTarget
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrTarget
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(body) > 65536 || !utf8.Valid(body) {
		return ErrTarget
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if !UniqueJSON(decoder, 0) {
		return ErrTarget
	}
	if _, err = decoder.Token(); err != io.EOF {
		return ErrTarget
	}
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
			Status   *int   `json:"status"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &result) != nil || !result.Success || result.Data.ID != target || result.Data.Username != username || result.Data.Status == nil || *result.Data.Status != 1 {
		return ErrTarget
	}
	return nil
}
