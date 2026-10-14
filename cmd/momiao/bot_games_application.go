package main

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cy4268/momiao/internal/botgames"
	"github.com/cy4268/momiao/internal/botroulette"
	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
)

var errBotGamesStartup = errors.New("bot games startup failed")
var errBotGamesListener = errors.New("bot games listener failed")

type botGamesApplication struct {
	listener net.Listener
	server   *http.Server
	once     sync.Once
}

func readBotGamesKey(path string) ([32]byte, error) {
	var key [32]byte
	raw, e := readPokerPrivateFile(path, 66)
	if e != nil {
		return key, errBotGamesStartup
	}
	text := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	decoded, e := hex.DecodeString(text)
	if e != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != text {
		return key, errBotGamesStartup
	}
	copy(key[:], decoded)
	if key == [32]byte{} {
		return key, errBotGamesStartup
	}
	return key, nil
}

func openBotGamesApplication(ctx context.Context, cfg config, store *platform.Store, engine *games.Service) (*botGamesApplication, error) {
	if cfg.BotGames == (botGamesConfig{}) {
		return nil, nil
	}
	if cfg.ProcessRole != "platform" || store == nil || engine == nil || cfg.accessDeclaration == nil || cfg.WalletDSNFile == "" || !filepath.IsAbs(cfg.BotGames.Socket) {
		return nil, errBotGamesStartup
	}
	if cfg.BotGames.RouletteEnabled && cfg.roulette == nil {
		return nil, errBotGamesStartup
	}
	token, e := readBotGamesKey(cfg.BotGames.TokenFile)
	if e != nil {
		return nil, errBotGamesStartup
	}
	key, e := readBotGamesKey(cfg.BotGames.QuoteKeyFile)
	if e != nil || key == token {
		return nil, errBotGamesStartup
	}
	fairness, e := games.ReadKeyring(cfg.GameFairnessKeyringFile)
	if e != nil {
		return nil, errBotGamesStartup
	}
	for _, k := range fairness.Keys {
		if k == key || k == token {
			return nil, errBotGamesStartup
		}
	}
	resolver := botGamesResolver{platform: store, declaration: cfg.accessDeclaration}
	service, e := botgames.NewService(engine, resolver, key, time.Now)
	if e != nil {
		return nil, errBotGamesStartup
	}
	handler, e := newBotGamesHandler(service, hex.EncodeToString(token[:]))
	if e != nil {
		return nil, errBotGamesStartup
	}
	if cfg.BotGames.RouletteEnabled {
		rouletteService, err := botroulette.NewService(cfg.roulette, resolver, key, time.Now)
		if err != nil {
			return nil, errBotGamesStartup
		}
		rouletteHandler, err := newBotRouletteHandler(rouletteService, hex.EncodeToString(token[:]))
		if err != nil {
			return nil, errBotGamesStartup
		}
		legacyHandler := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if botRouletteRoute(r.URL.Path) {
				rouletteHandler.ServeHTTP(w, r)
				return
			}
			legacyHandler.ServeHTTP(w, r)
		})
	}
	listener, e := openBotGamesListener(cfg.BotGames.Socket)
	if e != nil {
		return nil, errBotGamesStartup
	}
	app := &botGamesApplication{listener: listener, server: &http.Server{Handler: handler, ReadHeaderTimeout: time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 4096, ErrorLog: log.New(io.Discard, "", 0)}}
	return app, nil
}
func (a *botGamesApplication) Run(ctx context.Context, timeout time.Duration) error {
	if e := serve(ctx, a.server, a.listener, timeout); e != nil {
		return errBotGamesListener
	}
	return nil
}
func (a *botGamesApplication) Close() error {
	if a == nil {
		return nil
	}
	a.once.Do(func() {
		if a.server != nil {
			_ = a.server.Close()
		}
		if a.listener != nil {
			_ = a.listener.Close()
		}
	})
	return nil
}
