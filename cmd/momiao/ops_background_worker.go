package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

type opsBackgroundHealth struct {
	mu        sync.RWMutex
	checkedAt time.Time
	failed    bool
}

func (h *opsBackgroundHealth) observe(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkedAt = time.Now()
	h.failed = err != nil
}
func (h *opsBackgroundHealth) check(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.failed || h.checkedAt.IsZero() || time.Since(h.checkedAt) > 15*time.Second {
		return errors.New("Ops background worker unavailable")
	}
	return nil
}

func runOpsBackgroundWorker(ctx context.Context, health *opsBackgroundHealth, step func(context.Context) (bool, error)) {
	for {
		if ctx.Err() != nil {
			return
		}
		call, cancel := context.WithTimeout(ctx, 5*time.Second)
		progressed, err := step(call)
		cancel()
		health.observe(err)
		if err == nil && progressed {
			continue
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
