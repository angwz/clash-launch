package ruleset

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Descriptions 是规则集说明目录（catalog/rulesets/descriptions.yaml，routing 第 13 节第 1 步）：
// 按规则地址的正则给出常见上游规则集的用途，只收录上游文档里说明过的内容。
type Descriptions struct {
	Sources []DescriptionSource `yaml:"sources"`
}

// DescriptionSource 是一个上游：仓库、许可、核对日期与依据，以及若干条按地址匹配的说明。
type DescriptionSource struct {
	ID      string           `yaml:"id"`
	Repo    string           `yaml:"repo"`
	License string           `yaml:"license"`
	Checked string           `yaml:"checked"`
	Ref     string           `yaml:"ref"` // 依据：上游的哪份文档、哪个提交
	Sets    []DescriptionSet `yaml:"sets"`
}

// DescriptionSet 是一条说明。
type DescriptionSet struct {
	Match string `yaml:"match"`
	ZH    string `yaml:"zh"`
	EN    string `yaml:"en"`
	re    *regexp.Regexp
}

// Description 是一个地址查到的说明，附带所属上游的信息，供接口直接返回。
type Description struct {
	Source  string            `json:"source"`
	Repo    string            `json:"repo"`
	License string            `json:"license"`
	Checked string            `json:"checked"`
	Text    map[string]string `json:"text"` // en、zh
}

// LoadDescriptions 读取并校验说明目录：字段齐全、正则可编译。
func LoadDescriptions(fsys fs.FS) (*Descriptions, error) {
	b, err := fs.ReadFile(fsys, "rulesets/descriptions.yaml")
	if err != nil {
		return nil, err
	}
	var d Descriptions
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("ruleset: 解析规则集说明: %w", err)
	}
	for i := range d.Sources {
		s := &d.Sources[i]
		if s.ID == "" || s.Repo == "" || s.License == "" || s.Checked == "" || s.Ref == "" || len(s.Sets) == 0 {
			return nil, fmt.Errorf("ruleset: 规则集说明的上游 #%d（%s）缺少 id、repo、license、checked、ref 或说明", i, s.ID)
		}
		for j := range s.Sets {
			set := &s.Sets[j]
			if set.ZH == "" || set.EN == "" {
				return nil, fmt.Errorf("ruleset: 规则集说明 %s#%d 缺少中英文说明", s.ID, j)
			}
			re, err := regexp.Compile(set.Match)
			if err != nil {
				return nil, fmt.Errorf("ruleset: 规则集说明 %s#%d 的正则无效: %w", s.ID, j, err)
			}
			set.re = re
		}
	}
	return &d, nil
}

// Describe 返回地址对应的说明；没有收录时返回 nil。目录保证一个地址最多匹配一条。
func (d *Descriptions) Describe(url string) *Description {
	for _, s := range d.Sources {
		for _, set := range s.Sets {
			if set.re.MatchString(url) {
				return &Description{Source: s.ID, Repo: s.Repo, License: s.License, Checked: s.Checked, Text: map[string]string{"en": set.EN, "zh": set.ZH}}
			}
		}
	}
	return nil
}
