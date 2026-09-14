package main

import (
 "strings"
 "github.com/cy4268/momiao/internal/platform"
)

var opsBrowserPermissions=map[string]string{
 "/ops":"operations.read","/ops/models":"models.read","/ops/announcements":"announcements.read",
 "/ops/games":"games.read","/ops/poker":"poker.read","/ops/economy":"economy.read","/ops/rewards":"rewards.read",
 "/ops/rankings":"rankings.read","/ops/users":"users.read","/ops/records":"records.read","/ops/support-cases":"support-cases.read","/ops/incidents":"incidents.read",
 "/ops/maintenance":"maintenance.read","/ops/service-health":"service-health.read","/ops/jobs":"jobs.read","/ops/attention":"attention.read",
 "/ops/operations":"operations.read","/ops/audit":"audit.read","/ops/access-control":"access_control.read",
}
func opsBrowserPermission(route string) string {
 if permission:=opsBrowserPermissions[route];permission!=""{return permission}
 for _,prefix:=range []string{"/ops/operations/","/ops/audit/"}{if strings.HasPrefix(route,prefix)&&platform.ValidOperationKey(strings.TrimPrefix(route,prefix)){if prefix=="/ops/audit/"{return "audit.read"};return "operations.read"}}
 return ""
}
