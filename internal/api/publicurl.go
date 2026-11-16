package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// 对外地址（需求 R9）：CLASH_LAUNCH_PUBLIC_URL 可以写多个，逗号分隔，用于同一个服务挂在多个域名的反向代理之后。
//
//   - 第一个是主地址：通行密钥的依赖方（浏览器要求它与单个域名绑定）、离线生成的内容（配置中托管节点的地址）
//     与启动日志使用它；
//   - 每个请求按 Host 选用列表中域名相同的那个，生成的链接、Cookie 的 Secure、HSTS 与 Origin 校验都以它为准；
//   - Host 不在列表中时按主地址处理：写请求的 Origin 对不上而被拒绝，生成的链接指向主地址。
//     列表因此同时是域名白名单，伪造的 Host 不会进入订阅链接。
//   - 不设置时保持原行为：按请求推断（只在直连地址是受信任代理时采信 X-Forwarded-Proto）。

// ParsePublicURLs 解析逗号分隔的对外地址：每项必须是 http 或 https、带主机名、没有路径以外的部分；去掉结尾斜杠。
// 同一主机名出现两次视为配置错误（无法决定用哪个）。
func ParsePublicURLs(v string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, raw := range strings.Split(v, ",") {
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "/")
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return nil, fmt.Errorf("「%s」不是有效的对外地址，应形如 https://cl.example.com", raw)
		}
		host := strings.ToLower(u.Host)
		if seen[host] {
			return nil, fmt.Errorf("域名 %s 重复出现", u.Host)
		}
		seen[host] = true
		out = append(out, u.Scheme+"://"+u.Host)
	}
	return out, nil
}

// publicFor 返回这个请求对应的对外地址；没有配置时返回空串。
func (s *Server) publicFor(r *http.Request) string {
	if len(s.Cfg.PublicURLs) == 0 {
		return ""
	}
	if r != nil {
		for _, p := range s.Cfg.PublicURLs {
			if u, err := url.Parse(p); err == nil && strings.EqualFold(u.Host, r.Host) {
				return p
			}
		}
	}
	return s.Cfg.PublicURLs[0]
}

// secure 报告这个请求是否经由 https 的对外地址访问（决定 Cookie 的 Secure 与名称前缀、HSTS）。
func (s *Server) secure(r *http.Request) bool {
	return strings.HasPrefix(s.publicFor(r), "https://")
}
