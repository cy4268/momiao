package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/cy4268/momiao/internal/bffauth"
	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/cy4268/momiao/internal/relay"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const apiProxyRequestTimeout = 30 * time.Second

var browserRoutes = map[string]bool{
	"/":                 true,
	"/login":            true,
	"/register":         true,
	"/oauth/discord":    true,
	"/account":          true,
	"/account/security": true,
	"/welcome":          true,
	"/dashboard":        true,
	"/me":               true,
	"/rewards":          true,
	"/wallet":           true,
	"/wallet/activate":  true,
	"/master-profile":   true,
	"/games/dice":       true,
	"/keys":             true,
	"/logs":             true,
	"/models":           true,
	"/rankings":         true,
	"/api/access":       true,
	"/ops/models":       true,
	"/admin/channels":   true,
}

func healthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = io.WriteString(w, "{\"status\":\"ok\"}\n")
	})
}

func newServer(cfg config) *http.Server {
	return &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           newPortalHandler(cfg, nil),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
}

func newPortalHandler(cfg config, transport http.RoundTripper) http.Handler {
	if cfg.RecoveryLock {
		return newRecoveryLockedHandler(cfg)
	}
	if cfg.maintenanceNotices == nil {
		cfg.maintenanceNotices = newMaintenanceNoticesHandler(nil, "")
	}
	models := relay.ModelsHandler(cfg.relay)
	generation := relay.Handler(cfg.relay)
	if cfg.WebDir == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/channel" || strings.HasPrefix(r.URL.Path, "/api/channel/") {
				walletError(w,404,"NOT_FOUND")
				return
			}
			if r.URL.Path == "/v1/models" {
				models.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/v1/") {
				generation.ServeHTTP(w, r)
				return
			}
			if r.URL.Path == "/api/v1/maintenance/notices" {
				cfg.maintenanceNotices.ServeHTTP(w, r)
				return
			}
			if opsAPIRoute(r.URL.Path) {
				serveOpsRoute(cfg, w, r)
				return
			}
			if r.URL.Path == "/readyz" {
				newReadinessHandler(cfg.readiness).ServeHTTP(w, r)
				return
			}
			if r.URL.Path == "/api/v1/rankings" {
				newRankingsHandler(cfg.rankings, false).ServeHTTP(w, r)
				return
			}
			if historyAPIRoute(r.URL.Path) {
				domainHandler(cfg, cfg.history).ServeHTTP(w, r)
				return
			}
			if r.URL.Path == "/api/user/self" {
				domainHandler(cfg, newLocalSelfHandler(cfg.relay)).ServeHTTP(w, r)
				return
			}
			if localWorkspaceRoute(r.URL.Path) {
				domainHandler(cfg, newLocalWorkspaceHandler(cfg.relay)).ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/token/") {
				if cfg.relay == nil {
					walletError(w, 503, "KEY_UNAVAILABLE")
					return
				}
				domainHandler(cfg, newLocalKeysHandler(cfg.relay)).ServeHTTP(w, r)
				return
			}
			if r.URL.Path == "/platform/v1/announcements" || strings.HasPrefix(r.URL.Path, "/platform/v1/announcements/") || r.URL.Path == "/platform/v1/ops/announcements" || strings.HasPrefix(r.URL.Path, "/platform/v1/ops/announcements/") {
				domainHandler(cfg, newAnnouncementHandler(cfg.PublicOrigin, cfg.announcements)).ServeHTTP(w, r)
				return
			}
			if catalogAPIRoute(r.URL.Path) {
				domainHandler(cfg, newCatalogHandler(cfg)).ServeHTTP(w, r)
				return
			}
			if sessionAPIRoute(r.URL.Path) {
				serveSessionRoute(cfg.sessionAuth, w, r)
				return
			}
			if pokerAPIRoute(r.URL.Path) {
				servePokerRoute(domainHandler(cfg, cfg.poker), w, r)
				return
			}
			if r.URL.Path == "/platform/v1/admission/config" {
				newAdmissionConfigHandler(false).ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/platform/v1/master-profile") {
				walletError(w, 503, "PROFILE_UNAVAILABLE")
				return
			}
			if strings.HasPrefix(r.URL.Path, "/platform/v1/") {
				walletError(w, 503, "WALLET_UNAVAILABLE")
				return
			}
			if r.URL.Path == "/api/v1" || strings.HasPrefix(r.URL.Path, "/api/v1/") {
				walletError(w, 404, "NOT_FOUND")
				return
			}
			healthHandler().ServeHTTP(w, r)
		})
	}
	if root, err := filepath.EvalSymlinks(cfg.WebDir); err == nil {
		cfg.WebDir = root
	}
	if transport == nil {
		transport = newNativeTransport(cfg.NewAPISocket)
	}
	proxy := newNativeProxy(transport)
	readPoker := cfg.pokerCatalogRuntime
	if !cfg.Poker.Enabled || cfg.poker == nil {
		readPoker = nil
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/models":
			models.ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/v1/"):
			generation.ServeHTTP(w, r)
		case opsAPIRoute(r.URL.Path):
			serveOpsRoute(cfg, w, r)
		case r.URL.Path == "/api/v1/maintenance/notices":
			cfg.maintenanceNotices.ServeHTTP(w, r)
		case r.URL.Path == "/readyz":
			newReadinessHandler(cfg.readiness).ServeHTTP(w, r)
		case r.URL.Path == "/api/v1/rankings":
			newRankingsHandler(cfg.rankings, false).ServeHTTP(w, r)
		case r.URL.Path == "/api/v1/rankings/me":
			domainHandler(cfg, newRankingsHandler(cfg.rankings, true)).ServeHTTP(w, r)
		case r.URL.Path == "/api/v1/usage/rp":
			domainHandler(cfg, newRPUsageHandler(cfg)).ServeHTTP(w, r)
		case r.URL.Path == "/platform/v1/key-purposes" || strings.HasPrefix(r.URL.Path, "/platform/v1/key-purposes/"):
			walletError(w, http.StatusNotFound, "NOT_FOUND")
		case historyAPIRoute(r.URL.Path):
			domainHandler(cfg, cfg.history).ServeHTTP(w, r)
		case sessionAPIRoute(r.URL.Path):
			serveSessionRoute(cfg.sessionAuth, w, r)
		case cfg.Session.Enabled && legacyNativeBrowserAuthRoute(r.URL.Path):
			sessionError(w, bffauth.Fault{Status: http.StatusNotFound, Code: "AUTH_ROUTE_NOT_FOUND"})
		case strings.HasPrefix(r.URL.Path, "/api/token/"):
			if cfg.relay == nil {
				walletError(w, 503, "KEY_UNAVAILABLE")
				return
			}
			domainHandler(cfg, newLocalKeysHandler(cfg.relay)).ServeHTTP(w, r)
		case localWorkspaceRoute(r.URL.Path):
			domainHandler(cfg, newLocalWorkspaceHandler(cfg.relay)).ServeHTTP(w, r)
		case r.URL.Path == "/api/user/self":
			domainHandler(cfg, newLocalSelfHandler(cfg.relay)).ServeHTTP(w, r)
		case pokerAPIRoute(r.URL.Path):
			servePokerRoute(domainHandler(cfg, cfg.poker), w, r)
		case r.URL.Path == "/api/v1/games" || strings.HasPrefix(r.URL.Path, "/api/v1/games/") || r.URL.Path == "/api/v1/game-rounds" || strings.HasPrefix(r.URL.Path, "/api/v1/game-rounds/"):
			domainHandler(cfg, newGameHandler(cfg.PublicOrigin, cfg.games, transport, readPoker)).ServeHTTP(w, r)
		case r.URL.Path == "/api/v1/roulette" || strings.HasPrefix(r.URL.Path, "/api/v1/roulette/"):
			domainHandler(cfg, newRouletteHandler(cfg.PublicOrigin, cfg.roulette, cfg.games, transport)).ServeHTTP(w, r)
		case path.Clean(r.URL.Path) == "/internal" || strings.HasPrefix(path.Clean(r.URL.Path), "/internal/"):
			walletError(w, 404, "NOT_FOUND")
		case catalogAPIRoute(r.URL.Path):
			domainHandler(cfg, newCatalogHandler(cfg)).ServeHTTP(w, r)
		case r.URL.Path == "/platform/v1/announcements" || strings.HasPrefix(r.URL.Path, "/platform/v1/announcements/") || r.URL.Path == "/platform/v1/ops/announcements" || strings.HasPrefix(r.URL.Path, "/platform/v1/ops/announcements/"):
			domainHandler(cfg, newAnnouncementHandler(cfg.PublicOrigin, cfg.announcements)).ServeHTTP(w, r)
		case r.URL.Path == "/platform/v1/access-gate" || strings.HasPrefix(r.URL.Path, "/platform/v1/migration-notice"):
			domainHandler(cfg, newAccessGateHandler(cfg.PublicOrigin, cfg.accessGate, cfg.accessDeclaration, transport, cfg.Poker.Enabled && cfg.poker != nil)).ServeHTTP(w, r)
		case r.URL.Path == "/platform/v1/admission/config":
			newAdmissionConfigHandler(cfg.AdmissionEnabled).ServeHTTP(w, r)
		case r.URL.Path == "/platform/v1/admission" || strings.HasPrefix(r.URL.Path, "/platform/v1/admission/"):
			domainHandler(cfg, newAdmissionHandler(cfg.PublicOrigin, cfg.admission, transport)).ServeHTTP(w, r)
		case r.URL.Path == "/platform/v1/native-quota" || strings.HasPrefix(r.URL.Path, "/platform/v1/quota-transfers"):
			domainHandler(cfg, newQuotaHandler(cfg.PublicOrigin, cfg.transfers, cfg.nativeQuota, transport)).ServeHTTP(w, r)
		case r.URL.Path == "/platform/v1/wallet/exchange" || strings.HasPrefix(r.URL.Path, "/platform/v1/rewards/") || strings.HasPrefix(r.URL.Path, "/platform/v1/transactions"):
			domainHandler(cfg, newEconomyHandler(cfg.PublicOrigin, cfg.economy, transport)).ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/platform/v1/master-profile"):
			domainHandler(cfg, newProfileHandler(cfg.PublicOrigin, cfg.profile, transport)).ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/platform/v1/"):
			domainHandler(cfg, newWalletHandler(cfg.PublicOrigin, cfg.wallet, transport)).ServeHTTP(w, r)
		case r.URL.Path == "/healthz":
			healthHandler().ServeHTTP(w, r)
		case r.URL.Path == "/api/access":
			serveWebFile(cfg.WebDir, cfg.AssetCDNOrigin, w, r)
		case r.URL.Path == "/api/v1" || strings.HasPrefix(r.URL.Path, "/api/v1/"):
			walletError(w, 404, "NOT_FOUND")
		case strings.HasPrefix(r.URL.Path, "/api/"):
			if r.URL.Path == "/api/channel" || strings.HasPrefix(r.URL.Path, "/api/channel/") {
				walletError(w,404,"NOT_FOUND")
				return
			}
			if r.Header.Get("Upgrade") != "" || headerHasToken(r.Header.Get("Connection"), "upgrade") {
				writeJSONError(w, http.StatusBadRequest, "unsupported protocol upgrade")
				return
			}
			deadline := time.Now().Add(apiProxyRequestTimeout)
			if parentDeadline, ok := r.Context().Deadline(); ok && parentDeadline.Before(deadline) {
				deadline = parentDeadline
			}
			_ = http.NewResponseController(w).SetWriteDeadline(deadline)
			ctx, cancel := context.WithDeadline(r.Context(), deadline)
			defer cancel()
			proxy.ServeHTTP(w, r.WithContext(ctx))
		default:
			serveWebFile(cfg.WebDir, cfg.AssetCDNOrigin, w, r)
		}
	})
}

