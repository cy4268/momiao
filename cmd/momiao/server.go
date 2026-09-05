package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	apiProxyRequestTimeout = 30 * time.Second
	v1ProxyRequestTimeout  = 5 * time.Minute
)

var browserRoutes = map[string]bool{
	"/":               true,
	"/login":          true,
	"/sign-in":        true,
	"/dashboard":      true,
	"/wallet":         true,
	"/master-profile": true,
	"/keys":           true,
	"/logs":           true,
	"/models":         true,
	"/playground":     true,
	"/admin/channels": true,
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
	if cfg.WebDir == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/platform/v1/master-profile") {
				walletError(w, 503, "PROFILE_UNAVAILABLE")
				return
			}
			if strings.HasPrefix(r.URL.Path, "/platform/v1/") {
				walletError(w, 503, "WALLET_UNAVAILABLE")
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isRelay := strings.HasPrefix(r.URL.Path, "/v1/") || r.URL.Path == "/pg/chat/completions"
		switch {
		case strings.HasPrefix(r.URL.Path, "/platform/v1/master-profile"):
			newProfileHandler(cfg.PublicOrigin, cfg.profile, transport).ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/platform/v1/"):
			newWalletHandler(cfg.PublicOrigin, cfg.wallet, transport).ServeHTTP(w, r)
		case r.URL.Path == "/healthz":
			healthHandler().ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/api/") || isRelay:
			if r.Header.Get("Upgrade") != "" || headerHasToken(r.Header.Get("Connection"), "upgrade") {
				writeJSONError(w, http.StatusBadRequest, "unsupported protocol upgrade")
				return
			}
			timeout := apiProxyRequestTimeout
			if isRelay {
				timeout = v1ProxyRequestTimeout
			}
			deadline := time.Now().Add(timeout)
			if parentDeadline, ok := r.Context().Deadline(); ok && parentDeadline.Before(deadline) {
				deadline = parentDeadline
			}
			_ = http.NewResponseController(w).SetWriteDeadline(deadline)
			ctx, cancel := context.WithDeadline(r.Context(), deadline)
			defer cancel()
			proxy.ServeHTTP(w, r.WithContext(ctx))
		default:
			serveWebFile(cfg.WebDir, w, r)
		}
	})
}

func newNativeTransport(socket string) *http.Transport {
	dialer := &net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socket)
		},
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       60 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

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

func serveWebFile(root string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if browserRoutes[r.URL.Path] || r.URL.Path == "/index.html" {
		serveIndex(root, w, r)
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

func serveIndex(root string, w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self' data:; img-src 'self' data:; connect-src 'self'")
	http.ServeContent(w, r, "index.html", info.ModTime(), file)
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
