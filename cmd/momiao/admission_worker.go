package main

import (
	"context"
	"github.com/cy4268/momiao/internal/platform"
	"net/http"
	"time"
)

type registrationWorkerStore interface {
	RegistrationCursor(context.Context) (int64, error)
	IngestRegistrationPage(context.Context, int64, platform.RegistrationPage) error
	MarkRegistrationSourceUnavailable(context.Context) error
	RecoverRegistrationGrant(context.Context) (bool, error)
}

func registrationWorkerCycle(ctx context.Context, store registrationWorkerStore, transport http.RoundTripper, key string) {
	sourceCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	after, err := store.RegistrationCursor(sourceCtx)
	if err == nil {
		var page platform.RegistrationPage
		page, err = readRegistrationPage(sourceCtx, transport, key, after, 100)
		if err == nil {
			err = store.IngestRegistrationPage(sourceCtx, after, page)
		}
	}
	cancel()
	if err != nil {
		statusCtx, stop := context.WithTimeout(ctx, 3*time.Second)
		_ = store.MarkRegistrationSourceUnavailable(statusCtx)
		stop()
	}
	// Durable grants remain recoverable while the native source is unavailable.
	for i := 0; i < 20 && ctx.Err() == nil; i++ {
		jobCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		found, e := store.RecoverRegistrationGrant(jobCtx)
		stop()
		if e != nil || !found {
			break
		}
	}
}
func runAdmissionWorker(ctx context.Context, store registrationWorkerStore, transport http.RoundTripper, key string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		registrationWorkerCycle(ctx, store, transport, key)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
