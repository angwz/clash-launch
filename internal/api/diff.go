package api

import "strings"

// DiffLine 是行差异中的一行：op 为 " "（相同）、"-"（删除）、"+"（新增）。
type DiffLine struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

// Diff 计算两段文本的行差异（Myers 算法，O((N+M)·D)），只保留改动处前后各 3 行的上下文。
func Diff(a, b string) []DiffLine {
	x, y := splitLines(a), splitLines(b)
	n, m := len(x), len(y)
	max := n + m
	if max == 0 {
		return []DiffLine{}
	}
	v := make([]int, 2*max+2)
	var trace [][]int
	for d := 0; d <= max; d++ {
		snap := make([]int, len(v))
		copy(snap, v)
		trace = append(trace, snap)
		for k := -d; k <= d; k += 2 {
			var i int
			if k == -d || (k != d && v[max+k-1] < v[max+k+1]) {
				i = v[max+k+1]
			} else {
				i = v[max+k-1] + 1
			}
			j := i - k
			for i < n && j < m && x[i] == y[j] {
				i, j = i+1, j+1
			}
			v[max+k] = i
			if i >= n && j >= m {
				return context3(backtrack(trace, x, y, max))
			}
		}
	}
	return nil
}

func backtrack(trace [][]int, x, y []string, max int) []DiffLine {
	var out []DiffLine
	i, j := len(x), len(y)
	for d := len(trace) - 1; d >= 0 && (i > 0 || j > 0); d-- {
		v := trace[d]
		k := i - j
		var pk int
		if k == -d || (k != d && v[max+k-1] < v[max+k+1]) {
			pk = k + 1
		} else {
			pk = k - 1
		}
		pi := v[max+pk]
		pj := pi - pk
		for i > pi && j > pj {
			out = append(out, DiffLine{" ", x[i-1]})
			i, j = i-1, j-1
		}
		if d > 0 {
			if i == pi {
				out = append(out, DiffLine{"+", y[j-1]})
				j--
			} else {
				out = append(out, DiffLine{"-", x[i-1]})
				i--
			}
		}
	}
	for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
		out[l], out[r] = out[r], out[l]
	}
	return out
}

func context3(lines []DiffLine) []DiffLine {
	keep := make([]bool, len(lines))
	for i, l := range lines {
		if l.Op != " " {
			for j := max(0, i-3); j <= min(len(lines)-1, i+3); j++ {
				keep[j] = true
			}
		}
	}
	out := []DiffLine{}
	for i, l := range lines {
		if keep[i] {
			out = append(out, l)
		}
	}
	return out
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
