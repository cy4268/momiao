package main

import (
 "context"
 "net/http"
 "strconv"
 "strings"
 "time"

 "github.com/cy4268/momiao/internal/games"
 "github.com/cy4268/momiao/internal/platform"
 "github.com/cy4268/momiao/internal/session"
)

func newOpsGamesHandler(sessions *session.Service,store *platform.Store,service *games.Service,next http.Handler)http.Handler{
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  const path="/api/v1/ops/games"
  if r.URL.Path!=path&&!strings.HasPrefix(r.URL.Path,path+"/"){if next!=nil{next.ServeHTTP(w,r)}else{walletError(w,404,"NOT_FOUND")};return}
  w.Header().Set("Cache-Control","no-store");w.Header().Set("X-Content-Type-Options","nosniff")
  if !opsRequestSafe(r)||r.ContentLength!=0||len(r.TransferEncoding)!=0{walletError(w,400,"OPS_INPUT_INVALID");return}
  if !requireMethod(w,r,http.MethodGet)||!requireOpsQuery(w,r,"authz_epoch"){return}
  slug:=strings.TrimPrefix(r.URL.Path,path)
  if slug!=""{slug=strings.TrimPrefix(slug,"/");switch slug{case "dice","scratch","summon","slot","blackjack","poker","devil-roulette","pressure-roulette":default:walletError(w,404,"NOT_FOUND");return}}
  epoch,valid:=opsEpoch(r.URL.Query().Get("authz_epoch"));if !valid{walletError(w,400,"OPS_INPUT_INVALID");return}
  if sessions==nil||store==nil||service==nil{walletError(w,503,"OPS_UNAVAILABLE");return}
  ctx,cancel:=context.WithTimeout(r.Context(),10*time.Second);defer cancel();r=r.WithContext(ctx)
  verified,err:=sessions.VerifyPrivateRequest(r);if err!=nil{sessionError(w,err);return}
  actor,err:=strconv.ParseInt(verified.View().UserID,10,64);if err!=nil||actor<=0{walletError(w,401,"SESSION_UNAUTHORIZED");return}
  if _,err=store.RequireOpsPermission(ctx,actor,epoch,"games.read");err!=nil{writeOpsError(w,err);return}
  if slug==""{result,err:=service.ReadOps(ctx);if err!=nil{gameError(w,err);return};sessionEnvelope(w,200,result);return}
  result,err:=service.ReadOpsGame(ctx,slug);if err!=nil{gameError(w,err);return};sessionEnvelope(w,200,result)
 })
}
