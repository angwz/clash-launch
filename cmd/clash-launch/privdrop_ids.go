package main

import (
	"fmt"
	"strconv"
)

// 容器中运行服务的默认用户（distroless 的 nonroot）。
const defaultUID, defaultGID = 65532, 65532

// runIDs 读取 PUID、PGID（群晖、威联通、Unraid 等需要让文件属于宿主机上的某个用户）；
// 不设置时使用 65532。不允许 0：以 root 运行服务正是降权要避免的（需求 R18）。
func runIDs(getenv func(string) string) (int, int, error) {
	parse := func(key string, def int) (int, error) {
		v := getenv(key)
		if v == "" {
			return def, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 1<<31-2 {
			return 0, fmt.Errorf("%s=%q 无效：应为 1 到 2147483646 之间的数字（不能是 0，服务不以 root 运行）", key, v)
		}
		return n, nil
	}
	uid, err := parse("PUID", defaultUID)
	if err != nil {
		return 0, 0, err
	}
	gid, err := parse("PGID", defaultGID)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}
