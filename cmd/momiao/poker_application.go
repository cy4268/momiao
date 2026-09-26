package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cy4268/momiao/internal/poker"
	"github.com/cy4268/momiao/internal/poker/authbridge"
	"github.com/cy4268/momiao/internal/poker/connectticket"
	pt "github.com/cy4268/momiao/internal/poker/transport"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

var errPokerStartup = errors.New("poker startup failed")

type pokerApplication struct {
	handler             *pt.Handler
	service             *poker.Service
	authPool, pokerPool *pgxpool.Pool
	tickets             *redis.Client
	native              *http.Transport
	once                sync.Once
}

func (a *pokerApplication) Close() {
	if a == nil {
		return
	}
	a.once.Do(func() {
		if a.handler != nil {
			a.handler.Close()
		}
		if a.service != nil {
			a.service.Close()
		}
		if a.tickets != nil {
			_ = a.tickets.Close()
		}
		if a.pokerPool != nil {
			a.pokerPool.Close()
		}
		if a.authPool != nil {
			a.authPool.Close()
		}
		if a.native != nil {
			a.native.CloseIdleConnections()
		}
	})
}

// Startup owns resources but never creates/migrates schema, grants privileges,
// provisions native users or generates replacement encryption/signing keys.
func openPokerApplication(ctx context.Context, cfg config) (app *pokerApplication, err error) {
	if !cfg.Poker.Enabled {
		return nil, nil
	}
	if ctx == nil {
		return nil, errPokerStartup
	}
	a := &pokerApplication{}
	defer func() {
		if err != nil {
			a.Close()
			app = nil
			err = errPokerStartup
		}
	}()
	startedAt := time.Now().UTC()
	state, err := readPokerStateKeys(cfg.Poker.StateKeyringFile)
	if err != nil {
		return nil, err
	}
	var signing pokerTicketKeys
	if cfg.ProcessRole == "poker" { signing.public, err = readPokerPublicKeys(cfg.Poker.TicketKeyringFile) } else { signing, err = readPokerTicketKeys(cfg.Poker.TicketKeyringFile) }
	if err != nil { return nil, err }
	readerKey, err := readPokerReaderKey(cfg.Poker.ReaderKeyFile)
	if err != nil {
		return nil, err
	}
	leaseConfig, ticketOptions, err := readPokerRedisConfig(cfg.Poker.RedisConfigFile)
	if err != nil {
		return nil, err
	}
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	a.authPool, err = openPokerPool(startup, cfg.WalletDSNFile)
	if err != nil {
		return nil, err
	}
	a.pokerPool, err = openPokerPool(startup, cfg.Poker.DSNFile)
	if err != nil {
		return nil, err
	}
	if err = validatePokerPools(startup, a.authPool, a.pokerPool); err != nil {
		return nil, err
	}
	if cfg.ProcessRole=="poker"{if err=validatePokerAuthorityReader(startup,a.authPool);err!=nil{return nil,err}}
	var password *poker.PasswordRuntime
	if cfg.Poker.Password != nil {
		password, err = poker.NewPasswordRuntime(cfg.Poker.Password.Policy)
		if err != nil {
			return nil, err
		}
	}
	if err = poker.CheckPasswordProfiles(startup, a.pokerPool, password); err != nil {
		return nil, err
	}
	a.native = newNativeTransport(cfg.NewAPISocket)
	reader, err := authbridge.NewNativeReader(a.native, readerKey)
	if err != nil {
		return nil, err
	}
	authority, err := authbridge.New(a.authPool, reader.Check)
	if err != nil {
		return nil, err
	}
	live := pokerLiveSession(authority.Check)
	a.tickets = redis.NewClient(ticketOptions)
	pingCtx, pingCancel := context.WithTimeout(startup, 2*time.Second)
	err = a.tickets.Ping(pingCtx).Err()
	pingCancel()
	if err != nil {
		return nil, err
	}
	consume, err := connectticket.RedisConsumer(a.tickets)
	if err != nil {
		return nil, err
	}
	var issuer *connectticket.Issuer
	if cfg.ProcessRole != "poker" {
		issuer, err = connectticket.NewIssuer(connectticket.IssuerOptions{KeyID: signing.active, PrivateKey: signing.private, CheckSession: authority.Check})
		if err != nil { return nil, err }
	}
	verifier, err := connectticket.NewVerifier(connectticket.VerifierOptions{PublicKeys: signing.public, StartedAt: startedAt, CheckSession: authority.Check, Consume: consume})
	if err != nil {
		return nil, err
	}
	var target atomic.Pointer[pt.Handler]
	serviceOptions := poker.Options{EconomyObserver:cfg.economicObserver,
		Pool: a.pokerPool, Keyring: state,
		ValidateSession: func(ctx context.Context, s poker.AuthSession) error {
			return live(ctx, pt.Principal{UserID: s.UserID, SessionIDHash: s.SessionIDHash, SessionVersion: s.SessionVersion, SecurityEpoch: s.SecurityEpoch})
		},
		AfterCommit: func(table string, version uint64) {
			if h := target.Load(); h != nil {
				h.NotifyCommitted(table, version)
			}
		},
		AfterControlChanged: func(table, connection string) {
			if h := target.Load(); h != nil {
				h.NotifyControlChanged(pt.ConnectionRef{ID: connection, TableID: table})
			}
		},
		AfterSpectatorRemoved: func(table string, user int64) {
			if h := target.Load(); h != nil {
				h.CancelTableUser(table, user)
			}
		},
	}
	if password != nil {
		passwordConfig := cfg.Poker.Password
		serviceOptions.Password = password
		serviceOptions.PasswordLimits = poker.PasswordLimits{OwnerAttempts: passwordConfig.OwnerAttempts, OwnerWindow: passwordConfig.OwnerWindow, OwnerTableAttempts: passwordConfig.OwnerTableAttempts, OwnerTableWindow: passwordConfig.OwnerTableWindow}
		serviceOptions.SessionDeadline = func(ctx context.Context, s poker.AuthSession) (time.Time, error) {
			return authority.CheckUntil(ctx, pokerSession(pt.Principal{UserID: s.UserID, SessionIDHash: s.SessionIDHash, SessionVersion: s.SessionVersion, SecurityEpoch: s.SecurityEpoch}))
		}
	}
	a.service, err = poker.NewWithRedis(startup, serviceOptions, leaseConfig)
	if err != nil {
		return nil, err
	}
	adapter := &pokerAdapter{service: a.service, wakeControl: func(ref pt.ConnectionRef) {
		if h := target.Load(); h != nil {
			h.NotifyControlChanged(ref)
		}
	}}
	ports := adapter.ports()
	ports.MintTicket = pokerTicketMint(issuer)
	httpAuth := newPokerHTTPAuth(a.native, authority.Bind)
	if cfg.ProcessRole == "poker" { httpAuth = pokerAssertedHTTPAuth; ports.MintTicket=nil }
	a.handler, err = pt.New(pt.Options{Origin: cfg.PublicOrigin, AuthHTTP: httpAuth, AuthenticateTicket: pokerTicketAuthentication(verifier), ValidateSession: live, Connected: adapter.connected, Disconnected: adapter.disconnected, AuthorizeControl: adapter.authorize, Snapshot: adapter.snapshot, Ports: ports})
	if err != nil {
		return nil, err
	}
	target.Store(a.handler)
	return a, nil
}

