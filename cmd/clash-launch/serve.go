package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pelletier/go-toml/v2"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/api"
	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/kernel"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/source"
	"github.com/angwz/clash-launch/internal/store"
	"github.com/angwz/clash-launch/internal/target"
	"github.com/angwz/clash-launch/internal/version"
	"github.com/angwz/clash-launch/web"
)

// config 是服务的配置（第 18.2 节）。环境变量为主；CLASH_LAUNCH_CONFIG 指向的 TOML 文件提供默认值，
// 同名设置以环境变量为准。TOML 的键为环境变量去掉前缀后的小写短横线形式（如 public-url）。
type config struct {
	Listen         string
	DataDir        string
	DatabaseURL    string
	PublicURL      string
	TrustedProxies string
	FetchAllowlist string
	LogLevel       string
	Mihomo         string
	Geodata        string
	PoWBits        int // 注册与找回的工作量证明难度（前导零位数），默认 20，约 1 秒
}

var configKeys = []string{"LISTEN", "DATA_DIR", "DATABASE_URL", "PUBLIC_URL", "TRUSTED_PROXIES", "FETCH_ALLOWLIST", "LOG_LEVEL", "MIHOMO", "GEODATA", "POW_BITS"}

func loadConfig() (config, error) {
	vals := map[string]string{}
	if path := os.Getenv("CLASH_LAUNCH_CONFIG"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return config{}, err
		}
		var file map[string]any
		if err := toml.Unmarshal(b, &file); err != nil {
			return config{}, fmt.Errorf("%s: %w", path, err)
		}
		for k, v := range file {
			vals[strings.ToUpper(strings.ReplaceAll(k, "-", "_"))] = fmt.Sprint(v)
		}
	}
	for _, k := range configKeys {
		if v, ok := os.LookupEnv("CLASH_LAUNCH_" + k); ok {
			vals[k] = v
		}
	}
	get := func(k, def string) string {
		if v := strings.TrimSpace(vals[k]); v != "" {
			return v
		}
		return def
	}
	c := config{
		Listen: get("LISTEN", ":8080"), DataDir: get("DATA_DIR", "/data"), PublicURL: strings.TrimSuffix(get("PUBLIC_URL", ""), "/"),
		TrustedProxies: get("TRUSTED_PROXIES", ""),
		FetchAllowlist: get("FETCH_ALLOWLIST", ""), LogLevel: get("LOG_LEVEL", "info"),
		Mihomo: get("MIHOMO", "/opt/mihomo/mihomo"), Geodata: get("GEODATA", "/opt/mihomo/geodata"),
	}
	bits, err := strconv.Atoi(get("POW_BITS", "20"))
	if err != nil || bits < 1 || bits > 32 {
		return config{}, fmt.Errorf("CLASH_LAUNCH_POW_BITS 需要 1–32 的整数")
	}
	c.PoWBits = bits
	c.DatabaseURL = get("DATABASE_URL", "sqlite://"+filepath.ToSlash(filepath.Join(c.DataDir, "clash-launch.db")))
	return c, nil
}

func logger(level string) *slog.Logger {
	var l slog.Level
	_ = l.UnmarshalText([]byte(level))
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: l}))
}

// openStore 读取主密钥、打开数据库并执行迁移（迁移前自动备份到 <数据目录>/backups/）。
func openStore(ctx context.Context, c config, log *slog.Logger) (*store.DB, error) {
	keys, ks, err := store.LoadKeyring(c.DataDir, os.Getenv)
	if err != nil {
		return nil, fmt.Errorf("主密钥: %w", err)
	}
	if ks.Generated {
		log.Warn("已生成新的主密钥：备份数据时必须同时备份它，丢失密钥就无法解密", "path", ks.Path)
	}
	db, err := store.Open(ctx, c.DatabaseURL, keys)
	if err != nil {
		return nil, err
	}
	done, err := db.Migrate(ctx, filepath.Join(c.DataDir, "backups"))
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if len(done) > 0 {
		log.Info("数据库迁移完成", "versions", done)
	}
	return db, nil
}

