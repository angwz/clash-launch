# 开发环境

## 工具

| 工具 | 版本 | 用途 |
| --- | --- | --- |
| Go | 1.27.3 | 后端、工具（`go.mod` 固定 `toolchain go1.27.3`） |
| Node.js | 26.13.0 LTS | 前端（M4 起） |
| git | 任意近期版本 | — |
| Docker | 任意近期版本 | 镜像构建与运行验证（M3 起）；不可用时记录为未验证 |

本机没有安装 Go 或 Node.js 时，可以把官方便携包解压到被忽略的 `.cache/toolchain/`，不改动系统环境：

```sh
# Windows（PowerShell 或 Git Bash 均可），以 Go 为例；下载后与 go.dev/dl 公布的 SHA-256 核对
curl -LO https://go.dev/dl/go1.27.3.windows-amd64.zip && sha256sum go1.27.3.windows-amd64.zip
unzip -q go1.27.3.windows-amd64.zip -d .cache/toolchain
export PATH="$PWD/.cache/toolchain/go/bin:$PATH" GOPATH="$PWD/.cache/gopath" GOCACHE="$PWD/.cache/gocache"
```

`staticcheck` 与 `golangci-lint` 用 `go install` 安装到 `GOPATH/bin`。

## 内核与地理数据

```sh
go run ./tools/toolsync          # 按 tools.lock.json 下载 mihomo 与地理数据到 .cache/tools/，校验 SHA-256
```

之后 `go test -tags kernel ./internal/kernel/` 与 `clash-launch render -check` 可用；也可以用环境变量 `CLASH_LAUNCH_MIHOMO`、`CLASH_LAUNCH_GEODATA` 指定其他位置。

## 参考资料

只读，放在 `.cache/ref/`（不入库、不打包）：

```sh
git clone https://github.com/angwz/domainrouter_v2 .cache/ref/domainrouter_v2 && git -C .cache/ref/domainrouter_v2 checkout 2c812f9da217a12d16507c9af8d0b89a5f413e4c
git clone --depth 1 --branch v1.19.33 https://github.com/MetaCubeX/mihomo .cache/ref/mihomo
git clone --depth 1 --branch v1.19.32 https://github.com/MetaCubeX/mihomo .cache/ref/mihomo-1.19.32
git clone https://github.com/angwz/subconverter .cache/ref/subconverter && git -C .cache/ref/subconverter checkout a33d49e
git clone --depth 1 https://github.com/ACL4SSR/ACL4SSR .cache/ref/acl4ssr
```

`TestScanMihomo` 在 `.cache/ref/mihomo` 存在时运行，否则跳过。
