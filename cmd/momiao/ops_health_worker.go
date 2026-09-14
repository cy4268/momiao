package main

import (
 "context"
 "encoding/json"
 "time"
 "github.com/cy4268/momiao/internal/platform"
)

func runOpsHealthWorker(ctx context.Context,store *platform.Store,environment string,checks map[string]readinessCheck){
 tick:=time.NewTicker(15*time.Second);defer tick.Stop()
 for {
  if ctx.Err()!=nil{return}
  view:=readReadiness(ctx,checks);raw,err:=json.Marshal(view)
  if err==nil{bounded,cancel:=context.WithTimeout(ctx,2*time.Second);_ = store.WriteOpsHealthSnapshot(bounded,environment,raw,view.CheckedAt,45*time.Second);cancel()}
  select{case <-ctx.Done():return;case <-tick.C:}
 }
}
