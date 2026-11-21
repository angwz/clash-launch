// Package store 是唯一的持久化包：按领域划分的仓储接口、SQLite 与 PostgreSQL 实现、迁移、加密（第 17.1 节）。
//
// 业务代码只依赖这里的接口，不出现 SQL。每个仓储方法都以所有者 ID 为第一个参数（在 ctx 之后），
// 不提供绕过所有者的查询；管理用途另设只返回元数据的函数（第 14.3 节）。
package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// ErrNotFound 表示资源不存在，或不属于当前用户（两者对调用方不可区分，避免泄露资源存在）。
var ErrNotFound = errors.New("store: 不存在")

// ErrConflict 表示版本不一致（ETag/If-Match）。
var ErrConflict = errors.New("store: 版本冲突")

// ErrExists 表示违反唯一约束（如用户名已存在）。与版本冲突分开，API 分别返回 409 与 412。
var ErrExists = errors.New("store: 已存在")

var (
	idMu   sync.Mutex
	lastID [16]byte
)

// NewID 生成 UUIDv7（RFC 9562）：前 48 位为毫秒时间戳，其余为随机数。按时间有序，两种数据库通用。
// 同一进程内严格递增（RFC 9562 第 6.2 节）：同一毫秒内随机数更小或时钟回拨时，在上一个 ID 上加 1，
// 因此「时间相同按 ID 排序」就是生成顺序（订阅请求记录依赖这一点）。
func NewID() string {
	var b [16]byte
	binary.BigEndian.PutUint64(b[0:8], uint64(time.Now().UnixMilli())<<16)
	if _, err := rand.Read(b[6:]); err != nil {
		panic("store: 随机数不可用：" + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x70 // 版本 7
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 变体
	idMu.Lock()
	if bytes.Compare(b[:], lastID[:]) <= 0 {
		b = lastID
		// 只在最后 7 个字节（随机部分）进位，不碰版本与变体位；56 位在同一毫秒内不会溢出
		for i := 15; i >= 9; i-- {
			b[i]++
			if b[i] != 0 {
				break
			}
		}
	}
	lastID = b
	idMu.Unlock()
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
	Invited      bool // 经邀请注册（反馈 #46）：可自选节点写法
	Lang         string
	Timezone     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      int64
	// 需求 R4、R5、R12
	Nickname          string    // 昵称：可含空格，不唯一
	UsernameChangedAt time.Time // 最近一次改用户名，零值表示从未改过
	InvitedBy         string    // 邀请人的用户 ID
	InvitedByName     string    // 邀请人的用户名快照（邀请人注销后仍能显示）
	InviteID          string
	InviteBanned      bool // 管理员收回了邀请权
	// 需求 R40：同意的用户协议版本与时间；0 表示尚未同意
	TermsVersion int
	TermsAt      time.Time
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
	// LogoutAt 是定时退出的时间（需求 R8），零值表示未设定；生效时间取它与 ExpiresAt 中较早的
	LogoutAt time.Time
	// Location 是登录时得出的所在地（需求 R7），只用于展示
	Location string
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
	// Limits 是访问限制（需求 R42）；零值表示不限制
	Limits LinkLimits
}

// LinkLimits 是订阅链接的访问限制（需求 R42）。空列表或 0 表示该项不限制。
type LinkLimits struct {
	Clients   []string `json:"clients,omitempty"`   // 允许的客户端名称（User-Agent 中第一个「名称/版本」的名称，不区分大小写）
	Countries []string `json:"countries,omitempty"` // 允许的国家或地区（ISO 3166-1 alpha-2）
	PerDay    int      `json:"perDay,omitempty"`    // 每天最多成功拉取的次数（按所有者的时区计算「天」）
}

// Empty 报告是否没有任何限制。
func (l LinkLimits) Empty() bool {
	return len(l.Clients) == 0 && len(l.Countries) == 0 && l.PerDay == 0
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
	Templates() TemplateRepo
	Tokens() TokenRepo
	Notifications() NotificationRepo
	Stats() StatsRepo
	ShortLinks() ShortLinkRepo
	Messages() MessageRepo
	Contacts() ContactRepo
	Grants() GrantRepo
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
	// Search 按用户名前缀查找（不区分大小写，按用户名排序，最多 limit 个），用于收件人联想（反馈 #75）。
	Search(ctx context.Context, prefix string, limit int) ([]User, error)
	Update(ctx context.Context, u *User) error
	// Prefs 与 SetPrefs 读写界面偏好（JSON 对象，反馈 #37）；不改变 version。
	Prefs(ctx context.Context, id string) ([]byte, error)
	SetPrefs(ctx context.Context, id string, prefs []byte) error
	// SetInvited 记录账号是否经邀请注册（不改变 version：不是用户可编辑的资料）。
	SetInvited(ctx context.Context, id string, invited bool) error
	// —— 需求 R4、R5、R12 ——
	// SetNickname 设置昵称（空串清除）。
	SetNickname(ctx context.Context, id, nickname string) error
	// Rename 改用户名：旧名记入 username_history，记录修改时间；新名已被占用时返回 ErrExists。
	Rename(ctx context.Context, id, username string, at time.Time) error
	// NameReserved 报告用户名（规范形式）是否在 since 之后被 exceptUser 以外的人用过（改名后的保留期）。
	NameReserved(ctx context.Context, norm, exceptUser string, since time.Time) (bool, error)
	// SetInviter 记录邀请关系。
	SetInviter(ctx context.Context, id, inviterID, inviterName, inviteID string) error
	// SetInviteBanned 收回或恢复邀请权。
	SetInviteBanned(ctx context.Context, id string, banned bool) error
	// SetTerms 记录用户同意的用户协议版本与时间（需求 R40；不改变 version）。
	SetTerms(ctx context.Context, id string, version int, at time.Time) error
	// Invitees 返回由 inviterID 邀请注册的用户（注册早的在前）。
	Invitees(ctx context.Context, inviterID string) ([]User, error)
	Count(ctx context.Context) (int, error)
	List(ctx context.Context, p Page) ([]User, error) // 管理用：只含元数据
	// Paged 按页返回用户（按用户名排序）与总数；q 非空时按用户名包含 q 过滤（管理页分页，反馈 #76）
	Paged(ctx context.Context, q string, offset, limit int) ([]User, int, error)
}

// SessionRepo 管理会话。
type SessionRepo interface {
	Create(ctx context.Context, s *Session) error
	ByHash(ctx context.Context, idHash string) (*Session, error)
	Touch(ctx context.Context, idHash string, at time.Time) error
	Delete(ctx context.Context, userID, idHash string) error
	DeleteAll(ctx context.Context, userID string) error
	List(ctx context.Context, userID string) ([]Session, error)
	// SetLogoutAt 设定或取消（零值）定时退出。
	SetLogoutAt(ctx context.Context, userID, idHash string, at time.Time) error
}

// Notification 是一条站内提醒（需求 R23、R24）。
type Notification struct {
	ID        string
	OwnerID   string
	Kind      string
	Dedup     string // 同一件事只提醒一次
	Detail    []byte // JSON
	CreatedAt time.Time
	ReadAt    time.Time // 零值表示未读
}

// NotificationRepo 管理站内提醒。
type NotificationRepo interface {
	// Add 新增一条提醒；同一用户已有相同 dedup 时不插入，返回 false。顺带清理 180 天前的提醒。
	Add(ctx context.Context, n *Notification) (bool, error)
	// List 返回最近的提醒（新的在前），最多 limit 条。
	List(ctx context.Context, owner string, limit int) ([]Notification, error)
	Unread(ctx context.Context, owner string) (int, error)
	// MarkRead 把本人的提醒标为已读；ids 为空表示全部。
	MarkRead(ctx context.Context, owner string, ids []string, at time.Time) error
}

// SeenValue 是订阅链接见过的一个国家或客户端（需求 R23）。
type SeenValue struct {
	LinkID, OwnerID string
	Kind            string // country | client
	Value           string
	At              time.Time
}

// AccessToken 是个人访问令牌（需求 R25）。令牌只以哈希保存。
type AccessToken struct {
	ID         string
	OwnerID    string
	Name       string
	TokenHash  string
	Scope      string // read | write
	CreatedAt  time.Time
	ExpiresAt  time.Time // 零值表示不过期
	LastUsedAt time.Time
	LastIP     string
}

// TokenRepo 管理个人访问令牌。
type TokenRepo interface {
	Create(ctx context.Context, t *AccessToken) error
	ByHash(ctx context.Context, hash string) (*AccessToken, error)
	// List 返回本人的令牌（新的在前）。
	List(ctx context.Context, owner string) ([]AccessToken, error)
	Count(ctx context.Context, owner string) (int, error)
	Delete(ctx context.Context, owner, id string) error
	// DeleteAll 吊销本人的全部令牌（找回密码、管理员重设密码时）。
	DeleteAll(ctx context.Context, owner string) error
	Touch(ctx context.Context, id string, at time.Time, ip string) error
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
	// Autosave 与 Update 相同，但若最近一个版本也是自动保存且在 window 之内，就把它改写为新版本，
	// 而不是再加一个：连续编辑在历史中只留一条（反馈 #35）。
	Autosave(ctx context.Context, owner string, p *Profile, window time.Duration) error
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
	// Paged 按页返回本人的记录（新的在前）与总数；PagedAll 是管理员视图（反馈 #75：审计日志分页）。
	// Paged 与 PagedAll 按页读取；总数最多数到 maxCount+1（R14d：大表上不做完整计数），
	// 返回值大于 maxCount 表示还有更多。
	Paged(ctx context.Context, owner string, offset, limit, maxCount int) ([]AuditEntry, int, error)
	PagedAll(ctx context.Context, offset, limit, maxCount int) ([]AuditEntry, int, error)
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
	UsedBy    string // 第一次使用的用户
	MaxUses   int    // 可用次数上限（需求 R12）
	Uses      int
	RevokedAt time.Time
}

// InviteRepo 管理邀请（管理员功能；Use 供注册使用）。
type InviteRepo interface {
	Create(ctx context.Context, inv *Invite) error
	List(ctx context.Context, p Page) ([]Invite, error)
	Delete(ctx context.Context, id string) error
	// Use 原子地把一次使用记到未作废、未过期、未用完的邀请上，并记录使用者；邀请无效时返回 ErrNotFound。
	Use(ctx context.Context, tokenHash, userID string, now time.Time) (*Invite, error)
	// ByToken 按令牌哈希查找邀请（注册页显示邀请人；不判断是否仍有效）。
	ByToken(ctx context.Context, tokenHash string) (*Invite, error)
	// ByCreator 返回某人发出的邀请（新的在前）。
	ByCreator(ctx context.Context, creator string) ([]Invite, error)
	// CountSince 返回某人在 since 之后发出的邀请数（名额）。
	CountSince(ctx context.Context, creator string, since time.Time) (int, error)
	// Revoke 作废邀请；creator 非空时只能作废自己发出的。
	Revoke(ctx context.Context, id, creator string, at time.Time) error
	// UsedBy 返回使用过这个邀请的用户 ID（按使用时间）。
	UsedBy(ctx context.Context, inviteID string) ([]string, error)
}

// Template 是配置模板（反馈 #51）：一份不含名称的配置文档，加上名称与说明（如适用的客户端、mihomo 版本）。
type Template struct {
	ID        string
	OwnerID   string
	Name      string
	Note      string
	Doc       []byte
	Default   bool // 新建配置时默认选中；每个用户最多一个
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TemplateRepo 管理用户的配置模板。
type TemplateRepo interface {
	List(ctx context.Context, owner string) ([]Template, error)
	Get(ctx context.Context, owner, id string) (*Template, error)
	// Create 与 Update 在 Default 为 true 时清除该用户其他模板的默认标记（同一事务）。
	Create(ctx context.Context, owner string, t *Template) error
	Update(ctx context.Context, owner string, t *Template) error
	Delete(ctx context.Context, owner, id string) error
}
