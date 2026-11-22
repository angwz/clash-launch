package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/angwz/clash-launch/internal/store"
)

// 访客试用（需求 R44）：未登录的人在一份只读的示例配置上试用编辑器与分流可视化，不保存。
//
// 示例环境是独立的：临时目录里的一个 SQLite 库，只有示例用户、示例节点、示例分流规则与示例配置；
// 与正式数据库完全隔离，访客的任何请求都碰不到真实用户的数据。示例数据全部内置，不联网：
// 节点地址都是文档专用的 192.0.2.0/24，分流规则是粘贴的文本（预设的规则源需要拉取，不适合试用）。
// 不配置内核（试用不做 mihomo 检查，避免被用来消耗服务器资源）。

// Demo 是示例环境。
type Demo struct {
	Svc       *Service
	User      *store.User
	ProfileID string
	db        *store.DB
	dir       string
}

// Close 关闭示例库并删除临时目录。
func (d *Demo) Close() error {
	err := d.db.Close()
	if rerr := os.RemoveAll(d.dir); err == nil {
		err = rerr
	}
	return err
}

// 示例节点：文档专用地址与明显虚构的凭据；名称覆盖几个地区，地区识别、分组与分流图都有东西可看。
const demoNodes = `- {name: 香港 01, type: ss, server: 192.0.2.11, port: 8388, cipher: aes-128-gcm, password: example-demo}
- {name: 香港 02, type: trojan, server: 192.0.2.12, port: 443, password: example-demo, sni: example.com}
- {name: 日本 东京 01, type: ss, server: 192.0.2.21, port: 8388, cipher: aes-128-gcm, password: example-demo}
- {name: 日本 大阪 02, type: vmess, server: 192.0.2.22, port: 443, uuid: 00000000-0000-4000-8000-000000000001, alterId: 0, cipher: auto, tls: true, servername: example.com}
- {name: 新加坡 01, type: trojan, server: 192.0.2.31, port: 443, password: example-demo, sni: example.com}
- {name: 美国 洛杉矶 01, type: ss, server: 192.0.2.41, port: 8388, cipher: aes-128-gcm, password: example-demo}
- {name: 美国 圣何塞 02, type: hysteria2, server: 192.0.2.42, port: 443, password: example-demo, sni: example.com}
- {name: 台湾 01, type: ss, server: 192.0.2.51, port: 8388, cipher: aes-128-gcm, password: example-demo}
`

// 示例分流规则（subconverter 的 groups.toml 与 rulesets.toml 写法）：常见的「节点选择 + 自动选择 + 地区 + 流媒体 + 广告拦截」结构。
const demoGroups = `[[custom_groups]]
name = "🚀 节点选择"
type = "select"
rule = ["[]♻️ 自动选择", "[]🇭🇰 香港", "[]🇯🇵 日本", "[]🇺🇸 美国", "[]DIRECT", ".*"]

[[custom_groups]]
name = "♻️ 自动选择"
type = "url-test"
rule = [".*"]
url = "https://www.gstatic.com/generate_204"
interval = 300
tolerance = 50

[[custom_groups]]
name = "🇭🇰 香港"
type = "url-test"
rule = ["港|HK|Hong Kong"]
url = "https://www.gstatic.com/generate_204"
interval = 300

[[custom_groups]]
name = "🇯🇵 日本"
type = "url-test"
rule = ["日本|JP|Japan"]
url = "https://www.gstatic.com/generate_204"
interval = 300

[[custom_groups]]
name = "🇺🇸 美国"
type = "url-test"
rule = ["美国|US|United States"]
url = "https://www.gstatic.com/generate_204"
interval = 300

[[custom_groups]]
name = "🎬 流媒体"
type = "select"
rule = ["[]🚀 节点选择", "[]🇭🇰 香港", "[]🇯🇵 日本", "[]🇺🇸 美国"]

[[custom_groups]]
name = "🛑 广告拦截"
type = "select"
rule = ["[]REJECT", "[]DIRECT"]

[[custom_groups]]
name = "🐟 漏网之鱼"
type = "select"
rule = ["[]🚀 节点选择", "[]DIRECT"]
`

