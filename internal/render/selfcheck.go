package render

import (
	"bytes"
	"errors"
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// ErrSelfCheck 表示格式自检失败：渲染器输出的文本与渲染前的树不一致。这是引擎缺陷。
var ErrSelfCheck = errors.New("render: 格式自检失败")

// SelfCheck 执行第 11.2 节的格式自检：
//  1. 重新解析 text，与 want 逐节点比较种类、标签、值与顺序；
//  2. 检查 YAML 层面的风险：重复键、在 YAML 1.1 解析器中有歧义的普通标量、制表符缩进、CRLF。
func SelfCheck(text []byte, want *yaml.Node, out *diag.List) error {
	fail := func(path, reason string) error {
		out.Add(diag.RenderSelfCheck, path, "path", path, "reason", reason)
		return fmt.Errorf("%w：%s：%s", ErrSelfCheck, path, reason)
	}
	if bytes.Contains(text, []byte("\r")) {
		return fail("", "output contains CR")
	}
	if len(text) == 0 || text[len(text)-1] != '\n' || bytes.HasSuffix(text, []byte("\n\n")) {
		return fail("", "output must end with exactly one newline")
	}
	for i, line := range bytes.Split(text, []byte("\n")) {
		trimmed := bytes.TrimLeft(line, " ")
		if bytes.HasPrefix(trimmed, []byte("\t")) {
			return fail(fmt.Sprintf("line %d", i+1), "tab indentation")
		}
	}
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(text))
	if err := dec.Decode(&doc); err != nil {
		return fail("", "output does not parse: "+err.Error())
	}
	if len(doc.Content) != 1 {
		return fail("", "output is not a single document")
	}
	got := doc.Content[0]
	var lst diag.List
	if _, err := yamlx.Parse(text, yamlx.Limits{}, "", &lst); err != nil {
		return fail("", "duplicate keys or invalid structure in output")
	}
	return compare(got, want, "", fail)
}

func compare(got, want *yaml.Node, path string, fail func(string, string) error) error {
	if got.Kind == yaml.AliasNode || got.Anchor != "" {
		return fail(path, "output contains anchors or aliases")
	}
	if got.Kind != want.Kind {
		return fail(path, fmt.Sprintf("kind %s, want %s", yamlx.KindName(got), yamlx.KindName(want)))
	}
	switch want.Kind {
	case yaml.ScalarNode:
		gt, wt := got.ShortTag(), want.ShortTag()
		if gt != wt {
			return fail(path, fmt.Sprintf("tag %s, want %s (value %q)", gt, wt, want.Value))
		}
		if wt != "!!null" && got.Value != want.Value {
			return fail(path, fmt.Sprintf("value %q, want %q", got.Value, want.Value))
		}
		if gt == "!!str" && got.Style == 0 && yaml11Ambiguous(got.Value) {
			return fail(path, fmt.Sprintf("plain scalar %q is ambiguous for YAML 1.1 parsers", got.Value))
		}
	case yaml.MappingNode:
		if len(got.Content) != len(want.Content) {
			return fail(path, fmt.Sprintf("%d keys, want %d", len(got.Content)/2, len(want.Content)/2))
		}
		for i := 0; i+1 < len(want.Content); i += 2 {
			k := want.Content[i].Value
			p := yamlx.Join(path, k)
			if err := compare(got.Content[i], want.Content[i], p+"(key)", fail); err != nil {
				return err
			}
			if err := compare(got.Content[i+1], want.Content[i+1], p, fail); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		if len(got.Content) != len(want.Content) {
			return fail(path, fmt.Sprintf("%d items, want %d", len(got.Content), len(want.Content)))
		}
		for i := range want.Content {
			if err := compare(got.Content[i], want.Content[i], yamlx.Index(path, i), fail); err != nil {
				return err
			}
		}
	}
	return nil
}
