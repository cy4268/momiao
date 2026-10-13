package main

import (
	"context"
	"errors"
	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/roulette"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/rankings"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"log"
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
	var sessionRedis *redis.Client
	var runtimeStore *platform.Store
	var campaign *platform.GamblerCampaignService
	if cfg.Session.Enabled {
		client, err := openSessionRedis(ctx, cfg.Session.RedisConfigFile)
		if err != nil {
			return err
		}
		defer client.Close()
		sessionRedis = client
		cfg.readiness["session_cache"] = func(ctx context.Context) error { return client.Ping(ctx).Err() }
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
		if sessionRedis != nil {
			cfg.localIdentity = &identity.HTTP{Store: &identity.Store{Pool: store.Pool()}, Sessions: identity.Sessions{Redis: sessionRedis}, Origin: cfg.PublicOrigin, Discord: cfg.Discord, RegistrationEnabled: cfg.AdmissionEnabled}
			cfg.sessionAuth = cfg.localIdentity
		}
		cfg.wallet = store
		cfg.profile = store
		cfg.accessGate = store
		cfg.economy = store
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
			ActivationTime: cfg.RankingActivationTime,
		})
		if !cfg.RankingActivationTime.IsZero() {
			rankingCtx, stopRankings := context.WithCancel(ctx)
			rankingDone := make(chan struct{})
			go func() { defer close(rankingDone); cfg.rankings.Run(rankingCtx) }()
			defer func() { stopRankings(); <-rankingDone }()
		}
		if cfg.OpsEnvironment != "" {
			campaign=platform.NewGamblerCampaignService(store,cfg.OpsEnvironment)
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
	}
	if cfg.OpsEnvironment != "" && runtimeStore != nil {
		healthCtx, stopHealth := context.WithCancel(ctx)
		healthDone := make(chan struct{})
		go func() {
			defer close(healthDone)
			runOpsHealthWorker(healthCtx, runtimeStore, cfg.OpsEnvironment, cfg.readiness)
		}()
		defer func() { stopHealth(); <-healthDone }()
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
