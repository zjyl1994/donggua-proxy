package config

import "donggua-proxy/utils"

var (
	ListenAddr     = utils.GetEnv("LISTEN_ADDR", ":8080")
	AccessPassword = utils.GetEnv("PROXY_PASSWORD", "")
)
