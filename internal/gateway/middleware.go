package gateway

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/store"
)

type contextKey string

const (
	ownerIDKey   = contextKey("ownerID")
	scopesKey    = contextKey("scopes")
	requestIDKey = contextKey("requestID")
)

var dockerHostGatewayResolver = discoverDockerHostGateways

const longRequestThreshold = 1000 * time.Millisecond

var gatewaySessionMessageTimeout = 12 * time.Second

// AuthMiddleware authenticates non-loopback requests using API keys or shared secret.
// Auth model:
//  1. Loopback bypass: localhost gets owner-level access for local tools.
//  2. X-API-Key / Bearer: API key lookup or sharedSecret when set.
func AuthMiddleware(db *sql.DB, sharedSecret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authStart := time.Now()
		authBranch := "unauthorized"
		requestID := RequestIDFromCtx(r.Context())
		logAuthSpan := func(stage string, start time.Time, attrs ...any) {
			fields := []any{
				"request_id", requestID,
				"stage", stage,
				"duration_ms", time.Since(start).Milliseconds(),
				"path", r.URL.Path,
			}
			fields = append(fields, attrs...)
			slog.Debug("gateway auth span", fields...)
		}
		defer func() {
			duration := time.Since(authStart)
			fields := []any{
				"request_id", requestID,
				"path", r.URL.Path,
				"method", r.Method,
				"auth_branch", authBranch,
				"duration_ms", duration.Milliseconds(),
			}
			if db != nil && duration > 250*time.Millisecond {
				fields = append(fields, dbStatsFields("db_stats", db.Stats())...)
			}
			slog.Info("gateway auth completed", fields...)
		}()

		// 1. Loopback — CLI and local tools get full owner access.
		loopbackStart := time.Now()
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		// Strip brackets from IPv6 if present
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")

		ip := net.ParseIP(host)

		slog.Debug("gateway: auth request received",
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"parsed_host", host,
			"ip", ip,
			"loopback", ip != nil && ip.IsLoopback(),
		)

		if ip != nil && ip.IsLoopback() {
			logAuthSpan("loopback_detection", loopbackStart, "result", "loopback")
			slog.Debug("gateway: bypassing auth for loopback request", "path", r.URL.Path, "remote_addr", r.RemoteAddr)
			ctx := context.WithValue(r.Context(), ownerIDKey, "local")
			ctx = context.WithValue(ctx, scopesKey, []string{"admin"})
			authBranch = "loopback"
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		logAuthSpan("loopback_detection", loopbackStart, "result", "not_loopback")
		bridgeStart := time.Now()
		if isDockerLocalhostBridgeRequest(r, ip) {
			logAuthSpan("docker_localhost_bridge_detection", bridgeStart, "result", "docker_bridge")
			slog.Debug("gateway: bypassing auth for localhost browser request via docker bridge", "path", r.URL.Path, "remote_addr", r.RemoteAddr)
			ctx := context.WithValue(r.Context(), ownerIDKey, "local")
			ctx = context.WithValue(ctx, scopesKey, []string{"admin"})
			authBranch = "docker_bridge"
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		logAuthSpan("docker_localhost_bridge_detection", bridgeStart, "result", "not_bridge")

		// 2. Onboarding endpoints bypass API-key auth but are gated at the
		//    route level by localOnboardingOnly (loopback/docker-bridge only).
		//    This branch skips DB key lookup for first-run browser flows.
		if strings.HasPrefix(r.URL.Path, "/api/onboarding") {
			authBranch = "onboarding_bypass"
			next.ServeHTTP(w, r)
			return
		}

		// 3. Setup endpoints open before onboarding complete (legacy CLI path).
		if db != nil && strings.HasPrefix(r.URL.Path, "/api/setup") {
			setupLookupStart := time.Now()
			val, found, _ := store.GetSetting(r.Context(), db, "setup_complete")
			logAuthSpan("setup_complete_lookup", setupLookupStart, "found", found, "value", val)
			if !found || val != "true" {
				ctx := context.WithValue(r.Context(), ownerIDKey, "local")
				authBranch = "setup_incomplete"
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// 4. API key auth via X-API-Key header.
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			// Also accept Authorization: Bearer ... for backward compat with
			// any tooling that sends the API key that way.
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				apiKey = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if apiKey != "" {
			sharedSecretStart := time.Now()
			if sharedSecret != "" && apiKey == sharedSecret {
				logAuthSpan("shared_secret_compare", sharedSecretStart, "matched", true)
				ctx := context.WithValue(r.Context(), ownerIDKey, "connector")
				ctx = context.WithValue(ctx, scopesKey, []string{"admin"})
				authBranch = "shared_secret"
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			logAuthSpan("shared_secret_compare", sharedSecretStart, "matched", false)
			if db != nil {
				lookupStart := time.Now()
				k, found, err := store.LookupAPIKeyByRaw(r.Context(), db, apiKey)
				logAuthSpan("api_key_lookup_by_raw", lookupStart, "found", found, "err", err)
				if err != nil {
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						slog.Warn("gateway auth: api key lookup cancelled or timed out",
							"request_id", requestID,
							"path", r.URL.Path,
							"error", err,
						)
					}
					http.Error(w, "gateway: api key lookup failed", http.StatusInternalServerError)
					return
				}
				if found {
					ctx := context.WithValue(r.Context(), ownerIDKey, k.OwnerID)
					ctx = context.WithValue(ctx, scopesKey, k.Scopes)
					authBranch = "api_key_db"
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		// Log failure for diagnostic purposes if it looks like it should have been loopback
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			slog.Warn("gateway: loopback-looking address rejected by auth middleware", "host", host, "remote_addr", r.RemoteAddr)
		}

		authBranch = "unauthorized"
		http.Error(w, "gateway: unauthorized", http.StatusUnauthorized)
	})
}

func (s *Server) localOnboardingOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
		ip := net.ParseIP(host)
		if ip != nil && ip.IsLoopback() {
			next.ServeHTTP(w, r)
			return
		}
		if isDockerLocalhostBridgeRequest(r, ip) {
			next.ServeHTTP(w, r)
			return
		}
		replyError(w, http.StatusForbidden, "onboarding is only available from the local machine")
	})
}

func (s *Server) requestTracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if requestID == "" {
			requestID = uuid.NewString()
		}
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-Id", requestID)
		trace := &statusCapturingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		slog.Info("gateway request start",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"host", r.Host,
		)
		next.ServeHTTP(trace, r)
		duration := time.Since(start)
		fields := []any{
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"host", r.Host,
			"duration_ms", duration.Milliseconds(),
			"status_code", trace.statusCode,
			"response_bytes", trace.responseBytes.Load(),
			"ctx_error", fmt.Sprint(r.Context().Err()),
		}
		if s.cfg.DB != nil && duration > longRequestThreshold {
			fields = append(fields, dbStatsFields("db_stats", s.cfg.DB.Stats())...)
		}
		slog.Info("gateway request end", fields...)
	})
}

