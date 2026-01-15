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

func TmdbAPIHandler(w http.ResponseWriter, r *http.Request) {
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
	if path == "/api" || path == "/api/" {
		HandleTMDBUsage(w)
		return
	}

	apiPath := strings.TrimPrefix(path, "/api")
	targetURL := "https://api.themoviedb.org" + apiPath
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	proxyTMDB(w, r, targetURL, false)
}

func TmdbImageHandler(w http.ResponseWriter, r *http.Request) {
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
	if path == "/t" || path == "/t/" {
		http.NotFound(w, r)
		return
	}

	targetURL := "https://image.tmdb.org" + path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	proxyTMDB(w, r, targetURL, true)
}

func proxyTMDB(w http.ResponseWriter, r *http.Request, targetURL string, isImage bool) {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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

	utils.CopyHeadersWithFilter(w, resp.Header, utils.DefaultExcludedResponseHeaders)

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
