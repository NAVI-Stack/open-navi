package gateway

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	authTokenLimit = 5           // requests
	authTokenBurst = 5           // per window
	authTokenWindow = time.Minute
)

// rate.Every(d) means 1 request per d. So 5 per minute = Every(12s).
var authTokenRate = rate.Every(authTokenWindow / authTokenLimit)

type ipLimiter struct {
	mu      sync.Mutex
	clients map[string]*rate.Limiter
	limit   rate.Limit
	burst   int
}

func newIPLimiter(limit rate.Limit, burst int) *ipLimiter {
	return &ipLimiter{
		clients: make(map[string]*rate.Limiter),
		limit:   limit,
		burst:   burst,
	}
}

func (m *ipLimiter) get(ip string) *rate.Limiter {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.clients[ip]
	if !ok {
		l = rate.NewLimiter(m.limit, m.burst)
		m.clients[ip] = l
	}
	return l
}

func (m *ipLimiter) allow(ip string) bool {
	return m.get(ip).Allow()
}

func clientIP(r *http.Request) string {
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		for i := 0; i < len(x); i++ {
			if x[i] == ',' {
				return x[:i]
			}
		}
		return x
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

func rateLimitAuthToken(next http.HandlerFunc) http.Handler {
	limiter := newIPLimiter(authTokenRate, authTokenBurst)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow(clientIP(r)) {
			replyError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next(w, r)
	})
}
