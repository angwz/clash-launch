# 反向代理

服务监听 HTTP，HTTPS 由前面的反向代理提供。订阅链接里带着长期有效的令牌，**必须走 HTTPS**：明文 HTTP 下，任何能看到流量的人都能复制这个令牌，拿到你的完整配置（节点、凭据）。

服务自身仍按公网标准防护（读写超时、请求头与请求体上限、限流、安全响应头），不依赖反向代理兜底；但只有把代理的地址加入 `CLASH_LAUNCH_TRUSTED_PROXIES`，服务才会采信 `X-Forwarded-For` 与 `X-Forwarded-Proto`，限流与登录记录才能看到真实的客户端地址。

## Caddy

```caddy
cl.example.com {
	encode zstd gzip
	reverse_proxy 127.0.0.1:20230
}
```

Caddy 自动申请证书并设置 `X-Forwarded-*`。服务与 Caddy 在同一台机器上时，`CLASH_LAUNCH_TRUSTED_PROXIES=127.0.0.1`。

## Nginx

```nginx
server {
    listen 443 ssl;
    http2 on;
    server_name cl.example.com;

    ssl_certificate     /etc/letsencrypt/live/cl.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/cl.example.com/privkey.pem;

    client_max_body_size 12m;           # 节点原文与导入的上限是 10 MiB

    location / {
        proxy_pass http://127.0.0.1:20230;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 60s;
    }

    # 访问日志中不记录订阅令牌（服务自身的日志只记前 6 个字符）
    location ~ ^/(s|p|r)/ {
        access_log off;
        proxy_pass http://127.0.0.1:20230;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

server {
    listen 80;
    server_name cl.example.com;
    return 301 https://$host$request_uri;
}
```

服务运行在 compose 中、Nginx 在宿主机上时，代理的来源地址是 compose 网络的网关，把该网段（如 `172.16.0.0/12`）加入 `CLASH_LAUNCH_TRUSTED_PROXIES`。

## 检查

```sh
curl -sI https://cl.example.com/healthz | grep -iE 'strict-transport|content-security'
```

`CLASH_LAUNCH_PUBLIC_URL` 以 `https://` 开头时，服务发送 HSTS，会话 Cookie 带 `Secure` 与 `__Host-` 前缀。

## 多个域名

同一个服务可以挂在多个域名的反向代理之后。把这些地址都写进 `CLASH_LAUNCH_PUBLIC_URL`，用逗号分隔，第一个为主地址：

```yaml
CLASH_LAUNCH_PUBLIC_URL: https://cl.example.com,https://cl2.example.com
```

- 每个请求按反代传来的 `Host` 选用同域名的那个地址：页面上生成的订阅链接、短链接与邀请链接都指向你正在访问的域名，Cookie 的 `Secure` 与 HSTS 也按它决定。因此每个反代都要传原始的 `Host`（上面的 `proxy_set_header Host $host;`，Caddy 默认如此）。
- 列表同时是白名单：经未列出的域名访问时，链接指向主地址，登录等写操作会被拒绝。
- 各域名的登录状态互相独立（Cookie 按域名保存），需要分别登录。
- 通行密钥由浏览器绑定到单个域名，只在主地址上可用。
- 配置中「交给客户端」的手动节点地址在生成配置时写入，使用主地址。
