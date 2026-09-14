package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestDisabledSessionEnvelope(t *testing.T) {
	for _, dir := range []string{"", t.TempDir()} {
		for _, path := range []string{"/api/v1/session/bootstrap", "/api/v1/auth/password/login"} {
			t.Run(dir+path, func(t *testing.T) {
				response := httptest.NewRecorder()
				newPortalHandler(config{WebDir: dir}, nil).ServeHTTP(response, httptest.NewRequest("GET", path, nil))
				var body struct {
					Success bool `json:"success"`
					Error   struct {
						Code string `json:"code"`
					} `json:"error"`
				}
				if json.Unmarshal(response.Body.Bytes(), &body) != nil || response.Code != 503 || body.Success || body.Error.Code != "SESSION_UNAVAILABLE" || response.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("disabled route violated session envelope: status=%d body=%s", response.Code, response.Body.String())
				}
			})
		}
	}
}
