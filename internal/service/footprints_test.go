package service

import (
	"testing"
	"time"
)

// streak：截至当天（含）连续有记录的天数；当天没有记录为 0；跨月与跨年按日历计算。
func TestStreak(t *testing.T) {
	now := time.Date(2026, 3, 2, 15, 0, 0, 0, time.UTC)
	cases := []struct {
		days []int
		want int
	}{
		{nil, 0},
		{[]int{20260302}, 1},
		{[]int{20260227, 20260228, 20260301, 20260302}, 4},
		{[]int{20260227, 20260301, 20260302}, 2},
		{[]int{20260228, 20260301}, 0},
	}
	for _, c := range cases {
		if got := streak(c.days, now); got != c.want {
			t.Errorf("streak(%v) = %d，期望 %d", c.days, got, c.want)
		}
	}
	// 跨年
	if got := streak([]int{20251231, 20260101}, time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)); got != 2 {
		t.Errorf("跨年：%d", got)
	}
}
