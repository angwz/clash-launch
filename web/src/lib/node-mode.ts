// 当前账号的节点写法策略（反馈 #46、#71）。同一次页面加载内缓存；请求失败时下次重新获取。
import { api } from "./api/client";

export interface NodeModePolicy {
  default: "inline" | "provider";
  /** 只能使用默认写法（开放注册的普通用户） */
  locked: boolean;
  /** 订阅只由客户端拉取，服务器不访问订阅地址 */
  clientFetch: boolean;
}

let cached: Promise<NodeModePolicy> | null = null;

export function nodeModePolicy(): Promise<NodeModePolicy> {
  cached ??= api.get<NodeModePolicy>("/api/v1/node-mode").catch((e: unknown) => {
    cached = null;
    throw e;
  });
  return cached;
}
