package api

import "testing"

func TestCapPage(t *testing.T) {
	for _, c := range []struct{ n, size, max, want int }{
		{1, 10, 10_000, 1}, {1000, 10, 10_000, 1000}, {1001, 10, 10_000, 1000}, {5000, 50, 50_000, 1000}, {3, 10, 25, 3}, {4, 10, 25, 3},
	} {
		if got := capPage(c.n, c.size, c.max); got != c.want {
			t.Errorf("capPage(%d, %d, %d) = %d，期望 %d", c.n, c.size, c.max, got, c.want)
		}
	}
}
