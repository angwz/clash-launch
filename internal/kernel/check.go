// Package kernel 是 mihomo 的适配层：调用固定版本 mihomo 的分享链接解析与节点校验（Go 库），
// 以及用 mihomo 可执行文件做配置检查（mihomo -t）。引擎与 mihomo 的全部接触都经过这里，
// 版本固定为 tools.lock.json 与 go.mod 中的同一个 v1.19.33。
package kernel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// Checker 用 mihomo -t 检查配置。零值不可用；用 NewChecker 构造。
//
// 不联网的保证（第 11.3 节，已对照 v1.19.33 源码核验）：
//   - -t 只执行 config.Parse，不调用 provider 的 Initial()，因此 proxy-providers 与
//     rule-providers 不会被下载；
//   - GEOIP、GEOSITE、IP-ASN 规则在解析时会调用 geodata.Init*，文件缺失才下载——
//     这里把固定版本的地理数据预置到临时目录中；
//   - 作为第二道保险，检查副本中的 geox-url 被改写为回环上的不可达地址，geo-auto-update 关闭。
//
// 因此本层检查覆盖：YAML 结构、字段类型、节点与策略组的解析和引用、规则语法与 geodata 引用；
// 不覆盖：provider 的实际内容、远程资源是否可达、geox-url 本身的取值。
type Checker struct {
	Binary   string        // mihomo 可执行文件
	Geodata  string        // 地理数据目录（geoip.metadb、GeoSite.dat、GeoIP.dat、ASN.mmdb）
	Timeout  time.Duration // 单次检查超时
	MemLimit string        // 传给子进程的 GOMEMLIMIT（Go 运行时的软上限）
	sem      chan struct{}
}

// NewChecker 构造检查器；concurrency 是同时进行的检查数上限。
func NewChecker(binary, geodata string, concurrency int) *Checker {
	if concurrency < 1 {
		concurrency = 1
	}
	// 子进程的工作目录是临时目录，相对路径必须先转换为绝对路径。
	if abs, err := filepath.Abs(binary); err == nil && binary != "" {
		binary = abs
	}
	if abs, err := filepath.Abs(geodata); err == nil && geodata != "" {
		geodata = abs
	}
	return &Checker{Binary: binary, Geodata: geodata, Timeout: 10 * time.Second, MemLimit: "256MiB", sem: make(chan struct{}, concurrency)}
}

// Status 是检查结论。
type Status string

const (
	Passed      Status = "passed"
	Failed      Status = "failed"
	Unavailable Status = "unavailable" // 没有内核可用（未配置、Stash 目标等）
	TimedOut    Status = "timeout"
)

// Result 是一次检查的结果。
type Result struct {
	Status  Status `json:"status"`
	Version string `json:"version,omitempty"`
	Message string `json:"message,omitempty"` // 已脱敏的内核输出摘要
	Path    string `json:"path,omitempty"`    // 尽量定位到的配置路径
}

// ErrNoKernel 表示没有配置内核可执行文件。
var ErrNoKernel = errors.New("kernel: 未配置 mihomo 可执行文件")

// Check 检查配置文本，并把结论写入诊断。ctx 取消时立即终止子进程。
func (c *Checker) Check(ctx context.Context, config []byte, out *diag.List) Result {
	if c == nil || c.Binary == "" {
		out.Add(diag.KernelUnavailable, "", "reason", ErrNoKernel.Error())
		return Result{Status: Unavailable, Message: ErrNoKernel.Error()}
	}
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		out.Add(diag.KernelUnavailable, "", "reason", ctx.Err().Error())
		return Result{Status: Unavailable, Message: ctx.Err().Error()}
	}
	res, err := c.run(ctx, config)
	if err != nil {
		out.Add(diag.KernelUnavailable, "", "reason", err.Error())
		return Result{Status: Unavailable, Message: err.Error()}
	}
	switch res.Status {
	case Passed:
		out.Add(diag.KernelPassed, "", "version", res.Version)
	case TimedOut:
		out.Add(diag.KernelTimeout, "", "timeout", c.Timeout.String())
	case Failed:
		out.Add(diag.KernelFailed, res.Path, "reason", res.Message)
	}
	return res
}

func (c *Checker) run(ctx context.Context, config []byte) (Result, error) {
	isolated, err := isolate(config)
	if err != nil {
		return Result{}, fmt.Errorf("准备检查副本: %w", err)
	}
	dir, err := os.MkdirTemp("", "clash-launch-check-")
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }() // 尽力清理；临时目录位于 tmpfs，残留不影响正确性
	if err := c.placeGeodata(dir); err != nil {
		return Result{}, err
	}
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, isolated, 0o600); err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Binary, "-t", "-d", dir, "-f", cfg)
	cmd.Dir = dir
	cmd.Env = childEnv(dir, c.MemLimit)
	var buf bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &buf, n: 64 << 10}
	cmd.Stderr = cmd.Stdout
	runErr := cmd.Run()
	output := buf.String()
	if ctx.Err() == context.DeadlineExceeded {
		return Result{Status: TimedOut, Message: "timeout"}, nil
	}
	version := c.Version()
	if runErr == nil && strings.Contains(output, "test is successful") {
		return Result{Status: Passed, Version: version}, nil
	}
	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		return Result{}, fmt.Errorf("启动 mihomo: %w", runErr)
	}
	msg := summarize(output)
	return Result{Status: Failed, Version: version, Message: msg, Path: locate(msg)}, nil
}

