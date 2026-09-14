package redislease

import (
	"context"
	"strconv"
	"strings"
	"time"
)

const accessAttemptPrefix = "chaldea:ratelimit:poker:table-password:"
const accessAttemptScript = `
local counts = {}
for i, key in ipairs(KEYS) do
  local raw = redis.call('GET', key)
  if raw then
    if #raw > 10 or not string.match(raw, '^[1-9][0-9]*$') then return 0 end
    local count = tonumber(raw)
    local ttl = redis.call('PTTL', key)
    if not count or count > 4294967295 or count >= tonumber(ARGV[2*i-1]) or ttl <= 0 or ttl > tonumber(ARGV[2*i]) then return 0 end
    counts[i] = count
  else
    counts[i] = 0
  end
end
for i, key in ipairs(KEYS) do
  if counts[i] == 0 then
    redis.call('SET', key, '1', 'PX', ARGV[2*i])
  else
    redis.call('INCR', key)
  end
end
return 1
`

// AccessAttempt reserves an attempt, not access. Identity/config are trusted
// caller inputs; success never resets a bucket. Empty tableID uses owner only.
// Every key is preflighted before writes; runtime errors can still consume budget.
func (s *Store) AccessAttempt(ctx context.Context, userID int64, tableID string, ownerMax uint32, ownerWindow time.Duration, tableMax uint32, tableWindow time.Duration) (bool, error) {
	if userID <= 0 || ownerMax == 0 || !accessWindow(ownerWindow) {
		return false, ErrInvalidInput
	}
	user := strconv.FormatInt(userID, 10)
	keys := []string{accessAttemptPrefix + "owner:" + user}
	if tableID != "" {
		if _, err := leaseKey(tableID, 1, strings.Repeat("0", 64)); err != nil || tableMax == 0 || !accessWindow(tableWindow) {
			return false, ErrInvalidInput
		}
		keys = append(keys, accessAttemptPrefix+"owner-table:"+user+":"+tableID)
	}
	return s.accessEval(ctx, keys, ownerMax, ownerWindow.Milliseconds(), tableMax, tableWindow.Milliseconds())
}

func accessWindow(window time.Duration) bool {
	return window >= time.Millisecond && window <= 24*time.Hour && window%time.Millisecond == 0
}

func (s *Store) accessEval(ctx context.Context, keys []string, args ...any) (bool, error) {
	if ctx == nil {
		return false, ErrInvalidInput
	}
	if s == nil || s.client == nil || s.timeout <= 0 {
		return false, ErrUnavailable
	}
	if ctx.Err() != nil {
		return false, unavailable(ctx)
	}
	bounded, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	value, err := s.client.Eval(bounded, accessAttemptScript, keys, args...).Result()
	if err != nil || bounded.Err() != nil {
		return false, unavailable(bounded)
	}
	result, ok := value.(int64)
	if !ok || result != 0 && result != 1 {
		return false, ErrUnavailable
	}
	return result == 1, nil
}
