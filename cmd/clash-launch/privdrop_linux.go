//go:build linux

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// prepareDataDir 让数据目录对运行用户可写，然后以普通用户继续运行（需求 R18）。
//
// 镜像默认以 root 启动：把数据目录及其内容的所有者改为运行用户（默认 65532，可用 PUID、PGID 指定），
// 再放弃 root。这样无论映射的是 Docker 数据卷、宿主机上由 root 创建的 ./data，还是 NAS 的共享目录，
// 都不需要手动 chown，compose 也不必写 user。这是 PostgreSQL、Redis 等官方镜像的常见做法。
// 降权后进程没有任何特权，之后创建的文件也都属于运行用户。
//
// 以普通用户启动时（例如 docker run --user、rootless 环境）只检查目录是否可写，不可写就给出修复命令。
func prepareDataDir(dir string) error {
	uid, gid, err := runIDs(os.Getenv)
	if err != nil {
		return err
	}
	if os.Getuid() != 0 {
		// 已经是普通用户：目录要交给的是当前用户，而不是 PUID 或默认的 65532
		return checkWritable(dir, os.Getuid(), os.Getgid())
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建数据目录 %s: %w", dir, err)
	}
	// 在 os.Root 内遍历与修改：路径不会经由符号链接逃出数据目录（遍历与 chown 之间被替换为链接也不行）
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("打开数据目录 %s: %w", dir, err)
	}
	defer func() { _ = root.Close() }()
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) == uid && int(st.Gid) == gid {
			return nil
		}
		return root.Lchown(p, uid, gid)
	})
	if err != nil {
		return fmt.Errorf("把数据目录 %s 交给运行用户 %d:%d 失败（使用加固叠加文件时需要 cap_add: CHOWN、DAC_OVERRIDE）: %w", dir, uid, gid, err)
	}
	// 顺序固定：先清附加组、再改组、最后改用户（改用户后就没有权限改组了）。
	// Go 1.16 起这些调用作用于进程的全部线程。
	if err := syscall.Setgroups([]int{}); err != nil {
		return dropErr(uid, err)
	}
	if err := syscall.Setgid(gid); err != nil {
		return dropErr(uid, err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return dropErr(uid, err)
	}
	return nil
}

func dropErr(uid int, err error) error {
	// 不允许继续以 root 运行：否则新建的文件属于 root，之后以普通用户启动时反而写不进去。
	return fmt.Errorf("切换到运行用户 %d 失败（使用加固叠加文件时需要 cap_add: SETUID、SETGID）: %w", uid, err)
}

// checkWritable 在数据目录中试写一个临时文件。
func checkWritable(dir string, uid, gid int) error {
	if err := os.MkdirAll(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return writableHint(dir, err, uid, gid)
	}
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return writableHint(dir, err, uid, gid)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

func writableHint(dir string, err error, uid, gid int) error {
	return fmt.Errorf("数据目录 %s 不可写（%w）。以普通用户 %d 启动时目录要先交给它，任选其一："+
		"① 去掉 --user / user: 设置，让镜像以 root 启动后自动整理并降权；"+
		"② 在宿主机上执行 sudo chown -R %d:%d <映射的目录>", dir, err, uid, uid, gid)
}
