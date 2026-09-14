package redislease

import (
	"context"
	"strconv"
	"strings"
	"time"
)

const accessPrefix = "chaldea:poker:table-access:"
const accessPutScript = `
local clock = redis.call('TIME')
local now = tonumber(clock[1])*1000 + math.floor(tonumber(clock[2])/1000)
if tonumber(ARGV[2]) <= now then return 0 end
redis.call('SET', KEYS[1], ARGV[1]..'|'..ARGV[2], 'PXAT', ARGV[2])
return 1
`
const accessValidScript = `
local value = redis.call('GET', KEYS[1])
local prefix = ARGV[1]..'|'
if not value or #value > 137 or string.sub(value, 1, #prefix) ~= prefix then return 0 end
local raw = string.sub(value, #prefix+1)
if not string.match(raw, '^[1-9][0-9]*$') then return 0 end
local issued = tonumber(raw)
if not issued or issued < 946684800000 or issued > 253402300799999 or issued > tonumber(ARGV[2]) then return 0 end
local clock = redis.call('TIME')
local now = tonumber(clock[1])*1000 + math.floor(tonumber(clock[2])/1000)
if issued <= now or redis.call('PTTL', KEYS[1]) <= 0 then return 0 end
if redis.call('PEXPIRETIME', KEYS[1]) ~= issued then return 0 end
return 1
`

// AccessPut is only for explicit successful password verification, never a read
// or reconnect refresh. until must come from the same fresh Native/PG check.
func (s *Store) AccessPut(ctx context.Context, sid, tableID, binding string, until time.Time) (bool, error) {
	key, err := accessKey(sid, tableID, binding, until)
	if err != nil {
		return false, err
	}
	return s.eval(ctx, accessPutScript, key, binding, until.UnixMilli())
}

// AccessValid checks the exact grant without renewing it. The caller separately
// revalidates Native identity and each protected operation's durable authority.
func (s *Store) AccessValid(ctx context.Context, sid, tableID, binding string, liveUntil time.Time) (bool, error) {
	key, err := accessKey(sid, tableID, binding, liveUntil)
	if err != nil {
		return false, err
	}
	return s.eval(ctx, accessValidScript, key, binding, liveUntil.UnixMilli())
}

func accessKey(sid, tableID, binding string, until time.Time) (string, error) {
	if _, err := leaseKey(tableID, 1, sid); err != nil || !validPGTime(until.UTC()) || len(binding) > 121 {
		return "", ErrInvalidInput
	}
	parts := strings.Split(binding, ":")
	if len(parts) != 5 || parts[0] != "v1" || len(parts[4]) != 64 {
		return "", ErrInvalidInput
	}
	user, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || user <= 0 || strconv.FormatInt(user, 10) != parts[1] {
		return "", ErrInvalidInput
	}
	for i := 2; i <= 3; i++ {
		n, err := strconv.ParseUint(parts[i], 10, 64)
		if err != nil || n > 9007199254740991 || i == 2 && n == 0 || strconv.FormatUint(n, 10) != parts[i] {
			return "", ErrInvalidInput
		}
	}
	for _, c := range parts[4] {
		if !lowerHex(c) {
			return "", ErrInvalidInput
		}
	}
	return accessPrefix + sid + ":" + tableID, nil
}
