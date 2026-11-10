package target

import (
	"strings"
	"testing"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/catalog"
)

// 预设的 managed 路径必须是目录中的字段：写错的路径不会触发任何提示，界面上也不会出现「接管」标记，
// 因此在加载层面锁定。
func TestPresetManagedPathsInCatalog(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS, "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	presets, err := Load(catalogdata.FS, "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	if Find(presets, "openclash") == nil {
		t.Fatal("缺少 openclash 预设")
	}
	for _, p := range presets {
		seen := map[string]bool{}
		for _, m := range p.Managed {
			if seen[m] {
				t.Errorf("%s: managed 路径 %s 重复", p.ID, m)
			}
			seen[m] = true
			f := cat.Root
			for _, seg := range strings.Split(m, ".") {
				f = f.Child(seg, nil)
			}
			if f == nil {
				t.Errorf("%s: managed 路径 %s 不在目录中", p.ID, m)
			}
		}
	}
}
