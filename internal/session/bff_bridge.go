package session

import (
	"context"
	"time"
)

// BFFBinding is the smallest server-side projection needed to locate the
// encrypted Native credentials for an already verified platform session.
// It intentionally exposes neither the opaque platform SID nor the Native SID.
type BFFBinding struct {
	UserID              string
	NativeSessionIDHash string
}

// BFFBinding revalidates the RequestSession against the live Native and
// platform authorities before returning its minimal credential-store key.
func (s *Service) BFFBinding(ctx context.Context, h RequestSession) (BFFBinding, error) {
	if s == nil || ctx == nil || h.owner != s || !digest(h.hash) {
		return BFFBinding{}, inputFault
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	r, err := s.load(ctx, h.hash)
	if err != nil {
		return BFFBinding{}, err
	}
	if r.UserID != h.view.UserID || !sameSecret(r.CSRF, h.view.CSRFToken) {
		return BFFBinding{}, authFault
	}
	if err = s.check(ctx, r); err != nil {
		return BFFBinding{}, err
	}
	return BFFBinding{UserID: r.UserID, NativeSessionIDHash: r.NativeHash}, nil
}
