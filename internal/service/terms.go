package service

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/angwz/clash-launch/internal/store"
)

// 用户协议（需求 R40）。
//
// 版本号是一个单调递增的整数，保存在 settings（terms.version，未设置时为 1，即随程序提供的默认文本）。
// 管理员每次修改或恢复默认文本都生成新版本；普通用户同意的版本低于当前版本时，必须先重新同意才能继续使用
// （API 层拦截，见 api.termsGate）。管理员维护协议本身，不受拦截。
//
// 文本中的 {{audit_days}}、{{request_days}} 在读取时替换为本站实际的保留期，管理员改写的文本也可以使用。

//go:embed terms/*.md
var termsFS embed.FS

const (
	settingTermsVersion = "terms.version"
	settingTermsText    = "terms.text" // JSON：{"zh-CN": "...", "en": "..."}；未设置或为空串时用默认文本
	settingTermsAt      = "terms.updated_at"

	// TermsMaxBytes 是每种语言协议文本的上限。
	TermsMaxBytes = 64 << 10
	// 读取当前版本号时的缓存时长：每个已登录请求都要检查，缓存避免逐请求读库；
	// 本进程修改后立即失效，多实例部署时其他实例最迟在这段时间后看到新版本。
	termsCacheTTL = 10 * time.Second
)

// TermsLangs 是协议提供的语言；第一个是缺省语言。
var TermsLangs = []string{"zh-CN", "en"}

var (
	// ErrTermsOutdated 表示提交的协议版本不是当前版本（页面打开期间协议被更新，或没有勾选同意）。
	ErrTermsOutdated = errors.New("service: 用户协议已更新，请阅读并同意最新版本")
	// ErrTermsText 表示管理员提交的协议文本无效。
	ErrTermsText = errors.New("service: 协议文本无效")
)

// Terms 是当前的用户协议。
type Terms struct {
	Version   int               `json:"version"`
	Texts     map[string]string `json:"texts"`  // 按语言，已替换保留期占位符
	Custom    bool              `json:"custom"` // 管理员改写过（否则为随程序提供的默认文本）
	UpdatedAt *time.Time        `json:"updatedAt,omitempty"`
}

type termsCache struct {
	mu      sync.Mutex
	version int
	at      time.Time
}

// DefaultTerms 返回随程序提供的默认文本（未替换占位符）。
func DefaultTerms() map[string]string {
	out := map[string]string{}
	for _, l := range TermsLangs {
		b, err := termsFS.ReadFile("terms/" + l + ".md")
		if err != nil {
			panic(fmt.Sprintf("内置的用户协议缺少 %s：%v", l, err)) // 构建时嵌入，缺失属于程序错误
		}
		out[l] = string(b)
	}
	return out
}

// TermsVersion 返回当前协议版本（带短时缓存）。
func (s *Service) TermsVersion(ctx context.Context) (int, error) {
	c := &s.terms
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.version > 0 && time.Since(c.at) < termsCacheTTL {
		return c.version, nil
	}
	v, err := s.Store.Settings().Get(ctx, settingTermsVersion)
	if errors.Is(err, store.ErrNotFound) {
		c.version, c.at = 1, time.Now()
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("全局设置 %s 的值无效：%q", settingTermsVersion, v)
	}
	c.version, c.at = n, time.Now()
	return n, nil
}

// CurrentTerms 返回当前协议的版本与文本（占位符已替换），供用户阅读。
func (s *Service) CurrentTerms(ctx context.Context) (Terms, error) {
	t, err := s.RawTerms(ctx)
	if err != nil {
		return Terms{}, err
	}
	for l, x := range t.Texts {
		t.Texts[l] = s.fillTerms(l, x)
	}
	return t, nil
}

// RawTerms 返回当前协议的版本与原始文本（占位符未替换），供管理员编辑。
func (s *Service) RawTerms(ctx context.Context) (Terms, error) {
	v, err := s.TermsVersion(ctx)
	if err != nil {
		return Terms{}, err
	}
	t := Terms{Version: v, Texts: DefaultTerms()}
	raw, err := s.Store.Settings().Get(ctx, settingTermsText)
	switch {
	case err == nil && raw != "":
		custom := map[string]string{}
		if err := json.Unmarshal([]byte(raw), &custom); err != nil {
			return Terms{}, fmt.Errorf("全局设置 %s 无法解析：%w", settingTermsText, err)
		}
		for l, x := range custom {
			t.Texts[l] = x
		}
		t.Custom = true
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return Terms{}, err
	}
	if at, err := s.Store.Settings().Get(ctx, settingTermsAt); err == nil {
		if ms, err := strconv.ParseInt(at, 10, 64); err == nil {
			x := time.UnixMilli(ms).UTC()
			t.UpdatedAt = &x
		}
	}
	return t, nil
}

