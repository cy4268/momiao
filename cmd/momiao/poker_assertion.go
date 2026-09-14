package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	pt "github.com/cy4268/momiao/internal/poker/transport"
)

// IS-500 canonical Service Assertion V1. Request bodies are buffered under the
// transport limit before signing/verifying; business operation IDs still own
// mutation idempotency. A signature authenticates the peer, never a user alone.
var pokerAssertionHeaders=[]string{"Key-Id","Issuer","Audience","Issued-At","Expires-At","Request-Id","Body-SHA256","Signature"}
const pokerAssertionPrefix="X-Chaldea-Service-"

// Key families must remain independent even if duplicate material is copied
// into two differently named configuration files.
func distinctPokerPublicKeys(first,second map[string]ed25519.PublicKey)bool{
	for _,a:=range first{for _,b:=range second{if bytes.Equal(a,b){return false}}}
	return len(first)>0&&len(second)>0
}

func readPokerPublicKeys(path string)(map[string]ed25519.PublicKey,error){
	raw,err:=readPokerPrivateFile(path,16*1024);if err!=nil{return nil,err}
	var value struct{Public map[string]string `json:"public_keys"`}
	if decodePokerObject(raw,&value,"public_keys")!=nil||len(value.Public)<1||len(value.Public)>16{return nil,errPokerConfig}
	out:=map[string]ed25519.PublicKey{}
	for id,text:=range value.Public{key,err:=hex.DecodeString(text);if err!=nil||!pokerKeyID(id)||len(key)!=ed25519.PublicKeySize{return nil,errPokerConfig};out[id]=ed25519.PublicKey(key)}
	return out,nil
}
func assertionCanonical(key,issuer,audience,request,method,uri string,iat,exp uint64,hash []byte)([]byte,error){
	var b bytes.Buffer;b.WriteString("CHALDEA-SERVICE-ASSERTION-V1");b.WriteByte(0)
	lp:=func(value string)bool{if len(value)==0||len(value)>65535||strings.ContainsAny(value,"\r\n\x00"){return false};var n [2]byte;binary.BigEndian.PutUint16(n[:],uint16(len(value)));b.Write(n[:]);b.WriteString(value);return true}
	if !lp(key)||!lp(issuer)||!lp(audience){return nil,errPokerConfig}
	var n [8]byte;binary.BigEndian.PutUint64(n[:],iat);b.Write(n[:]);binary.BigEndian.PutUint64(n[:],exp);b.Write(n[:])
	if method!=strings.ToUpper(method)||!lp(request)||!lp(method)||!lp(uri)||len(hash)!=sha256.Size{return nil,errPokerConfig};b.Write(hash);return b.Bytes(),nil
}
func signPokerRequest(r *http.Request,body []byte,keys pokerTicketKeys)error{
	if r==nil||r.URL==nil||len(keys.private)!=ed25519.PrivateKeySize{return errPokerConfig}
	var random [16]byte;if _,err:=rand.Read(random[:]);err!=nil{return errPokerConfig};id:=hex.EncodeToString(random[:])
	iat:=uint64(time.Now().Unix());exp:=iat+30;hash:=sha256.Sum256(body)
	canonical,err:=assertionCanonical(keys.active,"platform","poker",id,r.Method,r.URL.RequestURI(),iat,exp,hash[:]);if err!=nil{return err}
	values:=[]string{keys.active,"platform","poker",strconv.FormatUint(iat,10),strconv.FormatUint(exp,10),id,hex.EncodeToString(hash[:]),base64.RawURLEncoding.EncodeToString(ed25519.Sign(keys.private,canonical))}
	for i,name:=range pokerAssertionHeaders{r.Header.Set(pokerAssertionPrefix+name,values[i])};return nil
}
func verifyPokerRequest(r *http.Request,body []byte,keys map[string]ed25519.PublicKey)error{
	if r==nil||r.URL==nil{return errPokerConfig};values:=make([]string,len(pokerAssertionHeaders))
	for i,name:=range pokerAssertionHeaders{v:=r.Header.Values(pokerAssertionPrefix+name);if len(v)!=1||len(v[0])==0||len(v[0])>512{return errPokerConfig};values[i]=v[0]}
	key:=keys[values[0]];if len(key)!=ed25519.PublicKeySize||values[1]!="platform"||values[2]!="poker"{return errPokerConfig}
	iat,e1:=strconv.ParseUint(values[3],10,64);exp,e2:=strconv.ParseUint(values[4],10,64);now:=uint64(time.Now().Unix())
	if e1!=nil||e2!=nil||strconv.FormatUint(iat,10)!=values[3]||strconv.FormatUint(exp,10)!=values[4]||iat>now+5||iat+30!=exp||exp+5<now{return errPokerConfig}
	id,e:=hex.DecodeString(values[5]);if e!=nil||len(id)!=16||hex.EncodeToString(id)!=values[5]{return errPokerConfig}
	hash:=sha256.Sum256(body);if hex.EncodeToString(hash[:])!=values[6]{return errPokerConfig}
	sig,e:=base64.RawURLEncoding.DecodeString(values[7]);if e!=nil||len(sig)!=ed25519.SignatureSize{return errPokerConfig}
	canonical,e:=assertionCanonical(values[0],values[1],values[2],values[5],r.Method,r.URL.RequestURI(),iat,exp,hash[:]);if e!=nil||!ed25519.Verify(key,canonical,sig){return errPokerConfig};return nil
}
type pokerContextKey struct{}
type pokerRequestContext struct{
	UserID string `json:"newapi_user_id"`
	SessionIDHash string `json:"session_id_hash"`
	SessionVersion string `json:"session_version"`
	SecurityEpoch string `json:"security_epoch"`
}
type pokerRequestEnvelope struct{
	Context pokerRequestContext `json:"context"`
	Body []byte `json:"body"`
}
func pokerAssertedHTTPAuth(r *http.Request)(pt.Principal,error){
	p,ok:=r.Context().Value(pokerContextKey{}).(pt.Principal);if !ok||p.UserID<=0{return pt.Principal{},&pt.Fault{Status:401,Code:"POKER_AUTH_UNAUTHORIZED"}};return p,nil
}
func pokerEnvelopePrincipal(c pokerRequestContext)(pt.Principal,error){
	user,e:=strconv.ParseInt(c.UserID,10,64);if e!=nil||user<=0||strconv.FormatInt(user,10)!=c.UserID||!pokerSessionHash.MatchString(c.SessionIDHash){return pt.Principal{},errPokerConfig}
	version,e:=strconv.ParseUint(c.SessionVersion,10,64);if e!=nil||version==0||version>pt.MaxSafeInteger||strconv.FormatUint(version,10)!=c.SessionVersion{return pt.Principal{},errPokerConfig}
	epoch,e:=strconv.ParseUint(c.SecurityEpoch,10,64);if e!=nil||epoch>pt.MaxSafeInteger||strconv.FormatUint(epoch,10)!=c.SecurityEpoch{return pt.Principal{},errPokerConfig}
	return pt.Principal{UserID:user,SessionIDHash:c.SessionIDHash,SessionVersion:version,SecurityEpoch:epoch,ControlIntent:"READ_ONLY"},nil
}
func withPokerPrincipal(r *http.Request,p pt.Principal)*http.Request{return r.WithContext(context.WithValue(r.Context(),pokerContextKey{},p))}
