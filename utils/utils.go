package utils

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var DefaultClient = &http.Client{
	Timeout: 30 * time.Second,
}

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