func withGatewayTimeout(timeoutProvider func() time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timeout := timeoutProvider()
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		req := r.WithContext(ctx)
		done := make(chan struct{})
		buf := newBufferedResponseWriter()
		go func() {
			defer close(done)
			next.ServeHTTP(buf, req)
		}()
		select {
		case <-done:
			buf.WriteTo(w)
		case <-ctx.Done():
			slog.Warn("gateway request timed out",
				"request_id", RequestIDFromCtx(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"timeout_ms", timeout.Milliseconds(),
				"error", ctx.Err(),
			)
			replyJSON(w, http.StatusGatewayTimeout, map[string]any{
				"error": map[string]any{
					"code":    "GATEWAY_TIMEOUT",
					"message": "gateway request timed out",
					"details": map[string]any{
						"path":       r.URL.Path,
						"method":     r.Method,
						"request_id": RequestIDFromCtx(r.Context()),
						"timeout_ms": timeout.Milliseconds(),
					},
				},
			})
		}
	})
}

func (s *Server) startDBPoolWatchdog() {
	if s.cfg.DB == nil {
		return
	}
	if !s.watchdogStarted.CompareAndSwap(false, true) {
		return
	}
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	s.watchdogStop = stopCh
	s.watchdogDone = doneCh
	go func() {
		defer close(doneCh)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		var prevWaitCount int64
		var prevWaitDuration time.Duration
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				stats := s.cfg.DB.Stats()
				waitDelta := stats.WaitCount - prevWaitCount
				waitDurDelta := stats.WaitDuration - prevWaitDuration
				if stats.MaxOpenConnections > 0 && stats.InUse >= stats.MaxOpenConnections && waitDelta > 0 {
					slog.Warn("db pool saturated",
						"in_use", stats.InUse,
						"max_open_connections", stats.MaxOpenConnections,
						"wait_count_delta", waitDelta,
						"wait_duration_delta_ms", waitDurDelta.Milliseconds(),
						"wait_count", stats.WaitCount,
						"wait_duration_ms", stats.WaitDuration.Milliseconds(),
						"goroutines", runtime.NumGoroutine(),
					)
				}
				prevWaitCount = stats.WaitCount
				prevWaitDuration = stats.WaitDuration
			}
		}
	}()
}

func (s *Server) stopDBPoolWatchdog() {
	if s.watchdogStop != nil {
		close(s.watchdogStop)
		s.watchdogStop = nil
	}
	if s.watchdogDone != nil {
		<-s.watchdogDone
		s.watchdogDone = nil
	}
}

func RequestIDFromCtx(ctx context.Context) string {
	if val, ok := ctx.Value(requestIDKey).(string); ok {
		return val
	}
	return ""
}

type statusCapturingResponseWriter struct {
	http.ResponseWriter
	statusCode    int
	wroteHeader   bool
	responseBytes atomic.Int64
}

func (w *statusCapturingResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.statusCode = code
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusCapturingResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.responseBytes.Add(int64(n))
	return n, err
}

