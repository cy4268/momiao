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
	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errBotGamesStartup = errors.New("bot games startup failed")
var errBotGamesListener = errors.New("bot games listener failed")

type botGamesApplication struct {
	listener net.Listener
	server   *http.Server
	native   *pgxpool.Pool
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
	raw, e := readPokerPrivateFile(cfg.BotGames.NativeDSNFile, 8192)
	if e != nil || strings.TrimSpace(string(raw)) == "" {
		return nil, errBotGamesStartup
	}
	pc, e := pgxpool.ParseConfig(strings.TrimSpace(string(raw)))
	if e != nil {
		return nil, errBotGamesStartup
	}
	pc.MaxConns = 4
	pc.MinConns = 0
	pc.ConnConfig.ConnectTimeout = 2 * time.Second
	pc.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	openCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pool, e := pgxpool.NewWithConfig(openCtx, pc)
	if e != nil {
		return nil, errBotGamesStartup
	}
	ok := false
	defer func() {
		if !ok {
			pool.Close()
		}
	}()
	if pool.Ping(openCtx) != nil {
		return nil, errBotGamesStartup
	}
	service, e := botgames.NewService(engine, botGamesResolver{native: pool, platform: store, declaration: cfg.accessDeclaration}, key, time.Now)
	if e != nil {
		return nil, errBotGamesStartup
	}
	handler, e := newBotGamesHandler(service, hex.EncodeToString(token[:]))
	if e != nil {
		return nil, errBotGamesStartup
	}
	listener, e := openBotGamesListener(cfg.BotGames.Socket)
	if e != nil {
		return nil, errBotGamesStartup
	}
	app := &botGamesApplication{listener: listener, native: pool, server: &http.Server{Handler: handler, ReadHeaderTimeout: time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 4096, ErrorLog: log.New(io.Discard, "", 0)}}
	ok = true
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
		if a.native != nil {
			a.native.Close()
		}
	})
	return nil
}
