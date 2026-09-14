package platform

import (
 "context"
 "encoding/json"
 "errors"
 "time"
 "github.com/jackc/pgx/v5"
)

type OpsHealthSnapshot struct {
 Snapshot json.RawMessage `json:"snapshot"`
 ObservedAt time.Time `json:"observed_at"`
 StaleAfter time.Time `json:"stale_after"`
 Stale bool `json:"stale"`
}
// Only the process's code-owned checker calls this writer; it has no HTTP
// mutation route and accepts no administrator-defined probes or commands.
func(s *Store) WriteOpsHealthSnapshot(ctx context.Context,environment string,raw json.RawMessage,observed time.Time,ttl time.Duration)error{
 if len(raw)>16*1024||!json.Valid(raw)||ttl<5*time.Second||ttl>5*time.Minute{return ErrOpsInvalid}
 _,err:=s.pool.Exec(ctx,`INSERT INTO ops.health_snapshots(environment,snapshot,observed_at,stale_after) VALUES($1,$2,$3,$4)
 ON CONFLICT(environment) DO UPDATE SET snapshot=excluded.snapshot,observed_at=excluded.observed_at,stale_after=excluded.stale_after
 WHERE ops.health_snapshots.observed_at<excluded.observed_at`,environment,raw,observed,observed.Add(ttl));return err
}
func(s *Store) ReadOpsHealthSnapshot(ctx context.Context,user int64,environment string)(OpsHealthSnapshot,error){
 var result OpsHealthSnapshot
 if _,err:=s.RequireOpsPermission(ctx,user,0,"service-health.read");err!=nil{return result,err}
 err:=s.pool.QueryRow(ctx,`SELECT snapshot,observed_at,stale_after,stale_after<=clock_timestamp() FROM ops.health_snapshots WHERE environment=$1`,environment).Scan(&result.Snapshot,&result.ObservedAt,&result.StaleAfter,&result.Stale)
 if errors.Is(err,pgx.ErrNoRows){return result,ErrOpsUnavailable};return result,err
}
