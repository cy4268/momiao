package main

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "os"
 "path/filepath"
 "sort"
 "time"
)

type readinessCheck func(context.Context) error
type readinessComponent struct {Name string `json:"name"`;State string `json:"state"`}
type readinessView struct {Ready bool `json:"ready"`;CheckedAt time.Time `json:"checked_at"`;Components []readinessComponent `json:"components"`}

// Configured dependency readiness is distinct from process liveness and from
// acceptance of an end-to-end business operation. Errors never expose DSNs.
func readReadiness(ctx context.Context,checks map[string]readinessCheck) readinessView {
 view:=readinessView{Ready:len(checks)>0,CheckedAt:time.Now().UTC(),Components:[]readinessComponent{}}
 names:=make([]string,0,len(checks));for name:=range checks{names=append(names,name)};sort.Strings(names)
 bounded,cancel:=context.WithTimeout(ctx,2*time.Second);defer cancel()
 type outcome struct{name string;ok bool};done:=make(chan outcome,len(checks))
 for _,name:=range names{check:=checks[name];go func(name string,check readinessCheck){done<-outcome{name,check!=nil&&check(bounded)==nil}}(name,check)}
 results:=map[string]bool{}
 for range names{select{case result:=<-done:results[result.name]=result.ok;case <-bounded.Done():goto finished}}
finished:
 for _,name:=range names{state:="UNAVAILABLE";if results[name]{state="READY"}else{view.Ready=false};view.Components=append(view.Components,readinessComponent{name,state})}
 return view
}
func newReadinessHandler(checks map[string]readinessCheck) http.Handler {
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Cache-Control","no-store");w.Header().Set("X-Content-Type-Options","nosniff")
  if r.URL.Path!="/readyz"{walletError(w,404,"NOT_FOUND");return}
  if r.Method!=http.MethodGet&&r.Method!=http.MethodHead{w.Header().Set("Allow","GET, HEAD");walletError(w,405,"METHOD_NOT_ALLOWED");return}
  result:=readReadiness(r.Context(),checks);status:=http.StatusServiceUnavailable;if result.Ready{status=http.StatusOK}
  if r.Method==http.MethodHead{w.WriteHeader(status);return};walletJSON(w,status,result)
 })
}
func webReadiness(root string) readinessCheck {
 return func(ctx context.Context)error{if ctx.Err()!=nil{return ctx.Err()};info,err:=os.Stat(filepath.Join(root,"index.html"));if err!=nil||!info.Mode().IsRegular(){return errors.New("web unavailable")};return nil}
}
func nativeReadiness(transport http.RoundTripper) readinessCheck {
 return func(ctx context.Context)error{
  request,_:=http.NewRequestWithContext(ctx,http.MethodGet,"http://unix/api/status",nil);request.Host="localhost";request.Header.Set("Accept","application/json")
  response,err:=transport.RoundTrip(request)
  if err!=nil||response==nil {if response!=nil&&response.Body!=nil{_ = response.Body.Close()};return errors.New("native unavailable")}
  if response.Body==nil{return errors.New("native unavailable")};defer response.Body.Close()
  if response.StatusCode!=http.StatusOK{return errors.New("native unavailable")}
  raw,err:=io.ReadAll(io.LimitReader(response.Body,256*1024+1));if err!=nil||len(raw)>256*1024{return errors.New("native unavailable")}
  var result struct{Success bool `json:"success"`};if json.Unmarshal(raw,&result)!=nil||!result.Success{return errors.New("native unavailable")};return nil
 }
}
