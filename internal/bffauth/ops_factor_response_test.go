package bffauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOpsFactorRateLimitResponseBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, path, media, body   string
		status                    int
		transportError, wantKnown bool
	}{
		{name: "password empty 429", path: "/api/momiao/ops/fresh/password", status: 429, wantKnown: true},
		{name: "two factor empty 429", path: "/api/momiao/ops/fresh/2fa", status: 429, wantKnown: true},
		{name: "Discord login empty 429", path: "/api/momiao/auth/discord/login/start", status: 429, wantKnown: true},
		{name: "Discord registration empty 429", path: "/api/momiao/auth/discord/registration/start", status: 429, wantKnown: true},
		{name: "JSON 429", path: "/api/momiao/ops/fresh/password", media: "application/json", body: `{"success":false}`, status: 429, wantKnown: true},
		{name: "unrelated empty 429", path: "/api/momiao/account", status: 429},
		{name: "empty success remains invalid", path: "/api/momiao/ops/fresh/password", status: 200},
		{name: "empty server failure remains unknown", path: "/api/momiao/ops/fresh/password", status: 503},
		{name: "oversized rate response remains unknown", path: "/api/momiao/ops/fresh/password", body: strings.Repeat("x", 65537), status: 429},
		{name: "transport failure remains unknown", path: "/api/momiao/ops/fresh/password", transportError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			s := &Service{origin: "https://fixture.invalid", native: logoutTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != http.MethodPost || r.URL.Path != tc.path {
					t.Fatal("factor request differs")
				}
				if tc.transportError {
					return nil, errors.New("fixture transport failure")
				}
				header := make(http.Header)
				if tc.media != "" {
					header.Set("Content-Type", tc.media)
				}
				return &http.Response{StatusCode: tc.status, Header: header, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			if strings.HasPrefix(tc.path, "/api/momiao/auth/discord/") {
				purpose := strings.TrimSuffix(strings.TrimPrefix(tc.path, "/api/momiao/auth/discord/"), "/start")
				authorization, state, browser, err := s.nativeDiscordStart(context.Background(), purpose, nil)
				var fault Fault
				if !errors.As(err, &fault) || fault.Status != 429 || fault.Code != "RATE_LIMITED" || fault.ClearCookie || fault.Unknown || authorization != "" || state != "" || browser != "" || calls != 1 {
					t.Fatalf("Discord limiter must preserve preauth and not retry: fault=%+v calls=%d", fault, calls)
				}
				return
			}
			response, raw, err := s.momiaoCall(context.Background(), http.MethodPost, tc.path, nil, map[string]string{}, nil, "")
			if calls != 1 {
				t.Fatal("factor call was retried")
			}
			if tc.wantKnown {
				if err != nil || response == nil || response.StatusCode != http.StatusTooManyRequests {
					t.Fatalf("known rate limit lost: response=%v err=%v", response, err)
				}
			} else if err == nil || raw != nil {
				t.Fatal("uncertain response treated as known")
			}
		})
	}
}
