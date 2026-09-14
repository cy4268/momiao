package transport

import (
	"context"
	"encoding/json"
	"strconv"
)

type liveConnection struct {
	principal Principal
	ref       ConnectionRef
	ctx       context.Context
	sub       *subscriber
	control   *ControlView
}

// RevokeSession closes this runtime's sockets after the Native authority has
// revoked their SID. Registration and every subsequent operation independently
// recheck that authority, including handshakes racing with this enumeration.
func (h *Handler) RevokeSession(user int64, nativeHash string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	var targets []*subscriber
	for _, live := range h.live {
		if live.principal.UserID == user && live.principal.SessionIDHash == nativeHash {
			targets = append(targets, live.sub)
		}
	}
	h.mu.Unlock()
	for _, target := range targets {
		target.cancel()
	}
}

// CancelTableUser closes every live socket for one table-local moderation
// target. The domain commits the removal before invoking this callback.
func (h *Handler) CancelTableUser(table string, user int64) {
	if h == nil || !validUUID(table) || user <= 0 {
		return
	}
	h.mu.Lock()
	var targets []*subscriber
	for _, live := range h.live {
		if live.ref.TableID == table && live.principal.UserID == user {
			targets = append(targets, live.sub)
		}
	}
	h.mu.Unlock()
	for _, target := range targets {
		target.cancel()
	}
}

func validConnectionID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
func samePrincipal(a, b Principal) bool {
	return a.UserID == b.UserID && a.SessionIDHash == b.SessionIDHash && a.SessionVersion == b.SessionVersion && a.SecurityEpoch == b.SecurityEpoch
}
func controlEpoch(s string) (uint64, bool) {
	v, err := strconv.ParseUint(s, 10, 64)
	return v, err == nil && strconv.FormatUint(v, 10) == s
}
func sameControl(a, b *ControlView) bool { return a != nil && b != nil && *a == *b }

// NotifyControlChanged is an ephemeral-authority wakeup, not a commit or a
// payload. A single pending wakeup coalesces changes; the reader asks the real
// owner for the latest projection. It neither renews a lease nor blocks an actor.
func (h *Handler) NotifyControlChanged(ref ConnectionRef) {
	h.mu.Lock()
	defer h.mu.Unlock()
	live := h.live[ref.ID]
	if h.closed || live == nil || live.ref != ref || live.ctx.Err() != nil {
		return
	}
	select {
	case live.sub.control <- struct{}{}:
	default:
	}
}

func (h *Handler) rememberControl(ref ConnectionRef, control *ControlView) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if live := h.live[ref.ID]; live != nil && live.ref == ref && control != nil {
		copy := *control
		live.control = &copy
	}
}

func (h *Handler) takeOver(ctx context.Context, p Principal, session string, request TakeOverRequest) (json.RawMessage, error) {
	if h.opts.Ports.TakeOver == nil {
		return nil, errUnavailable
	}
	h.mu.Lock()
	live := h.live[request.ConnectionID]
	if live == nil || live.ctx.Err() != nil || !samePrincipal(p, live.principal) || live.principal.ControlIntent != "CLAIM_CONTROL" || live.control == nil || live.control.SessionID != session {
		h.mu.Unlock()
		return nil, &Fault{403, "CONTROL_TARGET_DENIED"}
	}
	request.Target = live.ref
	h.mu.Unlock()
	// The domain validates this target again: it may disconnect or change owner
	// after the lookup. The ID is only a target identifier, never a credential.
	return h.opts.Ports.TakeOver(ctx, p, session, request)
}

func validateControlRef(ref ControlRef, p Principal, connection ConnectionRef, snapshot Snapshot, expected uint64) error {
	if !samePrincipal(p, Principal{UserID: ref.UserID, SessionIDHash: ref.SessionIDHash, SessionVersion: ref.SessionVersion, SecurityEpoch: ref.SecurityEpoch}) || ref.ConnectionID != connection.ID || ref.TableID != connection.TableID || ref.RuntimeEpoch != snapshot.RuntimeEpoch || ref.ControlEpoch != expected || ref.ControlEpoch == 0 || !validUUID(ref.SessionID) || snapshot.Control == nil || snapshot.Control.Mode != "CONTROLLER" || snapshot.Control.SessionID != ref.SessionID || snapshot.Control.ControlEpoch != strconv.FormatUint(expected, 10) {
		return &Fault{409, "CONTROL_BINDING_MISMATCH"}
	}
	return nil
}
