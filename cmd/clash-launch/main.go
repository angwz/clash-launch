// Command clash-launch 是 Clash, Launch! 的命令行入口。
//
//	clash-launch render [选项] <方案文件>   渲染方案文档（离线，不联网）
//	clash-launch catalog export <目录>      导出表单元数据、JSON Schema 与 i18n 源消息
//	clash-launch version                    打印版本
//	clash-launch serve                      启动服务（配置见 docs/guide/deploy.md）
//	clash-launch admin create -username x   创建管理员（口令从标准输入读取）
//	clash-launch keys rotate                轮换主密钥并重新加密全部数据
//	clash-launch backup <文件>              生成一致的数据库快照（SQLite）
//	clash-launch db migrate | copy          执行迁移；在数据库之间复制数据
//	clash-launch healthcheck                容器健康检查
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
	// 会读写数据目录的子命令：先让数据目录对运行用户可写；以 root 启动时随后降为普通用户（见 privdrop_linux.go）。
	switch os.Args[1] {
	case "serve", "admin", "keys", "backup", "db":
		if c, err := loadConfig(); err == nil {
			if err := prepareDataDir(c.DataDir); err != nil {
				fmt.Fprintln(os.Stderr, "clash-launch:", err)
				os.Exit(1)
			}
		}
	}
	var code int
	switch os.Args[1] {
	case "render":
		code = cmdRender(os.Args[2:])
	case "catalog":
		code = cmdCatalog(os.Args[2:])
	case "serve":
		code = cmdServe(os.Args[2:])
	case "admin":
		code = cmdAdmin(os.Args[2:])
	case "keys":
		code = cmdKeys(os.Args[2:])
	case "backup":
		code = cmdBackup(os.Args[2:])
	case "db":
		code = cmdDB(os.Args[2:])
	case "healthcheck":
		code = cmdHealthcheck(os.Args[2:])
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
  serve                      启动服务
  admin create -username x   创建管理员（口令从标准输入读取）
  keys rotate                轮换主密钥并重新加密全部数据
  backup <文件>              备份数据库（SQLite）
  db migrate | db copy -from <DSN> -to <DSN>
  healthcheck                容器健康检查
  version                    打印版本

运行 clash-launch render -h 查看渲染选项。
`)
}
