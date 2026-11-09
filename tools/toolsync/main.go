// toolsync 按 tools.lock.json 下载内核检查所需的 mihomo 可执行文件与地理数据，并校验 SHA-256。
//
//	go run ./tools/toolsync [-lock tools.lock.json] [-platform linux-amd64] [-out .cache/tools]
//
// 输出目录结构：<out>/mihomo[.exe] 与 <out>/geodata/<文件名>。已存在且校验通过的文件不再下载。
// 只在开发与镜像构建阶段联网；服务运行时不调用本工具。
package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type lock struct {
	Tools struct {
		Mihomo struct {
			Version   string `json:"version"`
			Platforms map[string]struct {
				URL     string `json:"url"`
				SHA256  string `json:"sha256"`
				Archive string `json:"archive"`
			} `json:"platforms"`
		} `json:"mihomo"`
		Geodata struct {
			Files map[string]struct {
				URL    string `json:"url"`
				SHA256 string `json:"sha256"`
			} `json:"files"`
		} `json:"geodata"`
	} `json:"tools"`
}

func main() {
	lockPath := flag.String("lock", "tools.lock.json", "工具锁文件")
	platform := flag.String("platform", defaultPlatform(), "目标平台，如 linux-amd64、linux-arm-v7、windows-amd64")
	out := flag.String("out", filepath.Join(".cache", "tools"), "输出目录")
	flag.Parse()
	if err := run(*lockPath, *platform, *out); err != nil {
		fmt.Fprintln(os.Stderr, "toolsync:", err)
		os.Exit(1)
	}
}

func defaultPlatform() string {
	if runtime.GOARCH == "arm" {
		return "linux-arm-v7"
	}
	return runtime.GOOS + "-" + runtime.GOARCH
}

func run(lockPath, platform, out string) error {
	b, err := os.ReadFile(lockPath)
	if err != nil {
		return err
	}
	var l lock
	if err := json.Unmarshal(b, &l); err != nil {
		return fmt.Errorf("解析 %s: %w", lockPath, err)
	}
	p, ok := l.Tools.Mihomo.Platforms[platform]
	if !ok {
		return fmt.Errorf("tools.lock.json 中没有平台 %s", platform)
	}
	if err := os.MkdirAll(filepath.Join(out, "geodata"), 0o750); err != nil {
		return err
	}
	name := "mihomo"
	if strings.HasPrefix(platform, "windows") {
		name += ".exe"
	}
	bin := filepath.Join(out, name)
	stamp := bin + ".sha256"
	if old, err := os.ReadFile(stamp); err != nil || strings.TrimSpace(string(old)) != p.SHA256 {
		archive, err := fetch(p.URL, p.SHA256)
		if err != nil {
			return err
		}
		exe, err := extract(archive, p.Archive)
		if err != nil {
			return err
		}
		if err := os.WriteFile(bin, exe, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(stamp, []byte(p.SHA256+"\n"), 0o644); err != nil {
			return err
		}
		fmt.Printf("mihomo %s → %s\n", l.Tools.Mihomo.Version, bin)
	}
	for fname, f := range l.Tools.Geodata.Files {
		dst := filepath.Join(out, "geodata", fname)
		if data, err := os.ReadFile(dst); err == nil && sum(data) == f.SHA256 {
			continue
		}
		data, err := fetch(f.URL, f.SHA256)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
		fmt.Printf("geodata %s → %s\n", fname, dst)
	}
	return nil
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func fetch(url, want string) ([]byte, error) {
	c := &http.Client{Timeout: 10 * time.Minute}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载 %s: HTTP %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return nil, err
	}
	if got := sum(data); got != want {
		return nil, fmt.Errorf("校验失败 %s: SHA-256 %s，期望 %s", url, got, want)
	}
	return data, nil
}

func extract(data []byte, kind string) ([]byte, error) {
	switch kind {
	case "gz":
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(io.LimitReader(r, 512<<20))
	case "zip":
		z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, f := range z.File {
			if strings.HasSuffix(f.Name, ".exe") || !strings.Contains(f.Name, ".") {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer func() { _ = rc.Close() }()
				return io.ReadAll(io.LimitReader(rc, 512<<20))
			}
		}
		return nil, fmt.Errorf("压缩包中没有可执行文件")
	}
	return nil, fmt.Errorf("未知的压缩格式 %s", kind)
}
