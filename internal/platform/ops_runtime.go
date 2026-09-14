package platform

import (
 "context"
 "slices"
 "time"
)

type OpsJobSummary struct {
 Kind string `json:"kind"`
 Pending int64 `json:"pending_count,string"`
 Attention int64 `json:"attention_count,string"`
 Due int64 `json:"due_count,string"`
 OldestDueAt *time.Time `json:"oldest_due_at"`
}

func(s *Store) ReadOpsJobs(ctx context.Context,user int64)([]OpsJobSummary,error){
 principal,err:=s.RequireOpsPermission(ctx,user,0,"jobs.read");if err!=nil{return nil,err}
 rows,err:=s.pool.Query(ctx,`SELECT kind,pending_count,attention_count,due_count,oldest_due_at FROM ops.runtime_jobs_read() ORDER BY kind`)
 if err!=nil{return nil,err};defer rows.Close()
 result:=[]OpsJobSummary{}
 permissions:=map[string]string{"ANNOUNCEMENTS":"announcements.read","GAME_ROUNDS":"games.read","REGISTRATION_GRANTS":"rewards.read","KEY_PURPOSE_SYNC":"economy.read","QUOTA_TRANSFERS":"economy.read","MAINTENANCE":"maintenance.read","OPS_REMOTE":"poker.read","RANKING_REBUILDS":"rankings.read"}
 for rows.Next(){var item OpsJobSummary;if err:=rows.Scan(&item.Kind,&item.Pending,&item.Attention,&item.Due,&item.OldestDueAt);err!=nil{return nil,err};if slices.Contains(principal.Permissions,permissions[item.Kind]){result=append(result,item)}}
 return result,rows.Err()
}
