// 端到端测试的共用步骤：登录、经 API 准备数据。测试数据只用 example.com / 192.0.2.0/24 与虚构凭据。
import { createHmac } from "node:crypto";
import type { APIRequestContext, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { solve } from "../src/lib/sha256";

export const ADMIN = { username: "admin", password: "admin password 2026" };

export async function login(page: Page, user = ADMIN): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("用户名").fill(user.username);
  await page.getByLabel("口令", { exact: true }).fill(user.password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
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

// —— 两步验证与独立账号（不影响共用的管理员）——

/** RFC 6238：base32 密钥在时间步 step 的 6 位验证码。 */
export function totp(secret: string, step: number): string {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = "";
  for (const c of secret.replace(/\s+/g, "").toUpperCase()) bits += alphabet.indexOf(c).toString(2).padStart(5, "0");
  const key = Buffer.from(bits.match(/.{8}/g)!.map((b) => parseInt(b, 2)));
  const msg = Buffer.alloc(8);
  msg.writeBigUInt64BE(BigInt(step));
  const h = createHmac("sha1", key).update(msg).digest();
  const off = h[h.length - 1]! & 0x0f;
  return String((h.readUInt32BE(off) & 0x7fffffff) % 1_000_000).padStart(6, "0");
}

export const step = () => Math.floor(Date.now() / 1000 / 30);

/** 打开注册并经 API 创建一个普通用户（含安全问题与工作量证明），返回用户名。 */
export async function createUser(request: APIRequestContext, baseURL: string, prefix: string): Promise<string> {
  const admin = await apiAs(request, baseURL);
  await admin("PUT", "/api/v1/admin/registration", { open: true });
  const user = prefix + Date.now().toString(36);
  const ch = await (await request.get("/api/v1/pow?purpose=register")).json();
  const res = await request.post("/api/v1/register", {
    headers: { Origin: baseURL },
    data: {
      username: user,
      password: "a long enough password",
      questions: [{ builtin: 0 }, { builtin: 1 }, { builtin: 2 }],
      answers: ["aa", "bb", "cc"],
      pow: { challenge: ch.challenge, nonce: solve(ch.challenge, ch.bits) },
    },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
  await admin("PUT", "/api/v1/admin/registration", { open: false });
  return user;
}
