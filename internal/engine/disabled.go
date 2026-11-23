package engine

import (
	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// 未启用的分节整段不写入配置（反馈 #58）。
//
// 依据 mihomo v1.19.34 源码（hub/executor/executor.go、config/config.go）：下列顶层分节的 enable 为 false
// （或未写，默认 false）时，分节中的其他字段都不生效，省略整段与写上 enable: false 行为相同：
//
//	tun          updateTun 只在 enable 时启动监听
//	sniffer      updateSniffer 只在 enable 时启用
//	ntp          updateNTP 只在 enable 时同步
//	tuic-server  只在 enable 时启动
//	iptables     只在 enable 时设置
//	dns          updateDNS 在 !enable 时清空解析器；唯一例外是 parseTun 读取 fake-ip-range 作为
//	             TUN 的地址段，所以 TUN 开启且写了 fake-ip-range(6) 时 dns 整段保留。
//
// 移除的分节交给渲染器写成预览中的注释，并报告 render.disabled_section。
var switchedSections = []string{"dns", "tun", "sniffer", "ntp", "tuic-server", "iptables"}

// dnsKeptForTun 是 DNS 关闭、TUN 开启时仍然生效的 DNS 字段。
var dnsKeptForTun = []string{"fake-ip-range", "fake-ip-range6"}

func hasAny(m *yaml.Node, keys []string) bool {
	for _, k := range keys {
		if yamlx.Get(m, k) != nil {
			return true
		}
	}
	return false
}

func enabled(m *yaml.Node) bool {
	v := yamlx.Get(m, "enable")
	return v != nil && v.Kind == yaml.ScalarNode && v.Value == "true"
}

// pruneDisabled 从 root 中移除未启用的分节，返回移除的部分（键 → 分节）。
func pruneDisabled(root *yaml.Node, out *diag.List) map[string]*yaml.Node {
	removed := map[string]*yaml.Node{}
	tunOn := false
	if t := yamlx.Get(root, "tun"); t != nil && t.Kind == yaml.MappingNode {
		tunOn = enabled(t)
	}
	for _, key := range switchedSections {
		sec := yamlx.Get(root, key)
		if sec == nil || sec.Kind != yaml.MappingNode || enabled(sec) {
			continue
		}
		if key == "dns" && tunOn && hasAny(sec, dnsKeptForTun) {
			// TUN 读取自定义的 fake-ip-range：整段保留，避免同一个键既有生效部分又有注释部分
			continue
		}
		yamlx.Delete(root, key)
		removed[key] = sec
		out.Add(diag.RenderDisabledSection, key, "section", key)
	}
	return removed
}
