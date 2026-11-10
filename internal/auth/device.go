package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// DeviceName 把 User-Agent 归为粗粒度的「浏览器 · 系统」，用于会话列表与新设备判断。
// 只做显示与分组：不用于任何授权判断（User-Agent 可以被任意伪造）。
func DeviceName(ua string) string {
	l := strings.ToLower(ua)
	browser := "Other"
	switch {
	case strings.Contains(l, "edg/"):
		browser = "Edge"
	case strings.Contains(l, "opr/") || strings.Contains(l, "opera"):
		browser = "Opera"
	case strings.Contains(l, "firefox/") || strings.Contains(l, "fxios/"):
		browser = "Firefox"
	case strings.Contains(l, "chrome/") || strings.Contains(l, "crios/"):
		browser = "Chrome"
	case strings.Contains(l, "safari/"):
		browser = "Safari"
	case strings.Contains(l, "curl/") || strings.Contains(l, "go-http-client"):
		browser = "CLI"
	}
	os := "Unknown"
	switch {
	case strings.Contains(l, "android"):
		os = "Android"
	case strings.Contains(l, "iphone") || strings.Contains(l, "ipad") || strings.Contains(l, "ios"):
		os = "iOS"
	case strings.Contains(l, "windows"):
		os = "Windows"
	case strings.Contains(l, "mac os") || strings.Contains(l, "macintosh"):
		os = "macOS"
	case strings.Contains(l, "cros"):
		os = "ChromeOS"
	case strings.Contains(l, "linux"):
		os = "Linux"
	}
	return browser + " · " + os
}

// DeviceFingerprint 是「IP 前缀 + 浏览器族」的哈希：同一网络中的同一类浏览器视为同一设备。
func DeviceFingerprint(ipPrefix, ua string) string {
	h := sha256.Sum256([]byte(ipPrefix + "|" + DeviceName(ua)))
	return hex.EncodeToString(h[:16])
}
