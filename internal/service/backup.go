package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/store"
)

// 个人备份与还原（需求 R21，决定 0005 修订）。
//
// 备份文件包含账号资料（昵称、界面语言、时区、偏好）、节点源（含订阅地址与请求头）、规则源、配置与修订历史、
// 模板、订阅链接与短链接（含令牌）、提醒邮件与提醒渠道。文件用备份时输入的口令加密（argon2id + AES-256-GCM）：
// 里面有节点凭据，而服务器的主密钥不随文件走。
//
// 不在备份里、还原也不改动的：用户名、密码、两步验证、通行密钥、登录设备、访问令牌（登录方式不应因还原而改变）；
// 消息、好友、别人给你的授权，以及为好友生成的短链接（双方共有的关系）。
//
// 还原即整体覆盖：先为当前状态自动留底（服务器保存 7 天，可一键撤销），再在一个事务里删除当前的内容、写入备份的内容。
// ID、令牌与短码尽量沿用备份里的，订阅因此不断；已被其他账号占用时换新的，并在结果里列出。
// 还原后不再存在的节点源与为好友生成的短链接，先按 R2 收回授权，事后通知对方并重新生成对方的配置。

// 文件格式的标识与版本；格式变化时版本加一，旧版本的文件继续可以还原。
const (
	backupFormat     = "clash-launch/backup"
	backupFileFormat = "clash-launch/backup-encrypted"
	backupVersion    = 1
	// PreRestoreKind 是还原前自动留底的种类。
	PreRestoreKind = "pre_restore"
	// PreRestoreKeep 是自动留底的保留期。
	PreRestoreKeep = 7 * 24 * time.Hour
)

var (
	// ErrBackupPassphrase 表示备份口令错误或文件已损坏（AES-GCM 无法区分两者）。
	ErrBackupPassphrase = errors.New("service: 备份口令错误或文件已损坏")
	// ErrBackupFormat 表示不是本程序的备份文件，或版本更新、无法识别。
	ErrBackupFormat = errors.New("service: 不是可以识别的备份文件")
	// ErrBackupWeak 表示备份口令太短。
	ErrBackupWeak = errors.New("service: 备份口令至少 10 个字符")
)

// Backup 是备份的明文内容。字段名是文件格式的一部分，发布后只增不改。
type Backup struct {
	Format      string             `json:"format"`
	Version     int                `json:"version"`
	CreatedAt   time.Time          `json:"createdAt"`
	Server      string             `json:"server,omitempty"` // 备份来自哪个站点（只作说明）
	Account     BackupAccount      `json:"account"`
	Sources     []BackupSource     `json:"sources"`
	RuleSources []BackupRuleSource `json:"ruleSources"`
	Profiles    []BackupProfile    `json:"profiles"`
	Templates   []BackupTemplate   `json:"templates"`
	Links       []BackupLink       `json:"links"`
	ShortLinks  []BackupShortLink  `json:"shortLinks"`
	SMTP        *BackupSMTP        `json:"smtp,omitempty"`
	Channels    []BackupChannel    `json:"channels"`
}

type BackupAccount struct {
	Nickname string          `json:"nickname,omitempty"`
	Lang     string          `json:"lang,omitempty"`
	Timezone string          `json:"timezone,omitempty"`
	Prefs    json.RawMessage `json:"prefs,omitempty"`
}

