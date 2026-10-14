package main

import (
	"errors"
	"path/filepath"
)

var errBotGamesConfig = errors.New("bot games configuration invalid")

type botGamesConfig struct {
	Socket, TokenFile, QuoteKeyFile string
	RouletteEnabled                 bool
}

func loadBotGamesConfig(cfg *config, lookup func(string) (string, bool)) error {
	var b botGamesConfig
	if value, ok := lookup("MOMIAO_BOT_ROULETTE_ENABLED"); ok {
		switch value {
		case "0":
		case "1":
			b.RouletteEnabled = true
		default:
			return errBotGamesConfig
		}
	}
	fields := []struct {
		name   string
		target *string
	}{
		{"MOMIAO_BOT_GAMES_SOCKET", &b.Socket},
		{"MOMIAO_BOT_GAMES_TOKEN_FILE", &b.TokenFile},
		{"MOMIAO_BOT_GAMES_QUOTE_KEY_FILE", &b.QuoteKeyFile},
	}
	count := 0
	for _, f := range fields {
		if v, ok := lookup(f.name); ok {
			if v == "" || !filepath.IsAbs(v) {
				return errBotGamesConfig
			}
			*f.target = filepath.Clean(v)
			count++
		}
	}
	if count == 0 {
		if b.RouletteEnabled {
			return errBotGamesConfig
		}
		return nil
	}
	if count != len(fields) || cfg.ProcessRole != "platform" || cfg.WalletDSNFile == "" || cfg.GameFairnessKeyringFile == "" || cfg.accessDeclaration == nil {
		return errBotGamesConfig
	}
	seen := map[string]bool{}
	for _, p := range []string{cfg.ListenSocket, cfg.PokerRemoteSocket, cfg.WalletDSNFile, cfg.GameFairnessKeyringFile, cfg.CatalogAssets.CredentialsFile, cfg.PokerServiceKeyringFile, cfg.PokerPeerKeyringFile, cfg.Poker.DSNFile, cfg.Poker.StateKeyringFile, cfg.Poker.TicketKeyringFile, cfg.Poker.RedisConfigFile, cfg.Poker.SessionRedisConfigFile, cfg.Session.RedisConfigFile, cfg.History.ReaderDSNFile, cfg.History.WorkerDSNFile, cfg.History.PokerDSNFile, cfg.History.PokerKeyringFile} {
		if p != "" {
			seen[pokerPathKey(p)] = true
		}
	}
	for _, p := range []string{b.Socket, b.TokenFile, b.QuoteKeyFile} {
		k := pokerPathKey(p)
		if seen[k] {
			return errBotGamesConfig
		}
		seen[k] = true
	}
	cfg.BotGames = b
	return nil
}
