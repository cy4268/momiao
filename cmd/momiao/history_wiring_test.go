package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestHistoryWiringBoundary(t *testing.T) {
	env := map[string]string{"MOMIAO_HISTORY_ENABLED": "true"}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	if _, err := loadConfig(lookup); err == nil {
		t.Error("partial History configuration ignored")
	}
	for _, dir := range []string{"", t.TempDir()} {
		response := httptest.NewRecorder()
		newPortalHandler(config{WebDir: dir}, nil).ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/history", nil))
		var body struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(response.Body.Bytes(), &body) != nil || response.Code != 503 || body.Code != "HISTORY_UNAVAILABLE" || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("History prefix not owned: status=%d body=%s", response.Code, response.Body.String())
		}
	}
}