func buildService(ctx context.Context, c config, db store.Store, log *slog.Logger) (*service.Service, *api.Config, error) {
	cat, err := catalog.Load(catalogdata.FS, "mihomo")
	if err != nil {
		return nil, nil, err
	}
	digest, err := catalog.Digest(catalogdata.FS, "mihomo")
	if err != nil {
		return nil, nil, err
	}
	presets, err := target.Load(catalogdata.FS, "mihomo")
	if err != nil {
		return nil, nil, err
	}
	tools, err := node.LoadTools(catalogdata.FS)
	if err != nil {
		return nil, nil, err
	}
	guard, err := source.ParseAllowlist(c.FetchAllowlist)
	if err != nil {
		return nil, nil, err
	}
	fetcher, err := source.New(source.Config{Guard: guard})
	if err != nil {
		return nil, nil, err
	}
	var checker *kernel.Checker
	if _, err := os.Stat(c.Mihomo); err == nil {
		checker = kernel.NewChecker(c.Mihomo, c.Geodata, 2)
	} else {
		log.Warn("没有找到 mihomo，内核检查不可用", "path", c.Mihomo)
	}
	var trusted []netip.Prefix
	for _, p := range strings.Split(c.TrustedProxies, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if !strings.Contains(p, "/") {
			p += map[bool]string{true: "/128", false: "/32"}[strings.Contains(p, ":")]
		}
		pr, err := netip.ParsePrefix(p)
		if err != nil {
			return nil, nil, fmt.Errorf("CLASH_LAUNCH_TRUSTED_PROXIES: %w", err)
		}
		trusted = append(trusted, pr.Masked())
	}
	svc := &service.Service{Store: db, Catalog: cat, CatalogDigest: digest, Presets: presets, Tools: tools, Kernel: checker, Fetcher: fetcher, Log: log, PublicURL: c.PublicURL, PoWBits: c.PoWBits, Mailer: &source.Mailer{Guard: guard}}
	return svc, &api.Config{PublicURL: c.PublicURL, TrustedProxies: trusted, Static: web.Dist(), Guard: guard}, nil
}

func cmdServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	c, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	log := logger(c.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := openStore(ctx, c, log)
	if err != nil {
		log.Error("打开数据库失败", "error", err)
		return 1
	}
	defer func() { _ = db.Close() }()
	svc, apiCfg, err := buildService(ctx, c, db, log)
	if err != nil {
		log.Error("初始化失败", "error", err)
		return 1
	}
	srv := api.New(svc, *apiCfg, log)
	if st, err := svc.NewSetup(ctx); err == nil && st != nil {
		srv.SetSetup(st)
		base := c.PublicURL
		if base == "" {
			base = "http://<对外地址>"
		}
		// 设置链接只出现在日志中，15 分钟有效；也可以用 clash-launch admin create 创建管理员。
		log.Warn("还没有任何账号：用一次性设置链接创建首个管理员（15 分钟有效）", "url", base+"/setup#"+st.Token)
	}
	go svc.Run(ctx, time.Minute)
	hs := srv.HTTPServer(c.Listen)
	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	log.Info("Clash, Launch! 已启动", "version", version.Version, "listen", c.Listen, "database", db.Dialect(), "kernel", svc.Kernel != nil)
	select {
	case <-ctx.Done():
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("HTTP 服务退出", "error", err)
			return 1
		}
	}
	shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = hs.Shutdown(shut)
	return 0
}

// cmdAdmin 实现 admin create：创建管理员；口令从标准输入的第一行读取（不出现在命令行参数与 shell 历史中）。
func cmdAdmin(args []string) int {
	if len(args) < 1 || args[0] != "create" {
		fmt.Fprintln(os.Stderr, "用法：clash-launch admin create -username <用户名>  （口令从标准输入读取）")
		return 2
	}
	fs := flag.NewFlagSet("admin create", flag.ContinueOnError)
	username := fs.String("username", "", "管理员用户名")
	if err := fs.Parse(args[1:]); err != nil || *username == "" {
		fmt.Fprintln(os.Stderr, "需要 -username")
		return 2
	}
	fmt.Fprint(os.Stderr, "口令（至少 10 个字符）：")
	pw, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && pw == "" {
		fmt.Fprintln(os.Stderr, "读取口令失败：", err)
		return 1
	}
	pw = strings.TrimRight(pw, "\r\n")
	c, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	log := logger(c.LogLevel)
	ctx := context.Background()
	db, err := openStore(ctx, c, log)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = db.Close() }()
	svc := &service.Service{Store: db, Log: log}
	u, err := svc.CreateAdmin(ctx, *username, pw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "\n创建失败：", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "\n已创建管理员 %s（%s）\n", u.Username, u.ID)
	return 0
}

