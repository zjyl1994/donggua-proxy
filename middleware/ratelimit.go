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

	trustProxy  bool
	trustedNets []*net.IPNet
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

func (i *IPRateLimiter) EnableTrustedProxies(trustProxy bool, cidrs string) {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.trustProxy = trustProxy
	if !trustProxy {
		i.trustedNets = nil
		return
	}

	var nets []*net.IPNet
	for _, part := range strings.Split(cidrs, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		_, ipNet, err := net.ParseCIDR(part)
		if err != nil {
			continue
		}
		nets = append(nets, ipNet)
	}
	if len(nets) == 0 {
		for _, loopback := range []string{"127.0.0.1/8", "::1/128"} {
			_, ipNet, err := net.ParseCIDR(loopback)
			if err != nil {
				continue
			}
			nets = append(nets, ipNet)
		}
	}
	i.trustedNets = nets
}

func (i *IPRateLimiter) isTrustedProxy(remoteIP net.IP) bool {
	if remoteIP == nil {
		return false
	}
	i.mu.Lock()
	trustProxy := i.trustProxy
	nets := i.trustedNets
	i.mu.Unlock()

	if !trustProxy || len(nets) == 0 {
		return false
	}
	for _, n := range nets {
		if n.Contains(remoteIP) {
			return true
		}
	}
	return false
}

func extractClientIP(r *http.Request, fallback string) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := strings.TrimSpace(strings.Split(xff, ",")[0])
		if ip := net.ParseIP(first); ip != nil {
			return ip.String()
		}
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		if ip := net.ParseIP(xrip); ip != nil {
			return ip.String()
		}
	}
	return fallback
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
		ipStr, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			// If RemoteAddr doesn't have a port (e.g. some test environments), use it as is
			ipStr = r.RemoteAddr
			if strings.Contains(ipStr, ":") && !strings.Contains(ipStr, "[") { // ipv4:port format but SplitHostPort failed? unlikely, but safety check
				// handle weird cases or just fallback to full string
			}
		}

		remoteIP := net.ParseIP(strings.TrimSpace(ipStr))
		if i.isTrustedProxy(remoteIP) {
			ipStr = extractClientIP(r, ipStr)
		}

		limiter := i.GetLimiter(ipStr)
		if !limiter.Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
