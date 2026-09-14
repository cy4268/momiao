package main

import (
 "context"
 "net/http"
 "time"
 "github.com/cy4268/momiao/internal/platform"
)

func newMaintenanceNoticesHandler(store *platform.Store,environment string)http.Handler{
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if !requireMethod(w,r,http.MethodGet){return}
  if r.URL.RawPath!=""||r.URL.RawQuery!=""||r.URL.ForceQuery||r.ContentLength>0{walletError(w,400,"INVALID_REQUEST");return}
  if store==nil||environment==""{walletError(w,503,"MAINTENANCE_STATUS_UNAVAILABLE");return}
  ctx,cancel:=context.WithTimeout(r.Context(),2*time.Second);defer cancel()
  items,err:=store.ReadPublicMaintenanceNotices(ctx,environment);if err!=nil{walletError(w,503,"MAINTENANCE_STATUS_UNAVAILABLE");return}
  sessionEnvelope(w,200,map[string]any{"items":items})
 })
}
