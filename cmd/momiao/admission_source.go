package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/platform"
)

var errRegistrationSource = errors.New("registration source unavailable")

func validReaderKey(key string) bool {
	return len(key) >= 32 && len(key) <= 256 && strings.IndexFunc(key, func(r rune) bool { return r < 33 || r > 126 }) < 0
}

func readRegistrationPage(ctx context.Context, transport http.RoundTripper, key string, after int64, limit int) (platform.RegistrationPage, error) {
	var empty platform.RegistrationPage
	if transport == nil || !validReaderKey(key) || after < 0 || limit < 1 || limit > 100 {
		return empty, errRegistrationSource
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://unix/internal/momiao/registrations?after=%d&limit=%d", after, limit), nil)
	req.Host = "localhost"
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	// A fresh fixed-path request carries only the independent service credential.
	// RoundTrip cannot redirect and does not have a cookie jar.
	resp, err := transport.RoundTrip(req)
	if err != nil {
		return empty, errRegistrationSource
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return empty, errRegistrationSource
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024+1))
	if err != nil || len(body) > 128*1024 || !utf8.Valid(body) || uniqueJSON(body) != nil {
		return empty, platform.ErrRegistrationReceipt
	}
	var envelope struct {
		Success bool                      `json:"success"`
		Data    platform.RegistrationPage `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&envelope) != nil || !envelope.Success {
		return empty, platform.ErrRegistrationReceipt
	}
	if err := platform.ValidateRegistrationPage(envelope.Data, after, limit); err != nil {
		return empty, err
	}
	return envelope.Data, nil
}

// Reject duplicate JSON keys before decoding immutable source facts. Depth is
// bounded even though only a shallow native envelope is expected.
func uniqueJSON(body []byte) error {
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 8 {
			return platform.ErrRegistrationReceipt
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return platform.ErrRegistrationReceipt
				}
				seen[key] = true
				if err := value(depth + 1); err != nil {
					return err
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim('}') {
				return platform.ErrRegistrationReceipt
			}
		case '[':
			for d.More() {
				if err := value(depth + 1); err != nil {
					return err
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim(']') {
				return platform.ErrRegistrationReceipt
			}
		default:
			return platform.ErrRegistrationReceipt
		}
		return nil
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return platform.ErrRegistrationReceipt
	}
	return nil
}
