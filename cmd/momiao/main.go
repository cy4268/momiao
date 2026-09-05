package main

import (
	"context"
	"errors"
	"github.com/cy4268/momiao/internal/platform"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger := log.New(os.Stderr, "momiao: ", log.LstdFlags|log.LUTC)
	cfg, err := loadConfig(os.LookupEnv)
	if err != nil {
		logger.Print(err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, logger); err != nil {
		logger.Print(err)
		os.Exit(1)
	}
}
func run(ctx context.Context, cfg config, logger *log.Logger) error {
	if cfg.WalletDSNFile != "" {
		dsn, err := readWalletDSN(cfg.WalletDSNFile)
		if err != nil {
			return errors.New("wallet startup failed")
		}
		openCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		store, err := platform.OpenLazy(openCtx, dsn)
		cancel()
		if err != nil {
			return errors.New("wallet startup failed")
		}
		defer store.Close()
		cfg.wallet = store
		cfg.profile = store
	}
	listener, err := openListener(cfg)
	if err != nil {
		return err
	}
	defer listener.Close()
	logger.Printf("listening on %s (process liveness only)", listener.Addr())
	if err := serve(ctx, newServer(cfg), listener, cfg.ShutdownTimeout); err != nil {
		return err
	}
	logger.Print("stopped")
	return nil
}
