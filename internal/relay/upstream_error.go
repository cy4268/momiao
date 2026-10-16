package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

// Only error metadata is returned; never relay a raw HTML page or response body.
type upstreamError struct {
	Status  int    `json:"-"`
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code"`
}

var credentialParameter = regexp.MustCompile(`(?i)([?&](?:key|api_key|token|access_token)=)[^&\s'"<>]+`)

func redactUpstream(value, secret string) string {
	if secret != "" {
		value = strings.ReplaceAll(value, secret, "[credential]")
	}
	value = credentialParameter.ReplaceAllString(value, "${1}[credential]")
	runes := []rune(value)
	return string(runes[:min(len(runes), 2048)])
}

func parseUpstreamError(raw []byte, status int, secret string) *upstreamError {
	value := gjson.GetBytes(raw, "error")
	if !value.Exists() || value.Type == gjson.Null {
		value = gjson.GetBytes(raw, "response.error")
	}
	kind := gjson.GetBytes(raw, "type").String()
	failed := kind == "error" || kind == "response.failed" || gjson.GetBytes(raw, "status").String() == "failed"
	if (!value.Exists() || value.Type == gjson.Null) && !failed && status < 300 {
		return nil
	}
	if !value.IsObject() && failed {
		value = gjson.ParseBytes(raw)
	}
	code := value.Get("code")
	if status < 300 {
		status = 502
		for _, field := range []gjson.Result{value.Get("status_code"), code} {
			if n, err := strconv.Atoi(field.String()); err == nil && n >= 400 && n <= 599 {
				status = n
				break
			}
		}
	}
	message := value.Get("message").String()
	if message == "" && value.Type == gjson.String {
		message = value.String()
	}
	if message == "" {
		message = fmt.Sprintf("upstream HTTP %d", status)
	}
	result := &upstreamError{Status: status, Message: redactUpstream(message, secret), Type: "upstream_error", Code: status}
	if v := value.Get("type").String(); v != "" {
		result.Type = redactUpstream(v, secret)
	}
	if code.Type == gjson.String {
		result.Code = redactUpstream(code.String(), secret)
	}
	if code.Type == gjson.Number {
		result.Code = code.Int()
	}
	return result
}

func readUpstreamError(response *http.Response, secret string) *upstreamError {
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	return parseUpstreamError(raw, response.StatusCode, secret)
}

func transportFailure(err error, fallback string) *upstreamError {
	status, kind := 502, "upstream_connection_error"
	if err == nil {
		kind = "relay_error"
	}
	var network net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &network) && network.Timeout() {
		status, kind, fallback = 504, "upstream_timeout", "upstream request timed out"
	} else if errors.Is(err, context.Canceled) {
		status, kind, fallback = 499, "request_cancelled", "request cancelled"
	}
	return &upstreamError{Status: status, Message: fallback, Type: kind, Code: status}
}

func writeUpstreamError(w http.ResponseWriter, detail *upstreamError) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(detail.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": detail})
}
