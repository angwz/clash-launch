package ruleset

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Preset 是一个规则源预设（catalog/rulesources/）：官方仓库、许可、核对日期与可选的变体。
// 本项目只引用这些规则的地址，不复制、不再分发其内容（第 6.5 节）。
type Preset struct {
	ID          string            `yaml:"id" json:"id"`
	Label       map[string]string `yaml:"label" json:"label"`
	Repo        string            `yaml:"repo" json:"repo"`
	License     string            `yaml:"license" json:"license"`
	Checked     string            `yaml:"checked" json:"checked"`
	Stars       int               `yaml:"stars" json:"stars,omitempty"`
	Recommended bool              `yaml:"recommended" json:"recommended,omitempty"`
	// Rank 是默认展示顺序（小的在前，反馈 #73）：按社区使用面与口碑（GitHub 星数、能否直接组成完整分流）人工排定。
	// 用户可以拖动调整，保存在账号偏好中。
	Rank     int               `yaml:"rank" json:"rank"`
	Notes    map[string]string `yaml:"notes" json:"notes"`
	Variants []PresetVariant   `yaml:"variants" json:"variants"`
}

// PresetVariant 是预设的一个变体：一对 toml 地址、一份 ini 地址，或内联的 toml 模板。
type PresetVariant struct {
	ID           string            `yaml:"id" json:"id"`
	Label        map[string]string `yaml:"label" json:"label"`
	Groups       string            `yaml:"groups" json:"groups,omitempty"`
	Rulesets     string            `yaml:"rulesets" json:"rulesets,omitempty"`
	INI          string            `yaml:"ini" json:"ini,omitempty"`
	GroupsText   string            `yaml:"groups-text" json:"groupsText,omitempty"`
	RulesetsText string            `yaml:"rulesets-text" json:"rulesetsText,omitempty"`
}

// LoadPresets 加载全部规则源预设，按 rank 排序（相同时按 ID）。
func LoadPresets(fsys fs.FS) ([]Preset, error) {
	entries, err := fs.ReadDir(fsys, "rulesources")
	if err != nil {
		return nil, err
	}
	var out []Preset
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := fs.ReadFile(fsys, path.Join("rulesources", e.Name()))
		if err != nil {
			return nil, err
		}
		var p Preset
		dec := yaml.NewDecoder(strings.NewReader(string(b)))
		dec.KnownFields(true)
		if err := dec.Decode(&p); err != nil {
			return nil, fmt.Errorf("ruleset: 解析预设 %s: %w", e.Name(), err)
		}
		if p.ID+".yaml" != e.Name() || p.Repo == "" || p.License == "" || p.Checked == "" || p.Label["en"] == "" || p.Label["zh"] == "" || len(p.Variants) == 0 || p.Rank <= 0 {
			return nil, fmt.Errorf("ruleset: 预设 %s 缺少 id、repo、license、checked、rank、中英文名称或变体", e.Name())
		}
		for _, v := range p.Variants {
			urls := v.INI != "" || (v.Groups != "" && v.Rulesets != "")
			texts := v.GroupsText != "" && v.RulesetsText != ""
			if urls == texts {
				return nil, fmt.Errorf("ruleset: 预设 %s 的变体 %s 必须是 ini 地址、一对 toml 地址或内联模板之一", p.ID, v.ID)
			}
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Rank != out[j].Rank {
			return out[i].Rank < out[j].Rank
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
