// 分流规则地址允许的站点（反馈 #72）。普通用户只能用公共代码托管站与其 CDN 上的地址；管理员不受限制。
// 同一次页面加载内缓存；请求失败时下次重新获取。
import { api } from "./api/client";

export interface RuleHosts {
  restricted: boolean;
  hosts: string[];
}

let cached: Promise<RuleHosts> | null = null;

export function ruleHosts(): Promise<RuleHosts> {
  cached ??= api.get<RuleHosts>("/api/v1/rule-hosts").catch((e: unknown) => {
    cached = null;
    throw e;
  });
  return cached;
}
