package main

import (
	"errors"
	"path/filepath"
)

var errBotGamesConfig = errors.New("bot games configuration invalid")

type botGamesConfig struct{ Socket, TokenFile, QuoteKeyFile, NativeDSNFile string }

func loadBotGamesConfig(cfg *config, lookup func(string) (string, bool)) error {
	var b botGamesConfig
	fields := []struct {
		name   string
		target *string
	}{
		{"MOMIAO_BOT_GAMES_SOCKET", &b.Socket},
		{"MOMIAO_BOT_GAMES_TOKEN_FILE", &b.TokenFile},
		{"MOMIAO_BOT_GAMES_QUOTE_KEY_FILE", &b.QuoteKeyFile},
		{"MOMIAO_BOT_GAMES_NATIVE_DSN_FILE", &b.NativeDSNFile},
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
		return nil
	}
	if count != len(fields) || cfg.ProcessRole != "platform" || cfg.WalletDSNFile == "" || cfg.GameFairnessKeyringFile == "" || cfg.accessDeclaration == nil {
		return errBotGamesConfig
	}
	seen := map[string]bool{}
	for _, p := range []string{cfg.ListenSocket, cfg.NewAPISocket, cfg.RefillSocket, cfg.EconomyReadSocket, cfg.PokerRemoteSocket, cfg.Session.OpsSocket, cfg.WalletDSNFile, cfg.GameFairnessKeyringFile, cfg.NativeQuotaDSNFile, cfg.NativeQuotaKeyFile, cfg.RefillKeyFile, cfg.RegistrationReaderKeyFile, cfg.CatalogReaderKeyFile, cfg.CatalogAssets.CredentialsFile, cfg.PokerServiceKeyringFile, cfg.PokerPeerKeyringFile, cfg.Poker.DSNFile, cfg.Poker.ReaderKeyFile, cfg.Poker.StateKeyringFile, cfg.Poker.TicketKeyringFile, cfg.Poker.RedisConfigFile, cfg.Session.DSNFile, cfg.Session.RedisConfigFile, cfg.Session.ReaderKeyFile, cfg.Session.OpsKeyFile, cfg.Session.SessionSealKeyFile, cfg.Session.CredentialSealKeyFile, cfg.History.ReaderDSNFile, cfg.History.WorkerDSNFile, cfg.History.PokerDSNFile, cfg.History.PokerKeyringFile} {
		if p != "" {
			seen[pokerPathKey(p)] = true
		}
	}
	for _, p := range []string{b.Socket, b.TokenFile, b.QuoteKeyFile, b.NativeDSNFile} {
		k := pokerPathKey(p)
		if seen[k] {
			return errBotGamesConfig
		}
		seen[k] = true
	}
	cfg.BotGames = b
	return nil
}
