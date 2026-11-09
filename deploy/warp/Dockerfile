# WARP 出口边车（第 14.6 节）：Cloudflare 官方 Linux 客户端的代理模式，经 socat 暴露为 SOCKS5 :1080。
# 只作用于本服务：代理模式不修改路由表；只有 clash-launch 主动交给它的连接才经过它。
# 官方客户端只提供 amd64 与 arm64；arm/v7 主机请改用其他出口（CLASH_LAUNCH_EGRESS_PROXY）。
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl gnupg socat \
 && curl -fsSL https://pkg.cloudflareclient.com/pubkey.gpg | gpg --dearmor -o /usr/share/keyrings/cloudflare-warp-archive-keyring.gpg \
 && echo "deb [signed-by=/usr/share/keyrings/cloudflare-warp-archive-keyring.gpg] https://pkg.cloudflareclient.com/ bookworm main" > /etc/apt/sources.list.d/cloudflare-client.list \
 && apt-get update && apt-get install -y --no-install-recommends cloudflare-warp \
 && apt-get purge -y gnupg && apt-get autoremove -y && rm -rf /var/lib/apt/lists/*
COPY entrypoint.sh /entrypoint.sh
EXPOSE 1080
HEALTHCHECK --interval=60s --timeout=10s CMD warp-cli --accept-tos status | grep -q Connected || exit 1
ENTRYPOINT ["/bin/sh", "/entrypoint.sh"]
