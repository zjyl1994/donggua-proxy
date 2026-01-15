package utils

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// GetEnv 获取环境变量
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// SetCORSHeaders 统一设置 CORS
func SetCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, HEAD")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
	w.Header().Set("Access-Control-Max-Age", "86400")
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
