package service

import (
	"image"
	"image/color"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// 前端绘制预置头像（web/src/lib/avatars.ts），后端校验与随机分配：两边的图形与颜色数量必须一致。
func TestAvatarPresetsMatchFrontend(t *testing.T) {
	b, err := os.ReadFile("../../web/src/lib/avatars.ts")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	m := regexp.MustCompile(`SHAPES = \[([^\]]*)\]`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("没有找到 SHAPES")
	}
	var shapes []string
	for _, q := range regexp.MustCompile(`"([a-z]+)"`).FindAllStringSubmatch(m[1], -1) {
		shapes = append(shapes, q[1])
	}
	if !slices.Equal(shapes, AvatarShapes) {
		t.Errorf("图形不一致：前端 %v，后端 %v", shapes, AvatarShapes)
	}
	c := regexp.MustCompile(`COLORS = \[([^\]]*)\]`).FindStringSubmatch(src)
	if c == nil || strings.Count(c[1], "#") != AvatarColors {
		t.Errorf("颜色数量不一致：%v，后端 %d", c, AvatarColors)
	}
}

// 预置头像与昵称：随机结果总是有效；昵称不超过长度上限、词表没有重复。
func TestRandomPresetsAndNicknames(t *testing.T) {
	for range 200 {
		if k := RandomPreset(); !ValidPreset(k) {
			t.Fatalf("无效的随机头像：%s", k)
		}
		for _, lang := range []string{"zh-CN", "en", ""} {
			if n := RandomNickname(lang); n == "" || utf8.RuneCountInString(n) > maxNickname {
				t.Fatalf("昵称 %q（%s）", n, lang)
			}
		}
	}
	for _, list := range [][]string{nickAdjZH, nickNounZH, nickAdjEN, nickNounEN} {
		seen := map[string]bool{}
		for _, w := range list {
			if seen[w] {
				t.Errorf("词表重复：%s", w)
			}
			seen[w] = true
		}
		if len(list) < 40 {
			t.Errorf("词表只有 %d 个词", len(list))
		}
	}
}

// squareResize：居中裁剪；按面积平均，全透明像素不让颜色发暗。
func TestSquareResize(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 600, 300))
	for y := range 300 {
		for x := range 600 {
			c := color.NRGBA{R: 255, A: 255}
			if x < 150 || x >= 450 { // 两侧会被裁掉
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	out := squareResize(img, 100)
	if out.Bounds().Dx() != 100 || out.Bounds().Dy() != 100 {
		t.Fatalf("尺寸：%v", out.Bounds())
	}
	for _, p := range []image.Point{{0, 0}, {50, 50}, {99, 99}} {
		if c := out.NRGBAAt(p.X, p.Y); c.R != 255 || c.B != 0 {
			t.Errorf("%v 应只剩中间的红色：%+v", p, c)
		}
	}
	half := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for x := range 4 {
		half.SetNRGBA(x, 0, color.NRGBA{G: 200, A: 255}) // 其余全透明
	}
	if c := squareResize(half, 1).NRGBAAt(0, 0); c.G != 200 || c.A != 255/4 {
		t.Errorf("透明度加权：%+v", c)
	}
}
