package handlers

import (
	"bufio"
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/zjyl1994/donggua-proxy/config"
	"github.com/zjyl1994/donggua-proxy/utils"
)

// ProxyHandler 处理通用代理请求
func ProxyHandler(w http.ResponseWriter, r *http.Request) {
	// 1. 处理 CORS 预检
	utils.SetCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// 2. 参数获取与校验
	targetURLStr := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetURLStr == "" {
		HandleHttpInfo(w, r)
		return
	}
	if len(targetURLStr) > 8*1024 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	// 3. 验证访问密码 (Bearer Token)
	if config.AccessPassword != "" {
		auth := r.Header.Get("Authorization")
		expected := "Bearer " + config.AccessPassword
		if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}
	}

	targetURL, err := url.Parse(targetURLStr)
	if err != nil {
		utils.LogError(r, fmt.Errorf("invalid url: %w", err))
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	// SSRF 防护：检查目标 IP 是否为私有地址
	if err := utils.ValidateTargetURL(targetURL); err != nil {
		utils.LogError(r, fmt.Errorf("ssrf check failed: %w", err))
		http.Error(w, "Forbidden URL", http.StatusForbidden)
		return
	}

	// 4. 构建代理请求
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), r.Body)
	if err != nil {
		utils.LogError(r, fmt.Errorf("failed to create proxy request: %w", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// 设置伪装头信息
	proxyReq.Header.Set("Referer", targetURL.Scheme+"://"+targetURL.Host+"/")
	proxyReq.Header.Set("Origin", targetURL.Scheme+"://"+targetURL.Host)
	proxyReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	// 转发关键头
	if rRange := r.Header.Get("Range"); rRange != "" {
		proxyReq.Header.Set("Range", rRange)
	}
	if accept := r.Header.Get("Accept"); accept != "" {
		proxyReq.Header.Set("Accept", accept)
	}
	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		proxyReq.Header.Set("Content-Type", contentType)
	}

	resp, err := utils.DefaultClient.Do(proxyReq)
	if err != nil {
		utils.LogError(r, fmt.Errorf("proxy request failed: %w", err))
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 5. 复制目标服务器的响应头
	utils.CopyHeadersWithFilter(w, resp.Header, utils.DefaultExcludedResponseHeaders)
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		if loc := strings.TrimSpace(resp.Header.Get("Location")); loc != "" {
			if locURL, err := url.Parse(loc); err == nil {
				resolved := targetURL.ResolveReference(locURL)
				if err := utils.ValidateTargetURL(resolved); err == nil {
					proxyOrigin := utils.GetProxyOrigin(r, config.TrustProxy, config.TrustedProxyCIDRs)
					w.Header().Set("Location", proxyOrigin+"/?url="+url.QueryEscape(resolved.String()))
				}
			}
		}
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	isM3u8 := strings.HasSuffix(strings.ToLower(targetURL.Path), ".m3u8") ||
		strings.Contains(contentType, "mpegurl")

	// 6. 处理 M3U8 重写或直接流式透传
	if isM3u8 && resp.StatusCode == http.StatusOK {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Del("Content-Length")
		w.WriteHeader(resp.StatusCode)

		proxyOrigin := utils.GetProxyOrigin(r, config.TrustProxy, config.TrustedProxyCIDRs)
		if err := rewriteM3u8(w, resp.Body, targetURL, proxyOrigin); err != nil {
			utils.LogError(r, fmt.Errorf("rewrite m3u8 failed: %w", err))
		}
	} else {
		w.WriteHeader(resp.StatusCode)

		// 使用 BufferPool 优化 IO 复制
		bufPtr := utils.BufferPool.Get().(*[]byte)
		defer utils.BufferPool.Put(bufPtr)
		if _, err := io.CopyBuffer(w, resp.Body, *bufPtr); err != nil {
			utils.LogError(r, fmt.Errorf("copy response failed: %w", err))
		}
	}
}

// rewriteM3u8 实现流式重写，减少内存压力
func rewriteM3u8(w io.Writer, body io.Reader, baseURL *url.URL, proxyOrigin string) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	basePath := "/"
	if idx := strings.LastIndex(baseURL.Path, "/"); idx >= 0 {
		basePath = baseURL.Path[:idx+1]
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			fmt.Fprintln(w, line)
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			// 重写包含 URI 的标签（如加密 Key 或媒体描述）
			if strings.Contains(trimmed, `URI="`) {
				fmt.Fprintln(w, rewriteTagURIs(line, baseURL, proxyOrigin, basePath))
			} else {
				fmt.Fprintln(w, line)
			}
		} else {
			// 重写 TS 分片或嵌套的 M3U8 链接
			absolute := utils.ResolveURL(trimmed, baseURL, basePath)
			fmt.Fprintf(w, "%s/?url=%s\n", proxyOrigin, url.QueryEscape(absolute))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func rewriteTagURIs(line string, baseURL *url.URL, proxyOrigin, basePath string) string {
	parts := strings.Split(line, `URI="`)
	if len(parts) < 2 {
		return line
	}

	var result strings.Builder
	result.WriteString(parts[0])

	for i := 1; i < len(parts); i++ {
		endIdx := strings.Index(parts[i], `"`)
		if endIdx == -1 {
			result.WriteString(`URI="`)
			result.WriteString(parts[i])
			continue
		}

		uri := parts[i][:endIdx]
		absolute := utils.ResolveURL(uri, baseURL, basePath)

		result.WriteString(`URI="`)
		result.WriteString(proxyOrigin)
		result.WriteString("/?url=")
		result.WriteString(url.QueryEscape(absolute))
		result.WriteString(`"`)
		result.WriteString(parts[i][endIdx+1:])
	}
	return result.String()
}
