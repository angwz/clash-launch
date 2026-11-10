// 端到端测试的共用步骤：登录、经 API 准备数据。测试数据只用 example.com / 192.0.2.0/24 与虚构凭据。
import type { APIRequestContext, Page } from "@playwright/test";
import { expect } from "@playwright/test";

export const ADMIN = { username: "admin", password: "admin password 2026" };

export async function login(page: Page, user = ADMIN): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("用户名").fill(user.username);
  await page.getByLabel("口令", { exact: true }).fill(user.password);
  await page.getByRole("button", { name: "登录" }).click();
  await expect(page).toHaveURL(/\/$|\/welcome$/);
}

/** 以 API 登录并返回带 CSRF 令牌的调用函数。 */
export async function apiAs(request: APIRequestContext, baseURL: string, user = ADMIN) {
  const res = await request.post("/api/v1/session", { data: user, headers: { Origin: baseURL } });
  expect(res.ok()).toBeTruthy();
  const { csrf } = (await res.json()) as { csrf: string };
  return async <T>(method: string, path: string, data?: unknown, ifMatch?: number): Promise<T> => {
    const headers: Record<string, string> = { Origin: baseURL, "X-CSRF-Token": csrf };
    if (ifMatch !== undefined) headers["If-Match"] = `"v${ifMatch}"`;
    const r = await request.fetch(path, { method, data, headers });
    expect(r.ok(), `${method} ${path} → ${r.status()} ${await r.text()}`).toBeTruthy();
    return (r.status() === 204 ? undefined : await r.json()) as T;
  };
}

const NODES = [
  "ss://YWVzLTEyOC1nY206ZXhhbXBsZS1zcw==@192.0.2.13:8388#%E6%97%A5%E6%9C%AC%20%E4%B8%9C%E4%BA%AC%2002",
  "trojan://example-trojan@192.0.2.12:443?sni=example.com#%E9%A6%99%E6%B8%AF%2001",
].join("\n");

/** 一个带节点源的方案；返回方案与源的 ID。 */
export async function seedProfile(request: APIRequestContext, baseURL: string, name: string) {
  const call = await apiAs(request, baseURL);
  const src = await call<{ id: string }>("POST", "/api/v1/sources", { name: name + " 节点", kind: "batch", content: NODES });
  const doc = [
    "clash-launch/profile: v1",
    `name: ${name}`,
    'target: { pack: mihomo, preset: mihomo, core: "1.19.33" }',
    "sources:",
    `  - { ref: ${src.id} }`,
    "form:",
    "  mixed-port: 7890",
    "",
  ].join("\n");
  const p = await call<{ id: string; version: number }>("POST", "/api/v1/profiles", { doc, note: "" });
  return { profile: p.id, source: src.id, call };
}
