package service

import (
	"strings"
	"testing"
)

// 每种提醒在两种语言中都有标题与正文；细节按键名排序；非测试邮件带有「不是本人操作」的提示。
func TestAlertTexts(t *testing.T) {
	kinds := []Alert{AlertNewDevice, AlertPassword, AlertQuestions, AlertRecovered, AlertMFA, AlertPasskey, "test"}
	for _, lang := range []string{"zh", "en"} {
		for _, k := range kinds {
			subject, body := alertText(lang, string(k), map[string]string{"time": "2026-11-10 08:00 UTC", "ip": "192.0.2.0/24"})
			if subject == "" || body == "" {
				t.Errorf("%s/%s 缺少文字", lang, k)
			}
			if (k != "test") != strings.Contains(body, alertFooter[lang]) {
				t.Errorf("%s/%s 的提示语", lang, k)
			}
		}
	}
	_, body := alertText("zh", string(AlertNewDevice), map[string]string{"time": "t", "ip": "i", "device": "d"})
	if strings.Index(body, "设备: d") > strings.Index(body, "网络: i") || !strings.Contains(body, "时间: t") {
		t.Errorf("细节顺序或标签：%s", body)
	}
}
