package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // 注册解码器：只取第一帧
	"image/jpeg"
	"image/png"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/angwz/clash-launch/internal/store"
)

// 头像与默认昵称（需求 R19）。
//
// 预置头像：10 种「启动」主题的图形 × 6 种颜色，前端按标识绘制 SVG（web/src/components/Avatar.svelte），
// 后端只校验标识、随机分配。上传的图片在服务器裁成正方形、缩到 256 px 并重新编码：重新编码丢掉了 EXIF
// （可能含拍摄地点）与图片中的其他附加数据。支持 PNG、JPEG 与 GIF（第一帧）；WebP 需要 golang.org/x/image，
// 未引入（见决定 0001 #17：不把 golang.org/x/* 升到 mihomo 之上）。

// AvatarShapes 与 AvatarColors 与前端的 Avatar.svelte 一致（TestAvatarPresetsMatchFrontend 锁定）。
var AvatarShapes = []string{"rocket", "planet", "comet", "satellite", "constellation", "launchpad", "moon", "sun", "telescope", "orbit"}

const AvatarColors = 6

const (
	avatarMaxBytes  = 2 << 20  // 上传上限 2 MiB
	avatarMaxPixels = 40 << 20 // 解码前按尺寸拒绝过大的图片（解压炸弹）
	avatarSize      = 256
)

var (
	// ErrAvatar 表示头像标识无效或图片无法处理。
	ErrAvatar = errors.New("avatar: invalid")
	// ErrAvatarTooLarge 表示上传的图片超过 2 MiB 或尺寸过大。
	ErrAvatarTooLarge = errors.New("avatar: too large")
)

// ValidPreset 报告 key（如 p:rocket-3）是否是预置头像。
func ValidPreset(key string) bool {
	rest, ok := strings.CutPrefix(key, "p:")
	if !ok {
		return false
	}
	shape, n, ok := strings.Cut(rest, "-")
	if !ok || !slices.Contains(AvatarShapes, shape) {
		return false
	}
	c, err := strconv.Atoi(n)
	return err == nil && c >= 1 && c <= AvatarColors && strconv.Itoa(c) == n
}

func randInt(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(err) // crypto/rand 不会失败（Go 1.24 起失败时直接终止进程）
	}
	return int(v.Int64())
}

// RandomPreset 随机选一个预置头像。
func RandomPreset() string {
	return "p:" + AvatarShapes[randInt(len(AvatarShapes))] + "-" + strconv.Itoa(1+randInt(AvatarColors))
}

// 预置昵称：「形容词 + 名词」，取自启动与星空的意象。中文约 40 × 40、英文约 40 × 40 种组合。昵称不唯一，旁边总显示 @用户名。
var (
	nickAdjZH  = []string{"疾行的", "闪耀的", "安静的", "远航的", "微光的", "炽热的", "温柔的", "勇敢的", "自由的", "明亮的", "遥远的", "轻盈的", "好奇的", "沉稳的", "飞驰的", "闪烁的", "清澈的", "骄傲的", "迷路的", "守望的", "漫游的", "无畏的", "灿烂的", "深邃的", "悠闲的", "坚定的", "欢快的", "孤独的", "神秘的", "耀眼的", "优雅的", "敏捷的", "发光的", "醒着的", "追光的", "环绕的", "升空的", "破晓的", "回旋的", "不眠的"}
	nickNounZH = []string{"彗星", "火箭", "卫星", "行星", "星云", "流星", "月球", "探测器", "空间站", "极光", "星座", "银河", "宇航员", "望远镜", "发射台", "轨道", "光年", "小行星", "星舰", "太阳帆", "脉冲星", "星尘", "天文台", "登月舱", "星环", "新星", "着陆器", "指令舱", "推进器", "信标", "灯塔", "北极星", "晨星", "暮星", "星图", "天线", "舷窗", "火流星", "月牙", "星火"}
	nickAdjEN  = []string{"Swift", "Bright", "Quiet", "Distant", "Gentle", "Brave", "Free", "Curious", "Steady", "Glowing", "Shining", "Clear", "Proud", "Wandering", "Fearless", "Radiant", "Deep", "Calm", "Bold", "Lively", "Lonely", "Mystic", "Dazzling", "Graceful", "Nimble", "Wakeful", "Orbiting", "Rising", "Dawning", "Spinning", "Sleepless", "Silver", "Golden", "Cosmic", "Lunar", "Solar", "Stellar", "Polar", "Electric", "Hidden"}
	nickNounEN = []string{"Comet", "Rocket", "Satellite", "Planet", "Nebula", "Meteor", "Moon", "Probe", "Station", "Aurora", "Galaxy", "Astronaut", "Telescope", "Launchpad", "Orbit", "Lightyear", "Asteroid", "Starship", "Sail", "Pulsar", "Stardust", "Lander", "Quasar", "Ring", "Nova", "Module", "Thruster", "Beacon", "Lighthouse", "Polestar", "Starlight", "Horizon", "Starmap", "Antenna", "Capsule", "Voyager", "Eclipse", "Spark", "Crescent", "Fireball"}
)

