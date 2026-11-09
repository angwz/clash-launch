package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/engine"
	"github.com/angwz/clash-launch/internal/kernel"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/target"
)

func cmdRender(args []string) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	outPath := fs.String("o", "", "输出文件（默认标准输出）")
	check := fs.Bool("check", false, "渲染后用 mihomo -t 检查（需要 -mihomo 与 -geodata 或对应环境变量）")
	mihomo := fs.String("mihomo", os.Getenv("CLASH_LAUNCH_MIHOMO"), "mihomo 可执行文件（环境变量 CLASH_LAUNCH_MIHOMO）")
	geodata := fs.String("geodata", os.Getenv("CLASH_LAUNCH_GEODATA"), "地理数据目录（环境变量 CLASH_LAUNCH_GEODATA）")
	lang := fs.String("lang", "zh", "诊断语言：zh 或 en")
	updated := fs.String("updated", "", "文件头时间（RFC 3339）；默认取方案文件的修改时间")
	inputs := fs.String("inputs", "", "工作区文件：源与规则源的定义及其快照（见 docs/guide/cli.md）")
	quiet := fs.Bool("q", false, "只输出 error 级诊断")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法：clash-launch render [选项] <方案文件>")
		return 2
	}
	file := fs.Arg(0)
	text, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取方案：", err)
		return 1
	}
	var ds diag.List
	p, err := profile.Parse(text, &ds)
	if err != nil {
		printDiags(ds.Items(), *lang, *quiet)
		return 1
	}
	cat, err := catalog.Load(catalogdata.FS, p.Target.Pack)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	presets, err := target.Load(catalogdata.FS, p.Target.Pack)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	when, err := headerTime(file, *updated)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	in := engine.Input{Profile: p, Catalog: cat, Presets: presets, Diagnostics: ds.Items(), Updated: when}
	if *inputs != "" {
		gen, gds, latest, err := buildFromWorkspace(*inputs, p, cat)
		if err != nil {
			fmt.Fprintln(os.Stderr, "工作区：", err)
			return 1
		}
		in.Generated = gen
		in.Diagnostics = append(in.Diagnostics, gds...)
		if *updated == "" && latest.After(in.Updated) {
			in.Updated = latest
		}
	}
	out, err := engine.Render(in)
	diags := out.Diagnostics
	if err == nil && *check {
		var kd diag.List
		if out.Preset != nil && !out.Preset.Kernel {
			kd.Add(diag.KernelUnavailable, "", "reason", "preset "+out.Preset.ID+" has no mihomo kernel")
			err = errors.New("所选目标没有可用的内核检查")
		} else {
			c := kernel.NewChecker(*mihomo, *geodata, 1)
			if *mihomo == "" {
				c = nil
			}
			r := c.Check(context.Background(), out.YAML, &kd)
			if r.Status != kernel.Passed {
				err = errors.New("内核检查未通过或未能执行")
			}
		}
		diags = append(diags, kd.Items()...)
	}
	printDiags(diags, *lang, *quiet)
	if out.YAML != nil {
		var w io.Writer = os.Stdout
		if *outPath != "" {
			if werr := os.WriteFile(*outPath, out.YAML, 0o644); werr != nil {
				fmt.Fprintln(os.Stderr, werr)
				return 1
			}
		} else {
			_, _ = w.Write(out.YAML)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// headerTime 返回文件头时间：显式给出时用它，否则用方案文件的修改时间（同一文件得到同一结果）。
func headerTime(file, flagValue string) (time.Time, error) {
	if flagValue != "" {
		t, err := time.Parse(time.RFC3339, flagValue)
		if err != nil {
			return time.Time{}, fmt.Errorf("-updated 必须是 RFC 3339 时间：%w", err)
		}
		return t, nil
	}
	st, err := os.Stat(file)
	if err != nil {
		return time.Time{}, err
	}
	return st.ModTime().Truncate(time.Second), nil
}

func printDiags(ds []diag.Diagnostic, lang string, quiet bool) {
	for _, d := range diag.Sorted(ds) {
		if quiet && d.Severity != diag.Error {
			continue
		}
		where := ""
		if d.Path != "" {
			where = " [" + d.Path + "]"
		}
		fmt.Fprintf(os.Stderr, "%-7s %s%s：%s\n", d.Severity, d.Code, where, diag.Message(d, lang))
	}
}
