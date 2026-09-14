package main

import (
 "errors"
 "net/http"
 "net/url"
 "strconv"

 "github.com/cy4268/momiao/internal/nativeself"
 "github.com/cy4268/momiao/internal/rankings"
)

func newRankingsHandler(service *rankings.Service, private bool) http.Handler {
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Cache-Control","no-store")
  w.Header().Set("X-Content-Type-Options","nosniff")
  path:="/api/v1/rankings";if private{path+="/me"}
  if r.URL.Path!=path||r.URL.RawPath!=""{walletError(w,404,"NOT_FOUND");return}
  if r.Method!=http.MethodGet{w.Header().Set("Allow","GET");walletError(w,405,"METHOD_NOT_ALLOWED");return}
  var user int64
  if private {
   claimed,ok:=authHeader(r,"New-Api-User",19,true)
   id,e:=decimalInt(claimed)
   if !ok||e!=nil||id<=0||!nativeself.SessionCredential(r){walletError(w,401,"UNAUTHORIZED");return};user=id
  }
  query,e:=url.ParseQuery(r.URL.RawQuery)
  if e!=nil||len(r.URL.RawQuery)>2048||r.URL.ForceQuery {walletError(w,400,"RANKING_QUERY_INVALID");return}
  for key,values:=range query {
   if len(values)!=1{walletError(w,400,"RANKING_QUERY_INVALID");return}
   switch key{case "metric","period","date","model","page":default:walletError(w,400,"RANKING_QUERY_INVALID");return}
  }
  q:=rankings.Query{Metric:query.Get("metric"),Period:query.Get("period"),Date:query.Get("date"),Model:query.Get("model"),Page:1}
  if q.Metric==""{q.Metric="TOTAL_ASSETS"};if q.Period==""{if q.Metric=="TOTAL_ASSETS"{q.Period="CURRENT"}else{q.Period="DAY"}}
  if value:=query.Get("page");value!=""{q.Page,e=strconv.Atoi(value);if e!=nil{walletError(w,400,"RANKING_QUERY_INVALID");return}}
  result,e:=service.Read(r.Context(),q,user)
  if e!=nil{if errors.Is(e,rankings.ErrQuery){walletError(w,400,"RANKING_QUERY_INVALID")}else{walletError(w,503,"RANKINGS_UNAVAILABLE")};return}
  walletSuccess(w,result)
 })
}
