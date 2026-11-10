# syntax=docker/dockerfile:1
# Clash, Launch! 镜像（第 18.1 节）：前端构建 → Go 静态二进制 + 固定版本的 mihomo 与地理数据 → distroless。
# 多架构：linux/amd64、linux/arm64、linux/arm/v7（docker buildx build --platform …）。

FROM --platform=$BUILDPLATFORM node:26.13.0-bookworm-slim AS web
WORKDIR /src/web
COPY web/ ./
# M4 之前 web/ 中还没有 package.json：此时只保留空的 dist，服务端只提供 API。
RUN if [ -f package.json ]; then npm ci && npm run build; else mkdir -p dist; fi

FROM --platform=$BUILDPLATFORM golang:1.27.3-bookworm AS build
ARG TARGETOS TARGETARCH TARGETVARIANT
ARG VERSION=1.0.0-dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
# 交叉编译（CGO_ENABLED=0：SQLite 驱动为纯 Go）。
RUN GOARM="${TARGETVARIANT#v}" CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" \
    go build -trimpath -ldflags "-s -w -X github.com/angwz/clash-launch/internal/version.Version=${VERSION}" \
    -o /out/clash-launch ./cmd/clash-launch
# 按 tools.lock.json 下载并校验目标架构的 mihomo 与地理数据（toolsync 在构建机上运行）。
RUN case "$TARGETARCH$TARGETVARIANT" in \
      amd64) p=linux-amd64 ;; arm64) p=linux-arm64 ;; armv7) p=linux-arm-v7 ;; \
      *) echo "unsupported platform $TARGETARCH$TARGETVARIANT" && exit 1 ;; \
    esac && go run ./tools/toolsync -platform "$p" -out /out/mihomo && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/clash-launch /clash-launch
COPY --from=build /out/mihomo/mihomo /opt/mihomo/mihomo
COPY --from=build /out/mihomo/geodata /opt/mihomo/geodata
COPY --from=build --chown=65532:65532 /out/data /data
ENV CLASH_LAUNCH_LISTEN=:20230 \
    CLASH_LAUNCH_DATA_DIR=/data \
    CLASH_LAUNCH_MIHOMO=/opt/mihomo/mihomo \
    CLASH_LAUNCH_GEODATA=/opt/mihomo/geodata
USER 65532:65532
VOLUME /data
EXPOSE 20230
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/clash-launch", "healthcheck"]
ENTRYPOINT ["/clash-launch"]
CMD ["serve"]
