//go:build linux

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// 容器中运行服务的普通用户（与 Dockerfile 的 USER、distroless 的 nonroot 一致）。
const runUID, runGID = 65532, 65532

// prepareDataDir 让数据目录对运行用户可写，然后以普通用户继续运行。
//
// 以 root 启动时（compose.yaml 写了 user: "0:0"），把数据目录及其内容的所有者改为 65532，
// 再放弃 root：这样无论映射的是 Docker 数据卷还是宿主机上由 root 创建的 ./data，都不需要手动 chown。
// 这是 PostgreSQL、Redis 等官方镜像的常见做法。降权后进程没有任何特权，之后创建的文件也都属于 65532。
//
// 以普通用户启动时只检查目录是否可写，不可写就给出可以直接执行的修复命令。
func prepareDataDir(dir string) error {
	if os.Getuid() != 0 {
		return checkWritable(dir)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建数据目录 %s: %w", dir, err)
	}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Uid == runUID && st.Gid == runGID {
			return nil
		}
		return os.Lchown(p, runUID, runGID)
	})
	if err != nil {
		return fmt.Errorf("把数据目录 %s 交给运行用户 %d 失败（compose.yaml 需要 cap_add: CHOWN、DAC_OVERRIDE）: %w", dir, runUID, err)
	}
	// 顺序固定：先清附加组、再改组、最后改用户（改用户后就没有权限改组了）。
	// Go 1.16 起这些调用作用于进程的全部线程。
	if err := syscall.Setgroups([]int{}); err != nil {
		return dropErr(err)
	}
	if err := syscall.Setgid(runGID); err != nil {
		return dropErr(err)
	}
	if err := syscall.Setuid(runUID); err != nil {
		return dropErr(err)
	}
	return nil
}

func dropErr(err error) error {
	// 不允许继续以 root 运行：否则新建的文件属于 root，之后以普通用户启动时反而写不进去。
	return fmt.Errorf("切换到运行用户 %d 失败（compose.yaml 需要 cap_add: SETUID、SETGID）: %w", runUID, err)
}

// checkWritable 在数据目录中试写一个临时文件。
func checkWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return writableHint(dir, err)
	}
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return writableHint(dir, err)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

func writableHint(dir string, err error) error {
	return fmt.Errorf("数据目录 %s 不可写（%w）。如果映射的是宿主机目录（如 ./data），任选其一："+
		"① 使用仓库中最新的 compose.yaml（以 root 启动后自动交给运行用户）；"+
		"② 在宿主机上执行 sudo chown -R %d:%d <映射的目录>", dir, err, runUID, runGID)
}
