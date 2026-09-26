package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPokerApplicationConfigRequiresCompleteExplicitAuthority(t *testing.T) {
	dir := t.TempDir()
	base := config{WebDir: dir, NewAPISocket: filepath.Join(dir, "native.sock"), PublicOrigin: "https://poker.example", WalletDSNFile: filepath.Join(dir, "platform.dsn"), GameFairnessKeyringFile: filepath.Join(dir, "games.json")}
	values := map[string]string{
		"MOMIAO_POKER_ENABLED":                 "true",
		"MOMIAO_POKER_DSN_FILE":                filepath.Join(dir, "poker.dsn"),
		"MOMIAO_POKER_SESSION_READER_KEY_FILE": filepath.Join(dir, "reader.key"),
		"MOMIAO_POKER_STATE_KEYRING_FILE":      filepath.Join(dir, "state.json"),
		"MOMIAO_POKER_TICKET_KEYRING_FILE":     filepath.Join(dir, "tickets.json"),
		"MOMIAO_POKER_REDIS_CONFIG_FILE":       filepath.Join(dir, "redis.json"),
	}
	lookup := func(m map[string]string) func(string) (string, bool) {
		return func(key string) (string, bool) { v, ok := m[key]; return v, ok }
	}
	cfg := base
	if err := loadPokerConfig(&cfg, lookup(nil)); err != nil || cfg.Poker.Enabled {
		t.Fatal("Poker must default off", err)
	}
	for key := range values {
		copy := make(map[string]string, len(values))
		for k, v := range values {
			copy[k] = v
		}
		delete(copy, key)
		cfg = base
		if err := loadPokerConfig(&cfg, lookup(copy)); err == nil {
			t.Fatalf("missing %s accepted", key)
		}
	}
	cfg = base
	if err := loadPokerConfig(&cfg, lookup(values)); err != nil || !cfg.Poker.Enabled {
		t.Fatal("complete explicit configuration", err)
	}
	if cfg.GameFairnessKeyringFile != base.GameFairnessKeyringFile {
		t.Fatal("Direct Play fairness configuration changed")
	}
	for _, change := range []func(map[string]string){
		func(v map[string]string) { v["MOMIAO_POKER_ENABLED"] = "TRUE" },
		func(v map[string]string) { v["MOMIAO_POKER_ENABLED"] = "false" },
		func(v map[string]string) { v["MOMIAO_POKER_DSN_FILE"] = "relative.dsn" },
		func(v map[string]string) { v["MOMIAO_POKER_DSN_FILE"] = base.WalletDSNFile },
		func(v map[string]string) { v["MOMIAO_POKER_STATE_KEYRING_FILE"] = base.GameFairnessKeyringFile },
		func(v map[string]string) {
			v["MOMIAO_POKER_SESSION_READER_KEY_FILE"] = v["MOMIAO_POKER_TICKET_KEYRING_FILE"]
		},
	} {
		copy := make(map[string]string, len(values))
		for k, v := range values {
			copy[k] = v
		}
		change(copy)
		cfg = base
		if err := loadPokerConfig(&cfg, lookup(copy)); err == nil {
			t.Fatal("partial/reused authority accepted")
		}
	}
	process := config{ProcessRole: "poker", ListenSocket: filepath.Join(dir, "poker.sock"), NewAPISocket: base.NewAPISocket}
	processValues := map[string]string{"MOMIAO_POKER_SERVICE_KEYRING_FILE": filepath.Join(dir, "service.json"), "MOMIAO_POKER_PEER_KEYRING_FILE": filepath.Join(dir, "peer.json")}
	if loadPokerProcessConfig(&process, lookup(processValues)) == nil {
		t.Fatal("missing reverse read authority accepted")
	}
	processValues["MOMIAO_ECONOMY_READ_SOCKET"] = filepath.Join(dir, "economy-read.sock")
	if err := loadPokerProcessConfig(&process, lookup(processValues)); err != nil || process.EconomyReadSocket == "" {
		t.Fatal("explicit readonly authority", err)
	}
	processValues["MOMIAO_ECONOMY_READ_SOCKET"] = process.ListenSocket
	if loadPokerProcessConfig(&process, lookup(processValues)) == nil {
		t.Fatal("self-loop socket accepted")
	}

}

