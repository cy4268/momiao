package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/cy4268/momiao/internal/bffauth"
	"github.com/cy4268/momiao/internal/session"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

var errSessionStartup = errors.New("session startup failed")

// sessionStartupStage is deliberately a closed set of non-secret diagnostics.
// Its value may identify only the failed constructor boundary; it must never
// include an underlying error, endpoint, path, role, credential or key.
type sessionStartupStage string

const (
	sessionStageUnknown            sessionStartupStage = "UNKNOWN"
	sessionStageContext            sessionStartupStage = "CONTEXT"
	sessionStageReaderKey          sessionStartupStage = "READER_KEY"
	sessionStageOpsKey             sessionStartupStage = "OPS_KEY"
	sessionStageSessionSealKey     sessionStartupStage = "SESSION_SEAL_KEY"
	sessionStageCredentialSealKey  sessionStartupStage = "CREDENTIAL_SEAL_KEY"
	sessionStageKeyDistinctness    sessionStartupStage = "KEY_DISTINCTNESS"
	sessionStagePGDSNFile          sessionStartupStage = "PG_DSN_FILE"
	sessionStagePGDSNParse         sessionStartupStage = "PG_DSN_PARSE"
	sessionStagePGPoolCreate       sessionStartupStage = "PG_POOL_CREATE"
	sessionStagePGPing             sessionStartupStage = "PG_PING"
	sessionStagePGPermissions      sessionStartupStage = "PG_PERMISSIONS"
	sessionStageRedisConfig        sessionStartupStage = "REDIS_CONFIG"
	sessionStageRedisPing          sessionStartupStage = "REDIS_PING"
	sessionStageSessionConstructor sessionStartupStage = "SESSION_CONSTRUCTOR"
	sessionStageBFFAuthConstructor sessionStartupStage = "BFFAUTH_CONSTRUCTOR"
)

type sessionStartupFailure struct{ stage sessionStartupStage }

// Error intentionally preserves the existing public error. Recovery-only code
// can extract the constant stage with sessionStartupFailureStage.
func (*sessionStartupFailure) Error() string { return errSessionStartup.Error() }

func newSessionStartupFailure(stage sessionStartupStage) error {
	return &sessionStartupFailure{stage: stage}
}

func sessionStartupFailureStage(err error) sessionStartupStage {
	var failure *sessionStartupFailure
	if errors.As(err, &failure) {
		return failure.stage
	}
	return sessionStageUnknown
}

func sanitizeSessionStartupFailure(err error, fallback sessionStartupStage) error {
	if sessionStartupFailureStage(err) != sessionStageUnknown {
		return err
	}
	return newSessionStartupFailure(fallback)
}

type sessionApplication struct {
	auth     *bffauth.Service
	sessions *session.Service
	pool     *pgxpool.Pool
	redis    *redis.Client
	native   interface{ CloseIdleConnections() }
	once     sync.Once
}

func (a *sessionApplication) Close() {
	if a == nil {
		return
	}
	a.once.Do(func() {
		if a.native != nil {
			a.native.CloseIdleConnections()
		}
		if a.sessions != nil {
			a.sessions.Close()
		}
		if a.redis != nil {
			_ = a.redis.Close()
		}
		if a.pool != nil {
			a.pool.Close()
		}
	})
}

