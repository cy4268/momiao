package main

import (
	"slices"
	"strconv"
	"strings"

	"github.com/cy4268/momiao/internal/platform"
)

func opsBrowserRoute(route string) bool {
	if slices.Contains([]string{
		"/ops", "/ops/models", "/ops/channels", "/ops/groups", "/ops/announcements",
		"/ops/games", "/ops/poker", "/ops/economy", "/ops/gambler-campaign", "/ops/rewards",
		"/ops/logs", "/ops/rankings", "/ops/users", "/ops/records", "/ops/support-cases", "/ops/incidents",
		"/ops/maintenance", "/ops/service-health", "/ops/jobs", "/ops/attention", "/ops/audit",
	}, route) {
		return true
	}
	for _, prefix := range []string{"/ops/users/", "/ops/channels/"} {
		if strings.HasPrefix(route, prefix) {
			id, err := strconv.ParseInt(strings.TrimPrefix(route, prefix), 10, 64)
			return err == nil && id > 0
		}
	}
	return strings.HasPrefix(route, "/ops/audit/") && platform.ValidOperationKey(strings.TrimPrefix(route, "/ops/audit/"))
}
