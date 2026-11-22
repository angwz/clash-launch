package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/store"
)

// 改用户名与昵称（需求 R4、R5）。
//
// 用户名用于登录、好友与消息地址，改动的代价高：两次修改至少间隔 renameCooldown；改掉的旧名保留
// nameReserve，期间别人不能注册或改用（防冒充），本人可以改回。昵称只用于显示，可含空格、不唯一、随时可改，
// 所以列表里昵称旁总是同时显示 @用户名。

const (
	renameCooldown = 180 * 24 * time.Hour
	nameReserve    = 180 * 24 * time.Hour
	maxNickname    = 32
)

// RenameWaitError 表示距离上次改名不足 renameCooldown。
type RenameWaitError struct{ Until time.Time }

func (e *RenameWaitError) Error() string {
	return "service: 用户名每 180 天只能修改一次，下次可修改的时间：" + e.Until.UTC().Format(time.RFC3339)
}

// ErrNameReserved 表示用户名是别人最近改掉的旧名，仍在保留期内。
var ErrNameReserved = errors.New("service: 这个用户名最近被别人使用过，暂时不能使用")

// ErrNickname 表示昵称不合规。
var ErrNickname = errors.New("service: 昵称最多 32 个字符，不能包含控制字符，首尾不能是空白")

// NextRename 返回下一次可以改用户名的时间；从未改过时为零值（随时可改）。
func NextRename(u *store.User) time.Time {
	if u.UsernameChangedAt.IsZero() {
		return time.Time{}
	}
	return u.UsernameChangedAt.Add(renameCooldown)
}

// checkNameFree 检查用户名没有被别人在保留期内用过（注册与改名共用；已被现有用户占用由唯一约束保证）。
func (s *Service) checkNameFree(ctx context.Context, name, self string) error {
	reserved, err := s.Store.Users().NameReserved(ctx, store.NormalizeUsername(name), self, s.now().Add(-nameReserve))
	if err != nil {
		return err
	}
	if reserved {
		return ErrNameReserved
	}
	return nil
}

// Rename 改用户名：需要当前密码。只改大小写也算一次修改。
func (s *Service) Rename(ctx context.Context, userID, password, name string) (*store.User, error) {
	u, err := s.Store.Users().ByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !auth.VerifyPassword(u.PasswordHash, password) {
		return nil, ErrWrongPassword
	}
	if err := CheckUsername(name); err != nil {
		return nil, err
	}
	if name == u.Username {
		return u, nil
	}
	if next := NextRename(u); !next.IsZero() && s.now().Before(next) {
		return nil, &RenameWaitError{Until: next}
	}
	if err := s.checkNameFree(ctx, name, u.ID); err != nil {
		return nil, err
	}
	old := u.Username
	if err := s.Store.Users().Rename(ctx, u.ID, name, s.now()); err != nil {
		return nil, err
	}
	s.audit(ctx, u.ID, "account.username", map[string]string{"from": old, "to": name})
	return s.Store.Users().ByID(ctx, u.ID)
}

// CheckNickname 规范并校验昵称：去掉首尾空白，内部空白原样保留（用户写下的内容是有意为之）。
func CheckNickname(nick string) (string, error) {
	nick = strings.TrimSpace(nick)
	if utf8.RuneCountInString(nick) > maxNickname {
		return "", ErrNickname
	}
	for _, r := range nick {
		if unicode.IsControl(r) || r == 0x2028 || r == 0x2029 {
			return "", ErrNickname
		}
	}
	return nick, nil
}

// SetNickname 设置昵称，空串表示清除。
func (s *Service) SetNickname(ctx context.Context, userID, nick string) (string, error) {
	nick, err := CheckNickname(nick)
	if err != nil {
		return "", err
	}
	if err := s.Store.Users().SetNickname(ctx, userID, nick); err != nil {
		return "", err
	}
	s.audit(ctx, userID, "account.nickname", map[string]string{"nickname": nick})
	return nick, nil
}

// ResolveFormerName 把旧用户名解析为对方现在的用户名（O10）：旧的消息地址 /messages/<旧名> 据此跳到新名。
// 只解析与 me 有过对话的人（任一方向的联系人），不能借此查出陌生人改过什么名；名字现在仍属于某个人时原样返回。
func (s *Service) ResolveFormerName(ctx context.Context, me *store.User, name string) (string, error) {
	if u, err := s.Store.Users().ByUsername(ctx, name); err == nil {
		return u.Username, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	id, err := s.Store.Users().FormerOwner(ctx, name)
	if err != nil {
		return "", err
	}
	_, errMine := s.Store.Contacts().Get(ctx, me.ID, id)
	_, errTheirs := s.Store.Contacts().Get(ctx, id, me.ID)
	if errMine != nil && errTheirs != nil { // 两个方向都不是联系人
		return "", store.ErrNotFound
	}
	u, err := s.Store.Users().ByID(ctx, id)
	if err != nil || !u.DeletedAt.IsZero() {
		return "", store.ErrNotFound
	}
	return u.Username, nil
}
