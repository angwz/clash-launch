package main

import "testing"

func TestRunIDs(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if u, g, err := runIDs(env(nil)); err != nil || u != 65532 || g != 65532 {
		t.Errorf("默认：%d %d %v", u, g, err)
	}
	if u, g, err := runIDs(env(map[string]string{"PUID": "1026", "PGID": "100"})); err != nil || u != 1026 || g != 100 {
		t.Errorf("群晖常见的 1026:100：%d %d %v", u, g, err)
	}
	for _, bad := range []map[string]string{{"PUID": "0"}, {"PGID": "0"}, {"PUID": "abc"}, {"PUID": "-1"}, {"PGID": "99999999999"}} {
		if _, _, err := runIDs(env(bad)); err == nil {
			t.Errorf("%v 应被拒绝", bad)
		}
	}
}
