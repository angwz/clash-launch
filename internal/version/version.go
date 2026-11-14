// Package version 记录程序版本与项目地址。三个变量在发布构建时通过 -ldflags -X 注入（见 Dockerfile 与 image 工作流）。
package version

// Version 是程序版本。发布标签构建为 1.2.3；main 分支构建为 git describe 的形式
// 1.2.3-14-g769fef4（标签 v1.2.3 之后第 14 次提交，短哈希 769fef4）；本机开发构建为 1.0.0-dev。
var Version = "1.0.0-dev"

// Commit 是构建所用的完整提交哈希；未注入时为空。
var Commit = ""

// BuildDate 是构建时间（RFC 3339，UTC）；未注入时为空。
var BuildDate = ""

// Name 是展示名，逗号与感叹号是名字的一部分。
const Name = "Clash, Launch!"

// Home 是项目地址。
const Home = "https://github.com/angwz/clash-launch"
