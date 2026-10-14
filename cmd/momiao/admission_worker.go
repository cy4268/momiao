package main

import (
	"context"
	"github.com/cy4268/momiao/internal/platform"
	"time"
)

type registrationWorkerStore interface {
	RegistrationCursor(context.Context) (int64, error)
	ReadRegistrationPage(context.Context, int64, int) (platform.RegistrationPage, error)
	IngestRegistrationPage(context.Context, int64, platform.RegistrationPage) error
	MarkRegistrationSourceUnavailable(context.Context) error
	RecoverRegistrationGrant(context.Context) (bool, error)
}

func registrationWorkerCycle(ctx context.Context, store registrationWorkerStore) {
	sourceCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	after, err := store.RegistrationCursor(sourceCtx)
	if err == nil {
		var page platform.RegistrationPage
		page, err = store.ReadRegistrationPage(sourceCtx, after, 100)
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
	// Already-ingested grants remain recoverable if reading new receipts fails.
	for i := 0; i < 20 && ctx.Err() == nil; i++ {
		jobCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		found, e := store.RecoverRegistrationGrant(jobCtx)
		stop()
		if e != nil || !found {
			break
		}
	}
}
func runAdmissionWorker(ctx context.Context, store registrationWorkerStore) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		registrationWorkerCycle(ctx, store)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
