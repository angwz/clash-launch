//go:build !linux

package main

// prepareDataDir 在 Linux 以外的系统上不做任何事：容器只发布 Linux 镜像，
// 本机开发（Windows、macOS）直接以当前用户运行。
func prepareDataDir(string) error { return nil }
