package main

import (
	"errors"
	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/rankings"
	"github.com/cy4268/momiao/internal/roulette"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type config struct {
 pokerOps PokerOpsPort
	Discord identity.Discord
	BotGames botGamesConfig
	RecoveryLock              bool
	RecoveryAuthProbes        bool
	ProcessRole               string
	PokerRemoteSocket         string
	PokerServiceKeyringFile   string
	PokerPeerKeyringFile      string
	opsMaintenance            http.Handler
	opsAudit                  http.Handler
	maintenanceNotices        http.Handler
	OpsEnvironment            string
	readiness                 map[string]readinessCheck
	rankings                  *rankings.Service
	relay                     *platform.Store
	rpUsage                   *platform.Store
	RankingActivationTime     time.Time
	History                   historyConfig
	history                   *historyHTTP
	Session                   sessionConfig
	sessionAuth               http.Handler
	localIdentity             *identity.HTTP
	Poker                     pokerConfig
	poker                     http.Handler
	pokerCatalogRuntime       games.PokerCatalogRuntime
	games                     *games.Service
	roulette                  *roulette.Service
	GameFairnessKeyringFile   string
	catalog                   catalogStore
	CatalogAssets             catalogAssetConfig
	catalogAssets             *catalogAssetStore
	APIBaseURL                string
	accessGate                accessGateStore
	accessDeclaration         *accessDeclaration
	announcements             announcementStore
	AdmissionEnabled          bool
	admission                 admissionStore
	WalletDSNFile             string
	PublicOrigin              string
	wallet                    walletStore
	profile                   profileStore
	economy                   economyStore
	ListenAddr                string
	ListenSocket              string
	AssetCDNOrigin            string
	WebDir                    string
	ShutdownTimeout           time.Duration
}

func loadConfig(lookup func(string) (string, bool)) (config, error) {
	cfg := config{ProcessRole: "platform", ListenAddr: "127.0.0.1:8080", ShutdownTimeout: 10 * time.Second}
	if err := loadRecoveryConfig(&cfg, lookup); err != nil { return config{}, err }
	if value, ok := lookup("MOMIAO_PROCESS_ROLE"); ok {
		if value != "platform" && value != "poker" { return config{}, errors.New("invalid process role") }
		cfg.ProcessRole = value
	}
	listenAddr, listenAddrSet := lookup("MOMIAO_LISTEN_ADDR")
	if listenAddrSet {
		if listenAddr == "" {
			return config{}, errors.New("MOMIAO_LISTEN_ADDR must not be empty")
		}
		cfg.ListenAddr = listenAddr
	}
	if value, ok := lookup("MOMIAO_LISTEN_SOCKET"); ok {
		if value == "" {
			return config{}, errors.New("MOMIAO_LISTEN_SOCKET must not be empty")
		}
		if !filepath.IsAbs(value) {
			return config{}, errors.New("MOMIAO_LISTEN_SOCKET must be an absolute path")
		}
		if listenAddrSet {
			return config{}, errors.New("MOMIAO_LISTEN_ADDR and MOMIAO_LISTEN_SOCKET are mutually exclusive")
		}
		cfg.ListenAddr = ""
		cfg.ListenSocket = filepath.Clean(value)
	}
	if cfg.ListenAddr != "" {
		host, port, err := net.SplitHostPort(cfg.ListenAddr)
		if err != nil || net.ParseIP(host) == nil {
			return config{}, errors.New("MOMIAO_LISTEN_ADDR must be a numeric IP and port (IPv6 in brackets)")
		}
		portNumber, err := strconv.ParseUint(port, 10, 16)
		if err != nil || portNumber == 0 {
			return config{}, errors.New("MOMIAO_LISTEN_ADDR port must be in 1..65535")
		}
	}
	if value, ok := lookup("MOMIAO_WEB_DIR"); ok {
		if value == "" {
			return config{}, errors.New("MOMIAO_WEB_DIR must not be empty")
		}
		if !filepath.IsAbs(value) {
			return config{}, errors.New("MOMIAO_WEB_DIR must be an absolute path")
		}
		resolved, err := filepath.EvalSymlinks(value)
		if err != nil {
			return config{}, errors.New("MOMIAO_WEB_DIR must be an existing directory")
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			return config{}, errors.New("MOMIAO_WEB_DIR must be an existing directory")
		}
		index, err := os.Stat(filepath.Join(resolved, "index.html"))
		if err != nil || !index.Mode().IsRegular() {
			return config{}, errors.New("MOMIAO_WEB_DIR must contain a regular index.html")
		}
		cfg.WebDir = resolved
	}
	if value, ok := lookup("MOMIAO_SHUTDOWN_TIMEOUT"); ok {
		if value == "" {
			return config{}, errors.New("MOMIAO_SHUTDOWN_TIMEOUT must not be empty")
		}
		var err error
		cfg.ShutdownTimeout, err = time.ParseDuration(value)
		if err != nil || cfg.ShutdownTimeout < time.Second || cfg.ShutdownTimeout > 30*time.Second {
			return config{}, errors.New("MOMIAO_SHUTDOWN_TIMEOUT must be a duration in 1s..30s")
		}
	}
	if err := walletConfig(&cfg, lookup); err != nil {
		return config{}, err
	}
	if value, ok := lookup("MOMIAO_GAME_FAIRNESS_KEYRING_FILE"); ok {
		if value == "" || !filepath.IsAbs(value) || cfg.WalletDSNFile == "" {
			return config{}, errors.New("game fairness keyring requires an absolute private path and platform wallet configuration")
		}
		cfg.GameFairnessKeyringFile = filepath.Clean(value)
	}
	if err := admissionConfig(&cfg, lookup); err != nil {
		return config{}, err
	}
	if value, ok := lookup("MOMIAO_RANKING_ACTIVATION_TIME"); ok {
		activation, err := time.Parse(time.RFC3339, value)
		if err != nil || activation.IsZero() || cfg.WalletDSNFile == "" {
			return config{}, errors.New("ranking activation requires an explicit RFC3339 cutover time and platform wallet")
		}
		cfg.RankingActivationTime = activation.UTC()
	}
	if path, ok := lookup("MOMIAO_ACCESS_GATE_DECLARATION_FILE"); ok {
		declaration, err := loadAccessDeclaration(path, cfg.PublicOrigin)
		if err != nil {
			return config{}, err
		}
		cfg.accessDeclaration = declaration
	}
	if err := catalogConfig(&cfg, lookup); err != nil {
		return config{}, err
	}
	assets, assetErr := loadCatalogAssetConfig(lookup)
	if assetErr != nil { return config{}, assetErr }
	cfg.CatalogAssets = assets
	if assets != (catalogAssetConfig{}) && (cfg.ProcessRole == "poker" || cfg.WalletDSNFile == "" || cfg.WebDir == "") { return config{}, errCatalogAssetConfig }
	if err := loadPokerProcessConfig(&cfg, lookup); err != nil { return config{}, err }
	if err := loadPokerConfig(&cfg, lookup); err != nil {
		return config{}, err
	}
	if err := loadSessionConfig(&cfg, lookup); err != nil {
		return config{}, err
	}
	if path, ok := lookup("MOMIAO_DISCORD_CONFIG_FILE"); ok {
		if !cfg.Session.Enabled || cfg.ProcessRole == "poker" {
			return config{}, errors.New("MOMIAO_DISCORD_CONFIG_FILE requires local platform sessions")
		}
		discord, err := readDiscordConfig(path, cfg.PublicOrigin)
		if err != nil {
			return config{}, err
		}
		cfg.Discord = discord
	}
	if cfg.PokerRemoteSocket != "" && !cfg.Session.Enabled { return config{}, errors.New("remote Poker requires platform local sessions") }
	history, err := loadHistoryConfig(lookup)
	if err != nil || history.Enabled && (!cfg.Session.Enabled || cfg.WalletDSNFile == "" || cfg.GameFairnessKeyringFile == "") {
		return config{}, errHistoryConfig
	}
	cfg.History = history
	if cfg.ProcessRole == "poker" && (cfg.Session.Enabled || cfg.History.Enabled || cfg.WebDir != "" || cfg.AssetCDNOrigin != "" || cfg.GameFairnessKeyringFile != "" || cfg.AdmissionEnabled || !cfg.Poker.Enabled) {
		return config{}, errors.New("Poker process must own only its Poker runtime")
	}
	if value, ok := lookup("MOMIAO_OPS_ENVIRONMENT"); ok {
		if (value != "DEVELOPMENT" && value != "STAGING" && value != "PRODUCTION") || !cfg.Session.Enabled || cfg.WalletDSNFile == "" {
			return config{}, errors.New("Ops requires an explicit DEVELOPMENT, STAGING or PRODUCTION environment, local sessions and platform wallet")
		}
		cfg.OpsEnvironment = value
	}
	if cfg.RecoveryAuthProbes && (!cfg.Session.Enabled || cfg.ProcessRole != "platform") {
		return config{}, errors.New("DR_RECOVERY_AUTH_PROBES requires platform local sessions")
	}
	if err := loadBotGamesConfig(&cfg, lookup); err != nil {
		return config{}, err
	}
	return cfg, nil
}
