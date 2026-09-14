package main

import (
	"errors"
	"github.com/cy4268/momiao/internal/bffauth"
	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/rankings"
	"github.com/cy4268/momiao/internal/session"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type config struct {
	RecoveryLock              bool
	RecoveryAuthProbes        bool
	ProcessRole               string
	PokerRemoteSocket         string
	PokerServiceKeyringFile   string
	PokerPeerKeyringFile      string
	ops                       http.Handler
	maintenanceNotices        http.Handler
	OpsEnvironment            string
	readiness                 map[string]readinessCheck
	rankings                  *rankings.Service
	keyPurposes               keyPurposeStore
	nativePurposes            platform.NativePurposeWriter
	rpUsage                   *platform.Store
	RankingActivationTime     time.Time
	AttributionSourceInstanceID string
	History                   historyConfig
	history                   *historyHTTP
	Session                   sessionConfig
	sessionAuth               http.Handler
	sessionBridge             *bffauth.Service
	sessions                  *session.Service
	Poker                     pokerConfig
	poker                     http.Handler
	pokerCatalogRuntime       games.PokerCatalogRuntime
	games                     *games.Service
	GameFairnessKeyringFile   string
	catalog                   catalogStore
	catalogSource             platform.CatalogSource
	CatalogReaderKeyFile      string
	CatalogSyncInterval       time.Duration
	CatalogStaleAfter         time.Duration
	CatalogDisableAfter       time.Duration
	APIBaseURL                string
	accessGate                accessGateStore
	accessDeclaration         *accessDeclaration
	announcements             announcementStore
	AdmissionEnabled          bool
	admission                 admissionStore
	RegistrationReaderKeyFile string
	NativeQuotaDSNFile        string
	NativeQuotaKeyFile        string
	RefillSocket             string
	RefillKeyFile            string
	RefillPolicy             platform.ActiveQuotaRefillPolicy
	transfers                 quotaTransferStore
	nativeQuota               nativeQuotaReader
	WalletDSNFile             string
	PublicOrigin              string
	wallet                    walletStore
	profile                   profileStore
	economy                   economyStore
	ListenAddr                string
	ListenSocket              string
	WebDir                    string
	NativeAuthWebDir          string
	NewAPISocket              string
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
	if value, ok := lookup("MOMIAO_NEWAPI_SOCKET"); ok {
		if value == "" {
			return config{}, errors.New("MOMIAO_NEWAPI_SOCKET must not be empty")
		}
		if !filepath.IsAbs(value) {
			return config{}, errors.New("MOMIAO_NEWAPI_SOCKET must be an absolute path")
		}
		cfg.NewAPISocket = filepath.Clean(value)
	}
	if cfg.WebDir != "" && cfg.NewAPISocket == "" {
		return config{}, errors.New("MOMIAO_NEWAPI_SOCKET is required when MOMIAO_WEB_DIR is set")
	}
	if cfg.WebDir == "" && cfg.NewAPISocket != "" && cfg.ProcessRole != "poker" {
		return config{}, errors.New("MOMIAO_NEWAPI_SOCKET requires MOMIAO_WEB_DIR")
	}
	if cfg.ListenSocket != "" && cfg.ListenSocket == cfg.NewAPISocket {
		return config{}, errors.New("MOMIAO_LISTEN_SOCKET and MOMIAO_NEWAPI_SOCKET must differ")
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
	if value, ok := lookup("MOMIAO_NATIVE_QUOTA_DSN_FILE"); ok {
		if value == "" || !filepath.IsAbs(value) || cfg.WalletDSNFile == "" {
			return config{}, errors.New("native quota DSN requires an absolute path and platform wallet configuration")
		}
		cfg.NativeQuotaDSNFile = filepath.Clean(value)
	}
	if value, ok := lookup("MOMIAO_NATIVE_QUOTA_KEY_FILE"); ok {
		if value == "" || !filepath.IsAbs(value) || cfg.WalletDSNFile == "" || cfg.NewAPISocket == "" || cfg.NativeQuotaDSNFile != "" {
			return config{}, errors.New("native quota port requires an absolute private key path, platform wallet and Native socket; legacy DSN cannot be combined")
		}
		cfg.NativeQuotaKeyFile = filepath.Clean(value)
	}
	if err := loadRefillConfig(&cfg, lookup); err != nil { return config{}, err }
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
	if value, ok := lookup("MOMIAO_ATTRIBUTION_SOURCE_INSTANCE_ID"); ok {
		if !platform.ValidOperationKey(value) || cfg.NativeQuotaKeyFile == "" {
			return config{}, errors.New("attribution source requires an immutable UUID and private Native quota port configuration")
		}
		cfg.AttributionSourceInstanceID = value
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
	if err := loadPokerProcessConfig(&cfg, lookup); err != nil { return config{}, err }
	if err := loadPokerConfig(&cfg, lookup); err != nil {
		return config{}, err
	}
	if err := loadSessionConfig(&cfg, lookup); err != nil {
		return config{}, err
	}
	if value, ok := lookup("MOMIAO_NATIVE_AUTH_WEB_DIR"); ok {
		if value == "" || !filepath.IsAbs(value) {
			return config{}, errors.New("MOMIAO_NATIVE_AUTH_WEB_DIR must be an absolute existing build directory")
		}
		resolved, err := filepath.EvalSymlinks(value)
		if err != nil {
			return config{}, errors.New("MOMIAO_NATIVE_AUTH_WEB_DIR must be an absolute existing build directory")
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			return config{}, errors.New("MOMIAO_NATIVE_AUTH_WEB_DIR must be an absolute existing build directory")
		}
		index, err := os.Stat(filepath.Join(resolved, "index.html"))
		if err != nil || !index.Mode().IsRegular() {
			return config{}, errors.New("MOMIAO_NATIVE_AUTH_WEB_DIR must contain a regular index.html")
		}
		sameBuild := resolved == cfg.WebDir
		if portalInfo, statErr := os.Stat(cfg.WebDir); statErr == nil && os.SameFile(info, portalInfo) {
			sameBuild = true
		}
		if !cfg.Session.Enabled || cfg.WebDir == "" || sameBuild {
			return config{}, errors.New("MOMIAO_NATIVE_AUTH_WEB_DIR requires opaque sessions and a distinct portal build")
		}
		cfg.NativeAuthWebDir = resolved
	}
	if cfg.PokerRemoteSocket != "" && !cfg.Session.Enabled { return config{}, errors.New("remote Poker requires platform opaque sessions") }
	history, err := loadHistoryConfig(lookup)
	if err != nil || history.Enabled && (!cfg.Session.Enabled || cfg.WalletDSNFile == "" || cfg.GameFairnessKeyringFile == "") {
		return config{}, errHistoryConfig
	}
	cfg.History = history
	if cfg.ProcessRole == "poker" && (cfg.Session.Enabled || cfg.History.Enabled || cfg.WebDir != "" || cfg.NativeAuthWebDir != "" || cfg.GameFairnessKeyringFile != "" || cfg.AdmissionEnabled || cfg.NativeQuotaKeyFile != "" || cfg.NativeQuotaDSNFile != "" || cfg.CatalogReaderKeyFile != "" || !cfg.Poker.Enabled) {
		return config{}, errors.New("Poker process must own only its Poker runtime")
	}
	if value, ok := lookup("MOMIAO_OPS_ENVIRONMENT"); ok {
		if (value != "DEVELOPMENT" && value != "STAGING" && value != "PRODUCTION") || !cfg.Session.Enabled || cfg.WalletDSNFile == "" || value != cfg.Session.Environment {
			return config{}, errors.New("Ops requires an explicit DEVELOPMENT, STAGING or PRODUCTION environment, opaque sessions and platform wallet")
		}
		cfg.OpsEnvironment = value
	}
	if cfg.RecoveryAuthProbes && (!cfg.Session.Enabled || cfg.ProcessRole != "platform") {
		return config{}, errors.New("DR_RECOVERY_AUTH_PROBES requires platform opaque sessions")
	}
	return cfg, nil
}