func serveSessionRoute(handler http.Handler, w http.ResponseWriter, r *http.Request) {
	if handler == nil {
		sessionError(w, bffauth.Fault{Status: http.StatusServiceUnavailable, Code: "SESSION_UNAVAILABLE"})
		return
	}
	handler.ServeHTTP(w, r)
}

func serveOpsRoute(cfg config, w http.ResponseWriter, r *http.Request) {
 if strings.HasPrefix(r.URL.Path,"/api/v1/ops/economy/gambler-campaigns/") {domainHandler(cfg,newOpsGamblerHandler(cfg.relay,cfg.OpsEnvironment)).ServeHTTP(w,r);return}
 if r.URL.Path=="/api/v1/ops/native-runtime"{walletError(w,404,"NOT_FOUND");return}
 if opsRuntimeRoute(r.URL.Path){domainHandler(cfg,newOpsRuntimeHandler(cfg.relay,cfg.OpsEnvironment)).ServeHTTP(w,r);return}
 if opsSupportRecordsRoute(r.URL.Path){domainHandler(cfg,newOpsSupportRecordsHandler(cfg.relay)).ServeHTTP(w,r);return}
 if opsPokerRoute(r.URL.Path){domainHandler(cfg,newOpsPokerHandler(cfg.pokerOps)).ServeHTTP(w,r);return}
 if opsGamesRoute(r.URL.Path) {
  domainHandler(cfg,newOpsGamesHandler(cfg.games)).ServeHTTP(w,r)
  return
 }

 if opsRankingsRoute(r.URL.Path) {
  domainHandler(cfg,newOpsRankingsHandler(cfg.rankings)).ServeHTTP(w,r)
  return
 }

 if opsEconomyRuntimeRoute(r.URL.Path) {
  domainHandler(cfg, newOpsEconomyRuntimeHandler(cfg.relay)).ServeHTTP(w, r)
  return
 }

 if opsRecordsRoute(r.URL.Path) {
  domainHandler(cfg, newLocalOpsRecordsHandler(cfg.relay, cfg.history)).ServeHTTP(w, r)
  return
 }

	if localOpsUsersRoute(r.URL.Path) {
		domainHandler(cfg,newLocalOpsUsersHandler(cfg.relay,cfg.localIdentity)).ServeHTTP(w,r)
		return
	}
	if opsRelayRoute(r.URL.Path) {
		domainHandler(cfg,newOpsRelayHandler(cfg.relay)).ServeHTTP(w,r)
		return
	}
	handler := cfg.ops
	if r.URL.Path == "/api/v1/ops/maintenance" && cfg.opsMaintenance != nil && cfg.OpsEnvironment != "" {
		handler = domainHandler(cfg, cfg.opsMaintenance)
	}
	if (r.URL.Path == "/api/v1/ops/audit" || strings.HasPrefix(r.URL.Path, "/api/v1/ops/audit/")) && cfg.opsAudit != nil && cfg.OpsEnvironment != "" {
		handler = domainHandler(cfg, cfg.opsAudit)
	}
	if handler == nil {
		walletError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE")
		return
	}
	handler.ServeHTTP(w, r)
}