// RandomNickname 按界面语言随机生成一个预置昵称：中文界面用中文词表，其余用英文。
func RandomNickname(lang string) string {
	if strings.HasPrefix(strings.ToLower(lang), "zh") {
		return nickAdjZH[randInt(len(nickAdjZH))] + nickNounZH[randInt(len(nickNounZH))]
	}
	return nickAdjEN[randInt(len(nickAdjEN))] + " " + nickNounEN[randInt(len(nickNounEN))]
}

// assignDefaults 为还没有头像的用户随机分配预置头像；昵称为空时一并分配预置昵称。只在头像为空（从未分配）时进行，
// 之后用户清空昵称不会被重新填上。直接写存储：默认值不是用户的操作，不进审计与足迹。
func (s *Service) assignDefaults(ctx context.Context, u *store.User, lang string) error {
	if u.Avatar != "" || !u.DeletedAt.IsZero() {
		return nil
	}
	if u.Nickname == "" {
		nick := RandomNickname(lang)
		if err := s.Store.Users().SetNickname(ctx, u.ID, nick); err != nil {
			return err
		}
		u.Nickname = nick
	}
	key := RandomPreset()
	if err := s.Store.Users().SetAvatar(ctx, u.ID, key); err != nil {
		return err
	}
	u.Avatar = key
	return nil
}

// AssignDefaults 为新用户分配预置头像与昵称；失败只记日志（不影响注册）。
func (s *Service) AssignDefaults(ctx context.Context, u *store.User, lang string) {
	if err := s.assignDefaults(ctx, u, lang); err != nil && s.Log != nil {
		s.Log.Warn("分配预置头像失败", "user", u.ID, "error", err)
	}
}

// AssignMissingAvatars 为上线前已有的用户补分配一次（头像为空即从未分配）；按界面语言选择词表，没有设置语言时用英文。
func (s *Service) AssignMissingAvatars(ctx context.Context) error {
	for after := ""; ; {
		users, err := s.Store.Users().List(ctx, store.Page{After: after, Limit: 500})
		if err != nil {
			return err
		}
		for i := range users {
			if err := s.assignDefaults(ctx, &users[i], users[i].Lang); err != nil {
				return fmt.Errorf("为 %s 分配头像：%w", users[i].ID, err)
			}
		}
		if len(users) < 500 {
			return nil
		}
		after = users[len(users)-1].ID
	}
}

func (s *Service) avatarJob(ctx context.Context) {
	if err := s.AssignMissingAvatars(ctx); err != nil {
		s.Log.Error("补分配头像失败", "error", err)
		jobFailed(ctx, err)
	}
}

