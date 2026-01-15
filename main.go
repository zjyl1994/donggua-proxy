package main

import (
	"fmt"
	"net/http"
	"time"

	"donggua-proxy/config"
	"donggua-proxy/handlers"
)

func main() {
	// TMDB 代理路由
	http.HandleFunc("/api/", handlers.TmdbHandler)
	http.HandleFunc("/t/", handlers.TmdbHandler)

	// Moon2Donggua 转换路由
	http.HandleFunc("/sub/moon2donggua", handlers.Moon2DongguaHandler)

	// 通用代理路由 (作为默认 fallback)
	http.HandleFunc("/", handlers.ProxyHandler)

	// 健康检查接口
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	fmt.Printf("DongguaTV Proxy is running on port %s\n", config.ListenAddr)
	server := &http.Server{
		Addr:         config.ListenAddr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		panic(err)
	}
}
