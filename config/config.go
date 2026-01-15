package config

import "github.com/zjyl1994/donggua-proxy/utils"

var (
	ListenAddr     = utils.GetEnv("LISTEN_ADDR", ":8080")
	AccessPassword = utils.GetEnv("PROXY_PASSWORD", "")

	TrustProxy        = utils.GetEnvBool("TRUST_PROXY", false)
	TrustedProxyCIDRs = utils.GetEnv("TRUSTED_PROXY_CIDRS", "")

	// RateLimit 每秒请求数限制 (默认 50)
	RateLimit = utils.GetEnvInt("RATE_LIMIT", 50)
	// BurstLimit 突发请求数限制 (默认 100)
	BurstLimit = utils.GetEnvInt("BURST_LIMIT", 100)
)
