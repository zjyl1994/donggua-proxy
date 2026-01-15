package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/zjyl1994/donggua-proxy/utils"
)

// HandleTMDBUsage 返回 TMDB 使用说明
func HandleTMDBUsage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"message": "TMDB Proxy is running",
		"usage": map[string]string{
			"api":   "/api/3/movie/popular?api_key=YOUR_KEY&language=zh-CN",
			"image": "/t/p/w500/YOUR_IMAGE_PATH.jpg",
		},
	})
}

// TmdbHandler 处理 TMDB 请求
func TmdbHandler(w http.ResponseWriter, r *http.Request) {
	if utils.HandleCORS(w, r) {
		return
	}

	switch r.Method {
	case http.MethodGet, http.MethodHead:
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Path
	var targetURL string

	if strings.HasPrefix(path, "/api/") {
		apiPath := strings.TrimPrefix(path, "/api")
		if r.URL.RawQuery != "" {
			targetURL = "https://api.themoviedb.org" + apiPath + "?" + r.URL.RawQuery
		} else {
			targetURL = "https://api.themoviedb.org" + apiPath
		}
	} else if strings.HasPrefix(path, "/t/") {
		// 图片请求 - 代理到 image.tmdb.org
		if r.URL.RawQuery != "" {
			targetURL = "https://image.tmdb.org" + path + "?" + r.URL.RawQuery
		} else {
			targetURL = "https://image.tmdb.org" + path
		}
	} else {
		http.NotFound(w, r)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 设置伪装头信息
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", r.Header.Get("Accept"))
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "*/*")
	}

	resp, err := utils.DefaultClient.Do(req)
	if err != nil {
		utils.LogError(r, fmt.Errorf("tmdb request failed: %w", err))
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 复制目标服务器的响应头
	utils.CopyHeadersWithFilter(w, resp.Header, utils.DefaultExcludedResponseHeaders)

	// 缓存控制
	isImage := strings.HasPrefix(path, "/t/")
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if isImage {
			w.Header().Set("Cache-Control", "public, max-age=604800") // 7天
		} else {
			w.Header().Set("Cache-Control", "public, max-age=600") // 10分钟
		}
	}

	w.WriteHeader(resp.StatusCode)

	// 使用 BufferPool 优化 IO 复制
	bufPtr := utils.BufferPool.Get().(*[]byte)
	defer utils.BufferPool.Put(bufPtr)
	if _, err := io.CopyBuffer(w, resp.Body, *bufPtr); err != nil {
		utils.LogError(r, fmt.Errorf("copy response failed: %w", err))
	}
}