func openPokerPool(ctx context.Context, path string) (*pgxpool.Pool, error) {
	raw, err := readPokerPrivateFile(path, 8192)
	if err != nil {
		return nil, err
	}
	dsn := strings.TrimSpace(string(raw))
	if dsn == "" {
		return nil, errPokerConfig
	}
	c, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errPokerConfig
	}
	if !validPokerEndpoint(c) {
		return nil, errPokerConfig
	}
	c.MaxConns = 4
	c.MinConns = 0
	c.ConnConfig.ConnectTimeout = 2 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return nil, errPokerConfig
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errPokerStartup
	}
	return pool, nil
}

func validPokerEndpoint(c *pgxpool.Config) bool {
	// This bounded composition intentionally supports one PG endpoint only.
	// Reject every pgx fallback, including sslmode=prefer/allow and multihost,
	// instead of permitting an unreviewed plaintext or cross-instance path.
	if c == nil || c.ConnConfig == nil || len(c.ConnConfig.Fallbacks) != 0 {
		return false
	}
	host := c.ConnConfig.Host
	local := host == "localhost" || strings.HasPrefix(host, "/")
	if ip := net.ParseIP(host); ip != nil {
		local = ip.IsLoopback()
	}
	if !local && (c.ConnConfig.TLSConfig == nil || c.ConnConfig.TLSConfig.InsecureSkipVerify) {
		return false
	}
	return true
}

func samePokerEndpoint(a, b *pgxpool.Config) bool {
	return validPokerEndpoint(a) && validPokerEndpoint(b) && a.ConnConfig.Host == b.ConnConfig.Host && a.ConnConfig.Port == b.ConnConfig.Port && a.ConnConfig.Database == b.ConnConfig.Database
}

