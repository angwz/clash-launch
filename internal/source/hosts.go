package source

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// 分流规则地址的来源限制（反馈 #72）。
//
// 规则源（groups.toml、rulesets.toml、外部配置 ini）必须由服务器拉取，没法像订阅那样交给客户端。
// 普通用户只能使用公共代码托管站与它们的 CDN 上的地址：内容公开、站点稳定，服务器不会被当作
// 访问任意网址的工具，也不会去连接用户自建的服务器。其他来源可以改为粘贴文本。管理员不受限制。
//
// 匹配规则：只接受 https；主机名精确匹配（不区分大小写），不含子域名的通配，避免把整个顶级站点的
// 用户子域名（如 *.github.io）一起放行。重定向的每一跳同样检查（例如 github.com → raw.githubusercontent.com）。

// ErrHostNotAllowed 表示地址不在允许的站点内。它同时是 ErrBlocked，按安全策略拒绝处理。
var ErrHostNotAllowed = fmt.Errorf("%w：不在允许的站点内", ErrBlocked)

// DefaultRuleHosts 是内置的允许站点：GitHub 及其原始文件与发布附件域名、jsDelivr（GitHub 的公共 CDN 镜像）、
// GitLab、Codeberg、Gitee，以及内置预设用到的 ruleset.skk.moe。
var DefaultRuleHosts = Hosts{
	"github.com", "raw.githubusercontent.com", "gist.githubusercontent.com",
	"objects.githubusercontent.com", "release-assets.githubusercontent.com",
	"cdn.jsdelivr.net", "fastly.jsdelivr.net", "gcore.jsdelivr.net", "testingcf.jsdelivr.net",
	"gitlab.com", "codeberg.org", "gitee.com",
	"ruleset.skk.moe",
}

// Hosts 是允许的主机名列表（小写）。
type Hosts []string

// ParseHosts 解析逗号分隔的主机名列表（CLASH_LAUNCH_RULE_HOSTS）；空字符串返回 nil。
func ParseHosts(s string) (Hosts, error) {
	var out Hosts
	for _, h := range strings.Split(s, ",") {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if strings.ContainsAny(h, "/:*@ ") {
			return nil, fmt.Errorf("source: 主机名 %q 无效（只写主机名，例如 raw.githubusercontent.com）", h)
		}
		out = append(out, h)
	}
	return out, nil
}

// Allows 报告 u 是否是允许站点上的 https 地址。
func (h Hosts) Allows(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, x := range h {
		if host == x {
			return true
		}
	}
	return false
}

// Check 检查地址字符串；不允许时返回带主机名的 ErrHostNotAllowed。
func (h Hosts) Check(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !h.Allows(u) {
		host := raw
		if err == nil && u.Host != "" {
			host = u.Hostname()
		}
		return fmt.Errorf("%w：%s", ErrHostNotAllowed, host)
	}
	return nil
}

type hostsKey struct{}

// withHosts 把本次拉取的站点限制放进请求上下文，供重定向检查读取。
func withHosts(ctx context.Context, h Hosts) context.Context {
	return context.WithValue(ctx, hostsKey{}, h)
}

func hostsFrom(ctx context.Context) (Hosts, bool) {
	h, ok := ctx.Value(hostsKey{}).(Hosts)
	return h, ok
}
