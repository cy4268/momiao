// Package historyaccess carries server-authenticated identities, never request bodies.
package historyaccess

import (
	"context"
	"errors"
)

var (
	ErrInvalid     = errors.New("history: invalid request")
	ErrNotFound    = errors.New("history: record not found")
	ErrUnavailable = errors.New("history: source unavailable")
)

// RecordsVerifier must check current server-side Records Scope and audit access.
// Success is bound to both identities; absence or revocation must return an error.
type RecordsVerifier func(context.Context, int64, int64) error

type Access struct {
	viewer, subject int64
	verify          RecordsVerifier
	alwaysVerify    bool
}

func Own(authenticatedUser int64) Access {
	return Access{viewer: authenticatedUser, subject: authenticatedUser}
}

// Records is an internal BFF seam, not a scope issuer or a client assertion.
func Records(authenticatedViewer, subject int64, verify RecordsVerifier) Access {
	return Access{viewer: authenticatedViewer, subject: subject, verify: verify, alwaysVerify: true}
}

// Check is called afresh for each read, before acquiring a source connection.
func (a Access) Check(ctx context.Context) (int64, int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	if a.viewer <= 0 || a.subject <= 0 {
		return 0, 0, ErrInvalid
	}
	if a.viewer != a.subject || a.alwaysVerify {
		if a.verify == nil || a.verify(ctx, a.viewer, a.subject) != nil {
			return 0, 0, ErrNotFound
		}
	}
	return a.viewer, a.subject, nil
}