// Version 返回内核版本（管理页的系统状态也使用）。
func (c *Checker) Version() string {
	// 版本取自工具锁：与 go.mod 中的依赖一致，避免每次检查多启动一次进程。
	return "v1.19.33"
}

// isolate 返回用于检查的副本：geox-url 指向回环上的不可达地址，关闭 geo-auto-update。
// 只改这两个与下载有关的键，其余内容逐字保留语义。
func isolate(config []byte) ([]byte, error) {
	var l diag.List
	root, err := yamlx.Parse(config, yamlx.DefaultLimits, "kernel", &l)
	if err != nil || root == nil || root.Kind != yaml.MappingNode {
		return config, nil // 交给内核报告语法错误
	}
	const blackhole = "http://127.0.0.1:9/clash-launch-offline-check"
	yamlx.Set(root, "geox-url", yamlx.Map("geoip", blackhole, "geosite", blackhole, "mmdb", blackhole, "asn", blackhole))
	yamlx.Set(root, "geo-auto-update", yamlx.Bool(false))
	return yaml.Marshal(root)
}

// placeGeodata 把地理数据放进临时目录：优先符号链接，其次硬链接，最后复制。
func (c *Checker) placeGeodata(dir string) error {
	if c.Geodata == "" {
		return nil
	}
	for _, name := range []string{"geoip.metadb", "GeoSite.dat", "GeoIP.dat", "ASN.mmdb"} {
		src := filepath.Join(c.Geodata, name)
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("缺少地理数据 %s（运行 go run ./tools/toolsync）: %w", name, err)
		}
		dst := filepath.Join(dir, name)
		if os.Symlink(src, dst) == nil || os.Link(src, dst) == nil {
			continue
		}
		if err := copyFile(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }() // 只读文件，关闭失败不影响结果
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close() // 已有更早的错误要返回
		return err
	}
	return out.Close()
}

// childEnv 只传递子进程必需的环境变量，不把服务的密钥与代理设置泄露给内核。
func childEnv(dir, memLimit string) []string {
	env := []string{"HOME=" + dir, "GOMEMLIMIT=" + memLimit}
	if runtime.GOOS == "windows" {
		for _, k := range []string{"SystemRoot", "TEMP", "TMP"} {
			if v := os.Getenv(k); v != "" {
				env = append(env, k+"="+v)
			}
		}
	}
	return env
}

type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n <= 0 {
		return len(p), nil
	}
	q := p
	if len(q) > l.n {
		q = q[:l.n]
	}
	l.n -= len(q)
	_, err := l.w.Write(q)
	return len(p), err
}

var (
	ansi        = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	logLine     = regexp.MustCompile(`level=(error|fatal)\s+msg="?(.*?)"?$`)
	userinfo    = regexp.MustCompile(`://[^/\s:@]+:[^/\s@]+@`)
	longToken   = regexp.MustCompile(`[A-Za-z0-9+/_=-]{32,}`)
	proxyIdx    = regexp.MustCompile(`^proxy (\d+):`)
	groupIdx    = regexp.MustCompile(`^proxy group\[(\d+)\]`)
	ruleIdx     = regexp.MustCompile(`^rules\[(\d+)\]`)
	providerKey = regexp.MustCompile(`^parse (proxy|rule) provider (\S+) error`)
)

// summarize 从内核输出中取出错误信息并脱敏：去掉 ANSI 颜色、URL 中的凭据与长令牌，截断长度。
func summarize(output string) string {
	output = ansi.ReplaceAllString(output, "")
	var msg string
	for _, line := range strings.Split(output, "\n") {
		if m := logLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			msg = m[2]
		}
	}
	if msg == "" {
		lines := strings.Fields(strings.ReplaceAll(output, "\n", " | "))
		msg = strings.Join(lines, " ")
	}
	msg = userinfo.ReplaceAllString(msg, "://***@")
	msg = longToken.ReplaceAllString(msg, "***")
	if len(msg) > 500 {
		msg = msg[:500] + "…"
	}
	return msg
}

// locate 把内核的错误前缀映射为配置路径。
func locate(msg string) string {
	switch {
	case proxyIdx.MatchString(msg):
		return "proxies[" + proxyIdx.FindStringSubmatch(msg)[1] + "]"
	case groupIdx.MatchString(msg):
		return "proxy-groups[" + groupIdx.FindStringSubmatch(msg)[1] + "]"
	case ruleIdx.MatchString(msg):
		return "rules[" + ruleIdx.FindStringSubmatch(msg)[1] + "]"
	case providerKey.MatchString(msg):
		m := providerKey.FindStringSubmatch(msg)
		return m[1] + "-providers." + m[2]
	}
	return ""
}
