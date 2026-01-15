package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// IPRateLimiter manages rate limiters for each IP address
type IPRateLimiter struct {
	ips      map[string]*rate.Limiter
	lastSeen map[string]time.Time
	mu       sync.Mutex
	r        rate.Limit
	b        int
}

// NewIPRateLimiter creates a new IPRateLimiter
// r: requests per second
// b: burst size
func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	i := &IPRateLimiter{
		ips:      make(map[string]*rate.Limiter),
		lastSeen: make(map[string]time.Time),
		r:        r,
		b:        b,
	}

	// Start background cleanup goroutine
	go i.cleanupLoop()

	return i
}

// AddIP creates a new limiter for an IP if it doesn't exist
func (i *IPRateLimiter) AddIP(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	limiter, exists := i.ips[ip]
	if !exists {
		limiter = rate.NewLimiter(i.r, i.b)
		i.ips[ip] = limiter
	}

	i.lastSeen[ip] = time.Now()
	return limiter
}

// GetLimiter returns the limiter for a given IP
func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	limiter, exists := i.ips[ip]

	if !exists {
		i.mu.Unlock()
		return i.AddIP(ip)
	}

	i.lastSeen[ip] = time.Now()
	i.mu.Unlock()
	return limiter
}

// cleanupLoop removes old entries to prevent memory leaks
func (i *IPRateLimiter) cleanupLoop() {
	for {
		time.Sleep(1 * time.Minute)
		i.mu.Lock()
		for ip, lastSeen := range i.lastSeen {
			if time.Since(lastSeen) > 3*time.Minute {
				delete(i.ips, ip)
				delete(i.lastSeen, ip)
			}
		}
		i.mu.Unlock()
	}
}

// LimitMiddleware wraps an http.Handler with rate limiting
func (i *IPRateLimiter) LimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			// If RemoteAddr doesn't have a port (e.g. some test environments), use it as is
			ip = r.RemoteAddr
			if strings.Contains(ip, ":") && !strings.Contains(ip, "[") { // ipv4:port format but SplitHostPort failed? unlikely, but safety check
				// handle weird cases or just fallback to full string
			}
		}
		
		// Handle X-Forwarded-For if behind a proxy (optional, but good for public proxy)
		// WARNING: Only trust this if you trust the upstream proxy. 
		// For a general public proxy code, it's safer to rely on RemoteAddr unless configured otherwise.
		// We will stick to RemoteAddr for security unless explicitly asked to support reverse proxies.

		limiter := i.GetLimiter(ip)
		if !limiter.Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
