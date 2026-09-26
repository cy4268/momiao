package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/poker/authbridge"
	"github.com/cy4268/momiao/internal/poker/connectticket"
	pt "github.com/cy4268/momiao/internal/poker/transport"
	"github.com/cy4268/momiao/internal/session"
	"github.com/jackc/pgx/v5/pgxpool"
)

const pokerInternalPrefix = "/internal/v1/poker/"
var pokerSessionHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// A separate process is the deployment composition. Embedded mode remains for
// existing callers; remote mode never opens a second Poker service or lease pool.
func loadPokerProcessConfig(cfg *config, lookup func(string) (string, bool)) error {
	readSocket, hasRead := lookup("MOMIAO_ECONOMY_READ_SOCKET")
 remote, hasRemote := lookup("MOMIAO_POKER_REMOTE_SOCKET")
	key, hasKey := lookup("MOMIAO_POKER_SERVICE_KEYRING_FILE")
	peer,hasPeer:=lookup("MOMIAO_POKER_PEER_KEYRING_FILE")
	if !hasRead && !hasRemote && !hasKey && !hasPeer && cfg.ProcessRole != "poker" { return nil }
	if !hasKey || !filepath.IsAbs(key) || !hasPeer || !filepath.IsAbs(peer) || pokerPathKey(key)==pokerPathKey(peer) { return errPokerConfig }
	if !hasRead || !filepath.IsAbs(readSocket) || pokerPathKey(readSocket)==pokerPathKey(cfg.ListenSocket) || pokerPathKey(readSocket)==pokerPathKey(cfg.NewAPISocket) || (cfg.RefillSocket!="" && pokerPathKey(readSocket)==pokerPathKey(cfg.RefillSocket)) || (hasRemote && pokerPathKey(readSocket)==pokerPathKey(remote)) { return errPokerConfig }
 cfg.EconomyReadSocket=filepath.Clean(readSocket)
 cfg.PokerServiceKeyringFile = filepath.Clean(key)
	cfg.PokerPeerKeyringFile = filepath.Clean(peer)
	if cfg.ProcessRole == "poker" {
		if hasRemote || cfg.ListenSocket == "" || cfg.ListenSocket == cfg.NewAPISocket { return errPokerConfig }
	} else {
		if !hasRemote || !filepath.IsAbs(remote) || filepath.Clean(remote) == cfg.NewAPISocket || filepath.Clean(remote) == cfg.ListenSocket { return errPokerConfig }
		cfg.PokerRemoteSocket = filepath.Clean(remote)
	}
	return nil
}

