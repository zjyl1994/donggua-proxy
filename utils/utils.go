package utils

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	DefaultExcludedResponseHeaders = map[string]bool{
		"access-control-allow-origin":      true,
		"access-control-allow-methods":     true,
		"access-control-allow-headers":     true,
		"access-control-expose-headers":    true,
		"access-control-max-age":           true,
		"access-control-allow-credentials": true,
		"content-encoding":                 true,
		"transfer-encoding":                true,
		"connection":                       true,
		"keep-alive":                       true,
		"proxy-connection":                 true,
		"te":                               true,
		"trailer":                          true,
		"upgrade":                          true,
		"host":                             true,
	}

	// DefaultClient 全局复用的 HTTP 客户端，针对高并发场景优化
	DefaultClient = &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           SafeDialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          1000,
			MaxIdleConnsPerHost:   100,
			MaxConnsPerHost:       200,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		Timeout: 30 * time.Second,
	}

	// BufferPool 复用 IO 缓冲区，减少 GC 压力
	BufferPool = sync.Pool{
		New: func() interface{} {
			// 32KB buffer，与 io.Copy 默认一致
			b := make([]byte, 32*1024)
			return &b
		},
	}

	// DNS 缓存
	dnsCache = sync.Map{}
)

type dnsCacheEntry struct {
	ips    []net.IP
	expiry time.Time
}

// lookupIPSafe 解析 IP，带缓存和 SSRF 检查
func lookupIPSafe(host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if IsPrivateIP(ip) {
			return nil, fmt.Errorf("SSRF detected: %s is private IP", host)
		}
		return []net.IP{ip}, nil
	}

	// 1. 检查缓存
	if val, ok := dnsCache.Load(host); ok {
		entry := val.(dnsCacheEntry)
		if time.Now().Before(entry.expiry) {
			return entry.ips, nil
		}
		dnsCache.Delete(host)
	}

	// 2. DNS 解析
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}

	// 3. SSRF 检查
	for _, ip := range ips {
		if IsPrivateIP(ip) {
			return nil, fmt.Errorf("SSRF detected: %s resolves to private IP %s", host, ip.String())
		}
	}

	// 4. 写入缓存 (TTL 5分钟)
	dnsCache.Store(host, dnsCacheEntry{
		ips:    ips,
		expiry: time.Now().Add(5 * time.Minute),
	})

	return ips, nil
}

// SafeDialContext 安全的 DialContext，包含 DNS 缓存和 SSRF 检查
func SafeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	ips, err := lookupIPSafe(host)
	if err != nil {
		return nil, err
	}

	// 尝试连接解析出的 IP
	var lastErr error
	for _, ip := range ips {
		// 构造 TCP 地址
		targetAddr := net.JoinHostPort(ip.String(), port)
		conn, err := (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext(ctx, network, targetAddr)

		if err == nil {
			return conn, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no IP addresses found for %s", host)
}

// GetEnv 获取环境变量
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func GetEnvBool(key string, fallback bool) bool {
	valueStr := strings.TrimSpace(strings.ToLower(GetEnv(key, "")))
	if valueStr == "" {
		return fallback
	}
	switch valueStr {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

// GetEnvInt 获取环境变量并转换为 int，如果转换失败则返回默认值
func GetEnvInt(key string, fallback int) int {
	valueStr := GetEnv(key, "")
	if valueStr == "" {
		return fallback
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return fallback
	}
	return value
}

// SetCORSHeaders 统一设置 CORS
func SetCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, HEAD")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

// HandleCORS 设置 CORS 头并处理 OPTIONS 请求
// 返回 true 表示请求已被处理（OPTIONS），调用者应直接返回
func HandleCORS(w http.ResponseWriter, r *http.Request) bool {
	SetCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

// CopyHeaders 复制 HTTP 头
func CopyHeaders(w http.ResponseWriter, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
}

// CopyHeadersWithFilter 复制 HTTP 头，支持过滤
func CopyHeadersWithFilter(w http.ResponseWriter, src http.Header, exclude map[string]bool) {
	for k, vv := range src {
		if exclude != nil && exclude[strings.ToLower(k)] {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
}

// GetProxyOrigin 自动感知 Caddy 转发后的 Host 和协议
func GetProxyOrigin(r *http.Request) string {
	scheme := "http"
	// 感知 Caddy/Nginx 转发的协议
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	// r.Host 会自动获取 Host 请求头
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

// ResolveURL 解析相对 URL 为绝对 URL
func ResolveURL(u string, baseURL *url.URL, basePath string) string {
	if strings.HasPrefix(u, "http") {
		return u
	}
	if strings.HasPrefix(u, "//") {
		return baseURL.Scheme + ":" + u
	}
	if strings.HasPrefix(u, "/") {
		return baseURL.Scheme + "://" + baseURL.Host + u
	}
	return baseURL.Scheme + "://" + baseURL.Host + basePath + u
}

// IsPrivateIP 检查 IP 是否为私有地址或回环地址
func IsPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	return false
}

// ValidateTargetURL 验证目标 URL 的 Scheme
// 注意：DNS 解析和私有 IP 检查已移动到 SafeDialContext 中
func ValidateTargetURL(targetURL *url.URL) error {
	if targetURL == nil {
		return fmt.Errorf("nil url")
	}
	scheme := strings.ToLower(targetURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported scheme: %s", scheme)
	}
	if targetURL.Host == "" {
		return fmt.Errorf("missing host")
	}
	if targetURL.User != nil {
		return fmt.Errorf("userinfo not allowed")
	}
	return nil
}

// LogError 记录错误日志
func LogError(r *http.Request, err error) {
	log.Printf("[ERROR] %s %s: %v", r.Method, r.URL.Path, err)
}
