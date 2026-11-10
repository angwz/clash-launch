// API 客户端：类型来自 api/openapi.yaml 生成的 schema.d.ts（npm run gen:api）。
// 职责只有三件事：带上 CSRF 令牌与 If-Match、把 RFC 9457 错误转换为 ApiError、解析 JSON。
// 不在前端重写任何配置语义（第 16.6 节）。
import type { components } from "./schema";

export type Schemas = components["schemas"];
export type User = Schemas["User"];
export type Session = Schemas["Session"];
export type Source = Schemas["Source"];
export type SourceInput = Schemas["SourceInput"];
export type Node = Schemas["Node"];
export type RuleSource = Schemas["RuleSource"];
export type RuleSourceInput = Schemas["RuleSourceInput"];
export type Profile = Schemas["Profile"];
export type Revision = Schemas["Revision"];
export type Preview = Schemas["Preview"];
export type Link = Schemas["Link"];
export type Invite = Schemas["Invite"];
export type Diagnostic = Schemas["Diagnostic"];
export type Problem = Schemas["Problem"];
export type List<T> = { items: T[]; next?: string };

export class ApiError extends Error {
  readonly status: number;
  readonly type: string;
  readonly detail: string;
  readonly diagnostics: Diagnostic[];

  constructor(status: number, p: Partial<Problem>) {
    super(p.detail || p.title || `HTTP ${status}`);
    this.status = status;
    this.type = p.type ?? "unknown";
    this.detail = p.detail ?? "";
    this.diagnostics = p.diagnostics ?? [];
  }
}

let csrf = "";

/** 登录、/me 返回 CSRF 令牌后由会话模块设置。 */
export function setCsrf(token: string): void {
  csrf = token;
}

type Method = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

interface Options {
  body?: unknown;
  /** 乐观锁：资源的 version（第 12.1 节） */
  ifMatch?: number;
  query?: Record<string, string | number | boolean | undefined>;
  signal?: AbortSignal;
}

export async function request<T>(method: Method, path: string, opts: Options = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (opts.body !== undefined) headers["Content-Type"] = "application/json";
  if (method !== "GET" && csrf) headers["X-CSRF-Token"] = csrf;
  if (opts.ifMatch !== undefined) headers["If-Match"] = `"v${opts.ifMatch}"`;
  let url = path;
  if (opts.query) {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(opts.query)) if (v !== undefined) q.set(k, String(v));
    const s = q.toString();
    if (s) url += "?" + s;
  }
  const init: RequestInit = { method, headers, credentials: "same-origin" };
  if (opts.body !== undefined) init.body = JSON.stringify(opts.body);
  if (opts.signal) init.signal = opts.signal;
  const res = await fetch(url, init);
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  if (!res.ok) {
    let p: Partial<Problem> = {};
    try {
      p = JSON.parse(text) as Partial<Problem>;
    } catch {
      p = { detail: text.slice(0, 200) };
    }
    throw new ApiError(res.status, p);
  }
  const type = res.headers.get("Content-Type") ?? "";
  if (!type.includes("json")) return text as T;
  return JSON.parse(text) as T;
}

export const api = {
  get: <T>(path: string, opts?: Options) => request<T>("GET", path, opts),
  post: <T>(path: string, body?: unknown, opts?: Options) => request<T>("POST", path, { ...opts, body: body ?? {} }),
  put: <T>(path: string, body: unknown, opts?: Options) => request<T>("PUT", path, { ...opts, body }),
  patch: <T>(path: string, body: unknown, opts?: Options) => request<T>("PATCH", path, { ...opts, body }),
  del: (path: string, opts?: Options) => request<void>("DELETE", path, opts),
};

/** 逐页取完一个列表（服务端每页最多 100 条）。 */
export async function listAll<T>(path: string): Promise<T[]> {
  const out: T[] = [];
  let after: string | undefined;
  for (let i = 0; i < 50; i++) {
    const page = await api.get<List<T>>(path, { query: { limit: 100, cursor: after } });
    out.push(...page.items);
    if (!page.next) break;
    after = page.next;
  }
  return out;
}
