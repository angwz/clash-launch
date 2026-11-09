// Package web 嵌入前端构建产物（web/dist）。没有构建前端时只有 .gitkeep，服务端只提供 API。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist 返回前端产物的文件系统；没有 index.html 时返回 nil。
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
