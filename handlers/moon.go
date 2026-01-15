package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
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

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(moonUrl)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching URL: %s", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("Error: remote server returned %d", resp.StatusCode), http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading body: %s", err), http.StatusInternalServerError)
		return
	}

	var moonSub MoonSub
	if err := json.Unmarshal(body, &moonSub); err != nil {
		http.Error(w, fmt.Sprintf("Error unmarshaling JSON: %s", err), http.StatusInternalServerError)
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
		http.Error(w, fmt.Sprintf("Error encoding response: %s", err), http.StatusInternalServerError)
	}
}
