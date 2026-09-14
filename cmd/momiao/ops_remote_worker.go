package main

import (
 "context"
 "errors"
 "sync"
 "time"

 "github.com/cy4268/momiao/internal/platform"
)

type opsRemoteWorkerHealth struct{mu sync.RWMutex;checkedAt time.Time;failed bool}
func(h *opsRemoteWorkerHealth)check(ctx context.Context)error{
 if ctx.Err()!=nil{return ctx.Err()};h.mu.RLock();defer h.mu.RUnlock()
 if h.failed||h.checkedAt.IsZero()||time.Since(h.checkedAt)>45*time.Second{return errors.New("Ops remote worker unavailable")};return nil
}
func runObservedOpsRemoteWorker(ctx context.Context,service *platform.OpsService,health *opsRemoteWorkerHealth){
 for{
  if ctx.Err()!=nil{return}
  call,cancel:=context.WithTimeout(ctx,35*time.Second)
  progressed,err:=service.RunRemoteDispatchStep(call);cancel()
  health.mu.Lock();health.checkedAt=time.Now();health.failed=err!=nil;health.mu.Unlock()
  if err==nil&&progressed{continue}
  timer:=time.NewTimer(2*time.Second)
  select{case <-ctx.Done():timer.Stop();return;case <-timer.C:}
 }
}