func openSessionApplication(ctx context.Context, cfg config, controls ...bffauth.ControlRevoker) (app *sessionApplication, err error) {
	if !cfg.Session.Enabled {
		return nil, nil
	}
	if ctx == nil {
		return nil, newSessionStartupFailure(sessionStageContext)
	}
	a := &sessionApplication{}
	stage := sessionStageUnknown
	defer func() {
		if err != nil {
			a.Close()
			app, err = nil, sanitizeSessionStartupFailure(err, stage)
		}
	}()
	stage = sessionStageReaderKey
	readerKey, err := readPokerReaderKey(cfg.Session.ReaderKeyFile)
	if err != nil {
		return nil, err
	}
	stage = sessionStageOpsKey
	opsKey, err := readPokerReaderKey(cfg.Session.OpsKeyFile)
	if err != nil {
		return nil, err
	}
	stage = sessionStageSessionSealKey
	sessionKey, err := readSessionKey(cfg.Session.SessionSealKeyFile)
	if err != nil {
		return nil, err
	}
	stage = sessionStageCredentialSealKey
	credentialKey, err := readSessionKey(cfg.Session.CredentialSealKeyFile)
	if err != nil {
		return nil, err
	}
	readerBytes, _ := hex.DecodeString(readerKey)
	opsBytes, _ := hex.DecodeString(opsKey)
	stage = sessionStageKeyDistinctness
	if readerKey == opsKey || bytes.Equal(sessionKey[:], credentialKey[:]) || bytes.Equal(sessionKey[:], readerBytes) || bytes.Equal(sessionKey[:], opsBytes) || bytes.Equal(credentialKey[:], readerBytes) || bytes.Equal(credentialKey[:], opsBytes) {
		return nil, errSessionConfig
	}
	startup, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	stage = sessionStagePGPoolCreate
	a.pool, err = openSessionPool(startup, cfg.Session.DSNFile)
	if err != nil {
		return nil, err
	}
	stage = sessionStageRedisConfig
	redisOptions, err := readSessionRedisConfig(cfg.Session.RedisConfigFile)
	if err != nil {
		return nil, err
	}
	a.redis = redis.NewClient(redisOptions)
	stage = sessionStageRedisPing
	ping, stopPing := context.WithTimeout(startup, 2*time.Second)
	err = a.redis.Ping(ping).Err()
	stopPing()
	if err != nil {
		return nil, err
	}
	stage = sessionStageSessionConstructor
	a.sessions, err = session.New(session.Options{
		Redis: a.redis, Pool: a.pool, NativeSocket: cfg.NewAPISocket,
		NativeReaderKey: readerKey, OpsSocket: cfg.Session.OpsSocket, OpsKey: opsKey,
		SessionSealKey: sessionKey, Origin: cfg.PublicOrigin, Environment: cfg.Session.Environment,
		Lifetime: session.Lifetime{Idle: 7 * 24 * time.Hour, Absolute: 30 * 24 * time.Hour, Touch: 5 * time.Minute},
	})
	if err != nil {
		return nil, err
	}
	native := newNativeTransport(cfg.NewAPISocket)
	a.native = native
	var control bffauth.ControlRevoker
	if len(controls) > 0 {
		control = controls[0]
	}
	stage = sessionStageBFFAuthConstructor
	a.auth, err = bffauth.New(bffauth.Options{Redis: a.redis, Sessions: a.sessions, Native: native, Origin: cfg.PublicOrigin, CredentialSealKey: credentialKey, Control: control})
	if err != nil {
		return nil, err
	}
	return a, nil
}

func openSessionPool(ctx context.Context, path string) (*pgxpool.Pool, error) {
	raw, err := readPokerPrivateFile(path, 8192)
	if err != nil {
		return nil, newSessionStartupFailure(sessionStagePGDSNFile)
	}
	config, err := pgxpool.ParseConfig(strings.TrimSpace(string(raw)))
	if err != nil || !validPokerEndpoint(config) {
		return nil, newSessionStartupFailure(sessionStagePGDSNParse)
	}
	config.MaxConns, config.MinConns, config.ConnConfig.ConnectTimeout = 4, 0, 2*time.Second
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, newSessionStartupFailure(sessionStagePGPoolCreate)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, newSessionStartupFailure(sessionStagePGPing)
	}
	if err = validateSessionPool(ctx, pool); err != nil {
		pool.Close()
		return nil, newSessionStartupFailure(sessionStagePGPermissions)
	}
	return pool, nil
}

func validateSessionPool(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errSessionConfig
	}
	var valid bool
	err := pool.QueryRow(ctx, sessionPoolPermissionsSQL, pool.Config().ConnConfig.User).Scan(&valid)
	if err != nil || !valid {
		return errSessionConfig
	}
	return nil
}

