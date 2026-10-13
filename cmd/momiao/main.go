package main

import (
	"context"
	"errors"
	"github.com/cy4268/momiao/internal/bffauth"
	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/roulette"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/rankings"
	"github.com/jackc/pgx/v5"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger := log.New(os.Stderr, "momiao: ", log.LstdFlags|log.LUTC)
	cfg, err := loadConfig(os.LookupEnv)
	if err != nil {
		logger.Print(err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, logger); err != nil {
		logger.Print(err)
		os.Exit(1)
	}
}
func run(ctx context.Context, cfg config, logger *log.Logger) error {
	if cfg.RecoveryLock { return runRecoveryLockedProcess(ctx, cfg, logger) }
	if cfg.ProcessRole == "poker" { return runPokerProcess(ctx, cfg, logger) }
	ctx, cancelRun := context.WithCancelCause(ctx)
	defer cancelRun(nil)
	cfg.readiness = make(map[string]readinessCheck)
	if cfg.WebDir != "" {
		cfg.readiness["web"] = webReadiness(cfg.WebDir)
	}
	if cfg.NewAPISocket != "" {
		transport := newNativeTransport(cfg.NewAPISocket)
		defer transport.CloseIdleConnections()
		cfg.readiness["native"] = nativeReadiness(transport)
	}
	var sessionApp *sessionApplication
	var runtimeStore *platform.Store
	var campaign *platform.GamblerCampaignService
	var pokerControl *sessionPokerControl
	if cfg.Session.Enabled && cfg.Poker.Enabled {
		pokerControl = &sessionPokerControl{}
	}
	if cfg.Session.Enabled {
		var controls []bffauth.ControlRevoker
		if pokerControl != nil {
			controls = append(controls, pokerControl)
		}
		application, err := openSessionApplication(ctx, cfg, controls...)
		if err != nil {
			return errSessionStartup
		}
		defer application.Close()
		sessionApp = application
		cfg.readiness["session_database"] = application.pool.Ping
		cfg.readiness["session_cache"] = func(ctx context.Context) error { return application.redis.Ping(ctx).Err() }
		cfg.sessions = application.sessions
		cfg.sessionBridge = application.auth
	}
	if cfg.WalletDSNFile != "" {
		var rankingAssets rankings.AssetReader
		var unifiedAssets *platform.UnifiedAssetReader
		dsn, err := readWalletDSN(cfg.WalletDSNFile)
		if err != nil {
			return errors.New("wallet startup failed")
		}
		openCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		store, err := platform.OpenLazy(openCtx, dsn)
		cancel()
		if err != nil {
			return errors.New("wallet startup failed")
		}
		defer store.Close()
		runtimeStore=store
		cfg.readiness["platform_database"] = func(ctx context.Context) error {
			return store.WithTx(ctx, func(tx pgx.Tx) error { var one int; return tx.QueryRow(ctx, "SELECT 1").Scan(&one) })
		}
		if sessionApp != nil {
			cfg.localIdentity = &identity.HTTP{Store: &identity.Store{Pool: store.Pool()}, Sessions: identity.Sessions{Redis: sessionApp.redis}, Origin: cfg.PublicOrigin, Discord: cfg.Discord, RegistrationEnabled: cfg.AdmissionEnabled}
			cfg.sessionAuth = cfg.localIdentity
		}
		cfg.wallet = store
		cfg.profile = store
		cfg.accessGate = store
		cfg.economy = store
		cfg.transfers = store
		cfg.announcements = store
		cfg.catalog = store
		cfg.catalogAssets, err = newCatalogAssetStore(cfg.CatalogAssets, nil)
		if err != nil { return errCatalogAssetConfig }
		cfg.relay = store
		cfg.rpUsage = store
		unifiedAssets = platform.NewUnifiedAssetReader(store)
		rankingAssets = unifiedAssets
		cfg.economy, err = platform.NewEconomyService(store, unifiedAssets)
		if err != nil { return err }
		announcementCtx, stopAnnouncements := context.WithCancel(ctx)
		announcementsDone := make(chan struct{})
		go func() { defer close(announcementsDone); runAnnouncementWorker(announcementCtx, store) }()
		defer func() { stopAnnouncements(); <-announcementsDone }()
		if cfg.OpsEnvironment != "" {
		cfg.maintenanceNotices=newMaintenanceNoticesHandler(store,cfg.OpsEnvironment)
		cfg.opsMaintenance=newOpsMaintenanceHandler(store,cfg.OpsEnvironment)
		cfg.opsAudit=newOpsAuditHandler(store)

		}
		if cfg.NativeQuotaKeyFile != "" {
			key, err := readPokerReaderKey(cfg.NativeQuotaKeyFile)
			if err != nil {
				return errors.New("native quota port startup failed")
			}
			transport := newNativeTransport(cfg.NewAPISocket)
			defer transport.CloseIdleConnections()
			native, err := platform.NewNativeQuotaPort(transport, key)
			if err != nil {
				return errors.New("native quota port startup failed")
			}
			cfg.nativeQuota = native

			if cfg.RefillPolicy.Enabled {
				refillKey, err := readPokerReaderKey(cfg.RefillKeyFile)
				if err != nil { return errors.New("refill startup failed") }
				service, err := platform.NewActiveQuotaRefillService(store, native, cfg.RefillPolicy)
				if err != nil { return errors.New("refill startup failed") }
				handler, err := newActiveQuotaRefillHandler(service, refillKey)
				if err != nil { return errors.New("refill startup failed") }
				listener, err := openListener(config{ListenSocket:cfg.RefillSocket})
				if err != nil { return errors.New("refill startup failed") }
				refillCtx, stopRefill := context.WithCancel(ctx)
				refillDone := make(chan struct{})
				server := &http.Server{Handler:handler,ReadHeaderTimeout:time.Second,ReadTimeout:2*time.Second,WriteTimeout:2*time.Second,IdleTimeout:10*time.Second,MaxHeaderBytes:4096}
				go func(){defer close(refillDone);if err:=serve(refillCtx,server,listener,cfg.ShutdownTimeout);err!=nil{cancelRun(errors.New("refill listener failed"))}}()
				defer func(){stopRefill();<-refillDone;_ = listener.Close()}()
			}
			if cfg.AttributionSourceInstanceID != "" {
				attribution, err := platform.NewNativeAttributionPort(transport, key, cfg.AttributionSourceInstanceID)
				if err != nil { return errors.New("attribution startup failed") }
				attributionCtx, stopAttribution := context.WithCancel(ctx)
				attributionDone := make(chan struct{})
				go func() { defer close(attributionDone); platform.RunAttributionWorker(attributionCtx, store, attribution, cfg.AttributionSourceInstanceID) }()
				defer func() { stopAttribution(); <-attributionDone }()
				cfg.readiness["request_attribution"] = func(ctx context.Context) error { _, err := attributionObserved(ctx, store, cfg.AttributionSourceInstanceID); return err }
			}
			workerCtx, cancel := context.WithCancel(ctx)
			done := make(chan struct{})
			go func() { defer close(done); runQuotaWorker(workerCtx, store, native) }()
			defer func() { cancel(); <-done }()
		}
		if cfg.NativeQuotaDSNFile != "" {
			dsn, err := readWalletDSN(cfg.NativeQuotaDSNFile)
			if err != nil {
				return errors.New("native quota startup failed")
			}
			native, err := platform.OpenNativeQuota(ctx, dsn)
			if err != nil {
				return errors.New("native quota startup failed")
			}
			defer native.Close()
			cfg.nativeQuota = native

			workerCtx, cancel := context.WithCancel(ctx)
			done := make(chan struct{})
			go func() { defer close(done); runQuotaWorker(workerCtx, store, native) }()
			defer func() { cancel(); <-done }()
		}
		if cfg.GameFairnessKeyringFile != "" {
			keyring, err := games.ReadKeyring(cfg.GameFairnessKeyringFile)
			if err != nil {
				return errors.New("game fairness startup failed")
			}
			cfg.games, err = games.NewService(store, keyring)
			if err != nil {
				return errors.New("game fairness startup failed")
			}
			cfg.roulette, err = roulette.NewService(store, roulette.Keyring{Active:keyring.Active, Keys:keyring.Keys})
			if err != nil { return errors.New("roulette startup failed") }
			rouletteCtx, stopRoulette := context.WithCancel(ctx)
			rouletteDone := make(chan struct{})
			go func(){ defer close(rouletteDone); cfg.roulette.RunWorker(rouletteCtx) }()
			defer func(){ stopRoulette(); <-rouletteDone }()
			if cfg.History.Enabled {
				application, err := openHistoryApplication(ctx, cfg.History, store.Pool(), cfg.games, store, cfg.roulette)
				if err != nil {
					return errHistoryStartup
				}
				defer application.Close()
				cfg.history = application.handler
				cfg.readiness["history_reader"] = application.readerPool.Ping
				cfg.readiness["history_worker_database"] = application.workerPool.Ping
				cfg.readiness["history_poker_reader"] = application.pokerPool.Ping
			}
			gamesCtx, stopGames := context.WithCancel(ctx)
			gamesDone := make(chan struct{})
			go func() { defer close(gamesDone); cfg.games.RunWorker(gamesCtx) }()
			defer func() { stopGames(); <-gamesDone }()
			if cfg.BotGames.Socket != "" {
				application, err := openBotGamesApplication(ctx, cfg, store, cfg.games)
				if err != nil {
					return errBotGamesStartup
				}
				botCtx, stopBot := context.WithCancel(ctx)
				botDone := make(chan struct{})
				go func() {
					defer close(botDone)
					if err := application.Run(botCtx, cfg.ShutdownTimeout); err != nil {
						cancelRun(errBotGamesListener)
					}
				}()
				defer func() { stopBot(); <-botDone; _ = application.Close() }()
			}
		}
		if cfg.AdmissionEnabled {
			cfg.admission = store
			workerCtx, cancel := context.WithCancel(ctx)
			done := make(chan struct{})
			go func() { defer close(done); runAdmissionWorker(workerCtx, store) }()
			defer func() { cancel(); <-done }()
		}
		cfg.rankings = rankings.NewService(store, rankings.Options{
			Assets: rankingAssets,
			ActivationTime: cfg.RankingActivationTime, SourceInstanceID: cfg.AttributionSourceInstanceID,
			AttributionHealth: func(ctx context.Context) (time.Time,error) { return attributionObserved(ctx, store, cfg.AttributionSourceInstanceID) },
		})
		if !cfg.RankingActivationTime.IsZero() {
			rankingCtx, stopRankings := context.WithCancel(ctx)
			rankingDone := make(chan struct{})
			go func() { defer close(rankingDone); cfg.rankings.Run(rankingCtx) }()
			defer func() { stopRankings(); <-rankingDone }()
		}
		if cfg.OpsEnvironment != "" {
			if sessionApp == nil { return errors.New("Ops startup requires opaque sessions") }
			campaign,err=platform.NewGamblerCampaignService(store,unifiedAssets)
			if err!=nil{return errors.New("campaign startup failed")}
		backgroundCtx, stopBackground := context.WithCancel(ctx)
		backgroundDone := make(chan struct{})
		backgroundHealth := &opsBackgroundHealth{}
		cfg.readiness["ops_background_worker"] = backgroundHealth.check
		go func() { defer close(backgroundDone); runOpsBackgroundWorker(backgroundCtx, backgroundHealth,func(work context.Context)(bool,error){
                more,e:=campaign.RunBatch(work,platform.GamblerCampaignID,1);if e!=nil||more{return more,e}
                return campaign.RefreshRankings(work,func(c context.Context)(string,error){return cfg.rankings.Build(c,"ASSETS","CURRENT","")})
            }) }()
		defer func() { stopBackground(); <-backgroundDone }()
		}
	}
	if cfg.Poker.Enabled && cfg.PokerRemoteSocket != "" {
		remote, err := openPokerRemote(ctx,cfg)
		if err != nil { return errPokerStartup }
		defer remote.Close()
		cfg.poker = remote.handler
		cfg.pokerOps=remote
		cfg.pokerCatalogRuntime = remote.catalogRuntime
		cfg.readiness["poker_process"] = remote.ready
		if pokerControl != nil { pokerControl.remote = remote }
	} else if cfg.Poker.Enabled {
		application, err := openPokerApplication(ctx, cfg)
		if err != nil {
			return errPokerStartup
		}
		defer application.Close()
		cfg.poker = application.handler
		cfg.pokerOps=application.service
		cfg.readiness["poker_database"] = application.pokerPool.Ping
		cfg.readiness["poker_auth_database"] = application.authPool.Ping
		cfg.readiness["poker_cache"] = func(ctx context.Context) error { return application.tickets.Ping(ctx).Err() }
		cfg.pokerCatalogRuntime = application.service.CatalogRuntime
		if pokerControl != nil {
			pokerControl.poker = application
		}
	}
	if cfg.OpsEnvironment!=""&&runtimeStore!=nil {
		bindings := campaign.OpsBindings()
		opsService, err := platform.NewOpsService(runtimeStore, cfg.OpsEnvironment, bindings...)
		if err != nil { return errors.New("Ops startup failed") }
		opsReads:=newOpsGamblerHandler(cfg.sessions,campaign,http.NotFoundHandler())
		cfg.ops = newOpsHandler(cfg.sessions, opsService, opsFactorAdapter{auth:sessionApp.auth}, "UNKNOWN", opsReads)
		remoteCtx,stopRemote:=context.WithCancel(ctx);remoteDone:=make(chan struct{})
		remoteHealth:=&opsRemoteWorkerHealth{}
		cfg.readiness["ops_remote_worker"]=remoteHealth.check
		go func(){defer close(remoteDone);runObservedOpsRemoteWorker(remoteCtx,opsService,remoteHealth)}()
		defer func(){stopRemote();<-remoteDone}()
		healthCtx,stopHealth:=context.WithCancel(ctx);healthDone:=make(chan struct{})
		go func(){defer close(healthDone);runOpsHealthWorker(healthCtx,runtimeStore,cfg.OpsEnvironment,cfg.readiness)}()
		defer func(){stopHealth();<-healthDone}()
	}
	listener, err := openListener(cfg)
	if err != nil {
		return err
	}
	defer listener.Close()
	logger.Printf("listening on %s (process liveness only)", listener.Addr())
	if err := serve(ctx, newServer(cfg), listener, cfg.ShutdownTimeout); err != nil {
		return err
	}
	if cause:=context.Cause(ctx);cause!=nil&&!errors.Is(cause,context.Canceled){return cause}
	logger.Print("stopped")
	return nil
}
