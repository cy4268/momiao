package platform

import (
	"context"
	"sync"
	"time"
)

// The Native ledger is deterministic and isolated from every real Native account.
type exchangeNative struct {
	mu        sync.Mutex
	quota     int64
	effects   map[string]NativeQuotaReceipt
	ambiguous bool
	unknown   bool
	forged    bool
}

func (n *exchangeNative) ReadNativeQuota(_ context.Context, user int64) (NativeQuotaSnapshot, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return NativeQuotaSnapshot{UserID: user, RawQuota: n.quota, Enabled: true, Result: "APPLIED", ObservedAt: time.Now().UTC()}, nil
}
func (n *exchangeNative) QueryQuotaOperation(_ context.Context, id string, user int64) (NativeQuotaReceipt, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.unknown {
		return NativeQuotaReceipt{ID: id, UserID: user, Result: "UNKNOWN"}, nil
	}
	if n.forged {
		return NativeQuotaReceipt{ID: id, UserID: user + 1, Result: "NOT_APPLIED"}, nil
	}
	if r, ok := n.effects[id]; ok {
		return r, nil
	}
	return NativeQuotaReceipt{ID: id, UserID: user, Result: "NOT_APPLIED"}, nil
}
func (n *exchangeNative) ApplyRawQuotaDelta(_ context.Context, id string, user, delta int64) (NativeQuotaReceipt, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.unknown {
		return NativeQuotaReceipt{ID: id, UserID: user, DeltaRawQuota: delta, Result: "UNKNOWN"}, nil
	}
	if n.forged {
		return NativeQuotaReceipt{ID: id, UserID: user + 1, DeltaRawQuota: delta, Result: "APPLIED"}, nil
	}
	if r, ok := n.effects[id]; ok {
		if r.UserID != user || r.DeltaRawQuota != delta {
			return NativeQuotaReceipt{}, ErrIdempotencyConflict
		}
		return r, nil
	}
	before := n.quota
	after := before + delta
	r := NativeQuotaReceipt{ID: id, UserID: user, DeltaRawQuota: delta, Before: &before, After: &after, Result: "APPLIED"}
	if after < 0 {
		r.Result = "INSUFFICIENT"
		unchanged := before
		r.After = &unchanged
	} else {
		n.quota = after
	}
	if n.effects == nil {
		n.effects = map[string]NativeQuotaReceipt{}
	}
	n.effects[id] = r
	if n.ambiguous {
		n.ambiguous = false
		return NativeQuotaReceipt{}, ErrNativeQuotaDependency
	}
	return r, nil
}
func (n *exchangeNative) Credit(ctx context.Context, id string, user, amount int64) (NativeQuotaReceipt, error) {
	return n.ApplyRawQuotaDelta(ctx, id, user, amount)
}
