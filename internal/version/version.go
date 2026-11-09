// Package version 记录程序版本与项目地址。Version 在发布构建时通过
// -ldflags "-X github.com/angwz/clash-launch/internal/version.Version=1.0.0" 注入。
package version

// Version 是程序版本；开发构建为 1.0.0-dev。
var Version = "1.0.0-dev"

// Name 是展示名，逗号与感叹号是名字的一部分。
const Name = "Clash, Launch!"

// Home 是项目地址。
const Home = "https://github.com/angwz/clash-launch"
