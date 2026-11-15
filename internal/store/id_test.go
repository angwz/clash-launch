package store

import "testing"

// NewID 在同一进程内严格递增：同一毫秒内连续生成也保持生成顺序。
func TestNewIDMonotonic(t *testing.T) {
	prev := NewID()
	for i := 0; i < 20000; i++ {
		id := NewID()
		if id <= prev {
			t.Fatalf("第 %d 个 ID 没有递增：%s <= %s", i, id, prev)
		}
		if id[14] != '7' || (id[19] != '8' && id[19] != '9' && id[19] != 'a' && id[19] != 'b') {
			t.Fatalf("版本或变体位被破坏：%s", id)
		}
		prev = id
	}
}