func runPokerProcess(ctx context.Context, cfg config, logger *log.Logger) error {
	keys, err := readPokerPublicKeys(cfg.PokerPeerKeyringFile)
	if err != nil { return errPokerStartup }
	// Each process owns its own signer and only the peer's public keys. No
	// reverse business request is fabricated merely to exercise that key.
	signer,err:=readPokerTicketKeys(cfg.PokerServiceKeyringFile)
	if err!=nil||!distinctPokerPublicKeys(keys,signer.public){return errPokerStartup}
	ticketKeys,err:=readPokerPublicKeys(cfg.Poker.TicketKeyringFile)
	if err!=nil||!distinctPokerPublicKeys(keys,ticketKeys)||!distinctPokerPublicKeys(signer.public,ticketKeys){return errPokerStartup}
	transport := newNativeTransport(cfg.EconomyReadSocket)
 defer transport.CloseIdleConnections()
 cfg.economicObserver,err=newEconomyQuotaObserver(transport,signer)
 if err!=nil{return errPokerStartup}
 app, err := openPokerApplication(ctx, cfg)
	if err != nil || app == nil { return errPokerStartup }
	defer app.Close()
	opsRPC:=newPokerOpsRPCHandler(app.service)
	checks := map[string]readinessCheck{
		"poker_database": app.pokerPool.Ping,
		"poker_auth_database": app.authPool.Ping,
		"poker_cache": func(ctx context.Context) error { return app.tickets.Ping(ctx).Err() },
		"native": nativeReadiness(app.native),
	}
	listener, err := openListener(cfg)
	if err != nil { return err }
	defer listener.Close()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Public realtime is routed by the edge directly to this process. The
		// first frame's one-use ticket and live authority check authenticate it.
		// Ordinary REST and internal commands always require the service signature.
		if r.URL.Path=="/ws/poker" {
			if r.URL.RawPath!=""||r.URL.RawQuery!=""||r.URL.ForceQuery||r.URL.Fragment!=""||r.ContentLength>0{walletError(w,400,"POKER_INVALID_REQUEST");return}
			app.handler.ServeHTTP(w,r);return
		}
		var raw []byte
		var readErr error
		if r.Body != nil { raw,readErr=io.ReadAll(io.LimitReader(r.Body,128*1024+1));_ = r.Body.Close() }
		if readErr!=nil || len(raw)>128*1024 || verifyPokerRequest(r,raw,keys)!=nil {
			walletError(w, http.StatusForbidden, "POKER_BRIDGE_DENIED"); return
		}
		for _,name:=range pokerAssertionHeaders{r.Header.Del(pokerAssertionPrefix+name)}
		for _,name:=range []string{"Authorization","X-Auth-Session","New-Api-User","Cookie"}{if r.Header.Get(name)!=""{walletError(w,400,"POKER_INVALID_REQUEST");return}}
		if r.URL.RawPath != "" || r.URL.ForceQuery || r.URL.Fragment != "" { walletError(w,400,"POKER_INVALID_REQUEST");return }
		if pokerAPIRoute(r.URL.Path) {
			if r.URL.Path=="/ws/poker" { if len(raw)!=0 {walletError(w,400,"POKER_INVALID_REQUEST");return};r.Body=http.NoBody;app.handler.ServeHTTP(w,r);return }
			var envelope pokerRequestEnvelope
			if decodePokerObject(raw,&envelope,"context","body")!=nil||len(envelope.Body)>64*1024{walletError(w,400,"POKER_INVALID_REQUEST");return}
			principal,err:=pokerEnvelopePrincipal(envelope.Context);if err!=nil{walletError(w,401,"POKER_AUTH_UNAUTHORIZED");return}
			r=withPokerPrincipal(r,principal);r.Body=io.NopCloser(bytes.NewReader(envelope.Body));r.ContentLength=int64(len(envelope.Body))
			app.handler.ServeHTTP(w,r);return
		}
		r.Body=io.NopCloser(bytes.NewReader(raw))
		if r.URL.RawQuery != "" { walletError(w,400,"POKER_INVALID_REQUEST");return }
		if strings.HasPrefix(r.URL.Path,pokerOpsRPCPrefix){opsRPC.ServeHTTP(w,r);return}
		switch r.URL.Path {
		case pokerInternalPrefix+"ready":
			if r.Method != http.MethodGet { walletError(w,405,"METHOD_NOT_ALLOWED");return }
			view := readReadiness(r.Context(),checks);status:=503;if view.Ready {status=200};walletJSON(w,status,view)
		case pokerInternalPrefix+"catalog":
			if r.Method != http.MethodGet { walletError(w,405,"METHOD_NOT_ALLOWED");return }
			state,err:=app.service.CatalogRuntime(r.Context());if err!=nil {walletError(w,503,"POKER_SERVICE_UNAVAILABLE");return};walletJSON(w,200,map[string]string{"state":state})
		case pokerInternalPrefix+"revoke":
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {walletError(w,400,"POKER_INVALID_REQUEST");return}
			raw,err:=io.ReadAll(io.LimitReader(r.Body,1025));var input pokerRevokeInput
			if err!=nil||len(raw)>1024||decodePokerObject(raw,&input,"user_id","native_session_id_hash")!=nil||!pokerSessionHash.MatchString(input.NativeSessionIDHash){walletError(w,400,"POKER_INVALID_REQUEST");return}
			user,err:=strconv.ParseInt(input.UserID,10,64);if err!=nil||user<=0||strconv.FormatInt(user,10)!=input.UserID {walletError(w,400,"POKER_INVALID_REQUEST");return}
			bounded,cancel:=context.WithTimeout(r.Context(),5*time.Second);defer cancel()
			err=app.service.RevokeSessionControls(bounded,user,input.NativeSessionIDHash)
			app.handler.RevokeSession(user,input.NativeSessionIDHash)
			if err!=nil {walletError(w,503,"POKER_SERVICE_UNAVAILABLE");return};walletJSON(w,200,map[string]bool{"revoked":true})
		default: walletError(w,404,"NOT_FOUND")
		}
	})
	server := &http.Server{Handler:handler, ReadHeaderTimeout:5*time.Second, IdleTimeout:60*time.Second, MaxHeaderBytes:16*1024, ErrorLog:log.New(io.Discard,"",0)}
	logger.Print("Poker process listening on private Unix socket")
	// Shutdown first drains HTTP; Close then fences the service and closes all
	// hijacked WebSockets before closing pools. A platform restart leaves this
	// process running and reconnects through the normal ticket/control protocol.
	return serve(ctx,server,listener,cfg.ShutdownTimeout)
}

