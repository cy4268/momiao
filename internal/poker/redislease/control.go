package redislease

import (
	"context"
	"time"
)

const ControlKeyPrefix = "chaldea:poker:control:"
const ControlTTL = 30 * time.Second
const ControlRenewInterval = 10 * time.Second

// Opaque, canonical owner values are compared in full. Neither CAS nor renewal
// can turn a different connection into an owner. PG epochs remain the authority.
const controlLoadScript = `
local v=redis.call('GET',KEYS[1])
if not v then return '' end
local ttl=redis.call('PTTL',KEYS[1])
if ttl <= 0 or ttl > 30000 then return redis.error_reply('invalid control TTL') end
return v
`
const controlAssignScript = `
local v=redis.call('GET',KEYS[1])
if ARGV[1] == '' then
 if v then return 0 end
elseif v ~= ARGV[1] then return 0 end
redis.call('SET',KEYS[1],ARGV[2],'PX',30000)
return 1
`
const controlRenewScript = `
if redis.call('GET',KEYS[1]) ~= ARGV[1] then return 0 end
local ttl=redis.call('PTTL',KEYS[1])
if ttl <= 0 or ttl > 30000 then return 0 end
redis.call('SET',KEYS[1],ARGV[1],'PX',30000)
return 1
`

func controlKey(session string) (string, error) {
	// Reuse the UUID validator, never compose arbitrary Redis keys.
	if _, err := leaseKey(session, 1, "0000000000000000000000000000000000000000000000000000000000000000"); err != nil {
		return "", err
	}
	return ControlKeyPrefix + session, nil
}
func validControlValue(v string) bool { return len(v) > 0 && len(v) <= 2048 }
func (s *Store) ControlLoad(ctx context.Context, session string) (string, error) {
	key, err := controlKey(session)
	if err != nil {
		return "", err
	}
	if ctx == nil {
		return "", ErrInvalidInput
	}
	if s == nil || s.client == nil || s.timeout <= 0 {
		return "", ErrUnavailable
	}
	bounded, cancel := context.WithTimeout(ctx, min(s.timeout, 2*time.Second))
	defer cancel()
	value, err := s.client.Eval(bounded, controlLoadScript, []string{key}).Result()
	if err != nil || bounded.Err() != nil {
		return "", unavailable(bounded)
	}
	v, ok := value.(string)
	if !ok || (v != "" && !validControlValue(v)) {
		return "", ErrUnavailable
	}
	return v, nil
}
func (s *Store) ControlAssign(ctx context.Context, session, expected, next string) (bool, error) {
	key, err := controlKey(session)
	if err != nil {
		return false, err
	}
	if !validControlValue(next) || (expected != "" && !validControlValue(expected)) {
		return false, ErrInvalidInput
	}
	return s.controlEval(ctx, controlAssignScript, key, expected, next)
}
func (s *Store) ControlRenew(ctx context.Context, session, owner string) (bool, error) {
	key, err := controlKey(session)
	if err != nil {
		return false, err
	}
	if !validControlValue(owner) {
		return false, ErrInvalidInput
	}
	return s.controlEval(ctx, controlRenewScript, key, owner)
}
func (s *Store) ControlRelease(ctx context.Context, session, owner string) (bool, error) {
	key, err := controlKey(session)
	if err != nil {
		return false, err
	}
	if !validControlValue(owner) {
		return false, ErrInvalidInput
	}
	return s.controlEval(ctx, releaseScript, key, owner)
}
func (s *Store) controlEval(ctx context.Context, script, key string, args ...any) (bool, error) {
	if ctx == nil {
		return false, ErrInvalidInput
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.eval(bounded, script, key, args...)
}
