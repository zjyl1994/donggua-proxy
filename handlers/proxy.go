package handlers

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"donggua-proxy/config"
	"donggua-proxy/utils"
)

var excludeHeaders = map[string]bool{
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
	"host":                             true,
}

// ProxyHandler 处理通用代理请求
func ProxyHandler(w http.ResponseWriter, r *http.Request) {
	// 1. 处理 CORS 预检
	utils.SetCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// 2. 参数获取与校验
	targetURLStr := r.URL.Query().Get("url")
	if targetURLStr == "" {
		HandleHttpInfo(w, r)
		return
	}

	// 3. 验证访问密码 (Bearer Token)
	if config.AccessPassword != "" {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+config.AccessPassword {
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}
	}

	targetURL, err := url.Parse(targetURLStr)
	if err != nil {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	// 4. 构建代理请求
	proxyReq, err := http.NewRequest(r.Method, targetURL.String(), r.Body)
	if err != nil {
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

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, "Proxy Error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 5. 复制目标服务器的响应头
	for k, vv := range resp.Header {
		if !excludeHeaders[strings.ToLower(k)] {
			for _, v := range vv {
				w.Header().Add(k, v)
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

		proxyOrigin := utils.GetProxyOrigin(r)
		rewriteM3u8(w, resp.Body, targetURL, proxyOrigin)
	} else {
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

// rewriteM3u8 实现流式重写，减少内存压力
func rewriteM3u8(w io.Writer, body io.Reader, baseURL *url.URL, proxyOrigin string) {
	scanner := bufio.NewScanner(body)
	basePath := baseURL.Path[:strings.LastIndex(baseURL.Path, "/")+1]

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
