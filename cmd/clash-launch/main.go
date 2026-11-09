// Command clash-launch 是 Clash, Launch! 的命令行入口。
//
//	clash-launch render [选项] <方案文件>   渲染方案文档（离线，不联网）
//	clash-launch catalog export <目录>      导出表单元数据、JSON Schema 与 i18n 源消息
//	clash-launch version                    打印版本
//
// 服务端子命令（serve、admin、keys、backup、db、healthcheck）在后续里程碑加入。
package main

import (
	"fmt"
	"os"

	_ "time/tzdata" // 内嵌时区数据：distroless 镜像中也能解析方案的 timezone

	"github.com/angwz/clash-launch/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var code int
	switch os.Args[1] {
	case "render":
		code = cmdRender(os.Args[2:])
	case "catalog":
		code = cmdCatalog(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println(version.Name, version.Version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知子命令：%s\n\n", os.Args[1])
		usage()
		code = 2
	}
	os.Exit(code)
}

func usage() {
	fmt.Fprint(os.Stderr, `用法：clash-launch <子命令> [选项]

  render [选项] <方案文件>   渲染方案文档；YAML 写到标准输出，诊断写到标准错误
  catalog export <目录>      导出字段目录（表单元数据、JSON Schema、i18n 源消息）
  version                    打印版本

运行 clash-launch render -h 查看渲染选项。
`)
}
