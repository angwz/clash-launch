# 开发环境

## 工具

| 工具 | 版本 | 用途 |
| --- | --- | --- |
| Go | 1.27.3 | 后端、工具（`go.mod` 固定 `toolchain go1.27.3`） |
| Node.js | 26.13.0 LTS | 前端（`web/`） |
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

### 本机 PostgreSQL（可选）

存储与 API 测试设置 `CLASH_LAUNCH_TEST_POSTGRES` 时在 PostgreSQL 上运行（每个测试新建一个空库，与 CI 的 `postgres` 任务相同）。本机没有 PostgreSQL 时可以用 EnterpriseDB 的便携包，不装系统服务：

```sh
# 解压 postgresql-17.x-windows-x64-binaries.zip 到 .cache/toolchain/pg17
# 包内不带 VC++ 运行库：没有安装 Visual C++ Redistributable 的机器，把 vcruntime140.dll、vcruntime140_1.dll、msvcp140.dll
# 复制到 pgsql/bin（包内 pgAdmin 4/python 与 Edge 的安装目录里都有微软签名的副本）
B=.cache/toolchain/pg17/pgsql/bin
$B/initdb -D .cache/pgdata -U postgres --pwfile=<含示例密码的临时文件> -A scram-sha-256 -E UTF8 --locale=C
printf "listen_addresses = '127.0.0.1'
port = 55432
" >> .cache/pgdata/postgresql.conf
$B/pg_ctl -D .cache/pgdata -l .cache/pgdata/server.log start      # 停止：pg_ctl -D .cache/pgdata stop
export CLASH_LAUNCH_TEST_POSTGRES="postgres://postgres:<密码>@127.0.0.1:55432/postgres?sslmode=disable"
go test ./internal/store/... ./internal/api/
```

只监听 127.0.0.1；数据目录在被忽略的 `.cache/pgdata`，删掉即清除。

## 前端

```sh
cd web
npm ci                      # 按 package-lock.json 安装；版本策略见 decisions/0004
npm run dev                 # Vite 开发服务器（:5173），/api、/s、/p 代理到 127.0.0.1:18080 的后端
npm run check               # svelte-check（TypeScript strict，警告即失败）
npm test                    # Vitest：方案文档、差异、高亮、消息完整性、样式约束
npm run build && npm run budget   # 构建到 web/dist（由 Go 嵌入）并检查首屏 150 KiB 预算
npm run e2e                 # Playwright：真实二进制 + 临时数据目录；默认使用本机 Edge
npm run gen:api             # api/openapi.yaml 改动后重新生成 src/lib/api/schema.d.ts
```

端到端测试默认用本机已安装的 Edge（`E2E_CHANNEL=msedge`），不下载浏览器；设为 `chrome` 或空字符串可改用其他浏览器。其中 `e2e/a11y.spec.ts` 用 axe 在亮色与暗色下检查主要页面的 WCAG 2.1 A/AA 问题，有任何问题即失败，没有豁免。前端改动后需要重新 `npm run build` 再构建 Go 二进制，嵌入的才是新版本。

## 内核与地理数据

```sh
go run ./tools/toolsync          # 按 tools.lock.json 下载 mihomo 与地理数据到 .cache/tools/，校验 SHA-256
```

之后 `go test -tags kernel ./internal/kernel/` 与 `clash-launch render -check` 可用；也可以用环境变量 `CLASH_LAUNCH_MIHOMO`、`CLASH_LAUNCH_GEODATA` 指定其他位置。

## 参考资料

只读，放在 `.cache/ref/`（不入库、不打包）：

```sh
git clone https://github.com/angwz/domainrouter_v2 .cache/ref/domainrouter_v2 && git -C .cache/ref/domainrouter_v2 checkout 2c812f9da217a12d16507c9af8d0b89a5f413e4c
git clone --depth 1 --branch v1.19.34 https://github.com/MetaCubeX/mihomo .cache/ref/mihomo
git clone --depth 1 --branch v1.19.32 https://github.com/MetaCubeX/mihomo .cache/ref/mihomo-1.19.32
git clone https://github.com/angwz/subconverter .cache/ref/subconverter && git -C .cache/ref/subconverter checkout a33d49e
git clone --depth 1 https://github.com/ACL4SSR/ACL4SSR .cache/ref/acl4ssr
git clone https://github.com/vernesong/OpenClash .cache/ref/openclash && git -C .cache/ref/openclash checkout e553e82e01c697aefbe1555261a503f425e00ccd
```

`TestScanMihomo` 在 `.cache/ref/mihomo` 存在时运行，否则跳过。
