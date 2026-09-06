package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type admissionTransport func(*http.Request) (*http.Response, error)

func (f admissionTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const syntheticReceipt = `{"ordinal":1,"operation_id":"42684268-0000-4000-8000-000000000001","native_user_id":12,"discord_subject":"123456789012345678","source":"NEW_DISCORD_REGISTRATION","policy_version":"v1","created_at":"2026-09-06T00:00:00Z"}`

func readerResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}
func TestAdmissionReaderPrivateTransport(t *testing.T) {
	key := strings.Repeat("k", 32)
	transport := admissionTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "http://unix/internal/momiao/registrations?after=0&limit=10" || r.Host != "localhost" || r.Method != "GET" {
			t.Fatal("reader destination is not fixed")
		}
		if r.Header.Get("Authorization") != "Bearer "+key || r.Header.Get("Cookie") != "" || r.Header.Get("New-Api-User") != "" || len(r.Header) != 2 {
			t.Fatal("reader authority changed")
		}
		if _, ok := r.Context().Deadline(); !ok {
			t.Fatal("missing bounded deadline")
		}
		return readerResponse(200, `{"success":true,"data":{"receipts":[`+syntheticReceipt+`],"next_cursor":1}}`), nil
	})
	page, err := readRegistrationPage(context.Background(), transport, key, 0, 10)
	if err != nil || page.NextCursor != 1 || len(page.Receipts) != 1 || page.Receipts[0].NativeUserID != 12 {
		t.Fatalf("valid receipt failed: %v", err)
	}
}
func TestAdmissionReaderRejectsUnsafePages(t *testing.T) {
	valid := `{"success":true,"data":{"receipts":[` + syntheticReceipt + `],"next_cursor":1}}`
	for name, body := range map[string]string{
		"duplicate receipt": strings.Replace(valid, syntheticReceipt, syntheticReceipt+","+syntheticReceipt, 1),
		"cursor ahead":      strings.Replace(valid, `"next_cursor":1`, `"next_cursor":2`, 1),
		"duplicate key":     strings.Replace(valid, `"ordinal":1`, `"ordinal":1,"ordinal":2`, 1),
		"wrong source":      strings.Replace(valid, "NEW_DISCORD_REGISTRATION", "LEGACY", 1),
		"bad id":            strings.Replace(valid, `"native_user_id":12`, `"native_user_id":0`, 1),
		"bad uuid":          strings.Replace(valid, "42684268-0000-4000-8000-000000000001", "x", 1),
		"oversize":          strings.Repeat(" ", 128*1024) + valid,
		"trailing":          valid + valid,
		"unknown field":     strings.Replace(valid, `"next_cursor":1`, `"next_cursor":1,"registered":true`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := readRegistrationPage(context.Background(), admissionTransport(func(*http.Request) (*http.Response, error) { return readerResponse(200, body), nil }), strings.Repeat("k", 32), 0, 10)
			if err == nil {
				t.Fatal("accepted invalid receipt page")
			}
			if strings.Contains(err.Error(), "42684268") {
				t.Fatal("leaked receipt")
			}
		})
	}
}
func TestAdmissionReaderOutageAndCursor(t *testing.T) {
	for _, status := range []int{301, 401, 403, 429, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			_, err := readRegistrationPage(context.Background(), admissionTransport(func(*http.Request) (*http.Response, error) {
				return readerResponse(status, "sensitive provider detail"), nil
			}), strings.Repeat("k", 32), 0, 10)
			if err == nil || strings.Contains(err.Error(), "sensitive") {
				t.Fatal("unsafe error")
			}
		})
	}
	for _, after := range []int64{0, 18} {
		page, err := readRegistrationPage(context.Background(), admissionTransport(func(*http.Request) (*http.Response, error) {
			return readerResponse(200, fmt.Sprintf(`{"success":true,"data":{"receipts":[],"next_cursor":%d}}`, after)), nil
		}), strings.Repeat("k", 32), after, 10)
		if err != nil || page.NextCursor != after {
			t.Fatal("empty page moved cursor")
		}
	}
	called := false
	_, err := readRegistrationPage(context.Background(), admissionTransport(func(*http.Request) (*http.Response, error) { called = true; return nil, fmt.Errorf("private detail") }), "short", 0, 10)
	if err == nil || called {
		t.Fatal("invalid credential made request")
	}
}
