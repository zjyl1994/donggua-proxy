# Donggua-Proxy

Go 实现的多合一 DongguaTV 代理
结合 https://github.com/EdNovas/dongguaTV 这个项目使用

# 使用
在DongguaTV实例的环境变量中设置 TMDB_PROXY_URL 和 CORS_PROXY_URL 指向部署的实例即可。
(替换proxy.example.com为你部署的实例地址)
```
TMDB_PROXY_URL=http://proxy.example.com
CORS_PROXY_URL=http://proxy.example.com
```

# 配置
本服务支持通过环境变量进行配置：

| 环境变量 | 说明 | 默认值 |
| :--- | :--- | :--- |
| `LISTEN_ADDR` | 服务监听地址 | `:8080` |
| `PROXY_PASSWORD` | 访问密码，和你 DongguaTV 中设置的保持一致 | (空) |
| `TRUST_PROXY` | 是否信任上游代理 | `false` |
| `TRUSTED_PROXY_CIDRS` | 信任的代理 IP 网段 (CIDR)，多个用逗号分隔 | (空) |
| `RATE_LIMIT` | 每秒请求数限制 | `50` |
| `BURST_LIMIT` | 突发请求数限制 | `100` |


# Systemd Unit
```
[Unit]
Description=DongguaTV Proxy Service
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/dgproxy
Restart=always
Environment="LISTEN_ADDR=127.0.0.1:8080"
Environment="TRUST_PROXY=on"
```

# Caddy 反代
建议配置以下 Caddyfile 进行反代：
```caddyfile
proxy.example.com {
    reverse_proxy http://127.0.0.1:8080 {
        header_up X-Real-IP {remote_host}
    }
}
```
