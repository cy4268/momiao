package bffauth

import (
 "context"
 "encoding/json"
 "net/http"
 "unicode/utf8"

 "github.com/cy4268/momiao/internal/session"
)

// Only short-lived factor results cross this seam. Native SID, access and
// refresh credentials remain in the existing sealed server-side credential store.
type OpsFactorResult struct{Proof string;Require2FA bool;FlowToken string}
func(OpsFactorResult) String()string{return "OpsFactorResult([redacted])"}
func(OpsFactorResult) GoString()string{return "OpsFactorResult([redacted])"}

func(s *Service) OpsPassword(ctx context.Context,verified session.RequestSession,token,password string)(OpsFactorResult,error){
 if !opaque(token)||password==""||len(password)>256||!utf8.ValidString(password){return OpsFactorResult{},errInput}
 return s.opsFactor(ctx,verified,false,map[string]string{"challenge_token":token,"password":password})
}
func(s *Service) OpsTwoFactor(ctx context.Context,verified session.RequestSession,token,code string)(OpsFactorResult,error){
 if !opaque(token)||code==""||len(code)>64||!utf8.ValidString(code){return OpsFactorResult{},errInput}
 return s.opsFactor(ctx,verified,true,map[string]string{"flow_token":token,"code":code})
}
func(s *Service) opsFactor(ctx context.Context,verified session.RequestSession,twoFA bool,body map[string]string)(OpsFactorResult,error){
 var result OpsFactorResult
 if s==nil||s.sessions==nil{return result,errUnavailable}
 binding,err:=s.sessions.BFFBinding(ctx,verified);if err!=nil{return result,mapSessionError(err)}
 credential,err:=s.ensureCredential(ctx,binding);if err!=nil{return result,err}
 current,err:=s.sessions.BFFBinding(ctx,verified);if err!=nil{return result,mapSessionError(err)};if current!=binding{return result,errUnauthorized}
 path:="/api/momiao/ops/fresh/password";if twoFA{path="/api/momiao/ops/fresh/2fa"}
 response,raw,err:=s.momiaoCall(ctx,http.MethodPost,path,nil,body,&credential,"");if err!=nil{return result,Fault{Status:503,Code:"OPS_FACTOR_RESULT_UNKNOWN",Unknown:true}}
 if response.StatusCode==429{return result,Fault{Status:429,Code:"AUTH_RATE_LIMITED"}}
 envelope,ok:=parseMomiaoEnvelope(raw);if !ok||len(response.Header.Values("Set-Cookie"))!=0{return result,errUnavailable}
 if response.StatusCode!=200||!envelope.Success{
  if envelope.Code=="MOMIAO_INVALID_REQUEST"||envelope.Code=="MOMIAO_2FA_INVALID"{return result,Fault{Status:422,Code:"OPS_FACTOR_REJECTED"}}
  if envelope.Code=="MOMIAO_FLOW_CONSUMED"{return result,Fault{Status:409,Code:"OPS_FACTOR_CONSUMED"}}
  return result,errUnavailable
 }
 object,ok:=exactObject(envelope.Data,"proof","require_2fa","flow_token");if !ok{return result,errUnavailable}
 var decoded struct{Proof string `json:"proof"`;Require2FA bool `json:"require_2fa"`;FlowToken string `json:"flow_token"`}
 if json.Unmarshal(envelope.Data,&decoded)!=nil{return result,errUnavailable}
 if len(object)==1&&opaque(decoded.Proof)&&!decoded.Require2FA&&decoded.FlowToken==""{result.Proof=decoded.Proof}else if !twoFA&&len(object)==2&&decoded.Proof==""&&decoded.Require2FA&&opaque(decoded.FlowToken){result.Require2FA=true;result.FlowToken=decoded.FlowToken}else{return result,errUnavailable}
 current,err=s.sessions.BFFBinding(ctx,verified);if err!=nil{return OpsFactorResult{},mapSessionError(err)};if current!=binding{return OpsFactorResult{},errUnauthorized}
 return result,nil
}