type BackupSource struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Kind       string            `json:"kind"`
	URL        string            `json:"url,omitempty"`
	Content    string            `json:"content,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	UA         string            `json:"ua,omitempty"`
	Interval   int               `json:"interval,omitempty"`
	Processing json.RawMessage   `json:"processing,omitempty"`
}

type BackupRuleSource struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Preset       string `json:"preset,omitempty"`
	GroupsURL    string `json:"groupsUrl,omitempty"`
	RulesetsURL  string `json:"rulesetsUrl,omitempty"`
	INIURL       string `json:"iniUrl,omitempty"`
	GroupsText   string `json:"groupsText,omitempty"`
	RulesetsText string `json:"rulesetsText,omitempty"`
	INIText      string `json:"iniText,omitempty"`
	Interval     int    `json:"interval,omitempty"`
}

type BackupProfile struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Doc       string           `json:"doc"`
	Version   int64            `json:"version"`
	Protected bool             `json:"protected,omitempty"`
	Revisions []BackupRevision `json:"revisions"`
}

type BackupRevision struct {
	N         int       `json:"n"`
	Note      string    `json:"note,omitempty"`
	Doc       string    `json:"doc"`
	CreatedAt time.Time `json:"createdAt"`
}

type BackupTemplate struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Note    string `json:"note,omitempty"`
	Doc     string `json:"doc"`
	Default bool   `json:"default,omitempty"`
}

type BackupLink struct {
	ID        string           `json:"id"`
	ProfileID string           `json:"profileId"`
	Token     string           `json:"token"`
	Note      string           `json:"note,omitempty"`
	Limits    store.LinkLimits `json:"limits"`
}

type BackupShortLink struct {
	ID        string     `json:"id"`
	LinkID    string     `json:"linkId"`
	Code      string     `json:"code"`
	Note      string     `json:"note,omitempty"`
	Disabled  bool       `json:"disabled,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

type BackupSMTP struct {
	Host       string     `json:"host"`
	Port       int        `json:"port"`
	Security   string     `json:"security"`
	Username   string     `json:"username,omitempty"`
	Password   string     `json:"password,omitempty"`
	From       string     `json:"from"`
	To         string     `json:"to"`
	Enabled    bool       `json:"enabled,omitempty"`
	VerifiedAt *time.Time `json:"verifiedAt,omitempty"`
}

type BackupChannel struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	Name       string          `json:"name"`
	Config     json.RawMessage `json:"config"`
	Enabled    bool            `json:"enabled,omitempty"`
	VerifiedAt *time.Time      `json:"verifiedAt,omitempty"`
}

// BackupSummary 是一份备份（或当前账号）的内容数量，还原前给用户比较。
type BackupSummary struct {
	Sources     int       `json:"sources"`
	RuleSources int       `json:"ruleSources"`
	Profiles    int       `json:"profiles"`
	Revisions   int       `json:"revisions"`
	Templates   int       `json:"templates"`
	Links       int       `json:"links"`
	ShortLinks  int       `json:"shortLinks"`
	Channels    int       `json:"channels"`
	SMTP        bool      `json:"smtp"`
	CreatedAt   time.Time `json:"createdAt"`
	Server      string    `json:"server,omitempty"`
}

// Summary 统计备份的内容。
func (b *Backup) Summary() BackupSummary {
	n := BackupSummary{Sources: len(b.Sources), RuleSources: len(b.RuleSources), Profiles: len(b.Profiles), Templates: len(b.Templates),
		Links: len(b.Links), ShortLinks: len(b.ShortLinks), Channels: len(b.Channels), SMTP: b.SMTP != nil, CreatedAt: b.CreatedAt, Server: b.Server}
	for _, p := range b.Profiles {
		n.Revisions += len(p.Revisions)
	}
	return n
}

func timeOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func nilTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// BuildBackup 读取用户当前的全部个人数据。
func (s *Service) BuildBackup(ctx context.Context, u *store.User) (*Backup, error) {
	st := s.Store
	b := &Backup{Format: backupFormat, Version: backupVersion, CreatedAt: s.now().UTC(), Server: s.PublicURL,
		Sources: []BackupSource{}, RuleSources: []BackupRuleSource{}, Profiles: []BackupProfile{}, Templates: []BackupTemplate{},
		Links: []BackupLink{}, ShortLinks: []BackupShortLink{}, Channels: []BackupChannel{}}
	b.Account = BackupAccount{Nickname: u.Nickname, Lang: u.Lang, Timezone: u.Timezone}
	if p, err := st.Users().Prefs(ctx, u.ID); err == nil && len(p) > 0 {
		b.Account.Prefs = p
	}
	srcs, err := listPaged(func(after string) ([]store.Source, error) {
		return st.Sources().List(ctx, u.ID, store.Page{After: after, Limit: 200})
	}, func(x store.Source) string { return x.ID })
	if err != nil {
		return nil, err
	}
	for _, x := range srcs {
		b.Sources = append(b.Sources, BackupSource{ID: x.ID, Name: x.Name, Kind: x.Kind, URL: x.URL, Content: string(x.Content), Headers: x.Headers, UA: x.UA, Interval: x.Interval, Processing: rawJSON(x.Processing)})
	}
	rss, err := listPaged(func(after string) ([]store.RuleSource, error) {
		return st.RuleSources().List(ctx, u.ID, store.Page{After: after, Limit: 200})
	}, func(x store.RuleSource) string { return x.ID })
	if err != nil {
		return nil, err
	}
	for _, x := range rss {
		b.RuleSources = append(b.RuleSources, BackupRuleSource{ID: x.ID, Name: x.Name, Kind: x.Kind, Preset: x.Preset, GroupsURL: x.GroupsURL, RulesetsURL: x.RulesetsURL, INIURL: x.INIURL, GroupsText: x.GroupsText, RulesetsText: x.RulesetsText, INIText: x.INIText, Interval: x.Interval})
	}
	profs, err := listPaged(func(after string) ([]store.Profile, error) {
		return st.Profiles().List(ctx, u.ID, store.Page{After: after, Limit: 200})
	}, func(x store.Profile) string { return x.ID })
	if err != nil {
		return nil, err
	}
	for _, x := range profs {
		bp := BackupProfile{ID: x.ID, Name: x.Name, Doc: string(x.Doc), Version: x.Version, Protected: x.Protected, Revisions: []BackupRevision{}}
		// 修订按 N 从大到小分页
		after := ""
		for {
			revs, err := st.Profiles().Revisions(ctx, u.ID, x.ID, store.Page{After: after, Limit: 200})
			if err != nil {
				return nil, err
			}
			for _, v := range revs {
				bp.Revisions = append(bp.Revisions, BackupRevision{N: v.N, Note: v.Note, Doc: string(v.Doc), CreatedAt: v.CreatedAt.UTC()})
			}
			if len(revs) < 200 {
				break
			}
			after = fmt.Sprint(revs[len(revs)-1].N)
		}
		slices.SortFunc(bp.Revisions, func(a, c BackupRevision) int { return a.N - c.N })
		b.Profiles = append(b.Profiles, bp)
		links, err := st.Links().List(ctx, u.ID, x.ID)
		if err != nil {
			return nil, err
		}
		for _, l := range links {
			b.Links = append(b.Links, BackupLink{ID: l.ID, ProfileID: l.ProfileID, Token: l.Token, Note: l.Note, Limits: l.Limits})
		}
	}
	tpls, err := st.Templates().List(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	for _, x := range tpls {
		b.Templates = append(b.Templates, BackupTemplate{ID: x.ID, Name: x.Name, Note: x.Note, Doc: string(x.Doc), Default: x.Default})
	}
	shorts, err := st.ShortLinks().List(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	for _, x := range shorts {
		if x.GranteeID != "" {
			continue // 为好友生成的短链接属于双方的授权关系，不在备份内
		}
		b.ShortLinks = append(b.ShortLinks, BackupShortLink{ID: x.ID, LinkID: x.LinkID, Code: x.Code, Note: x.Note, Disabled: x.Disabled, ExpiresAt: timeOrNil(x.ExpiresAt)})
	}
	if m, err := st.SMTP().Get(ctx, u.ID); err == nil {
		b.SMTP = &BackupSMTP{Host: m.Host, Port: m.Port, Security: m.Security, Username: m.Username, Password: m.Password, From: m.From, To: m.To, Enabled: m.Enabled, VerifiedAt: timeOrNil(m.VerifiedAt)}
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	chs, err := st.Channels().List(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	for _, c := range chs {
		b.Channels = append(b.Channels, BackupChannel{ID: c.ID, Kind: c.Kind, Name: c.Name, Config: rawJSON(c.Config), Enabled: c.Enabled, VerifiedAt: timeOrNil(c.VerifiedAt)})
	}
	return b, nil
}

// listPaged 按 ID 游标取完一个分页列表。
func listPaged[T any](page func(after string) ([]T, error), id func(T) string) ([]T, error) {
	var out []T
	after := ""
	for {
		xs, err := page(after)
		if err != nil {
			return nil, err
		}
		out = append(out, xs...)
		if len(xs) < 200 {
			return out, nil
		}
		after = id(xs[len(xs)-1])
	}
}

func rawJSON(b []byte) json.RawMessage {
	if len(b) == 0 || !json.Valid(b) {
		return nil
	}
	return json.RawMessage(b)
}

// —— 文件加密 ——

// argon2id 参数：备份与还原都不频繁，比登录口令更重的参数（m=64 MiB, t=3, p=1）抵抗离线猜测。
const backupArgonTime, backupArgonMemory, backupArgonThreads = 3, 64 * 1024, 1

type backupFile struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	KDF     string `json:"kdf"`
	Time    uint32 `json:"t"`
	Memory  uint32 `json:"m"`
	Threads uint8  `json:"p"`
	Salt    []byte `json:"salt"`
	Nonce   []byte `json:"nonce"`
	Data    []byte `json:"data"`
}

// SealBackup 用口令加密备份，返回文件内容（JSON 外壳，正文是 AES-256-GCM 密文）。
func SealBackup(b *Backup, passphrase string) ([]byte, error) {
	if len([]rune(passphrase)) < 10 {
		return nil, ErrBackupWeak
	}
	plain, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	f := backupFile{Format: backupFileFormat, Version: backupVersion, KDF: "argon2id", Time: backupArgonTime, Memory: backupArgonMemory, Threads: backupArgonThreads,
		Salt: make([]byte, 16), Nonce: make([]byte, 12)}
	if _, err := rand.Read(f.Salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(f.Nonce); err != nil {
		return nil, err
	}
	aead, err := backupAEAD(passphrase, f)
	if err != nil {
		return nil, err
	}
	// 外壳的格式与参数作为附加数据：被篡改时解密失败
	f.Data = aead.Seal(nil, f.Nonce, plain, backupAD(f))
	return json.MarshalIndent(f, "", "  ")
}

// OpenBackup 解密并解析备份文件。
func OpenBackup(file []byte, passphrase string) (*Backup, error) {
	var f backupFile
	if err := json.Unmarshal(file, &f); err != nil || f.Format != backupFileFormat || f.KDF != "argon2id" {
		return nil, ErrBackupFormat
	}
	if f.Version > backupVersion {
		return nil, fmt.Errorf("%w：文件版本 %d 比本程序新，请先升级", ErrBackupFormat, f.Version)
	}
	// 参数来自文件：限制范围，防止构造的文件让服务器耗尽内存或时间
	if f.Time < 1 || f.Time > 10 || f.Memory < 8*1024 || f.Memory > 256*1024 || f.Threads < 1 || f.Threads > 8 || len(f.Salt) < 16 || len(f.Nonce) != 12 {
		return nil, ErrBackupFormat
	}
	aead, err := backupAEAD(passphrase, f)
	if err != nil {
		return nil, err
	}
	plain, err := aead.Open(nil, f.Nonce, f.Data, backupAD(f))
	if err != nil {
		return nil, ErrBackupPassphrase
	}
	var b Backup
	if err := json.Unmarshal(plain, &b); err != nil || b.Format != backupFormat {
		return nil, ErrBackupFormat
	}
	return &b, nil
}

func backupAEAD(passphrase string, f backupFile) (cipher.AEAD, error) {
	key := argon2.IDKey([]byte(passphrase), f.Salt, f.Time, f.Memory, f.Threads, 32)
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(blk)
}

func backupAD(f backupFile) []byte {
	return fmt.Appendf(nil, "%s|%d|%s|%d|%d|%d", f.Format, f.Version, f.KDF, f.Time, f.Memory, f.Threads)
}

// —— 备份、还原与撤销 ——

// ExportBackup 生成加密的备份文件（需要当前密码），记入审计。
func (s *Service) ExportBackup(ctx context.Context, u *store.User, password, passphrase string) ([]byte, error) {
	if !auth.VerifyPassword(u.PasswordHash, password) {
		return nil, ErrWrongPassword
	}
	b, err := s.BuildBackup(ctx, u)
	if err != nil {
		return nil, err
	}
	file, err := SealBackup(b, passphrase)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, u.ID, "account.backup", nil)
	return file, nil
}

// RestorePreview 比较当前账号与备份的内容数量。
func (s *Service) RestorePreview(ctx context.Context, u *store.User, file []byte, passphrase string) (current, incoming BackupSummary, err error) {
	b, err := OpenBackup(file, passphrase)
	if err != nil {
		return current, incoming, err
	}
	cur, err := s.BuildBackup(ctx, u)
	if err != nil {
		return current, incoming, err
	}
	return cur.Summary(), b.Summary(), nil
}

// RestoreReport 是一次还原的结果。
type RestoreReport struct {
	Summary BackupSummary `json:"summary"`
	// 因已被其他账号使用而换新的令牌与短码：客户端里对应的订阅需要更新
	NewTokens []string  `json:"newTokens"` // 订阅链接的备注（没有备注时为 ID）
	NewCodes  []string  `json:"newCodes"`  // 原短码 → 新短码，形如 "abc → xyz"
	Revoked   int       `json:"revoked"`   // 因对象不再存在而收回的授权数
	UndoUntil time.Time `json:"undoUntil"`
}

// Restore 用备份整体覆盖当前账号的个人数据（需要当前密码）。覆盖前自动留底，7 天内可以撤销。
func (s *Service) Restore(ctx context.Context, u *store.User, file []byte, passphrase, password string) (*RestoreReport, error) {
	if !auth.VerifyPassword(u.PasswordHash, password) {
		return nil, ErrWrongPassword
	}
	b, err := OpenBackup(file, passphrase)
	if err != nil {
		return nil, err
	}
	// 自动留底：当前状态以服务器主密钥加密保存
	cur, err := s.BuildBackup(ctx, u)
	if err != nil {
		return nil, err
	}
	snap, err := json.Marshal(cur)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if err := s.Store.Backups().Put(ctx, &store.UserBackup{OwnerID: u.ID, Kind: PreRestoreKind, Data: snap, CreatedAt: now, ExpiresAt: now.Add(PreRestoreKeep)}); err != nil {
		return nil, err
	}
	rep, err := s.apply(ctx, u, b)
	if err != nil {
		return nil, err
	}
	rep.UndoUntil = now.Add(PreRestoreKeep)
	s.audit(ctx, u.ID, "account.restore", map[string]string{"profiles": fmt.Sprint(len(b.Profiles)), "sources": fmt.Sprint(len(b.Sources))})
	return rep, nil
}

// UndoStatus 返回可撤销的还原留底的时间；没有时返回 nil。
func (s *Service) UndoStatus(ctx context.Context, u *store.User) (*store.UserBackup, error) {
	x, err := s.Store.Backups().Get(ctx, u.ID, PreRestoreKind, s.now())
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	return x, err
}

// UndoRestore 用自动留底恢复到还原之前（需要当前密码），随后删除留底。
func (s *Service) UndoRestore(ctx context.Context, u *store.User, password string) (*RestoreReport, error) {
	if !auth.VerifyPassword(u.PasswordHash, password) {
		return nil, ErrWrongPassword
	}
	x, err := s.Store.Backups().Get(ctx, u.ID, PreRestoreKind, s.now())
	if err != nil {
		return nil, err
	}
	var b Backup
	if err := json.Unmarshal(x.Data, &b); err != nil {
		return nil, fmt.Errorf("读取还原前的留底：%w", err)
	}
	rep, err := s.apply(ctx, u, &b)
	if err != nil {
		return nil, err
	}
	if err := s.Store.Backups().Delete(ctx, u.ID, PreRestoreKind); err != nil {
		return nil, err
	}
	s.audit(ctx, u.ID, "account.restore_undo", nil)
	return rep, nil
}

// apply 把备份写入当前账号：收回将要消失的对象的授权、在一个事务中替换数据、事后通知与重新生成。
func (s *Service) apply(ctx context.Context, u *store.User, b *Backup) (*RestoreReport, error) {
	rep := &RestoreReport{Summary: b.Summary(), NewTokens: []string{}, NewCodes: []string{}}
	inBackup := func(ids []string, id string) bool { return slices.Contains(ids, id) }
	var srcIDs, linkIDs []string
	for _, x := range b.Sources {
		srcIDs = append(srcIDs, x.ID)
	}
	for _, x := range b.Links {
		linkIDs = append(linkIDs, x.ID)
	}

	// R2：还原后不再存在的节点源与为好友生成的短链接，先收回授权
	type gone struct {
		grantee, target, name string
	}
	var notices []gone
	var affected []store.Grant
	curSrcs, err := listPaged(func(after string) ([]store.Source, error) {
		return s.Store.Sources().List(ctx, u.ID, store.Page{After: after, Limit: 200})
	}, func(x store.Source) string { return x.ID })
	if err != nil {
		return nil, err
	}
	for _, x := range curSrcs {
		if inBackup(srcIDs, x.ID) {
			continue
		}
		gs, err := s.Store.Grants().ByTarget(ctx, u.ID, "source", x.ID)
		if err != nil {
			return nil, err
		}
		for _, g := range gs {
			notices = append(notices, gone{g.GranteeID, "source", x.Name})
		}
		affected = append(affected, gs...)
	}
	curShorts, err := s.Store.ShortLinks().List(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	var keepShared []store.ShortLink // 为好友生成、所指链接仍在备份里的短链接：原样保留
	for _, sl := range curShorts {
		if sl.GranteeID == "" {
			continue
		}
		if inBackup(linkIDs, sl.LinkID) {
			keepShared = append(keepShared, sl)
			continue
		}
		gs, err := s.shortGrants(ctx, u, []store.ShortLink{sl})
		if err != nil {
			return nil, err
		}
		for _, g := range gs {
			notices = append(notices, gone{g.GranteeID, "short", sl.Code})
		}
		affected = append(affected, gs...)
	}
	if err := s.revokeAll(ctx, u, affected); err != nil {
		return nil, err
	}
	rep.Revoked = len(affected)

	var profileIDs []string
	err = s.Store.Tx(ctx, func(st store.Store) error {
		var err error
		profileIDs, err = s.replaceAll(ctx, st, u, b, keepShared, rep)
		return err
	})
	if err != nil {
		return nil, err
	}

	for _, n := range notices {
		s.notifyDeleted(ctx, u, n.grantee, n.target, n.name)
	}
	for _, g := range affected {
		if g.Kind == "source" {
			s.rerenderUsing(ctx, g.GranteeID, usesSource(g.TargetID))
		}
	}
	for _, id := range profileIDs {
		if _, _, err := s.RenderProfile(ctx, u.ID, id); err != nil && s.Log != nil {
			s.Log.Warn("还原后重新生成配置失败", "profile", id, "error", err)
		}
	}
	return rep, nil
}

// replaceAll 在事务中删除当前的个人数据并写入备份。返回写入的配置 ID（事后重新生成）。
func (s *Service) replaceAll(ctx context.Context, st store.Store, u *store.User, b *Backup, keepShared []store.ShortLink, rep *RestoreReport) ([]string, error) {
	// 删除：配置级联删除修订、订阅链接、短链接与生成结果；其余逐项删除
	profs, err := listPaged(func(after string) ([]store.Profile, error) {
		return st.Profiles().List(ctx, u.ID, store.Page{After: after, Limit: 200})
	}, func(x store.Profile) string { return x.ID })
	if err != nil {
		return nil, err
	}
	for _, x := range profs {
		if err := st.Profiles().Delete(ctx, u.ID, x.ID); err != nil {
			return nil, err
		}
	}
	srcs, err := listPaged(func(after string) ([]store.Source, error) {
		return st.Sources().List(ctx, u.ID, store.Page{After: after, Limit: 200})
	}, func(x store.Source) string { return x.ID })
	if err != nil {
		return nil, err
	}
	for _, x := range srcs {
		if err := st.Sources().Delete(ctx, u.ID, x.ID); err != nil {
			return nil, err
		}
	}
	rss, err := listPaged(func(after string) ([]store.RuleSource, error) {
		return st.RuleSources().List(ctx, u.ID, store.Page{After: after, Limit: 200})
	}, func(x store.RuleSource) string { return x.ID })
	if err != nil {
		return nil, err
	}
	for _, x := range rss {
		if err := st.RuleSources().Delete(ctx, u.ID, x.ID); err != nil {
			return nil, err
		}
	}
	tpls, err := st.Templates().List(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	for _, x := range tpls {
		if err := st.Templates().Delete(ctx, u.ID, x.ID); err != nil {
			return nil, err
		}
	}
	chs, err := st.Channels().List(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	for _, x := range chs {
		if err := st.Channels().Delete(ctx, u.ID, x.ID); err != nil {
			return nil, err
		}
	}

	// ID 沿用备份里的；已被其他账号使用时换新的，并把文档里的引用一并替换
	remap := map[string]string{}
	pick := func(table, id string) (string, error) {
		if id == "" {
			return store.NewID(), nil
		}
		taken, err := st.Taken(ctx, table, id)
		if err != nil {
			return "", err
		}
		if taken {
			n := store.NewID()
			remap[id] = n
			return n, nil
		}
		return id, nil
	}
	fix := func(doc string) string {
		for from, to := range remap {
			doc = strings.ReplaceAll(doc, from, to)
		}
		return doc
	}
	for _, x := range b.Sources {
		id, err := pick("sources", x.ID)
		if err != nil {
			return nil, err
		}
		src := &store.Source{ID: id, Name: x.Name, Kind: x.Kind, URL: x.URL, Content: []byte(x.Content), Headers: x.Headers, UA: x.UA, Interval: x.Interval, Processing: []byte(x.Processing)}
		if err := st.Sources().Create(ctx, u.ID, src); err != nil {
			return nil, fmt.Errorf("还原节点源 %s：%w", x.Name, err)
		}
	}
	for _, x := range b.RuleSources {
		id, err := pick("rule_sources", x.ID)
		if err != nil {
			return nil, err
		}
		rs := &store.RuleSource{ID: id, Name: x.Name, Kind: x.Kind, Preset: x.Preset, GroupsURL: x.GroupsURL, RulesetsURL: x.RulesetsURL, INIURL: x.INIURL, GroupsText: x.GroupsText, RulesetsText: x.RulesetsText, INIText: x.INIText, Interval: x.Interval}
		if err := st.RuleSources().Create(ctx, u.ID, rs); err != nil {
			return nil, fmt.Errorf("还原规则源 %s：%w", x.Name, err)
		}
	}
	var profileIDs []string
	for _, x := range b.Profiles {
		id, err := pick("profiles", x.ID)
		if err != nil {
			return nil, err
		}
		p := &store.Profile{ID: id, Name: x.Name, Doc: []byte(fix(x.Doc)), Protected: x.Protected}
		if err := st.Profiles().Create(ctx, u.ID, p, ""); err != nil {
			return nil, fmt.Errorf("还原配置 %s：%w", x.Name, err)
		}
		revs := make([]store.Revision, 0, len(x.Revisions))
		for _, v := range x.Revisions {
			revs = append(revs, store.Revision{N: v.N, Note: v.Note, Doc: []byte(fix(v.Doc)), CreatedAt: v.CreatedAt})
		}
		version := x.Version
		if version < 1 {
			version = 1
		}
		if err := st.Profiles().SetRevisions(ctx, u.ID, id, version, revs); err != nil {
			return nil, err
		}
		if x.Protected {
			if err := st.Profiles().SetProtected(ctx, u.ID, id, true); err != nil {
				return nil, err
			}
		}
		profileIDs = append(profileIDs, id)
	}
	for _, x := range b.Links {
		id, err := pick("links", x.ID)
		if err != nil {
			return nil, err
		}
		pid := x.ProfileID
		if n, ok := remap[pid]; ok {
			pid = n
		}
		tok := x.Token
		if tok == "" {
			tok = auth.NewToken(32)
		}
		// 令牌已被其他账号的链接使用：换新令牌，客户端里的这条订阅需要更新
		if other, err := st.Links().ByTokenHash(ctx, auth.HashToken(tok)); err == nil && other != nil {
			tok = auth.NewToken(32)
			label := x.Note
			if label == "" {
				label = x.ID
			}
			rep.NewTokens = append(rep.NewTokens, label)
		} else if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		l := &store.Link{ID: id, ProfileID: pid, Token: tok, TokenHash: auth.HashToken(tok), Note: x.Note, Limits: x.Limits}
		if err := st.Links().Create(ctx, u.ID, l); err != nil {
			return nil, fmt.Errorf("还原订阅链接：%w", err)
		}
		if !x.Limits.Empty() {
			if err := st.Links().Update(ctx, u.ID, l); err != nil {
				return nil, err
			}
		}
	}
	for _, x := range b.ShortLinks {
		id, err := pick("short_links", x.ID)
		if err != nil {
			return nil, err
		}
		lid := x.LinkID
		if n, ok := remap[lid]; ok {
			lid = n
		}
		code := x.Code
		if other, err := st.ShortLinks().ByCode(ctx, code); err == nil && other != nil {
			code = randomCode(8)
			rep.NewCodes = append(rep.NewCodes, x.Code+" → "+code)
		} else if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		sl := &store.ShortLink{ID: id, LinkID: lid, Code: code, Note: x.Note, Disabled: x.Disabled, ExpiresAt: nilTime(x.ExpiresAt)}
		if err := st.ShortLinks().Create(ctx, u.ID, sl); err != nil {
			return nil, fmt.Errorf("还原短链接 %s：%w", x.Code, err)
		}
	}
	// 为好友生成的短链接：所指链接仍在，按原 ID 与短码放回（授权记录指向这个 ID）
	for _, sl := range keepShared {
		x := sl
		if err := st.ShortLinks().Create(ctx, u.ID, &x); err != nil {
			return nil, fmt.Errorf("保留为好友生成的短链接 %s：%w", sl.Code, err)
		}
	}
	for _, x := range b.Templates {
		id, err := pick("templates", x.ID)
		if err != nil {
			return nil, err
		}
		if err := st.Templates().Create(ctx, u.ID, &store.Template{ID: id, Name: x.Name, Note: x.Note, Doc: []byte(fix(x.Doc)), Default: x.Default}); err != nil {
			return nil, fmt.Errorf("还原模板 %s：%w", x.Name, err)
		}
	}
	for _, x := range b.Channels {
		id, err := pick("notify_channels", x.ID)
		if err != nil {
			return nil, err
		}
		if err := st.Channels().Create(ctx, &store.Channel{ID: id, OwnerID: u.ID, Kind: x.Kind, Name: x.Name, Config: []byte(x.Config), Enabled: x.Enabled, VerifiedAt: nilTime(x.VerifiedAt), CreatedAt: s.now()}); err != nil {
			return nil, err
		}
	}
	if b.SMTP != nil {
		m := b.SMTP
		if err := st.SMTP().Put(ctx, &store.SMTP{UserID: u.ID, Host: m.Host, Port: m.Port, Security: m.Security, Username: m.Username, Password: m.Password, From: m.From, To: m.To, Enabled: m.Enabled, VerifiedAt: nilTime(m.VerifiedAt)}); err != nil {
			return nil, err
		}
	} else if err := st.SMTP().Delete(ctx, u.ID); err != nil {
		return nil, err
	}
	// 账号资料：昵称、语言、时区与偏好；用户名与登录方式不变
	if err := st.Users().SetNickname(ctx, u.ID, b.Account.Nickname); err != nil {
		return nil, err
	}
	if len(b.Account.Prefs) > 0 {
		if err := st.Users().SetPrefs(ctx, u.ID, b.Account.Prefs); err != nil {
			return nil, err
		}
	}
	if b.Account.Lang != "" || b.Account.Timezone != "" {
		cur, err := st.Users().ByID(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		if b.Account.Lang != "" {
			cur.Lang = b.Account.Lang
		}
		if b.Account.Timezone != "" {
			cur.Timezone = b.Account.Timezone
		}
		if err := st.Users().Update(ctx, cur); err != nil {
			return nil, err
		}
	}
	return profileIDs, nil
}

// PurgeBackups 删除过期的还原留底（每天一次）。失败只记日志。
func (s *Service) PurgeBackups(ctx context.Context) {
	n, err := s.Store.Backups().Purge(ctx, s.now())
	if err != nil && s.Log != nil {
		s.Log.Warn("清理过期的还原留底失败", "error", err)
		return
	}
	if n > 0 && s.Log != nil {
		s.Log.Info("已清理过期的还原留底", "count", n)
	}
}