const demoRulesets = `[[rulesets]]
group = "DIRECT"
ruleset = "[]GEOSITE,private"

[[rulesets]]
group = "🛑 广告拦截"
ruleset = "[]GEOSITE,category-ads-all"

[[rulesets]]
group = "🎬 流媒体"
ruleset = "[]GEOSITE,netflix"

[[rulesets]]
group = "🎬 流媒体"
ruleset = "[]GEOSITE,youtube"

[[rulesets]]
group = "🚀 节点选择"
ruleset = "[]GEOSITE,github"

[[rulesets]]
group = "🚀 节点选择"
ruleset = "[]GEOSITE,geolocation-!cn"

[[rulesets]]
group = "DIRECT"
ruleset = "[]GEOSITE,cn"

[[rulesets]]
group = "DIRECT"
ruleset = "[]GEOIP,CN"

[[rulesets]]
group = "🐟 漏网之鱼"
ruleset = "[]MATCH"
`

// NewDemo 在 dir（应为空的临时目录）中建立示例环境，复用 base 的字段目录、客户端预设、节点工具与地理数据。
func NewDemo(ctx context.Context, base *Service, dir string) (*Demo, error) {
	keys, _, err := store.LoadKeyring(dir, func(string) string { return "" })
	if err != nil {
		return nil, fmt.Errorf("示例环境的密钥：%w", err)
	}
	db, err := store.Open(ctx, "sqlite:"+filepath.Join(dir, "demo.db"), keys)
	if err != nil {
		return nil, fmt.Errorf("示例环境的数据库：%w", err)
	}
	d := &Demo{db: db, dir: dir}
	fail := func(err error) (*Demo, error) {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Migrate(ctx, ""); err != nil {
		return fail(fmt.Errorf("示例环境的迁移：%w", err))
	}
	d.Svc = &Service{Store: db, Catalog: base.Catalog, CatalogDigest: base.CatalogDigest, Presets: base.Presets, Tools: base.Tools,
		Log: base.Log, Policy: base.Policy, Now: base.Now, Geo: base.Geo, Trace: base.Trace, PublicURL: base.PublicURL}
	u := &store.User{Username: "demo", PasswordHash: "!", Role: "user", Disabled: true} // 不能登录：密码哈希无效且已停用
	if err := db.Users().Create(ctx, u); err != nil {
		return fail(err)
	}
	src := &store.Source{Name: "示例节点", Kind: "batch", Content: []byte(demoNodes), Processing: []byte("{}")}
	if err := db.Sources().Create(ctx, u.ID, src); err != nil {
		return fail(err)
	}
	rs := &store.RuleSource{Name: "示例分流规则", Kind: "toml", GroupsText: demoGroups, RulesetsText: demoRulesets}
	if err := db.RuleSources().Create(ctx, u.ID, rs); err != nil {
		return fail(err)
	}
	doc := "clash-launch/profile: v1\nname: 示例配置\ntarget: {pack: mihomo, preset: mihomo, core: \"" + base.Catalog.Pack.CoreMax + "\"}\n" +
		"sources:\n  - ref: " + src.ID + "\nrules: {ref: " + rs.ID + "}\n" +
		"form:\n  mixed-port: 7890\n  allow-lan: false\n  mode: rule\n  log-level: info\n  dns:\n    enable: true\n    enhanced-mode: fake-ip\n"
	p := &store.Profile{Name: "示例配置", Doc: []byte(doc)}
	if err := db.Profiles().Create(ctx, u.ID, p, ""); err != nil {
		return fail(err)
	}
	d.User, d.ProfileID = u, p.ID
	return d, nil
}