func TestPokerApplicationPrivatePersistentKeyrings(t *testing.T) {
	dir := resolvedAppPrivateDir(t)
	write := func(name, body string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if e := os.WriteFile(path, []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
		return path
	}
	state := write("state.json", `{"active":"state-v1","keys":{"state-v1":"`+strings.Repeat("a", 64)+`","state-old":"`+strings.Repeat("b", 64)+`"}}`)
	keys, e := readPokerStateKeys(state)
	if e != nil || keys.Current != "state-v1" || len(keys.Keys) != 2 || len(keys.Keys["state-v1"]) != 32 {
		t.Fatal("persistent state keyring", e)
	}
	pub, priv, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(map[string]any{"active": "ticket-v1", "private_key": hex.EncodeToString(priv), "public_keys": map[string]string{"ticket-v1": hex.EncodeToString(pub)}})
	tickets := write("tickets.json", string(raw))
	signing, e := readPokerTicketKeys(tickets)
	if e != nil || signing.active != "ticket-v1" || len(signing.public) != 1 {
		t.Fatal("persistent signing keyring", e)
	}
	for _, body := range []string{
		`{"active":"state-v1","active":"state-v2","keys":{}}`,
		`{"Active":"state-v1","keys":{"state-v1":"` + strings.Repeat("a", 64) + `"}}`,
		`{"active":"state-v1","keys":{"state-v1":"short"}}`,
		`{"active":"state-v1","keys":{"state-v1":"` + strings.Repeat("A", 64) + `"}}`,
		`{"active":"state-v1","keys":{}}`,
		`{"active":"state-v1","keys":{"state-v1":"` + strings.Repeat("a", 64) + `"},"extra":true}`,
	} {
		if _, e = readPokerStateKeys(write("bad.json", body)); e == nil {
			t.Fatal("invalid key material accepted")
		}
	}
	raw, _ = json.Marshal(map[string]any{"active": "ticket-v1", "private_key": hex.EncodeToString(priv), "public_keys": map[string]string{"ticket-v1": strings.Repeat("a", 64)}})
	if _, e = readPokerTicketKeys(write("wrong-public.json", string(raw))); e == nil {
		t.Fatal("signer/verifier mismatch accepted")
	}
	if _, e = readPokerStateKeys(dir); e == nil {
		t.Fatal("directory accepted as secret")
	}
}

func resolvedAppPrivateDir(t *testing.T) string {
	t.Helper()
	dir := appPrivateDir(t)
	// Windows EvalSymlinks can fail below a junction, so peel its reparse ancestors first.
	for range 32 {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			return resolved
		}
		replaced := false
		for candidate := dir; ; candidate = filepath.Dir(candidate) {
			target, linkErr := os.Readlink(candidate)
			if linkErr == nil {
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(candidate), target)
				}
				suffix, relErr := filepath.Rel(candidate, dir)
				if relErr != nil {
					t.Fatal(relErr)
				}
				dir, replaced = filepath.Join(target, suffix), true
				break
			}
			if parent := filepath.Dir(candidate); parent == candidate {
				break
			}
		}
		if !replaced {
			t.Fatal(err)
		}
	}
	t.Fatal("private fixture path has too many links")
	return ""
}

func TestPokerApplicationDisabledRoutesNeverProxy(t *testing.T) {
	dir := t.TempDir()
	if e := os.WriteFile(filepath.Join(dir, "index.html"), []byte("fixture spa"), 0600); e != nil {
		t.Fatal(e)
	}
	for _, web := range []string{"", dir} {
		calls := 0
		h := newPortalHandler(config{WebDir: web}, roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			t.Fatal("disabled Poker reached native proxy")
			return nil, nil
		}))
		for _, path := range []string{"/api/v1/poker", "/api/v1/poker/tables", "/api/v1/poker/connect-tickets", "/ws/poker"} {
			r := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 503 || !strings.Contains(w.Body.String(), "POKER_UNAVAILABLE") || strings.Contains(w.Body.String(), "fixture spa") {
				t.Fatalf("disabled %s: %d %s", path, w.Code, w.Body.String())
			}
		}
		if calls != 0 {
			t.Fatal("proxy invoked")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
		if w.Code != 200 {
			t.Fatal("liveness changed")
		}
	}
}
