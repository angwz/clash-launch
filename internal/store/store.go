// Package store 是唯一的持久化包：按领域划分的仓储接口、SQLite 与 PostgreSQL 实现、迁移、加密（第 17.1 节）。
//
// 业务代码只依赖这里的接口，不出现 SQL。每个仓储方法都以所有者 ID 为第一个参数（在 ctx 之后），
// 不提供绕过所有者的查询；管理用途另设只返回元数据的函数（第 14.3 节）。
package store

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"time"
)

// ErrNotFound 表示资源不存在，或不属于当前用户（两者对调用方不可区分，避免泄露资源存在）。
var ErrNotFound = errors.New("store: 不存在")

// ErrConflict 表示版本不一致（ETag/If-Match）。
var ErrConflict = errors.New("store: 版本冲突")

// ErrExists 表示违反唯一约束（如用户名已存在）。与版本冲突分开，API 分别返回 409 与 412。
var ErrExists = errors.New("store: 已存在")

// NewID 生成 UUIDv7（RFC 9562）：前 48 位为毫秒时间戳，其余为随机数。按时间有序，两种数据库通用。
func NewID() string {
	var b [16]byte
	binary.BigEndian.PutUint64(b[0:8], uint64(time.Now().UnixMilli())<<16)
	if _, err := rand.Read(b[6:]); err != nil {
		panic("store: 随机数不可用：" + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x70 // 版本 7
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 变体
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// User 是账号。
type User struct {
	ID           string
	Username     string
	PasswordHash string
	Role         string // admin | user
	Disabled     bool
	Lang         string
	Timezone     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      int64
}

// Session 是登录会话。ID 只以哈希形式存储。
type Session struct {
	IDHash    string
	UserID    string
	CSRF      string
	CreatedAt time.Time
	LastSeen  time.Time
	ExpiresAt time.Time
	UA        string
	IPPrefix  string
}

// Source 是节点源。URL、内容与请求头在存储层加密。
type Source struct {
	ID         string
	OwnerID    string
	Name       string
	Kind       string // single | batch | subscription | file
	URL        string
	Content    []byte
	Headers    map[string]string
	UA         string
	Interval   int // 秒；0 表示默认
	Processing []byte
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Version    int64
}

// RuleSource 是规则源：URL 跟随更新，或在线编辑的文本。
type RuleSource struct {
	ID           string
	OwnerID      string
	Name         string
	Kind         string // toml | ini
	Preset       string
	GroupsURL    string
	RulesetsURL  string
	INIURL       string
	GroupsText   string
	RulesetsText string
	INIText      string
	Interval     int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      int64
}

// Snapshot 是拉取结果的快照，键为 "src/<id>" 或 "rs/<id>/<part>"。内容加密存储。
type Snapshot struct {
	Key          string
	OwnerID      string
	Content      []byte
	Hash         string
	Headers      map[string]string
	FetchedAt    time.Time
	Via          string
	LastError    string
	LastAttempt  time.Time
	FailingSince time.Time
}

// Profile 是方案；Doc 是方案文档（clash-launch/profile: v1），加密存储。
type Profile struct {
	ID        string
	OwnerID   string
	Name      string
	Doc       []byte
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int64
}

// Revision 是方案的一个版本。
type Revision struct {
	ID        string
	ProfileID string
	OwnerID   string
	N         int
	Doc       []byte
	Note      string
	CreatedAt time.Time
}

// Link 是订阅链接。令牌加密存储，另存 SHA-256 哈希用于查找。
type Link struct {
	ID         string
	OwnerID    string
	ProfileID  string
	Token      string
	TokenHash  string
	Note       string
	CreatedAt  time.Time
	LastAccess time.Time
	LastUA     string
	Version    int64
}

// Render 是方案的渲染状态：最近一次成功的结果（订阅端点提供它）与最近一次尝试。
type Render struct {
	ProfileID   string
	OwnerID     string
	GoodKey     string
	GoodYAML    []byte
	GoodETag    string
	GoodAt      time.Time
	GoodHeaders map[string]string
	LastKey     string
	LastStatus  string // ok | failed
	LastError   string
	LastDiags   []byte // JSON
	LastAt      time.Time
}

// AuditEntry 是一条审计记录。
type AuditEntry struct {
	ID      string
	OwnerID string
	At      time.Time
	Action  string
	Detail  []byte // JSON
}

// Page 是游标分页参数：After 为上一页最后一项的 ID（UUIDv7 按时间有序）。
type Page struct {
	After string
	Limit int
}

// Store 是全部仓储的入口。
type Store interface {
	Users() UserRepo
	Sessions() SessionRepo
	Sources() SourceRepo
	RuleSources() RuleSourceRepo
	Snapshots() SnapshotRepo
	Profiles() ProfileRepo
	Links() LinkRepo
	Renders() RenderRepo
	Audit() AuditRepo
	Settings() SettingsRepo
	Invites() InviteRepo
	Questions() QuestionRepo
	TOTP() TOTPRepo
	Passkeys() PasskeyRepo
	SMTP() SMTPRepo
	Quotas() QuotaRepo
	Devices() DeviceRepo
	// KeyStatus 返回当前主密钥 ID 与密钥环中的密钥数（管理页的系统状态）。
	KeyStatus() (current string, count int)
	// Tx 在一个事务中执行 fn；fn 返回错误时回滚。
	Tx(ctx context.Context, fn func(Store) error) error
	Close() error
	Dialect() string
}

// UserRepo 管理账号。账号本身没有所有者，ByID、ByUsername 供认证使用。
type UserRepo interface {
	Create(ctx context.Context, u *User) error
	ByID(ctx context.Context, id string) (*User, error)
	ByUsername(ctx context.Context, username string) (*User, error)
	Update(ctx context.Context, u *User) error
	Count(ctx context.Context) (int, error)
	List(ctx context.Context, p Page) ([]User, error) // 管理用：只含元数据
}

// SessionRepo 管理会话。
type SessionRepo interface {
	Create(ctx context.Context, s *Session) error
	ByHash(ctx context.Context, idHash string) (*Session, error)
	Touch(ctx context.Context, idHash string, at time.Time) error
	Delete(ctx context.Context, userID, idHash string) error
	DeleteAll(ctx context.Context, userID string) error
	List(ctx context.Context, userID string) ([]Session, error)
}

// SourceRepo 管理节点源。
type SourceRepo interface {
	Create(ctx context.Context, owner string, s *Source) error
	Get(ctx context.Context, owner, id string) (*Source, error)
	List(ctx context.Context, owner string, p Page) ([]Source, error)
	Update(ctx context.Context, owner string, s *Source) error // 按 Version 乐观锁
	Delete(ctx context.Context, owner, id string) error
	Due(ctx context.Context, now time.Time, limit int) ([]Source, error) // 后台拉取：到期的订阅（跨所有者，仅调度器使用）
}

// RuleSourceRepo 管理规则源。
type RuleSourceRepo interface {
	Create(ctx context.Context, owner string, r *RuleSource) error
	Get(ctx context.Context, owner, id string) (*RuleSource, error)
	List(ctx context.Context, owner string, p Page) ([]RuleSource, error)
	Update(ctx context.Context, owner string, r *RuleSource) error
	Delete(ctx context.Context, owner, id string) error
	All(ctx context.Context, limit int) ([]RuleSource, error) // 后台拉取（跨所有者，仅调度器使用）
}

// SnapshotRepo 管理快照。
type SnapshotRepo interface {
	Get(ctx context.Context, owner, key string) (*Snapshot, error)
	Put(ctx context.Context, owner string, s *Snapshot) error
	Delete(ctx context.Context, owner, prefix string) error
}

// ProfileRepo 管理方案与版本。
type ProfileRepo interface {
	Create(ctx context.Context, owner string, p *Profile, note string) error
	Get(ctx context.Context, owner, id string) (*Profile, error)
	List(ctx context.Context, owner string, p Page) ([]Profile, error)
	// Update 保存新文档并生成一个版本；Version 不一致时返回 ErrConflict。
	Update(ctx context.Context, owner string, p *Profile, note string) error
	Delete(ctx context.Context, owner, id string) error
	Revisions(ctx context.Context, owner, profileID string, p Page) ([]Revision, error)
	Revision(ctx context.Context, owner, profileID string, n int) (*Revision, error)
	All(ctx context.Context, limit int) ([]Profile, error) // 后台渲染（跨所有者，仅调度器使用）
}

// LinkRepo 管理订阅链接。
type LinkRepo interface {
	Create(ctx context.Context, owner string, l *Link) error
	Get(ctx context.Context, owner, id string) (*Link, error)
	List(ctx context.Context, owner, profileID string) ([]Link, error)
	Update(ctx context.Context, owner string, l *Link) error
	Delete(ctx context.Context, owner, id string) error
	// ByTokenHash 供订阅端点使用：按令牌哈希查找（调用方再做常数时间比较）。
	ByTokenHash(ctx context.Context, hash string) (*Link, error)
	Accessed(ctx context.Context, id string, at time.Time, ua string) error
}

// RenderRepo 管理渲染状态。
type RenderRepo interface {
	Get(ctx context.Context, owner, profileID string) (*Render, error)
	Put(ctx context.Context, owner string, r *Render) error
}

// AuditRepo 管理审计记录。
type AuditRepo interface {
	Add(ctx context.Context, e *AuditEntry) error
	List(ctx context.Context, owner string, p Page) ([]AuditEntry, error)
	// All 供管理员查看全部记录（新的在前）。
	All(ctx context.Context, p Page) ([]AuditEntry, error)
	// Prune 删除 before 之前的记录，返回删除的条数（保留期，第 14.7 节）。
	Prune(ctx context.Context, before time.Time) (int64, error)
}

// SettingsRepo 管理全局设置（注册模式、拉取策略等）。
type SettingsRepo interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

// Invite 是一次性邀请。令牌只以哈希存储。
type Invite struct {
	ID        string
	TokenHash string
	CreatedBy string
	Note      string
	CreatedAt time.Time
	ExpiresAt time.Time // 零值表示不过期
	UsedAt    time.Time
	UsedBy    string
}

// InviteRepo 管理邀请（管理员功能；Use 供注册使用）。
type InviteRepo interface {
	Create(ctx context.Context, inv *Invite) error
	List(ctx context.Context, p Page) ([]Invite, error)
	Delete(ctx context.Context, id string) error
	// Use 原子地把未使用、未过期的邀请标记为已用；邀请无效时返回 ErrNotFound。
	Use(ctx context.Context, tokenHash, userID string, now time.Time) (*Invite, error)
}
