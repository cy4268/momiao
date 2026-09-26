package main

import (
 "context"
 "errors"
 "sync"
 "time"

 "github.com/cy4268/momiao/internal/platform"
)

type maintenanceWorkerHealth struct {mu sync.RWMutex;checkedAt time.Time;failed bool}
func(h *maintenanceWorkerHealth) observe(err error){h.mu.Lock();defer h.mu.Unlock();h.checkedAt=time.Now();h.failed=err!=nil}
func(h *maintenanceWorkerHealth) check(ctx context.Context)error{
 if ctx.Err()!=nil{return ctx.Err()};h.mu.RLock();defer h.mu.RUnlock()
 if h.failed||h.checkedAt.IsZero()||time.Since(h.checkedAt)>15*time.Second{return errors.New("maintenance worker unavailable")};return nil
}

func runMaintenanceWorker(ctx context.Context, store *platform.Store,environment string,health *maintenanceWorkerHealth, extra ...func(context.Context)(bool,error)) {
 for {
  if ctx.Err()!=nil { return }
  call,cancel:=context.WithTimeout(ctx,5*time.Second)
  progressed,err:=store.RunMaintenanceStep(call,environment)
  cancel()
  if err==nil {for _,step:=range extra {call,cancel=context.WithTimeout(ctx,5*time.Second);more,e:=step(call);cancel();progressed=progressed||more;if e!=nil{err=e;break}}}
  health.observe(err)
  if err==nil&&progressed { continue }
  timer:=time.NewTimer(2*time.Second)
  select {case <-ctx.Done(): timer.Stop();return;case <-timer.C:}
 }
}
