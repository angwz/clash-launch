package catalog_test

import (
	"bufio"
	"encoding/json"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/catalog"
)

func load(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Load(catalogdata.FS, "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLoadMihomo(t *testing.T) {
	c := load(t)
	if n := len(c.Fields()); n < 300 {
		t.Fatalf("目录字段过少：%d", n)
	}
	t.Logf("目录字段数：%d", len(c.Fields()))
}

// 每个 doc 链接的锚点都必须出现在 wiki 快照中（testdata/catalog/wiki-anchors.txt）。
func TestDocAnchorsExist(t *testing.T) {
	c := load(t)
	f, err := os.Open("../../testdata/catalog/wiki-anchors.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	pages := map[string][]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		pages[parts[0]] = parts[1:]
	}
	for _, fd := range c.Fields() {
		u, err := url.Parse(fd.DocURL)
		if err != nil {
			t.Errorf("%s: 文档链接无效 %q", fd.ID, fd.DocURL)
			continue
		}
		page := strings.Trim(strings.TrimPrefix(u.Path, "/config/"), "/")
		if page == "" {
			continue
		}
		anchors, ok := pages[page]
		if !ok && u.Fragment != "" {
			t.Errorf("%s: 快照中没有页面 %s", fd.ID, page)
			continue
		}
		if u.Fragment != "" && !slices.Contains(anchors, u.Fragment) {
			t.Errorf("%s: 页面 %s 没有锚点 #%s", fd.ID, page, u.Fragment)
		}
	}
}

// 顶层顺序必须是官方示例顺序的保序扩展：示例中出现的键，相对顺序与示例一致。
// 示例顺序由 catalog-extract order 生成，保存在 testdata/catalog/example-order.json。
func TestTopLevelOrderFollowsExample(t *testing.T) {
	c := load(t)
	b, err := os.ReadFile("../../testdata/catalog/example-order.json")
	if err != nil {
		t.Fatal(err)
	}
	var orders map[string][]string
	if err := json.Unmarshal(b, &orders); err != nil {
		t.Fatal(err)
	}
	checkSubsequence(t, "(top)", c.Root.KeyOrder(nil), orders[""])
	for _, sec := range []string{"dns", "tun", "sniffer"} {
		checkSubsequence(t, sec, c.Root.Child(sec, nil).KeyOrder(nil), orders[sec])
	}
	items := c.Root.Child("proxies", nil).Items
	for _, v := range items.Variants {
		order := make([]string, len(v.Fields))
		for i, f := range v.Fields {
			order[i] = f.Key
		}
		checkSubsequence(t, "proxies."+v.Value, order, orders["proxies."+v.Value])
	}
	rp := c.Root.Child("rule-providers", nil).Values.KeyOrder(nil)
	checkSubsequence(t, "rule-providers", rp, orders["rule-providers.*"])
}

// checkSubsequence 检查 example 中同时出现在 catalog 里的键，在 catalog 中的相对顺序与 example 一致。
func checkSubsequence(t *testing.T, where string, catalogOrder, example []string) {
	t.Helper()
	pos := map[string]int{}
	for i, k := range catalogOrder {
		pos[k] = i
	}
	last, lastKey := -1, ""
	for _, k := range example {
		if where != "(top)" && strings.HasPrefix(where, "proxies.") && (k == "name" || k == "type" || k == "server" || k == "port") {
			continue // 节点的前四个键统一为 name、type、server、port（docs/decisions/0001）
		}
		p, ok := pos[k]
		if !ok {
			continue // 示例中的噪声键（如 hosts 的示例数据）或目录尚未收录
		}
		if p < last {
			t.Errorf("%s: %s 应排在 %s 之后（与官方示例一致）", where, k, lastKey)
		}
		last, lastKey = p, k
	}
}
