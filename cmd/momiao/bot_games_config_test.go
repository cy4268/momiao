package main

import (
	"os"
	"path/filepath"
	"testing"
)

func botGamesTestConfig(t *testing.T) (config, map[string]string) {
	t.Helper()
	root := t.TempDir()
	cfg := config{ProcessRole: "platform", WalletDSNFile: filepath.Join(root, "wallet"), GameFairnessKeyringFile: filepath.Join(root, "fairness"), accessDeclaration: &accessDeclaration{MigrationApplicability: "NO_MIGRATION_APPLICABLE", Resources: map[string]string{"EXPERIENCE": "AVAILABLE"}}}
	values := map[string]string{"MOMIAO_BOT_GAMES_SOCKET": filepath.Join(root, "bot.sock"), "MOMIAO_BOT_GAMES_TOKEN_FILE": filepath.Join(root, "token"), "MOMIAO_BOT_GAMES_QUOTE_KEY_FILE": filepath.Join(root, "quote"), "MOMIAO_BOT_GAMES_NATIVE_DSN_FILE": filepath.Join(root, "native")}
	return cfg, values
}
func TestBotGamesConfig(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		c, e := loadConfig(func(string) (string, bool) { return "", false })
		if e != nil || c.BotGames != (botGamesConfig{}) {
			t.Fatalf("default must remain disabled: %v", e)
		}
	})
	t.Run("complete", func(t *testing.T) {
		c, m := botGamesTestConfig(t)
		if e := loadBotGamesConfig(&c, func(k string) (string, bool) { v, ok := m[k]; return v, ok }); e != nil || c.BotGames.Socket != m["MOMIAO_BOT_GAMES_SOCKET"] {
			t.Fatalf("complete config rejected: %v", e)
		}
	})
	t.Run("process_config_wiring", func(t *testing.T) {
		c, m := botGamesTestConfig(t)
		web := t.TempDir()
		if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("portal"), 0600); err != nil {
			t.Fatal(err)
		}
		declaration := filepath.Join(t.TempDir(), "access.json")
		if err := os.WriteFile(declaration, []byte(`{"version":1,"environment":"DEVELOPMENT","origin":"https://example.test","evidence_ref":"synthetic-bot-fixture","migration_applicability":"NO_MIGRATION_APPLICABLE","resources":{"EXPERIENCE":"AVAILABLE"}}`), 0600); err != nil {
			t.Fatal(err)
		}
		m["MOMIAO_WEB_DIR"] = web
		m["MOMIAO_NEWAPI_SOCKET"] = filepath.Join(web, "newapi.sock")
		m["MOMIAO_WALLET_DSN_FILE"] = c.WalletDSNFile
		m["MOMIAO_GAME_FAIRNESS_KEYRING_FILE"] = c.GameFairnessKeyringFile
		m["MOMIAO_ACCESS_GATE_DECLARATION_FILE"] = declaration
		m["MOMIAO_PUBLIC_ORIGIN"] = "https://example.test"
		loaded, err := loadConfig(func(k string) (string, bool) { v, ok := m[k]; return v, ok })
		if err != nil || loaded.BotGames.Socket != m["MOMIAO_BOT_GAMES_SOCKET"] {
			t.Fatalf("process did not load bot configuration: %v", err)
		}
	})
	for _, field := range []string{"SOCKET", "TOKEN_FILE", "QUOTE_KEY_FILE", "NATIVE_DSN_FILE"} {
		for _, bad := range []string{"missing", "empty", "relative"} {
			t.Run(field+"/"+bad, func(t *testing.T) {
				c, m := botGamesTestConfig(t)
				k := "MOMIAO_BOT_GAMES_" + field
				switch bad {
				case "missing":
					delete(m, k)
				case "empty":
					m[k] = ""
				case "relative":
					m[k] = "relative"
				}
				if loadBotGamesConfig(&c, func(k string) (string, bool) { v, ok := m[k]; return v, ok }) == nil {
					t.Fatal("incomplete/path configuration accepted")
				}
			})
		}
	}
	for _, bad := range []string{"role", "wallet", "fairness", "declaration", "self_collision", "listen", "newapi", "refill", "economy", "session", "poker", "fairness_collision", "wallet_collision"} {
		t.Run(bad, func(t *testing.T) {
			c, m := botGamesTestConfig(t)
			p := m["MOMIAO_BOT_GAMES_SOCKET"]
			switch bad {
			case "role":
				c.ProcessRole = "poker"
			case "wallet":
				c.WalletDSNFile = ""
			case "fairness":
				c.GameFairnessKeyringFile = ""
			case "declaration":
				c.accessDeclaration = nil
			case "self_collision":
				m["MOMIAO_BOT_GAMES_TOKEN_FILE"] = p
			case "listen":
				c.ListenSocket = p
			case "newapi":
				c.NewAPISocket = p
			case "refill":
				c.RefillSocket = p
			case "economy":
				c.EconomyReadSocket = p
			case "session":
				c.Session.OpsSocket = p
			case "poker":
				c.PokerRemoteSocket = p
			case "fairness_collision":
				m["MOMIAO_BOT_GAMES_QUOTE_KEY_FILE"] = c.GameFairnessKeyringFile
			case "wallet_collision":
				m["MOMIAO_BOT_GAMES_NATIVE_DSN_FILE"] = c.WalletDSNFile
			}
			if loadBotGamesConfig(&c, func(k string) (string, bool) { v, ok := m[k]; return v, ok }) == nil {
				t.Fatal("invalid authority/collision accepted")
			}
		})
	}
}