func validatePokerAuthorityReader(ctx context.Context,pool *pgxpool.Pool)error{
	var isolated bool
	err:=pool.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname=current_user AND NOT rolinherit)
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members m JOIN pg_catalog.pg_roles r ON r.rolname=current_user WHERE m.member=r.oid OR m.roleid=r.oid)
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname IN('identity','economy','poker','games','platform_meta','ops','content','catalog','rewards','rankings','audit') AND c.relkind IN('r','p','v','m','f')
 AND (pg_catalog.has_any_column_privilege(current_user,c.oid,'INSERT,UPDATE,REFERENCES') OR pg_catalog.has_table_privilege(current_user,c.oid,'DELETE,TRUNCATE,TRIGGER')
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_attribute a WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped AND pg_catalog.has_column_privilege(current_user,c.oid,a.attnum,'SELECT')
 AND NOT(n.nspname='identity' AND ((c.relname='account_refs' AND a.attname IN('newapi_user_id','security_epoch','security_epoch_changed_at'))
 OR(c.relname='native_session_bindings' AND a.attname IN('session_id_hash','newapi_user_id','session_version','native_auth_version','security_epoch_snapshot','native_created_at')))))))
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname IN('identity','economy','poker','games','platform_meta','ops','content','catalog','rewards','rankings','audit') AND p.prorettype<>'trigger'::regtype AND pg_catalog.has_schema_privilege(current_user,n.oid,'USAGE') AND pg_catalog.has_function_privilege(current_user,p.oid,'EXECUTE'))`).Scan(&isolated)
	if err!=nil||!isolated{return errPokerConfig};return nil
}

func validatePokerPools(ctx context.Context, auth, domain *pgxpool.Pool) error {
	if auth == nil || domain == nil || !samePokerEndpoint(auth.Config(), domain.Config()) {
		return errPokerConfig
	}
	identity := func(pool *pgxpool.Pool) (string, string, error) {
		var role, database string
		var isolated bool
		err := pool.QueryRow(ctx, `SELECT current_user,json_build_array(current_database(),d.oid,inet_server_addr(),inet_server_port(),pg_postmaster_start_time())::text,session_user=current_user AND current_user=$1::name
 AND r.rolcanlogin AND NOT r.rolsuper AND NOT r.rolcreatedb AND NOT r.rolcreaterole AND NOT r.rolreplication AND NOT r.rolbypassrls
 AND NOT pg_catalog.pg_has_role(current_user,d.datdba,'MEMBER')
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_namespace n WHERE n.nspname IN('identity','economy','poker','games','platform_meta','ops','content','catalog','rewards','rankings','audit') AND pg_catalog.pg_has_role(current_user,n.nspowner,'MEMBER'))
 FROM pg_catalog.pg_roles r JOIN pg_catalog.pg_database d ON d.datname=current_database() WHERE r.rolname=current_user`, pool.Config().ConnConfig.User).Scan(&role, &database, &isolated)
		if err != nil || !isolated {
			return "", "", errPokerConfig
		}
		return role, database, nil
	}
	platformRole, platformDB, err := identity(auth)
	if err != nil {
		return err
	}
	pokerRole, pokerDB, err := identity(domain)
	if err != nil {
		return err
	}
	if platformDB != pokerDB || platformRole == pokerRole {
		return errPokerConfig
	}
	var inherited, directEconomy, gateways bool
	err = domain.QueryRow(ctx, `SELECT pg_catalog.pg_has_role(current_user,$1::name,'MEMBER'),
 EXISTS(SELECT 1 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='economy' AND c.relkind IN('r','p','v','m','f') AND (pg_catalog.has_any_column_privilege(current_user,c.oid,'SELECT,INSERT,UPDATE,REFERENCES') OR pg_catalog.has_table_privilege(current_user,c.oid,'DELETE,TRUNCATE,TRIGGER'))),
 pg_catalog.has_function_privilege(current_user,'economy.poker_buy_in_apply(uuid,bigint,uuid)','EXECUTE') AND pg_catalog.has_function_privilege(current_user,'economy.poker_top_up_apply(uuid,bigint,uuid)','EXECUTE') AND pg_catalog.has_function_privilege(current_user,'economy.poker_cash_out_apply(uuid,bigint,uuid)','EXECUTE')`, platformRole).Scan(&inherited, &directEconomy, &gateways)
	if err != nil || inherited || directEconomy || !gateways {
		return errPokerConfig
	}
	// Check the required live-auth schema/reads without inventing an account or
	// storing a fabricated authentication chain merely to pass startup readiness.
	if _, err = auth.Exec(ctx, `SELECT session_id_hash,newapi_user_id,session_version,native_auth_version,security_epoch_snapshot,native_created_at FROM identity.native_session_bindings WHERE false`); err != nil {
		return errPokerConfig
	}
	if _, err = auth.Exec(ctx, `SELECT newapi_user_id,security_epoch,security_epoch_changed_at FROM identity.account_refs WHERE false`); err != nil {
		return errPokerConfig
	}
	return nil
}
