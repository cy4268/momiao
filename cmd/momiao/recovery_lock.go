package main

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/redis/go-redis/v9"
)

// Deployment state, never reconstructed from product maintenance tables. An
// isolated restore controller must supply REQUIRED=ON as well as LOCK=ON. The
// default remains unchanged for ordinary deployments; unknown values never
// silently turn the lock off. There is deliberately no HTTP unlock operation.
func loadRecoveryConfig(cfg *config, lookup func(string) (string, bool)) error {
	values := make(map[string]bool, 3)
	for _, key := range []string{"DR_RECOVERY_LOCK", "DR_RECOVERY_REQUIRED", "DR_RECOVERY_AUTH_PROBES"} {
		value, set := lookup(key)
		if !set {
			continue
		}
		if value != "ON" && value != "OFF" {
			return errors.New(key + " must be ON or OFF")
		}
		values[key] = value == "ON"
	}
	if values["DR_RECOVERY_REQUIRED"] && !values["DR_RECOVERY_LOCK"] {
		return errors.New("DR_RECOVERY_REQUIRED requires DR_RECOVERY_LOCK=ON")
	}
	if values["DR_RECOVERY_AUTH_PROBES"] && !values["DR_RECOVERY_LOCK"] {
		return errors.New("DR_RECOVERY_AUTH_PROBES requires DR_RECOVERY_LOCK=ON")
	}
	cfg.RecoveryLock, cfg.RecoveryAuthProbes = values["DR_RECOVERY_LOCK"], values["DR_RECOVERY_AUTH_PROBES"]
	return nil
}

type recoveryStatus struct {
	ContractVersion        int                  `json:"contract_version"`
	Lock                   string               `json:"lock"`
	BusinessLocked         bool                 `json:"business_locked"`
	BusinessWorkersStarted bool                 `json:"business_workers_started"`
	Ready                  bool                 `json:"ready"`
	DependencyReady        bool                 `json:"dependency_ready"`
	AuthProbes             bool                 `json:"auth_probes"`
	Components             []readinessComponent `json:"components"`
}

// This is an allowlist, not a write-method filter: GETs, WebSockets, internal
// RPCs, unknown future routes and proxy endpoints can all have business effects.
// No request is ever forwarded to the ordinary portal/Poker/refill handlers.
func newRecoveryLockedHandler(cfg config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		canonical := r.URL.RawPath == "" && r.URL.RawQuery == "" && !r.URL.ForceQuery && r.URL.Fragment == "" && r.ContentLength == 0 && len(r.TransferEncoding) == 0 && r.Header.Get("Upgrade") == ""
		statusPath := r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/internal/v1/recovery/status"
		if canonical && statusPath && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			dependency := readReadiness(r.Context(), cfg.readiness)
			status := http.StatusOK
			if r.URL.Path == "/readyz" {
				status = http.StatusServiceUnavailable
			}
			if r.Method == http.MethodHead {
				w.WriteHeader(status)
				return
			}
			walletJSON(w, status, recoveryStatus{ContractVersion: 1, Lock: "ON", BusinessLocked: true, Ready: false, BusinessWorkersStarted: false, DependencyReady: dependency.Ready, AuthProbes: cfg.RecoveryAuthProbes, Components: dependency.Components})
			return
		}
		if canonical && cfg.RecoveryAuthProbes && cfg.ProcessRole == "platform" && cfg.Session.Enabled && r.Method == http.MethodGet && (r.URL.Path == "/api/v1/session/bootstrap" || r.URL.Path == "/api/v1/account") {
			// Explicitly permitted authentication-state changes include anonymous
			// Redis sessions, session/credential touches and live Native checks.
			// No registration, password login, OAuth or account mutation is allowed.
			serveSessionRoute(cfg.sessionAuth, w, r)
			return
		}
		walletError(w, http.StatusServiceUnavailable, "DR_RECOVERY_LOCKED")
	})
}

// Keep this composition separate from all business constructors. Several of
// those start a worker immediately (history, Poker actor recovery); wrapping
// only HTTP or cancelling a worker after starting it is too late. Dependency
// PING/role validation is not an admission test, key-decryption test or a full
// stack restore. The controller must validate those separately before unlock.
func runRecoveryLockedProcess(ctx context.Context, cfg config, logger *log.Logger) error {
	if !cfg.RecoveryLock || (cfg.RecoveryAuthProbes && (!cfg.Session.Enabled || cfg.ProcessRole != "platform")) {
		return errors.New("invalid DR_RECOVERY_LOCK runtime configuration")
	}
	cfg.readiness = make(map[string]readinessCheck)
	if cfg.WebDir != "" {
		cfg.readiness["web"] = webReadiness(cfg.WebDir)
	}
	if cfg.NewAPISocket != "" {
		transport := newNativeTransport(cfg.NewAPISocket)
		defer transport.CloseIdleConnections()
		cfg.readiness["native"] = nativeReadiness(transport)
	}
	if cfg.WalletDSNFile != "" {
		pool, err := openPokerPool(ctx, cfg.WalletDSNFile)
		if err != nil {
			return errors.New("recovery database startup failed")
		}
		defer pool.Close()
		cfg.readiness["platform_database"] = pool.Ping
	}
	if cfg.Session.Enabled {
		app, err := openSessionApplication(ctx, cfg)
		if err != nil {
			if logger != nil {
				logger.Printf("DR_RECOVERY_SESSION_STARTUP_STAGE=%s", sessionStartupFailureStage(err))
			}
			return errSessionStartup
		}
		defer app.Close()
		cfg.readiness["session_database"] = app.pool.Ping
		cfg.readiness["session_cache"] = func(ctx context.Context) error { return app.redis.Ping(ctx).Err() }
		if cfg.RecoveryAuthProbes {
			cfg.sessionAuth = newSessionHandler(app.auth, app.sessions)
		}
	}
	// Remote Platform owns no second Poker actor/client. The separately locked
	// Poker process checks its own PG and Redis without taking an admission lease.
	if cfg.Poker.Enabled && cfg.PokerRemoteSocket == "" {
		pool, err := openPokerPool(ctx, cfg.Poker.DSNFile)
		if err != nil {
			return errPokerStartup
		}
		defer pool.Close()
		cfg.readiness["poker_database"] = pool.Ping
		_, options, err := readPokerRedisConfig(cfg.Poker.RedisConfigFile)
		if err != nil {
			return errPokerStartup
		}
		cache := redis.NewClient(options)
		defer cache.Close()
		cfg.readiness["poker_cache"] = func(ctx context.Context) error { return cache.Ping(ctx).Err() }
	}
	listener, err := openListener(cfg)
	if err != nil {
		return err
	}
	defer listener.Close()
	logger.Print("DR_RECOVERY_LOCK=ON: business workers and admission disabled")
	return serve(ctx, newServer(cfg), listener, cfg.ShutdownTimeout)
}
