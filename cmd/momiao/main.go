package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
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
	listener, err := openListener(cfg)
	if err != nil {
		logger.Printf("listen failed: %v", err)
		os.Exit(1)
	}
	logger.Printf("listening on %s (process liveness only)", listener.Addr())
	if err := serve(ctx, newServer(cfg), listener, cfg.ShutdownTimeout); err != nil {
		logger.Print(err)
		os.Exit(1)
	}
	logger.Print("stopped")
}