const sessionPoolPermissionsSQL = `SELECT session_user=current_user AND current_user=$1::name
 AND r.rolcanlogin AND NOT r.rolinherit AND NOT r.rolsuper AND NOT r.rolcreatedb AND NOT r.rolcreaterole AND NOT r.rolreplication AND NOT r.rolbypassrls
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members m WHERE m.member=r.oid)
 AND NOT pg_catalog.pg_has_role(r.oid,d.datdba,'MEMBER')
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_namespace n WHERE n.nspname IN('identity','economy','poker','games','platform_meta','ops','content','catalog','rewards') AND pg_catalog.pg_has_role(r.oid,n.nspowner,'MEMBER'))
 AND pg_catalog.has_schema_privilege(current_user,'identity','USAGE')
 AND pg_catalog.has_schema_privilege(current_user,'ops','USAGE')
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_namespace n
   WHERE n.nspname IN('identity','economy','poker','games','platform_meta','ops','content','catalog','rewards')
   AND pg_catalog.has_schema_privilege(current_user,n.oid,'CREATE'))
 AND NOT EXISTS(
   SELECT 1 FROM pg_catalog.pg_attribute a JOIN pg_catalog.pg_class c ON c.oid=a.attrelid
   JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
   WHERE n.nspname IN('identity','economy','poker','games','platform_meta','ops','content','catalog','rewards')
   AND c.relkind IN('r','p','v','m','f') AND a.attnum>0 AND NOT a.attisdropped
   AND pg_catalog.has_column_privilege(current_user,c.oid,a.attnum,'SELECT')
   AND NOT ((n.nspname='identity' AND (
     (c.relname='account_refs' AND a.attname IN('newapi_user_id','security_epoch','security_epoch_changed_at')) OR
     (c.relname='native_session_bindings' AND a.attname IN('session_id_hash','newapi_user_id','session_version','native_auth_version','security_epoch_snapshot','native_created_at')))) OR
     (n.nspname='ops' AND c.relname='admin_principals' AND a.attname IN('authz_epoch','newapi_user_id','status'))))
 AND NOT EXISTS(
   SELECT 1 FROM (VALUES ('newapi_user_id'),('status'),('authz_epoch')) AS required(col)
   WHERE NOT pg_catalog.has_column_privilege(current_user,'ops.admin_principals',required.col,'SELECT'))
 AND pg_catalog.has_column_privilege(current_user,'identity.account_refs','newapi_user_id','SELECT')
 AND pg_catalog.has_column_privilege(current_user,'identity.account_refs','security_epoch','SELECT')
 AND pg_catalog.has_column_privilege(current_user,'identity.account_refs','security_epoch_changed_at','SELECT')
 AND NOT EXISTS(
   SELECT 1 FROM (VALUES ('session_id_hash'),('newapi_user_id'),('session_version'),('native_auth_version'),('security_epoch_snapshot'),('native_created_at')) AS required(col)
   CROSS JOIN (VALUES ('SELECT'),('INSERT')) AS permission(priv)
   WHERE NOT pg_catalog.has_column_privilege(current_user,'identity.native_session_bindings',required.col,permission.priv))
 AND pg_catalog.has_column_privilege(current_user,'identity.native_session_bindings','session_version','UPDATE')
 AND pg_catalog.has_column_privilege(current_user,'identity.native_session_bindings','native_auth_version','UPDATE')
 AND NOT EXISTS(
   SELECT 1 FROM pg_catalog.pg_attribute a
   WHERE a.attrelid='identity.native_session_bindings'::regclass AND a.attnum>0 AND NOT a.attisdropped
   AND (pg_catalog.has_column_privilege(current_user,a.attrelid,a.attnum,'REFERENCES')
     OR (a.attname NOT IN('session_version','native_auth_version') AND pg_catalog.has_column_privilege(current_user,a.attrelid,a.attnum,'UPDATE'))
     OR (a.attname NOT IN('session_id_hash','newapi_user_id','session_version','native_auth_version','security_epoch_snapshot','native_created_at') AND pg_catalog.has_column_privilege(current_user,a.attrelid,a.attnum,'INSERT'))))
 AND NOT pg_catalog.has_any_column_privilege(current_user,'identity.account_refs','INSERT,UPDATE,REFERENCES')
 AND NOT pg_catalog.has_table_privilege(current_user,'identity.native_session_bindings','DELETE,TRUNCATE,TRIGGER')
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname IN('identity','economy','poker','games','platform_meta','ops','content','catalog','rewards') AND c.oid<>'identity.native_session_bindings'::regclass AND c.relkind IN('r','p','v','m','f') AND (pg_catalog.has_any_column_privilege(current_user,c.oid,'INSERT,UPDATE,REFERENCES') OR pg_catalog.has_table_privilege(current_user,c.oid,'DELETE,TRUNCATE,TRIGGER')))
 FROM pg_catalog.pg_roles r JOIN pg_catalog.pg_database d ON d.datname=current_database() WHERE r.rolname=current_user`
