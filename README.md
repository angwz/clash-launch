# Clash, Launch!

一个由字段目录驱动的 Clash（mihomo）配置生成器，自托管、带网页界面。在网页里添加节点订阅、选择规则源（如 [domainrouter_v2](https://github.com/angwz/domainrouter_v2)、ACL4SSR），按官方文档逐字段设置，得到一条长期有效的订阅链接。每次生成的 YAML 都经过格式自检和 `mihomo -t` 检查。

```text
节点订阅 · 批量节点 ─┐
规则源（toml / ini）─┼─▶ 表单 ─▶ 原始编辑 ─▶ 目标适配 ─▶ mihomo -t ─▶ 订阅链接 /s/<令牌>
```

## 特点

- **字段即目录。** mihomo v1.19.33 的全部配置字段（2075 项，含 listeners、xhttp、AmneziaWG 等）都能在表单中设置，附中英文说明、默认值与官方文档链接。
- **格式与官方一致。** 字段顺序、缩进遵循官方示例；节点可选单行或多行写法；预览与订阅输出逐字节一致。
- **后备隐藏能源。** 原始编辑入口可以写入任意字段，包括目录里还没有的新字段。
- **有序订阅与地区识别。** 多个订阅按顺序编号，兼容 subconverter 的 `!!GROUPID=`；地区识别会忽略「广港」这类线路词。
- **目标客户端。** 内置 mihomo 通用与 OpenClash 预设，按对应内核检查。
- **自托管。** 单个 Docker 容器（amd64、arm64）；凭据加密存储；拉取订阅带 SSRF 防护；运行时不调用任何第三方服务。

## 部署

需要一台装好 Docker 的服务器。

```sh
mkdir clash-launch && cd clash-launch
curl -fsSLO https://raw.githubusercontent.com/angwz/clash-launch/main/compose.yaml
docker compose up -d
```

打开 `http://<服务器 IP>:20230`，第一个访问的人会进入「初始设置」并成为管理员。**启动后请尽快完成这一步**，之后这个入口自动关闭。

### 更新

每次推送到 `main`，GitHub Actions 会自动构建新镜像并发布到 `ghcr.io/angwz/clash-launch:latest`。在服务器上：

```sh
docker compose pull && docker compose up -d
```

数据默认保存在名为 `data` 的 Docker 卷中；想放在当前目录方便查看和打包，把 `compose.yaml` 中的 `- data:/data` 换成注释里的 `- ./data:/data` 即可，不需要手动改权限。两种方式更新都不会丢数据；启动时会自动迁移数据库，并在迁移前备份。想固定版本时，把 `compose.yaml` 中的 `:latest` 改为具体版本号（如 `:1.0.0`）。

更多配置项、备份与迁移见 [docs/guide/deploy.md](docs/guide/deploy.md)。

## 安全

默认配置为了方便试用，使用**明文 HTTP**，端口对**所有 IP** 开放。这意味着：

| 不加 HTTPS 时 | 会怎样 |
| --- | --- |
| 登录 | 用户名与口令以明文经过网络，同一网络中的人或中间节点可以看到 |
| 订阅链接 | 链接里的令牌以明文传输；拿到链接的人可以下载你的完整配置，包括节点地址与凭据 |
| 会话 | 登录 Cookie 无法使用 `Secure` 标记，可能被截获后冒用 |

建议：

1. **试用**：明文 HTTP 可以接受，但不要填入正在使用的真实订阅，或者只在局域网内访问。
2. **正式使用**：在前面加一个 HTTPS 反向代理（Caddy 可以自动申请证书，见 [reverse-proxy](docs/guide/reverse-proxy.md)），并设置 `CLASH_LAUNCH_PUBLIC_URL` 为 `https://你的域名`。反代与服务在同一台机器时，可以把 `compose.yaml` 的端口改为 `127.0.0.1:20230:20230`，让外部只能经 HTTPS 访问。
3. **加上 HTTPS 后修改口令**，并在「订阅链接」页**重置**之前的链接：明文期间用过的口令与令牌都应视为已泄露。

口令要求至少 5 位、包含 2 类字符（少于 8 位时其中一类必须是特殊字符）；连续输错会触发逐次加长的等待，同一 IP 的失败次数也有上限。

## 开发

本地开发环境、测试命令与工具链见 [docs/development/setup.md](docs/development/setup.md)；文档入口在 [docs/README.md](docs/README.md)。

## 许可

[GPL-3.0-or-later](LICENSE)。第三方组件与许可见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
