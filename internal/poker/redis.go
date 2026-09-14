package poker

import (
	"context"
	"github.com/cy4268/momiao/internal/poker/redislease"
)

var _ LeaseStore = (*redislease.Store)(nil)
var _ ControlStore = (*redislease.Store)(nil)
var _ TableAccessStore = (*redislease.Store)(nil)

// NewWithRedis composes the real external lease authority. Close stops actors
// before closing this owned client; the supplied PG pool remains caller-owned.
func NewWithRedis(ctx context.Context, opts Options, config redislease.Config) (*Service, error) {
	if opts.Leases != nil || opts.Controls != nil || opts.TableAccess != nil || opts.ValidateSession == nil {
		return nil, ErrInvalid
	}
	lease, err := redislease.Open(ctx, config)
	if err != nil {
		return nil, err
	}
	opts.Leases = lease
	opts.Controls = lease
	if opts.Password != nil {
		opts.TableAccess = lease
	}
	service, err := New(opts)
	if err != nil {
		_ = lease.Close()
		return nil, err
	}
	service.ownedLease = lease
	if _, err = service.Start(ctx); err != nil {
		service.Close()
		return nil, err
	}
	return service, nil
}