// cmdKeys 实现 keys rotate：生成新主密钥追加到密钥文件，并用它重新加密全部数据。
func cmdKeys(args []string) int {
	if len(args) != 1 || args[0] != "rotate" {
		fmt.Fprintln(os.Stderr, "用法：clash-launch keys rotate")
		return 2
	}
	c, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if os.Getenv("CLASH_LAUNCH_MASTER_KEY") != "" {
		fmt.Fprintln(os.Stderr, "主密钥来自环境变量 CLASH_LAUNCH_MASTER_KEY，无法自动轮换：请改用密钥文件")
		return 1
	}
	path := os.Getenv("CLASH_LAUNCH_MASTER_KEY_FILE")
	if path == "" {
		path = filepath.Join(c.DataDir, "master.key")
	}
	ctx := context.Background()
	log := logger(c.LogLevel)
	db, err := openStore(ctx, c, log)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = db.Close() }()
	id, err := store.AppendKey(path, db.Keys())
	if err != nil {
		fmt.Fprintln(os.Stderr, "追加密钥失败：", err)
		return 1
	}
	n, err := db.ReEncrypt(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "重新加密失败（新密钥已追加，旧密钥仍在文件中，可重试）：", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "当前密钥为 %s；重新加密了 %d 项。旧密钥保留在文件中，确认备份后可以手动删除。\n", id, n)
	return 0
}

func cmdBackup(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "用法：clash-launch backup <文件>")
		return 2
	}
	c, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ctx := context.Background()
	db, err := openStore(ctx, c, logger(c.LogLevel))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = db.Close() }()
	if err := db.Backup(ctx, args[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Fprintln(os.Stderr, "已备份到", args[0], "——请同时备份主密钥文件")
	return 0
}

// cmdDB 实现 db migrate 与 db copy --from <DSN> --to <DSN>（第 17.1 节）。
func cmdDB(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "用法：clash-launch db migrate | db copy -from <DSN> -to <DSN>")
		return 2
	}
	c, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ctx := context.Background()
	log := logger(c.LogLevel)
	switch args[0] {
	case "migrate":
		db, err := openStore(ctx, c, log)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		_ = db.Close()
		return 0
	case "copy":
		fs := flag.NewFlagSet("db copy", flag.ContinueOnError)
		from := fs.String("from", "", "源数据库 DSN")
		to := fs.String("to", "", "目标数据库 DSN（须为空）")
		if err := fs.Parse(args[1:]); err != nil || *from == "" || *to == "" {
			fmt.Fprintln(os.Stderr, "需要 -from 与 -to")
			return 2
		}
		keys, _, err := store.LoadKeyring(c.DataDir, os.Getenv)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		src, err := store.Open(ctx, *from, keys)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer func() { _ = src.Close() }()
		dst, err := store.Open(ctx, *to, keys)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer func() { _ = dst.Close() }()
		if _, err := dst.Migrate(ctx, ""); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		stats, err := store.Copy(ctx, src, dst)
		if err != nil {
			fmt.Fprintln(os.Stderr, "复制失败：", err)
			return 1
		}
		for _, s := range stats {
			fmt.Fprintf(os.Stderr, "%-14s %6d 行  %s\n", s.Table, s.Rows, s.Checksum[:12])
		}
		return 0
	}
	fmt.Fprintln(os.Stderr, "未知的 db 子命令")
	return 2
}

// cmdHealthcheck 请求本机的 /healthz（distroless 镜像中没有 curl）。
func cmdHealthcheck(_ []string) int {
	c, err := loadConfig()
	if err != nil {
		return 1
	}
	_, port, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return 1
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return 1
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
