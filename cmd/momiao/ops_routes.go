package main

import (
 "strings"
 "strconv"
 "github.com/cy4268/momiao/internal/platform"
)

var opsBrowserPermissions=map[string]string{
 "/ops/logs":"logs.read","/ops/channels":"channels.read","/ops/groups":"groups.read",
 "/ops":"operations.read","/ops/models":"models.read","/ops/announcements":"announcements.read",
 "/ops/games":"games.read","/ops/poker":"poker.read","/ops/economy":"economy.read","/ops/rewards":"rewards.read",
 "/ops/rankings":"rankings.read","/ops/users":"users.read","/ops/records":"records.read","/ops/support-cases":"support-cases.read","/ops/incidents":"incidents.read",
 "/ops/maintenance":"maintenance.read","/ops/service-health":"service-health.read","/ops/jobs":"jobs.read","/ops/attention":"attention.read",
 "/ops/operations":"operations.read","/ops/audit":"audit.read","/ops/access-control":"access_control.read",
}
func opsBrowserPermission(route string) string {
 if strings.HasPrefix(route,"/ops/users/"){id,err:=strconv.ParseInt(strings.TrimPrefix(route,"/ops/users/"),10,64);if err==nil&&id>0{return "users.read"}}
 if strings.HasPrefix(route,"/ops/channels/"){id,err:=strconv.ParseInt(strings.TrimPrefix(route,"/ops/channels/"),10,64);if err==nil&&id>0{return "channels.read"}}
 if permission:=opsBrowserPermissions[route];permission!=""{return permission}
 for _,prefix:=range []string{"/ops/operations/","/ops/audit/"}{if strings.HasPrefix(route,prefix)&&platform.ValidOperationKey(strings.TrimPrefix(route,prefix)){if prefix=="/ops/audit/"{return "audit.read"};return "operations.read"}}
 return ""
}
