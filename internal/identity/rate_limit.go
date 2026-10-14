// Copyright QuantumNous and contributors. AGPL-3.0-or-later.
// Adapted from official new-api@f1164142 middleware/rate-limit.go:
// unchanged fixed-window Lua/defaults, existing Redis client and net/http boundary.
package identity

import (
	"net/http"
	"net/netip"
	"strconv"
)

const redisFixedWindowScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
local ttl = redis.call('TTL', KEYS[1])
if ttl < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
  ttl = redis.call('TTL', KEYS[1])
end
if count > tonumber(ARGV[1]) then
  return {0, count, ttl}
end
return {1, count, ttl}
`

func (h *HTTP) limitPasswordLogin(w http.ResponseWriter, r *http.Request) error {
	ip := r.RemoteAddr
	if peer, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		ip = peer.Addr().Unmap().String()
		// The deployed private edge forwards Cloudflare's single IP over loopback.
		// Direct non-loopback clients never choose their bucket through headers.
		if peer.Addr().Unmap().IsLoopback() {
			if forwarded, err := netip.ParseAddr(r.Header.Get("CF-Connecting-IP")); err == nil {
				ip = forwarded.Unmap().String()
			}
		}
	}
	values, err := h.Sessions.Redis.Eval(r.Context(), redisFixedWindowScript,
		[]string{"chaldea:session:rate:CT:" + ip}, 20, 20*60).Int64Slice()
	if err != nil {
		return err
	}
	if values[0] == 0 {
		if values[2] > 0 {
			w.Header().Set("Retry-After", strconv.FormatInt(values[2], 10))
		}
		return Fault{429, "AUTH_RATE_LIMITED"}
	}
	return nil
}
