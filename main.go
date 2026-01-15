package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zjyl1994/donggua-proxy/config"
	"github.com/zjyl1994/donggua-proxy/handlers"
	"github.com/zjyl1994/donggua-proxy/middleware"
	"golang.org/x/time/rate"
)

func main() {
	// TMDB 代理路由
	http.HandleFunc("/api/", handlers.TmdbAPIHandler)
	http.HandleFunc("/t/", handlers.TmdbImageHandler)

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

	// 设置限流器: 从环境变量读取配置 (默认 50/100)
	limiter := middleware.NewIPRateLimiter(rate.Limit(config.RateLimit), config.BurstLimit)
	limiter.EnableTrustedProxies(config.TrustProxy, config.TrustedProxyCIDRs)

	server := &http.Server{
		Addr:              config.ListenAddr,
		Handler:           limiter.LimitMiddleware(http.DefaultServeMux),
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          log.New(os.Stderr, "", log.LstdFlags),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}
}