func pokerAPIRoute(path string) bool {
	return path == "/api/v1/poker" || strings.HasPrefix(path, "/api/v1/poker/") || path == "/ws/poker"
}

func servePokerRoute(handler http.Handler, w http.ResponseWriter, r *http.Request) {
	if handler == nil {
		walletError(w, http.StatusServiceUnavailable, "POKER_UNAVAILABLE")
		return
	}
	handler.ServeHTTP(w, r)
}

func newNativeTransport(socket string) *http.Transport { return nativeself.NewTransport(socket) }
func newNativeProxy(transport http.RoundTripper) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(proxyRequest *httputil.ProxyRequest) {
			request := proxyRequest.Out
			request.URL.Scheme = "http"
			request.URL.Host = "unix"
			request.Host = "localhost"
			for _, key := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-IP", "CF-Connecting-IP", "True-Client-IP"} {
				request.Header.Del(key)
			}
		},
		ErrorLog: log.New(io.Discard, "", 0),
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			writeJSONError(w, http.StatusBadGateway, "upstream unavailable")
		},
	}
}

func serveWebFile(root, assetCDNOrigin string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if browserRoutes[r.URL.Path] || opsBrowserPermission(r.URL.EscapedPath()) != "" || gameBrowserRoute(r.URL.EscapedPath()) || rouletteBrowserRoute(r.URL.EscapedPath()) || walletTransactionBrowserRoute(r.URL.EscapedPath()) || pokerBrowserRoute(r.URL.EscapedPath()) && r.URL.RawQuery == "" && !r.URL.ForceQuery && r.URL.Fragment == "" || announcementBrowserRoute(r.URL.Path) || catalogBrowserRoute(r.URL.EscapedPath()) || r.URL.Path == "/index.html" {
		serveIndex(root, assetCDNOrigin, w, r)
		return
	}
	if !safeAssetPath(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	name := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
	resolved, err := filepath.EvalSymlinks(name)
	if err != nil || !pathWithin(root, resolved) {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(resolved)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if contentType := mime.TypeByExtension(filepath.Ext(resolved)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if fingerprinted(filepath.Base(resolved)) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}

func serveIndex(root, assetCDNOrigin string, w http.ResponseWriter, r *http.Request) {
	file, err := os.Open(filepath.Join(root, "index.html"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	setIndexHeaders(w, assetCDNOrigin)
	http.ServeContent(w, r, "index.html", info.ModTime(), file)
}

func setIndexHeaders(w http.ResponseWriter, assetCDNOrigin string) {
	imageSources := "'self' data:"
	if assetCDNOrigin != "" {
		imageSources += " " + assetCDNOrigin
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'self' https://challenges.cloudflare.com; frame-src https://challenges.cloudflare.com; style-src 'self' 'unsafe-inline'; font-src 'self' data:; img-src "+imageSources+"; connect-src 'self'")
}

func safeAssetPath(name string) bool {
	if name == "" || name[0] != '/' || strings.HasSuffix(name, "/") {
		return false
	}
	segments := strings.Split(strings.TrimPrefix(name, "/"), "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || strings.HasPrefix(segment, ".") {
			return false
		}
		lower := strings.ToLower(segment)
		if lower == "src" || lower == "node_modules" {
			return false
		}
	}
	base := strings.ToLower(segments[len(segments)-1])
	if strings.HasSuffix(base, ".map") || strings.HasSuffix(base, ".ts") || strings.HasSuffix(base, ".tsx") || strings.HasSuffix(base, ".jsx") || strings.HasPrefix(base, "vite.config.") || strings.HasPrefix(base, "tsconfig") || base == "package.json" || strings.HasPrefix(base, "package-lock.") || strings.HasPrefix(base, "pnpm-lock.") || base == "yarn.lock" {
		return false
	}
	return true
}

func pathWithin(root, name string) bool {
	relative, err := filepath.Rel(root, name)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func fingerprinted(name string) bool {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	for _, separator := range []string{".", "-"} {
		index := strings.LastIndex(stem, separator)
		if index < 0 || len(stem)-index-1 < 8 {
			continue
		}
		candidate := stem[index+1:]
		valid := true
		for _, r := range candidate {
			if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' {
				valid = false
				break
			}
		}
		if valid {
			return true
		}
	}
	return false
}

func headerHasToken(value, token string) bool {
	for _, part := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "{\"error\":%q}\n", message)
}

func openListener(cfg config) (net.Listener, error) {
	if cfg.ListenSocket == "" {
		return net.Listen("tcp", cfg.ListenAddr)
	}
	if _, err := os.Lstat(cfg.ListenSocket); err == nil {
		return nil, errors.New("MOMIAO_LISTEN_SOCKET path already exists")
	} else if !os.IsNotExist(err) {
		return nil, errors.New("MOMIAO_LISTEN_SOCKET path cannot be inspected")
	}
	address, err := net.ResolveUnixAddr("unix", cfg.ListenSocket)
	if err != nil {
		return nil, errors.New("MOMIAO_LISTEN_SOCKET is invalid")
	}
	listener, err := net.ListenUnix("unix", address)
	if err != nil {
		return nil, fmt.Errorf("MOMIAO_LISTEN_SOCKET listen: %w", err)
	}
	listener.SetUnlinkOnClose(true)
	if err := os.Chmod(cfg.ListenSocket, 0o600); err != nil {
		_ = listener.Close()
		return nil, errors.New("MOMIAO_LISTEN_SOCKET permissions could not be set")
	}
	return listener, nil
}

func serve(ctx context.Context, server *http.Server, listener net.Listener, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP server: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			_ = server.Close()
		}
		serveErr := <-done
		if err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server: %w", serveErr)
		}
		return nil
	}
}
