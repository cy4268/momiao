package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/platform"
)

// Opt-in dev smoke: existing keys/accounts/channels, tiny real upstream calls.
// Charges and usage history remain in the independent dev database.
func TestLocalRelayHTTP(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires the existing dev database and upstream credentials")
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
	for _, web := range []string{"", "/unused-local-relay-web"} {
		h := newPortalHandler(config{WebDir: web, relay: store}, roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Error("generation reached native transport")
			return nil, errors.New("unexpected native request")
		}))
		for _, endpoint := range []string{"chat/completions", "responses", "messages"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/"+endpoint, strings.NewReader(`{"model":"gpt-5.6-sol"}`)))
			if w.Code != 401 {
				t.Fatal("local generation authentication", w.Code)
			}
		}
	}
	server := httptest.NewServer(newPortalHandler(config{relay: store}, nil))
	defer server.Close()
	client := &http.Client{Timeout: 5 * time.Minute}
	var totalCharged int64
	for _, c := range []struct {
		endpoint, model, input string
		stream                 bool
	}{
		{"chat/completions", "gpt-5.6-sol", `"messages":[{"role":"user","content":"Reply OK."}],"max_completion_tokens":32`, false},
		{"chat/completions", "gpt-5.6-sol", `"messages":[{"role":"user","content":"Reply OK."}],"max_completion_tokens":32`, true},
		{"responses", "gpt-5.6-sol", `"input":"Reply OK.","max_output_tokens":32`, false},
		{"messages", "gproxy/claude-opus-4-6", `"messages":[{"role":"user","content":"Reply OK."}],"max_tokens":8`, true},
		{"chat/completions", "vertex-proxy/gemini-3.7-flash", `"messages":[{"role":"user","content":"Reply OK."}],"max_tokens":16`, true},
	} {
		var key string
		var user, keyID, beforeBalance, beforeUsed int64
		err = store.Pool().QueryRow(t.Context(), `SELECT k.key,u.id,k.id,b.balance_units,k.used_quota FROM relay.api_keys k
 JOIN identity.users u ON u.id=k.user_id JOIN economy.wallet_balances b ON b.newapi_user_id=u.id AND b.asset_type='RESERVE_API_CREDIT'
 WHERE k.status=1 AND u.status=1 AND u.deleted_at IS NULL AND k.models IS NULL AND b.balance_units>0
 AND (k.expires_at IS NULL OR k.expires_at>now()) AND (k.quota_limit IS NULL OR k.used_quota<k.quota_limit)
 AND EXISTS(SELECT 1 FROM relay.channels c WHERE c.enabled AND $1=ANY(c.models) AND u.group_name=ANY(c.groups))
 ORDER BY k.id LIMIT 1`, c.model).Scan(&key, &user, &keyID, &beforeBalance, &beforeUsed)
		if err != nil {
			t.Fatal("read existing dev key for", c.model, err)
		}
		body, _ := json.Marshal(map[string]any{"model": c.model, "stream": c.stream})
		payload := strings.TrimSuffix(string(body), "}") + "," + c.input + "}"
		r, _ := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/v1/"+c.endpoint, strings.NewReader(payload))
		r.Header.Set("Authorization", "Bearer sk-"+key)
		r.Header.Set("Content-Type", "application/json")
		started := time.Now()
		response, err := client.Do(r)
		if err != nil {
			t.Fatal("local HTTP call", c.endpoint, err)
		}
		if response.StatusCode != 200 {
			var problem struct{ Error struct{ Message string } }
			_ = json.NewDecoder(response.Body).Decode(&problem)
			response.Body.Close()
			t.Error("upstream call", c.endpoint, c.model, "HTTP", response.StatusCode, "request", response.Header.Get("X-Request-ID"), problem.Error.Message)
			continue
		}
		var first time.Duration
		events := 0
		if c.stream {
			if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
				t.Fatal("stream content type")
			}
			reader := bufio.NewReader(response.Body)
			for {
				line, readErr := reader.ReadString('\n')
				if strings.HasPrefix(line, "data:") {
					events++
					if first == 0 {
						first = time.Since(started)
					}
				}
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					t.Fatal("read SSE", readErr)
				}
			}
			if events < 2 {
				t.Fatal("missing incremental SSE events")
			}
		} else {
			var data map[string]json.RawMessage
			if err = json.NewDecoder(response.Body).Decode(&data); err != nil || (data["error"] != nil && string(data["error"]) != "null") || data["usage"] == nil {
				t.Fatal("generation response/usage", c.endpoint, err)
			}
			t.Log("reported usage", string(data["usage"]))
		}
		response.Body.Close()
		id := response.Header.Get("X-Request-ID")
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		var cost, debit, afterBalance, afterUsed, logs, tokens int64
		var final string
		for {
			err = store.Pool().QueryRow(ctx, `SELECT l.cost_units,a.charged_raw_quota,b.balance_units,k.used_quota,a.final_status,
 (SELECT count(*) FROM relay.logs WHERE request_id=$1),l.prompt_tokens+l.completion_tokens
 FROM economy.request_attributions a JOIN relay.logs l ON l.id=a.source_event_id
 JOIN economy.wallet_balances b ON b.newapi_user_id=a.newapi_user_id AND b.asset_type='RESERVE_API_CREDIT'
 JOIN relay.api_keys k ON k.id=a.token_id WHERE a.source_instance_id=$2 AND a.logical_request_id=$1 AND a.newapi_user_id=$3 AND k.id=$4`,
				id, platform.LocalRelaySourceID, user, keyID).Scan(&cost, &debit, &afterBalance, &afterUsed, &final, &logs, &tokens)
			if err == nil || ctx.Err() != nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		cancel()
		if err != nil || final != "SUCCESS" || cost < 0 || tokens < 0 || logs < 1 || beforeBalance-afterBalance != debit || afterUsed-beforeUsed != cost {
			t.Fatal("atomic local usage/Reserve settlement", c.endpoint, err, final, cost, debit)
		}
		totalCharged += debit
		t.Logf("%s %s stream=%t events=%d first=%s total=%s cost=%d debit=%d request=%s", c.endpoint, c.model, c.stream, events, first.Round(time.Millisecond), time.Since(started).Round(time.Millisecond), cost, debit, id)
	}
	if totalCharged <= 0 {
		t.Fatal("no reported usage charged Reserve")
	}
}
