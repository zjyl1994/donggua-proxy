package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/zjyl1994/donggua-proxy/utils"
)

type MoonSub struct {
	ApiSite map[string]struct {
		Api  string `json:"api"`
		Name string `json:"name"`
	} `json:"api_site"`
}

type DongguaItem struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Api    string `json:"api"`
	Active bool   `json:"active"`
}

type DongguaSub struct {
	Sites []DongguaItem `json:"sites"`
}

// Moon2DongguaHandler 处理 MoonSub 到 DongguaSub 的转换
func Moon2DongguaHandler(w http.ResponseWriter, r *http.Request) {
	moonUrl := r.URL.Query().Get("url")
	if moonUrl == "" {
		http.Error(w, "Missing 'url' parameter", http.StatusBadRequest)
		return
	}

	targetURL, err := url.Parse(moonUrl)
	if err != nil {
		utils.LogError(r, fmt.Errorf("invalid url %s: %w", moonUrl, err))
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	if err := utils.ValidateTargetURL(targetURL); err != nil {
		utils.LogError(r, fmt.Errorf("ssrf check failed for %s: %w", moonUrl, err))
		http.Error(w, "Forbidden URL", http.StatusForbidden)
		return
	}

	resp, err := utils.DefaultClient.Get(moonUrl)
	if err != nil {
		utils.LogError(r, fmt.Errorf("failed to fetch moon sub: %w", err))
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		utils.LogError(r, fmt.Errorf("remote server returned %d", resp.StatusCode))
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	// 限制读取最大 1MB 防止内存耗尽
	limitedReader := io.LimitReader(resp.Body, 1*1024*1024)

	var moonSub MoonSub
	if err := json.NewDecoder(limitedReader).Decode(&moonSub); err != nil {
		utils.LogError(r, fmt.Errorf("error decoding JSON: %w", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var dongguaSub DongguaSub
	for key, site := range moonSub.ApiSite {
		dongguaSub.Sites = append(dongguaSub.Sites, DongguaItem{
			Key:    key,
			Name:   site.Name,
			Api:    site.Api,
			Active: true,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(dongguaSub); err != nil {
		utils.LogError(r, fmt.Errorf("failed to encode response: %w", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