// SetAvatarPreset 换成一个预置头像；上传过的图片随之删除。key 为空时随机选一个（「换一个」）。
func (s *Service) SetAvatarPreset(ctx context.Context, u *store.User, key string) (string, error) {
	if key == "" {
		for key = RandomPreset(); key == u.Avatar; key = RandomPreset() {
		}
	}
	if !ValidPreset(key) {
		return "", ErrAvatar
	}
	if err := s.Store.Users().DeleteAvatarImage(ctx, u.ID); err != nil {
		return "", err
	}
	if err := s.Store.Users().SetAvatar(ctx, u.ID, key); err != nil {
		return "", err
	}
	s.audit(ctx, u.ID, "account.avatar", map[string]string{"avatar": key})
	return key, nil
}

// UploadAvatar 保存上传的头像：校验大小与尺寸，居中裁成正方形，缩到 256 px，重新编码（不透明的图用 JPEG，否则 PNG）。
// 返回新的头像标识（u:<内容摘要>，图片变化时地址随之变化，浏览器可以长期缓存）。
func (s *Service) UploadAvatar(ctx context.Context, u *store.User, data []byte) (string, error) {
	if len(data) > avatarMaxBytes {
		return "", ErrAvatarTooLarge
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", ErrAvatar
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > avatarMaxPixels {
		return "", ErrAvatarTooLarge
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", ErrAvatar
	}
	out, mime, err := encodeAvatar(squareResize(src, avatarSize))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(out)
	key := "u:" + hex.EncodeToString(sum[:6])
	if err := s.Store.Users().PutAvatarImage(ctx, u.ID, mime, out, s.now()); err != nil {
		return "", err
	}
	if err := s.Store.Users().SetAvatar(ctx, u.ID, key); err != nil {
		return "", err
	}
	s.audit(ctx, u.ID, "account.avatar", map[string]string{"avatar": "upload"})
	return key, nil
}

// squareResize 居中裁成正方形并缩到 size（较小的图不放大）。按面积平均缩小（盒式滤波），缩小倍数大时也不出现锯齿。
func squareResize(src image.Image, size int) *image.NRGBA {
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	x0, y0 := b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2
	n := min(side, size)
	dst := image.NewNRGBA(image.Rect(0, 0, n, n))
	if n == side { // 不缩放：只裁剪
		draw.Draw(dst, dst.Bounds(), src, image.Pt(x0, y0), draw.Src)
		return dst
	}
	for y := range n {
		sy0, sy1 := y0+y*side/n, y0+(y+1)*side/n
		for x := range n {
			sx0, sx1 := x0+x*side/n, x0+(x+1)*side/n
			var r, g, bl, a, cnt uint64
			for yy := sy0; yy < sy1; yy++ {
				for xx := sx0; xx < sx1; xx++ {
					c := color.NRGBAModel.Convert(src.At(xx, yy)).(color.NRGBA)
					// 按透明度加权，半透明边缘不发暗
					r += uint64(c.R) * uint64(c.A)
					g += uint64(c.G) * uint64(c.A)
					bl += uint64(c.B) * uint64(c.A)
					a += uint64(c.A)
					cnt++
				}
			}
			if cnt == 0 {
				continue
			}
			px := color.NRGBA{A: clamp8(a / cnt)}
			if a > 0 {
				px.R, px.G, px.B = clamp8(r/a), clamp8(g/a), clamp8(bl/a)
			}
			dst.SetNRGBA(x, y, px)
		}
	}
	return dst
}

func encodeAvatar(img *image.NRGBA) ([]byte, string, error) {
	var buf bytes.Buffer
	if img.Opaque() {
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 88}); err != nil {
			return nil, "", fmt.Errorf("编码头像：%w", err)
		}
		return buf.Bytes(), "image/jpeg", nil
	}
	if err := png.Encode(&buf, img); err != nil {
		return nil, "", fmt.Errorf("编码头像：%w", err)
	}
	return buf.Bytes(), "image/png", nil
}

// clamp8 把 0–255 的平均值转回 uint8（平均值不会超过 255，上限检查只是让转换显式安全）。
func clamp8(v uint64) uint8 {
	if v > 255 {
		return 255
	}
	return uint8(v)
}
