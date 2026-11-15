// 一键导入到客户端（反馈 #74）：仍在维护、使用 mihomo 内核的客户端，以及它们文档中的导入链接格式。
// 都是 <scheme>://install-config?url=<编码后的订阅地址>&name=<名称>：客户端收到后新建一个订阅并下载配置。
// 依据（2026-11 核对）：
//   - Clash Verge Rev：官方文档 URL Schemes（clash://install-config）https://www.clashverge.dev/guide/url_schemes.html
//   - Mihomo Party：electron-builder.yml 注册 clash 与 mihomo 两个协议，导入使用 clash://install-config
//   - FlClash：README，clash:// clashmeta:// flclash:// 三种写法等价
//   - Clash Meta for Android：README，clash:// 或 clashmeta://install-config
// 没有导入链接的客户端（如 OpenWrt 上的 OpenClash）只能复制地址或下载文件。

export interface ClientApp {
  id: string;
  name: string;
  /** 适用的平台，界面上作说明 */
  platforms: string;
  scheme: string;
  home: string;
}

export const CLIENTS: readonly ClientApp[] = [
  { id: "verge", name: "Clash Verge Rev", platforms: "Windows · macOS · Linux", scheme: "clash", home: "https://github.com/clash-verge-rev/clash-verge-rev" },
  { id: "party", name: "Mihomo Party", platforms: "Windows · macOS · Linux", scheme: "clash", home: "https://github.com/mihomo-party-org/mihomo-party" },
  { id: "flclash", name: "FlClash", platforms: "Android · Windows · macOS · Linux", scheme: "flclash", home: "https://github.com/chen08209/FlClash" },
  { id: "cmfa", name: "Clash Meta for Android", platforms: "Android", scheme: "clashmeta", home: "https://github.com/MetaCubeX/ClashMetaForAndroid" },
];

/** 导入链接：订阅地址与名称都做 URI 编码（客户端文档要求） */
export function importURL(c: ClientApp, url: string, name: string): string {
  const q = "url=" + encodeURIComponent(url) + (name ? "&name=" + encodeURIComponent(name) : "");
  return `${c.scheme}://install-config?${q}`;
}
