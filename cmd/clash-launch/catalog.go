package main

import (
	"fmt"
	"os"
	"path/filepath"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/catalog"
)

// cmdCatalog 实现 catalog export：把字段目录导出为前端与编辑器使用的文件。
//
//	<目录>/<pack>/sections.json        表单元数据（按顶层分节，顺序即渲染顺序）
//	<目录>/<pack>/schema.<lang>.json   JSON Schema（draft 2020-12），说明文字为对应语言
//	<目录>/<pack>/messages.<lang>.json i18n 源消息
func cmdCatalog(args []string) int {
	if len(args) != 2 || args[0] != "export" {
		fmt.Fprintln(os.Stderr, "用法：clash-launch catalog export <目录>")
		return 2
	}
	dir := filepath.Join(args[1], "mihomo")
	cat, err := catalog.Load(catalogdata.FS, "mihomo")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	files := map[string]any{"sections.json": cat.Sections()}
	for _, lang := range []string{"en", "zh-CN"} {
		files["schema."+lang+".json"] = cat.JSONSchema(lang)
		files["messages."+lang+".json"] = cat.Messages(lang)
	}
	for name, v := range files {
		b, err := catalog.MarshalStable(v)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "已导出 %d 个字段到 %s\n", len(cat.Fields()), dir)
	return 0
}