// fillTerms 把保留期占位符替换为本站的实际设置。
func (s *Service) fillTerms(lang, text string) string {
	days := func(d time.Duration) string {
		n := int(d.Hours() / 24)
		switch {
		case n <= 0 && lang == "zh-CN":
			return "长期（不自动清理）"
		case n <= 0:
			return "an unlimited time (no automatic cleanup)"
		case lang == "zh-CN":
			return strconv.Itoa(n) + " 天"
		case n == 1:
			return "1 day"
		default:
			return strconv.Itoa(n) + " days"
		}
	}
	return strings.NewReplacer("{{audit_days}}", days(s.AuditRetention), "{{request_days}}", days(s.RequestRetention)).Replace(text)
}

// SetTerms 改写协议文本（管理员）；texts 为 nil 时恢复默认文本。两种情况都生成新版本，
// 普通用户下次使用时需要重新同意。返回新的协议。
func (s *Service) SetTerms(ctx context.Context, admin string, texts map[string]string) (Terms, error) {
	if texts != nil {
		for _, l := range TermsLangs {
			x := strings.TrimSpace(texts[l])
			if x == "" {
				return Terms{}, fmt.Errorf("%w：%s 的文本不能为空", ErrTermsText, l)
			}
			if len(x) > TermsMaxBytes || !utf8.ValidString(x) {
				return Terms{}, fmt.Errorf("%w：%s 的文本超过 %d 字节或不是有效的 UTF-8", ErrTermsText, l, TermsMaxBytes)
			}
		}
		for l := range texts {
			if !isTermsLang(l) {
				return Terms{}, fmt.Errorf("%w：不支持的语言 %q", ErrTermsText, l)
			}
		}
	}
	cur, err := s.TermsVersion(ctx)
	if err != nil {
		return Terms{}, err
	}
	next := cur + 1
	err = s.Store.Tx(ctx, func(tx store.Store) error {
		raw := "" // 空串表示恢复默认文本
		if texts != nil {
			b, err := json.Marshal(texts)
			if err != nil {
				return err
			}
			raw = string(b)
		}
		if err := tx.Settings().Set(ctx, settingTermsText, raw); err != nil {
			return err
		}
		if err := tx.Settings().Set(ctx, settingTermsVersion, strconv.Itoa(next)); err != nil {
			return err
		}
		if err := tx.Settings().Set(ctx, settingTermsAt, strconv.FormatInt(s.now().UnixMilli(), 10)); err != nil {
			return err
		}
		detail, _ := json.Marshal(map[string]any{"version": next, "default": texts == nil})
		return tx.Audit().Add(ctx, &store.AuditEntry{OwnerID: admin, Action: "admin.terms", Detail: detail})
	})
	if err != nil {
		return Terms{}, err
	}
	s.terms.mu.Lock()
	s.terms.version, s.terms.at = next, time.Now()
	s.terms.mu.Unlock()
	return s.CurrentTerms(ctx)
}

func isTermsLang(l string) bool {
	for _, x := range TermsLangs {
		if x == l {
			return true
		}
	}
	return false
}

// TermsPending 报告这个用户是否需要先同意最新的协议。管理员不受限制。
func (s *Service) TermsPending(ctx context.Context, u *store.User) (bool, error) {
	if u.Role == "admin" {
		return false, nil
	}
	v, err := s.TermsVersion(ctx)
	if err != nil {
		return false, err
	}
	return u.TermsVersion < v, nil
}

// AcceptTerms 记录用户同意了某个版本；版本不是当前版本时返回 ErrTermsOutdated（页面需要重新读取协议）。
func (s *Service) AcceptTerms(ctx context.Context, u *store.User, version int) error {
	cur, err := s.TermsVersion(ctx)
	if err != nil {
		return err
	}
	if version != cur {
		return ErrTermsOutdated
	}
	now := s.now()
	if err := s.Store.Users().SetTerms(ctx, u.ID, version, now); err != nil {
		return err
	}
	u.TermsVersion, u.TermsAt = version, now
	s.audit(ctx, u.ID, "account.terms_accept", map[string]string{"version": strconv.Itoa(version)})
	return nil
}

// DeclineTerms 记录用户拒绝了新版本的协议（随后由页面退出登录）。
func (s *Service) DeclineTerms(ctx context.Context, u *store.User, version int) {
	s.audit(ctx, u.ID, "account.terms_decline", map[string]string{"version": strconv.Itoa(version)})
}
