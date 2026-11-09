#!/bin/sh
# 启动 warp-svc，首次运行时注册设备（接受 Cloudflare 的服务条款），切换到代理模式并连接，
# 然后用 socat 把只监听本机的 127.0.0.1:40000 暴露为 0.0.0.0:1080（仅限 compose 内部网络）。
set -eu
warp-svc >/var/log/warp-svc.log 2>&1 &
i=0
until warp-cli --accept-tos status >/dev/null 2>&1; do
  i=$((i + 1)); [ "$i" -gt 60 ] && echo "warp-svc 未就绪" && exit 1; sleep 1
done
warp-cli --accept-tos registration show >/dev/null 2>&1 || warp-cli --accept-tos registration new
warp-cli --accept-tos mode proxy
warp-cli --accept-tos proxy port 40000
warp-cli --accept-tos connect
exec socat TCP-LISTEN:1080,fork,reuseaddr TCP:127.0.0.1:40000
