// Package workspace 读取命令行离线渲染用的工作区文件：源与规则源的定义及其快照。
// 服务端由数据库提供同样的内容；本包只做文件读取，不联网。
package workspace

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/engine"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/profile"
)

// 工作区文件的格式：
//
//	clash-launch/workspace: v1
//	sources:
//	  - id: src_a
//	    name: 机场 A
//	    kind: subscription          # single | batch | subscription | file
//	    url: https://…              # 只在 mode: provider 时写入配置
//	    snapshot: snapshots/a.txt   # 快照文件（相对工作区文件），或用 content 直接写
//	    fetched: 2026-11-09T00:00:00Z
//	    processing: { exclude: 测试, flags: prefix }
//	rule-sources:
//	  - id: rs_domainrouter
//	    groups: rules/groups.toml
//	    rulesets: rules/rulesets.toml   # 或 ini: rules/ACL4SSR_Online.ini
type workspace struct {
	Sources []struct {
		ID         string          `yaml:"id"`
		Name       string          `yaml:"name"`
		Kind       string          `yaml:"kind"`
		URL        string          `yaml:"url"`
		Snapshot   string          `yaml:"snapshot"`
		Content    string          `yaml:"content"`
		Fetched    time.Time       `yaml:"fetched"`
		Interval   int             `yaml:"interval"`
		Processing node.Processing `yaml:"processing"`
	} `yaml:"sources"`
	RuleSources []struct {
		ID       string    `yaml:"id"`
		Groups   string    `yaml:"groups"`
		Rulesets string    `yaml:"rulesets"`
		INI      string    `yaml:"ini"`
		Fetched  time.Time `yaml:"fetched"`
	} `yaml:"rule-sources"`
}

// Build 读取工作区，按方案中源的顺序组装输入并执行生成阶段。
func Build(path string, p *profile.Profile, cat *catalog.Catalog) (*yaml.Node, []diag.Diagnostic, time.Time, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	var top map[string]any
	if err := yaml.Unmarshal(b, &top); err != nil || top["clash-launch/workspace"] != "v1" {
		return nil, nil, time.Time{}, fmt.Errorf("%s 不是 clash-launch/workspace: v1 文档", path)
	}
	var ws workspace
	dec := yaml.NewDecoder(bytesReader(b))
	if err := dec.Decode(&ws); err != nil {
		return nil, nil, time.Time{}, err
	}
	dir := filepath.Dir(path)
	read := func(rel string) ([]byte, error) {
		if rel == "" {
			return nil, nil
		}
		return os.ReadFile(filepath.Join(dir, rel))
	}
	var ds diag.List
	in := engine.BuildInput{Catalog: cat, Chains: p.Chains, EmptyGroups: p.Output.EmptyGroups}
	tools, err := node.LoadTools(catalogdata.FS)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	in.Tools = tools
	for i, ref := range p.Refs() {
		found := false
		for _, s := range ws.Sources {
			if s.ID != ref.Ref {
				continue
			}
			found = true
			content := []byte(s.Content)
			if s.Snapshot != "" {
				if content, err = read(s.Snapshot); err != nil {
					return nil, nil, time.Time{}, fmt.Errorf("源 %s 的快照: %w", s.ID, err)
				}
			}
			name := s.Name
			if name == "" {
				name = s.ID
			}
			in.Sources = append(in.Sources, engine.SourceInput{
				Ref: s.ID, Name: name, Kind: engine.SourceKind(s.Kind), Mode: ref.Mode, Content: content,
				Updated: s.Fetched, URL: s.URL, Interval: s.Interval, Processing: s.Processing,
			})
		}
		if !found {
			ds.Add(diag.ProfileSourceRef, fmt.Sprintf("sources[%d]", i), "ref", ref.Ref)
		}
	}
	if p.Rules != nil {
		found := false
		for _, r := range ws.RuleSources {
			if r.ID != p.Rules.Ref {
				continue
			}
			found = true
			ri := &engine.RulesInput{Name: r.ID, Updated: r.Fetched}
			if ri.Groups, err = read(r.Groups); err != nil {
				return nil, nil, time.Time{}, err
			}
			if ri.Rulesets, err = read(r.Rulesets); err != nil {
				return nil, nil, time.Time{}, err
			}
			if ri.INI, err = read(r.INI); err != nil {
				return nil, nil, time.Time{}, err
			}
			in.Rules = ri
		}
		if !found {
			ds.Add(diag.ProfileRuleRef, "rules", "ref", p.Rules.Ref)
		}
	}
	built := engine.Build(in)
	ds.Append(built.Diagnostics...)
	return built.Generated, ds.Items(), built.Updated, nil
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