type pokerRevokeInput struct {
	UserID string `json:"user_id"`
	NativeSessionIDHash string `json:"native_session_id_hash"`
}
type pokerRemote struct {
	transport *http.Transport
	native *http.Transport
	authPool *pgxpool.Pool
	signing pokerTicketKeys
	auth func(*http.Request)(pt.Principal,error)
	mint *pt.Handler
	handler http.Handler
}

func openPokerRemote(ctx context.Context,cfg config) (*pokerRemote,error) {
	signing,err:=readPokerTicketKeys(cfg.PokerServiceKeyringFile);if err!=nil{return nil,errPokerStartup}
	peer,err:=readPokerPublicKeys(cfg.PokerPeerKeyringFile);if err!=nil||!distinctPokerPublicKeys(signing.public,peer){return nil,errPokerStartup}
	ticket,err:=readPokerTicketKeys(cfg.Poker.TicketKeyringFile);if err!=nil{return nil,errPokerStartup}
	if !distinctPokerPublicKeys(signing.public,ticket.public)||!distinctPokerPublicKeys(peer,ticket.public){return nil,errPokerStartup}
	readerKey,err:=readPokerReaderKey(cfg.Poker.ReaderKeyFile);if err!=nil{return nil,errPokerStartup}
	out:=&pokerRemote{transport:newNativeTransport(cfg.PokerRemoteSocket),native:newNativeTransport(cfg.NewAPISocket),signing:signing}
	out.authPool,err=openPokerPool(ctx,cfg.WalletDSNFile);if err!=nil{out.Close();return nil,errPokerStartup}
	reader,err:=authbridge.NewNativeReader(out.native,readerKey);if err!=nil{out.Close();return nil,errPokerStartup}
	authority,err:=authbridge.New(out.authPool,reader.Check);if err!=nil{out.Close();return nil,errPokerStartup}
	out.auth=newPokerHTTPAuth(out.native,authority.Bind)
	issuer,err:=connectticket.NewIssuer(connectticket.IssuerOptions{KeyID:ticket.active,PrivateKey:ticket.private,CheckSession:authority.Check});if err!=nil{out.Close();return nil,errPokerStartup}
	// Only /connect-tickets is dispatched to this transport instance. It owns
	// the existing mint/parser/live-auth path, with no table or WS capabilities.
	out.mint,err=pt.New(pt.Options{Origin:cfg.PublicOrigin,AuthHTTP:out.auth,ValidateSession:pokerLiveSession(authority.Check),Ports:pt.Ports{MintTicket:pokerTicketMint(issuer)},AuthenticateTicket:func(context.Context,pt.ConnectRequest)(pt.Principal,error){return pt.Principal{},pokerAuthFault(connectticket.ErrUnavailable)},Snapshot:func(context.Context,pt.Principal,pt.ConnectionRef)(pt.Snapshot,error){return pt.Snapshot{},pokerAuthFault(connectticket.ErrUnavailable)}})
	if err!=nil{out.Close();return nil,errPokerStartup}
	proxy:=&httputil.ReverseProxy{
		Transport:out.transport,ErrorLog:log.New(io.Discard,"",0),
		Rewrite:func(p *httputil.ProxyRequest){
			p.Out.URL.Scheme="http";p.Out.URL.Host="unix";p.Out.Host="localhost"
			for _,name:=range []string{"Cookie","Authorization","X-Auth-Session","New-Api-User","Forwarded","X-Forwarded-For","X-Forwarded-Host","X-Forwarded-Proto","X-Real-IP","CF-Connecting-IP","True-Client-IP"}{p.Out.Header.Del(name)}
		},
		ModifyResponse:func(r *http.Response)error{r.Header.Del("Set-Cookie");for _,name:=range pokerAssertionHeaders{r.Header.Del(pokerAssertionPrefix+name)};return nil},
		ErrorHandler:func(w http.ResponseWriter,_ *http.Request,_ error){walletError(w,503,"POKER_SERVICE_UNAVAILABLE")},
	}
	out.handler=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		if !pokerAPIRoute(r.URL.Path)||r.URL.RawPath!=""{walletError(w,404,"NOT_FOUND");return}
		if r.URL.Path=="/api/v1/poker/connect-tickets"{out.mint.ServeHTTP(w,r);return}
		var raw []byte
		if r.Body!=nil {var err error;raw,err=io.ReadAll(io.LimitReader(r.Body,64*1024+1));_ = r.Body.Close();if err!=nil||len(raw)>64*1024{walletError(w,400,"POKER_INVALID_REQUEST");return}}
		if r.URL.Path!="/ws/poker"{
			principal,err:=out.auth(r);if err!=nil{var f *pt.Fault;if errors.As(err,&f){walletError(w,f.Status,f.Code)}else{walletError(w,401,"POKER_AUTH_UNAUTHORIZED")};return}
			envelope:=pokerRequestEnvelope{Context:pokerRequestContext{strconv.FormatInt(principal.UserID,10),principal.SessionIDHash,strconv.FormatUint(principal.SessionVersion,10),strconv.FormatUint(principal.SecurityEpoch,10)},Body:raw}
			raw,err=json.Marshal(envelope);if err!=nil{walletError(w,503,"POKER_SERVICE_UNAVAILABLE");return}
		}else if len(raw)!=0{walletError(w,400,"POKER_INVALID_REQUEST");return}
		r=r.Clone(r.Context());r.Body=io.NopCloser(bytes.NewReader(raw));r.ContentLength=int64(len(raw));r.GetBody=nil
		if signPokerRequest(r,raw,out.signing)!=nil{walletError(w,503,"POKER_SERVICE_UNAVAILABLE");return}
		proxy.ServeHTTP(w,r)
	})
	return out,nil
}
func(p *pokerRemote)Close(){if p==nil{return};if p.mint!=nil{p.mint.Close()};if p.authPool!=nil{p.authPool.Close()};if p.native!=nil{p.native.CloseIdleConnections()};if p.transport!=nil{p.transport.CloseIdleConnections()}}
func(p *pokerRemote)call(ctx context.Context,method,path string,input,output any)error{
	if p==nil||p.transport==nil{return errPokerStartup}
	var raw []byte
	if input!=nil{var err error;raw,err=json.Marshal(input);if err!=nil{return errPokerStartup}}
	bounded,cancel:=context.WithTimeout(ctx,6*time.Second);defer cancel()
	r,err:=http.NewRequestWithContext(bounded,method,"http://unix"+pokerInternalPrefix+path,bytes.NewReader(raw));if err!=nil{return errPokerStartup}
	r.Host="localhost";r.Header.Set("Accept","application/json");if input!=nil{r.Header.Set("Content-Type","application/json")};if signPokerRequest(r,raw,p.signing)!=nil{return errPokerStartup}
	response,err:=p.transport.RoundTrip(r);if err!=nil||response==nil{if response!=nil&&response.Body!=nil{_ = response.Body.Close()};return errPokerStartup}
	if response.Body==nil{return errPokerStartup};defer response.Body.Close()
	raw,err=io.ReadAll(io.LimitReader(response.Body,64*1024+1));if err!=nil||len(raw)>64*1024||response.StatusCode!=200{return errPokerStartup}
	if output!=nil&&json.Unmarshal(raw,output)!=nil{return errPokerStartup};return nil
}
func(p *pokerRemote)catalogRuntime(ctx context.Context)(string,error){
	var out struct{State string `json:"state"`};if err:=p.call(ctx,http.MethodGet,"catalog",nil,&out);err!=nil{return "TEMPORARILY_UNAVAILABLE",err}
	switch out.State{case "PLAY","MAINTENANCE","TEMPORARILY_UNAVAILABLE":return out.State,nil;default:return "TEMPORARILY_UNAVAILABLE",errPokerStartup}
}
func(p *pokerRemote)ready(ctx context.Context)error{
	var out readinessView;if err:=p.call(ctx,http.MethodGet,"ready",nil,&out);err!=nil{return err};if !out.Ready||len(out.Components)==0{return errors.New("Poker unavailable")};return nil
}
func(p *pokerRemote)revoke(ctx context.Context,binding session.BFFBinding)error{
	var out struct{Revoked bool `json:"revoked"`};err:=p.call(ctx,http.MethodPost,"revoke",pokerRevokeInput{binding.UserID,binding.NativeSessionIDHash},&out)
	if err!=nil||!out.Revoked{return errPokerStartup};return nil
}
