package service

import "strings"

// 短链接直接放在站点根路径下（反馈 #75）：域名/短码。以下路径保留给本站页面、静态文件与软件项目常用的路径，
// 不能用作短码。前端路由的第一段由 TestReservedCoversFrontendRoutes 对照 web/src/lib/router.svelte.ts 检查。
var reservedCodes = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`
		api s p u assets static public dist build fonts images img css js media files file uploads upload download downloads
		favicon favicon.ico favicon.svg robots.txt sitemap.xml manifest.json sw.js index index.html
		healthz readyz health healthcheck status metrics ping version debug
		login logout signin signout signup register setup recover reset verify confirm auth oauth sso callback
		admin administrator root system account accounts user users me profile profiles settings preferences
		welcome home dashboard console panel app apps web www mail email
		sources source nodes node rules rule help docs doc about contact support terms privacy legal tos
		analytics stats statistics short-links shortlinks links link messages message inbox chat
		share shared invite invites sub subs subscribe subscription subscriptions config configs clash mihomo proxy proxies
		new edit delete create update search explore feed rss atom blog news
		graphql rpc ws wss socket webhook webhooks hook hooks git github gitlab well-known
		cdn null undefined true false
	`) {
		reservedCodes[w] = true
	}
}

// IsReservedCode 报告 code（小写）是否保留，不能用作短码。
func IsReservedCode(code string) bool { return reservedCodes[strings.ToLower(code)] }