func (w *statusCapturingResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusCapturingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (w *statusCapturingResponseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

type bufferedResponseWriter struct {
	header      http.Header
	statusCode  int
	body        strings.Builder
	wroteHeader bool
}

func newBufferedResponseWriter() *bufferedResponseWriter {
	return &bufferedResponseWriter{
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (w *bufferedResponseWriter) Header() http.Header { return w.header }

func (w *bufferedResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.statusCode = code
}

func (w *bufferedResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(p)
}

func (w *bufferedResponseWriter) WriteTo(dst http.ResponseWriter) {
	for key, values := range w.header {
		for _, value := range values {
			dst.Header().Add(key, value)
		}
	}
	dst.WriteHeader(w.statusCode)
	_, _ = dst.Write([]byte(w.body.String()))
}

func dbStatsFields(prefix string, stats sql.DBStats) []any {
	return []any{
		prefix + ".MaxOpenConnections", stats.MaxOpenConnections,
		prefix + ".OpenConnections", stats.OpenConnections,
		prefix + ".InUse", stats.InUse,
		prefix + ".Idle", stats.Idle,
		prefix + ".WaitCount", stats.WaitCount,
		prefix + ".WaitDurationMs", stats.WaitDuration.Milliseconds(),
		prefix + ".MaxIdleClosed", stats.MaxIdleClosed,
		prefix + ".MaxIdleTimeClosed", stats.MaxIdleTimeClosed,
		prefix + ".MaxLifetimeClosed", stats.MaxLifetimeClosed,
	}
}

func isDockerLocalhostBridgeRequest(r *http.Request, remoteIP net.IP) bool {
	if remoteIP == nil {
		return false
	}
	if !ipMatchesAny(remoteIP, dockerHostGatewayResolver()) {
		return false
	}
	if !isLoopbackHost(r.Host) {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" && !isLoopbackOrigin(origin) {
		return false
	}
	return true
}

func ipMatchesAny(ip net.IP, candidates []net.IP) bool {
	for _, candidate := range candidates {
		if candidate != nil && candidate.Equal(ip) {
			return true
		}
	}
	return false
}

func isLoopbackHost(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if host, port, err := net.SplitHostPort(raw); err == nil && port != "" {
		raw = host
	}
	raw = strings.TrimPrefix(strings.TrimSuffix(raw, "]"), "[")
	if strings.EqualFold(raw, "localhost") {
		return true
	}
	ip := net.ParseIP(raw)
	return ip != nil && ip.IsLoopback()
}

func isLoopbackOrigin(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return isLoopbackHost(u.Host)
}

func discoverDockerHostGateways() []net.IP {
	if ip := readDefaultGatewayFromProcRoute(); ip != nil {
		return []net.IP{ip}
	}
	return []net.IP{
		net.ParseIP("172.17.0.1"),
		net.ParseIP("172.18.0.1"),
		net.ParseIP("172.19.0.1"),
		net.ParseIP("192.168.65.1"),
		net.ParseIP("192.168.64.1"),
	}
}

func readDefaultGatewayFromProcRoute() net.IP {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	for idx, line := range lines {
		if idx == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if fields[1] != "00000000" {
			continue
		}
		value, err := strconv.ParseUint(fields[2], 16, 32)
		if err != nil {
			continue
		}
		return net.IPv4(
			byte(value),
			byte(value>>8),
			byte(value>>16),
			byte(value>>24),
		).To4()
	}
	return nil
}

// OwnerIDFromCtx extracts the ownerID set by AuthMiddleware.
func OwnerIDFromCtx(ctx context.Context) string {
	if val, ok := ctx.Value(ownerIDKey).(string); ok {
		return val
	}
	return ""
}

// ScopesFromCtx extracts the scopes set by AuthMiddleware.
func ScopesFromCtx(ctx context.Context) []string {
	if val, ok := ctx.Value(scopesKey).([]string); ok {
		return val
	}
	return nil
}

// HasScope returns true if context holds the given scope or "admin".
func HasScope(ctx context.Context, scope string) bool {
	for _, s := range ScopesFromCtx(ctx) {
		if s == scope || s == "admin" {
			return true
		}
	}
	return false
}

// corsMiddleware adds CORS headers for browser clients.
// If allowedOrigins is empty, any localhost origin is allowed.
func corsMiddleware(allowedOrigins []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && corsOriginAllowed(origin, allowedOrigins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers",
				"Content-Type, Authorization, X-API-Key, X-Owner-Secret")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		next.ServeHTTP(w, r)
	})
}

// corsPreflightHandler responds to OPTIONS preflight requests.
func corsPreflightHandler(allowedOrigins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && corsOriginAllowed(origin, allowedOrigins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Headers",
			"Content-Type, Authorization, X-API-Key, X-Owner-Secret")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.WriteHeader(http.StatusNoContent)
	})
}

func corsOriginAllowed(origin string, allowed []string) bool {
	if len(allowed) == 0 {
		return strings.HasPrefix(origin, "http://localhost") ||
			strings.HasPrefix(origin, "http://127.0.0.1")
	}
	for _, a := range allowed {
		if a == "*" || a == origin {
			return true
		}
	}
	return false
}

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if s.cfg.SaveErrorRecord != nil {
					msg := fmt.Sprint(v)
					_ = s.cfg.SaveErrorRecord(r.Context(), "gateway", "", "", "panic", msg, "{}")
				}
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
