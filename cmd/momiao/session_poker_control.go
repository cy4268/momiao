package main

import (
	"context"
	"strconv"

	"github.com/cy4268/momiao/internal/session"
)

// Assigned during startup, before the HTTP listener can accept requests.
type sessionPokerControl struct {
	poker *pokerApplication
	remote *pokerRemote
}

func (c *sessionPokerControl) RevokeSessionControl(ctx context.Context, binding session.BFFBinding) error {
	if c != nil && c.remote != nil { return c.remote.revoke(ctx, binding) }
	user, err := strconv.ParseInt(binding.UserID, 10, 64)
	if err != nil || user <= 0 || c == nil || c.poker == nil || c.poker.handler == nil || c.poker.service == nil {
		return errPokerStartup
	}
	err = c.poker.service.RevokeSessionControls(ctx, user, binding.NativeSessionIDHash)
	c.poker.handler.RevokeSession(user, binding.NativeSessionIDHash)
	return err
}
