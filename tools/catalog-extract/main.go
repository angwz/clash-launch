// catalog-extract 从 mihomo 源码与官方示例配置中提取字段全集，供字段目录的编写与跟进使用。
//
//	catalog-extract scan  -src <mihomo 源码目录> [-o fields.json]   字段全集与类型（来自结构体 tag）
//	catalog-extract order -src <mihomo 源码目录>                     官方示例 docs/config.yaml 中各位置的键顺序
//	catalog-extract diff  <旧 fields.json> <新 fields.json>          两个内核版本之间的新增、移除与类型变化
//
// 提取只读源码，不编译、不运行 mihomo。结果是「初稿」：类型来自 Go 类型的粗略映射，
// 说明、重要程度与文档链接由人工写入 catalog/。扫描范围见 roots。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
)

// Field 是提取出的一个字段。
type Field struct {
	Path   string `json:"path"`   // dns.enhanced-mode、proxies.vmess.ws-opts.path
	Type   string `json:"type"`   // string|int|float|bool|list<…>|map<…>|object|any|named:<名>
	Source string `json:"source"` // 源码位置：config/config.go:230
}

// Result 是 scan 的输出。
type Result struct {
	Version string  `json:"version"`
	Fields  []Field `json:"fields"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "scan":
		fs := flag.NewFlagSet("scan", flag.ExitOnError)
		src := fs.String("src", "", "mihomo 源码目录")
		out := fs.String("o", "", "输出文件（默认标准输出）")
		version := fs.String("version", "", "记录在输出中的版本号")
		_ = fs.Parse(os.Args[2:])
		if *src == "" {
			usage()
		}
		res, err := scan(*src)
		check(err)
		res.Version = *version
		writeJSON(*out, res)
	case "order":
		fs := flag.NewFlagSet("order", flag.ExitOnError)
		src := fs.String("src", "", "mihomo 源码目录")
		_ = fs.Parse(os.Args[2:])
		if *src == "" {
			usage()
		}
		orders, err := exampleOrder(*src + "/docs/config.yaml")
		check(err)
		writeJSON("", orders)
	case "diff":
		if len(os.Args) != 4 {
			usage()
		}
		var a, b Result
		readJSON(os.Args[2], &a)
		readJSON(os.Args[3], &b)
		d := diff(a, b)
		writeJSON("", d)
		if len(d.Added)+len(d.Removed)+len(d.Changed) > 0 {
			os.Exit(3) // 有差异：CI 据此开 Issue
		}
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "用法：catalog-extract scan|order|diff …（见源码注释）")
	os.Exit(2)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "catalog-extract:", err)
		os.Exit(1)
	}
}

func writeJSON(path string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	check(err)
	b = append(b, '\n')
	if path == "" {
		_, _ = os.Stdout.Write(b)
		return
	}
	check(os.WriteFile(path, b, 0o644))
}

func readJSON(path string, v any) {
	b, err := os.ReadFile(path)
	check(err)
	check(json.Unmarshal(b, v))
}

// Diff 是两个版本之间的字段差异。
type Diff struct {
	From    string    `json:"from"`
	To      string    `json:"to"`
	Added   []Field   `json:"added"`
	Removed []Field   `json:"removed"`
	Changed []Changed `json:"changed"`
}

// Changed 是类型发生变化的字段。
type Changed struct {
	Path   string `json:"path"`
	From   string `json:"from"`
	To     string `json:"to"`
	Source string `json:"source"`
}

func diff(a, b Result) Diff {
	am := map[string]Field{}
	for _, f := range a.Fields {
		am[f.Path] = f
	}
	bm := map[string]Field{}
	for _, f := range b.Fields {
		bm[f.Path] = f
	}
	d := Diff{From: a.Version, To: b.Version, Added: []Field{}, Removed: []Field{}, Changed: []Changed{}}
	for p, f := range bm {
		old, ok := am[p]
		switch {
		case !ok:
			d.Added = append(d.Added, f)
		case old.Type != f.Type:
			d.Changed = append(d.Changed, Changed{Path: p, From: old.Type, To: f.Type, Source: f.Source})
		}
	}
	for p, f := range am {
		if _, ok := bm[p]; !ok {
			d.Removed = append(d.Removed, f)
		}
	}
	sort.Slice(d.Added, func(i, j int) bool { return d.Added[i].Path < d.Added[j].Path })
	sort.Slice(d.Removed, func(i, j int) bool { return d.Removed[i].Path < d.Removed[j].Path })
	sort.Slice(d.Changed, func(i, j int) bool { return d.Changed[i].Path < d.Changed[j].Path })
	return d
}
