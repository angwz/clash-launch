// 截图（交维护者确认视觉）：用本机 Edge 打开正在运行的服务，同时收集控制台错误与 CSP 违规。
// 用法：node scripts/shots.mjs <输出目录> <方案 ID> <源 ID>
import { chromium } from "@playwright/test";

const [out, profile, source] = process.argv.slice(2);
const BASE = process.env.BASE ?? "http://127.0.0.1:18080";
const browser = await chromium.launch({ channel: "msedge" });
const problems = [];

async function page(opts = {}) {
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 1, locale: "zh-CN", colorScheme: "light", ...opts });
  const p = await ctx.newPage();
  p.on("console", (m) => {
    // 未登录时 /api/v1/me 的 401 是预期的会话探测，浏览器会把它记为资源加载错误
    if (m.text().includes("401 (Unauthorized)")) return;
    if (m.type() === "error" || m.type() === "warning") problems.push(`${m.type()}: ${m.text()}`);
  });
  p.on("pageerror", (e) => problems.push("pageerror: " + e.message));
  return { ctx, p };
}

async function login(p) {
  await p.goto(BASE + "/login");
  await p.getByLabel("用户名").fill("admin");
  await p.getByLabel("口令", { exact: true }).fill("admin password 2026");
  await p.getByRole("button", { name: "登录" }).click();
  await p.waitForURL(BASE + "/");
}

const shot = async (p, name) => {
  await p.waitForTimeout(400);
  await p.screenshot({ path: `${out}/${name}.png` });
};

// 1. 登录页
{
  const { ctx, p } = await page();
  await p.goto(BASE + "/login");
  await p.getByLabel("用户名").fill("admin");
  await shot(p, "01-login");
  await ctx.close();
}

// 2–8. 浅色桌面
{
  const { ctx, p } = await page();
  await login(p);
  await p.waitForSelector(".grid .card");
  await shot(p, "02-profiles");

  await p.goto(`${BASE}/profiles/${profile}`);
  await p.waitForSelector(".tabs");
  await p.waitForSelector(".yaml");
  await shot(p, "03-editor-sources");

  await p.goto(`${BASE}/profiles/${profile}?section=dns`);
  await p.waitForSelector("#sec-dns");
  await p.waitForSelector(".yaml");
  await shot(p, "04-editor-dns");

  await p.goto(`${BASE}/profiles/${profile}?section=general`);
  await p.waitForSelector("#sec-general");
  await p.getByRole("button", { name: /更多设置/ }).click();
  await shot(p, "05-editor-general-more");

  await p.getByRole("tab", { name: /诊断/ }).click();
  await shot(p, "06-editor-diagnostics");

  await p.keyboard.press("Control+K");
  await p.keyboard.type("fake-ip");
  await shot(p, "07-palette-search");
  await p.keyboard.press("Escape");
  await p.waitForTimeout(200);

  await p.keyboard.press("Control+K");
  await p.keyboard.type(">");
  await shot(p, "08-palette-commands");
  await p.keyboard.press("Enter");
  await p.waitForSelector(".cm-editor");
  await shot(p, "09-raw-editor");

  await p.goto(`${BASE}/sources/${source}`);
  await p.waitForSelector("table");
  await shot(p, "10-source-detail");

  await p.goto(`${BASE}/profiles/${profile}/links`);
  await p.waitForSelector(".item");
  await p.getByRole("button", { name: "订阅链接二维码" }).first().click();
  await shot(p, "11-links");

  await p.goto(`${BASE}/admin`);
  await p.waitForSelector("table");
  await shot(p, "12-admin");

  await p.goto(`${BASE}/rules`);
  await p.waitForSelector(".gallery");
  await shot(p, "13-rules");
  await ctx.close();
}

// 9. 深色桌面、平板抽屉、手机、伪 RTL
{
  const { ctx, p } = await page({ colorScheme: "dark" });
  await login(p);
  await p.goto(`${BASE}/profiles/${profile}?section=dns`);
  await p.waitForSelector(".yaml");
  await shot(p, "14-editor-dark");
  await ctx.close();
}
{
  const { ctx, p } = await page({ viewport: { width: 1024, height: 768 } });
  await login(p);
  await p.goto(`${BASE}/profiles/${profile}?section=tun`);
  await p.waitForSelector("#sec-tun");
  await shot(p, "15-tablet");
  await p.locator(".preview-toggle").click();
  await shot(p, "16-tablet-preview");
  await ctx.close();
}
{
  const { ctx, p } = await page({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true });
  await login(p);
  await p.goto(`${BASE}/profiles/${profile}?section=dns`);
  await p.waitForSelector("#sec-dns");
  await shot(p, "17-mobile-editor");
  await p.getByRole("tab", { name: /预览/ }).click();
  await shot(p, "18-mobile-preview");
  await ctx.close();
}
{
  const { ctx, p } = await page();
  await login(p);
  await p.goto(`${BASE}/profiles/${profile}?section=dns&lang=ar-XB`);
  await p.waitForSelector("#sec-dns");
  await shot(p, "19-pseudo-rtl");
  await ctx.close();
}

await browser.close();
console.log(problems.length ? "问题：\n" + [...new Set(problems)].join("\n") : "没有控制台错误");
