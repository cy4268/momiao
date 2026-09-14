package poker

import (
	"context"
	"errors"
	"slices"
	"strings"
)

var (
	ErrPasswordConfig      = errors.New("poker password configuration invalid")
	ErrPasswordBusy        = errors.New("poker password busy")
	ErrPasswordUnavailable = errors.New("poker password unavailable")
)

type PasswordPolicy struct {
	// Explicit per-process bounds, not production defaults or measured capacity.
	Current          PasswordConfig
	VerifyProfiles   []PasswordConfig
	MaxConcurrent    uint8
	MaxWorkMemoryKiB uint32
}

type PasswordRuntime struct {
	current  PasswordConfig
	profiles []PasswordConfig
	slots    chan struct{}
}

func passwordProfiles(p PasswordPolicy) ([]PasswordConfig, error) {
	if !p.Current.valid() || len(p.VerifyProfiles) > 4 || p.MaxConcurrent == 0 || p.MaxConcurrent > 16 || p.MaxWorkMemoryKiB == 0 || p.MaxWorkMemoryKiB > 262144 {
		return nil, ErrPasswordConfig
	}
	profiles, memory := []PasswordConfig{p.Current}, p.Current.MemoryKiB
	for _, profile := range p.VerifyProfiles {
		if !profile.valid() {
			return nil, ErrPasswordConfig
		}
		if !slices.Contains(profiles, profile) {
			profiles = append(profiles, profile)
		}
		memory = max(memory, profile.MemoryKiB)
	}
	if len(profiles) > 4 || uint64(p.MaxConcurrent)*uint64(memory) > uint64(p.MaxWorkMemoryKiB) {
		return nil, ErrPasswordConfig
	}
	return profiles, nil
}

func ValidatePasswordPolicy(p PasswordPolicy) error {
	_, err := passwordProfiles(p)
	return err
}

func NewPasswordRuntime(p PasswordPolicy) (*PasswordRuntime, error) {
	profiles, err := passwordProfiles(p)
	if err != nil {
		return nil, err
	}
	return &PasswordRuntime{current: p.Current, profiles: profiles, slots: make(chan struct{}, p.MaxConcurrent)}, nil
}

func (r *PasswordRuntime) Hash(ctx context.Context, password string) (string, error) {
	if !validTablePassword(password) {
		return "", ErrInvalid
	}
	var result string
	err := r.withSlot(ctx, func() error {
		var err error
		result, err = HashTablePassword(r.current, password)
		if err != nil {
			return ErrPasswordUnavailable
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return result, nil
}

func (r *PasswordRuntime) Verify(ctx context.Context, phc, password string) (bool, error) {
	if r == nil || r.slots == nil {
		return false, ErrPasswordConfig
	}
	if !validTablePassword(password) {
		return false, ErrInvalid
	}
	if len(phc) <= 512 {
		for _, profile := range r.profiles {
			if !strings.HasPrefix(phc, profile.prefix()) {
				continue
			}
			var matched bool
			err := r.withSlot(ctx, func() error {
				var err error
				matched, err = VerifyTablePassword(profile, phc, password)
				if err != nil {
					return ErrPasswordConfig
				}
				return nil
			})
			return matched && err == nil, err
		}
	}
	return false, ErrPasswordConfig
}

func (r *PasswordRuntime) withSlot(ctx context.Context, work func() error) error {
	if r == nil || r.slots == nil || work == nil {
		return ErrPasswordConfig
	}
	if err := passwordContextError(ctx); err != nil {
		return err
	}
	select {
	case r.slots <- struct{}{}:
		// Synchronous KDF work retains its slot even after context cancellation.
		defer func() { <-r.slots }()
	default:
		return ErrPasswordBusy
	}
	if err := passwordContextError(ctx); err != nil {
		return err
	}
	err := work()
	if cancelled := passwordContextError(ctx); cancelled != nil {
		return cancelled
	}
	return err
}

func passwordContextError(ctx context.Context) error {
	if ctx == nil {
		return ErrPasswordUnavailable
	}
	switch err := ctx.Err(); {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return errors.Join(ErrPasswordUnavailable, context.Canceled)
	case errors.Is(err, context.DeadlineExceeded):
		return errors.Join(ErrPasswordUnavailable, context.DeadlineExceeded)
	default:
		return ErrPasswordUnavailable
	}
}
