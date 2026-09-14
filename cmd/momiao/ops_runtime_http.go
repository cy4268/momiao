package main

import (
 "context"
 "net/http"
 "strconv"
 "time"

 "github.com/cy4268/momiao/internal/platform"
 "github.com/cy4268/momiao/internal/session"
)

func newOpsRuntimeHandler(sessions *session.Service,store *platform.Store,environment string)http.Handler{
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  permission:=map[string]string{"/api/v1/ops/maintenance":"maintenance.read","/api/v1/ops/jobs":"jobs.read","/api/v1/ops/service-health":"service-health.read","/api/v1/ops/attention":"attention.read"}[r.URL.Path]
  if permission=="" {walletError(w,404,"NOT_FOUND");return}
  if !opsRequestSafe(r)||r.URL.RawPath!=""||r.URL.RawQuery!=""||r.URL.ForceQuery||r.ContentLength!=0||len(r.TransferEncoding)!=0 {walletError(w,400,"OPS_INPUT_INVALID");return}
  if !requireMethod(w,r,http.MethodGet){return}
  if sessions==nil||store==nil {walletError(w,503,"OPS_UNAVAILABLE");return}
  ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second);defer cancel();r=r.WithContext(ctx)
  verified,err:=sessions.VerifyPrivateRequest(r);if err!=nil{sessionError(w,err);return}
  user,err:=strconv.ParseInt(verified.View().UserID,10,64);if err!=nil||user<=0{walletError(w,401,"SESSION_UNAUTHORIZED");return}
  if _,err=store.RequireOpsPermission(ctx,user,0,permission);err!=nil{writeOpsError(w,err);return}
  switch r.URL.Path {
  case "/api/v1/ops/maintenance":
   items,err:=store.ReadOpsMaintenance(ctx,user);if err!=nil{writeOpsError(w,err);return};sessionEnvelope(w,200,map[string]any{"items":items})
  case "/api/v1/ops/jobs":
   items,err:=store.ReadOpsJobs(ctx,user);if err!=nil{writeOpsError(w,err);return};sessionEnvelope(w,200,map[string]any{"items":items})
  case "/api/v1/ops/attention":
   jobs,err:=store.ReadOpsJobs(ctx,user);if err!=nil{writeOpsError(w,err);return}
   items:=[]platform.OpsJobSummary{};for _,job:=range jobs{if job.Attention>0{items=append(items,job)}}
   sessionEnvelope(w,200,map[string]any{"items":items})
  case "/api/v1/ops/service-health":
   snapshot,err:=store.ReadOpsHealthSnapshot(ctx,user,environment);if err!=nil{writeOpsError(w,err);return};sessionEnvelope(w,200,snapshot)
  }
 })
}
