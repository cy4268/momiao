package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cy4268/momiao/internal/platform"
)

// Model discovery must use local key ownership, never browser identity or native.
func TestLocalRelayModels(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires the existing dev database")
	}
	dsn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read dev DSN")
	}
	store, err := platform.Open(t.Context(), strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("open dev database")
	}
	defer store.Close()
	var key, group string
	err = store.Pool().QueryRow(t.Context(), `SELECT k.key,u.group_name FROM relay.api_keys k JOIN identity.users u ON u.id=k.user_id
 WHERE k.status=1 AND u.status=1 AND u.deleted_at IS NULL AND k.models IS NULL AND u.group_name='default'
 AND (k.expires_at IS NULL OR k.expires_at>now()) AND (k.quota_limit IS NULL OR k.used_quota<k.quota_limit) ORDER BY k.id LIMIT 1`).Scan(&key, &group)
	if err != nil {
		t.Fatal("read existing dev key")
	}
	var wanted []string
	err = store.Pool().QueryRow(t.Context(), `SELECT array_agg(DISTINCT model ORDER BY model) FROM relay.channels,unnest(models) model WHERE enabled AND $1=ANY(groups)`, group).Scan(&wanted)
	if err != nil || len(wanted) == 0 {
		t.Fatal("read existing channel models", err)
	}
	identity, err := store.AuthenticateRelay(t.Context(), key)
	if err != nil {
		t.Fatal("authenticate existing key", err)
	}
	identity.Models = []string{"gpt-5.6-sol", "gproxy/claude-opus-4-6"}
	limited, err := store.RelayModelsForKey(t.Context(), identity)
	if err != nil || !slices.Equal(limited, []string{"gpt-5.6-sol"}) {
		t.Fatal("key model discovery ignored account group or restriction", err)
	}
	identity.Models = []string{}
	limited, err = store.RelayModelsForKey(t.Context(), identity)
	if err != nil || len(limited) != 0 {
		t.Fatal("empty model restriction became unrestricted", err)
	}
	for _, webDir := range []string{"", "/unused-local-relay-web"} {
		handler := newPortalHandler(config{WebDir: webDir, relay: store})
		call := func(header, value string) *httptest.ResponseRecorder {
			r := httptest.NewRequest("GET", "https://dev.momiao.win/v1/models?group=工作", nil)
			r.Header.Set(header, value)
			r.Header.Set("New-Api-User", "1")
			r.AddCookie(&http.Cookie{Name: "session", Value: "not-an-api-key"})
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			return w
		}
		unauthorized := call("Authorization", "Bearer sk-invalid")
		if unauthorized.Code != 401 || !strings.Contains(unauthorized.Body.String(), `"error"`) {
			t.Fatal("local relay authentication", unauthorized.Code)
		}
		for _, header := range []string{"Authorization", "x-api-key"} {
			value := "sk-" + key
			if header == "Authorization" {
				value = "Bearer " + value
			}
			response := call(header, value)
			var body struct {
				Object string `json:"object"`
				Data   []struct {
					ID     string `json:"id"`
					Object string `json:"object"`
				} `json:"data"`
			}
			if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.Object != "list" {
				t.Fatal("local relay models", response.Code)
			}
			var got []string
			for _, model := range body.Data {
				if model.Object != "model" {
					t.Fatal("OpenAI model shape")
				}
				got = append(got, model.ID)
			}
			if !slices.Equal(got, wanted) {
				t.Fatal("models escaped account group or lost enabled models")
			}
			if strings.Contains(response.Body.String(), key) {
				t.Fatal("model response exposed credential")
			}
		}
	}
}
