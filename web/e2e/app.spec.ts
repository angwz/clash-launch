import { expect, test, type Page } from "@playwright/test";
import { apiAs, createUser, login, seedProfile, step, totp } from "./helpers";

// 三个安全问题使用默认选择的内置问题 0、1、2
async function fillAnswers(page: Page, answers = ["Beijing", "Mochi", "Sunrise School"]) {
  for (let i = 0; i < 3; i++) await page.getByLabel(`第 ${i + 1} 个问题的答案`).fill(answers[i]!);
}

// 收集页面错误与 CSP 违规：任何一条都让测试失败（第 14.2 节的 CSP 不放宽）。
function watch(page: Page): string[] {
  const problems: string[] = [];
  page.on("pageerror", (e) => problems.push("pageerror: " + e.message));
  page.on("console", (m) => {
    const t = m.text();
    if (t.includes("401 (Unauthorized)")) return; // 未登录时 /me 的会话探测
    if (m.type() === "error") problems.push("console: " + t);
  });
  return problems;
}

test("登录后看到方案列表，各页面没有错误与 CSP 违规", async ({ page, request, baseURL }) => {
  const problems = watch(page);
  // 两个项目共用同一个后端：名称带上项目名，避免重名
  const name = "列表测试 " + test.info().project.name;
  const { profile, source } = await seedProfile(request, baseURL!, name);
  await login(page);
  await expect(page.getByRole("heading", { name: "方案" })).toBeVisible();
  await expect(page.getByRole("link", { name })).toBeVisible();
  for (const path of [`/profiles/${profile}`, `/profiles/${profile}?section=dns`, `/profiles/${profile}/links`, `/profiles/${profile}/history`, `/sources`, `/sources/${source}`, `/rules`, `/admin`, `/account`, `/welcome`]) {
    await page.goto(path);
    await page.waitForLoadState("networkidle");
  }
  expect(problems).toEqual([]);
});

test("编辑字段：预览实时更新，保存后生成新版本", async ({ page, request, baseURL }) => {
  const { profile } = await seedProfile(request, baseURL!, "编辑测试");
  await login(page);
  await page.goto(`/profiles/${profile}?section=inbound`);
  const input = page.locator("#ctl-mixed-port");
  await input.fill("7891");
  await input.press("Enter");
  // 手机视图中预览在另一个标签页
  if (test.info().project.name === "mobile") await page.getByRole("tab", { name: /预览/ }).click();
  await expect(page.locator(".yaml").first()).toContainText("mixed-port: 7891");
  if (test.info().project.name === "mobile") await page.getByRole("tab", { name: /设置/ }).click();
  await page.getByRole("button", { name: /^保存/ }).click();
  await expect(page.getByText("已保存为第 2 版")).toBeVisible();
  await page.reload();
  await expect(page.locator("#ctl-mixed-port")).toHaveValue("7891");
});

test("命令面板：搜索字段并定位；命令模式进入原始编辑", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "键盘快捷键只在桌面测试");
  const { profile } = await seedProfile(request, baseURL!, "面板测试");
  await login(page);
  await page.goto(`/profiles/${profile}`);
  await page.locator(".yaml").first().waitFor();

  await page.keyboard.press("Control+K");
  await page.keyboard.type("增强模式");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/section=dns/);
  await expect(page.locator("#f-dns\\.enhanced-mode")).toBeVisible();

  // 「原始编辑」不出现在普通搜索中，只在 > 命令模式中出现（第 16.4 节）
  await page.keyboard.press("Control+K");
  await page.keyboard.type("原始编辑");
  await expect(page.getByRole("option", { name: /原始编辑/ })).toHaveCount(0);
  await page.locator("dialog.palette input").fill(">原始编辑");
  await page.keyboard.press("Enter");
  const editor = page.locator(".cm-content");
  await editor.click();
  await page.keyboard.type("ipv6: true\n");
  await expect(page.locator(".raw-flag")).toBeVisible(); // 顶栏 ⚡
  await page.getByRole("button", { name: "回到表单" }).click();
  await page.goto(`/profiles/${profile}?section=general`);
});

test("源排序：用键盘调整顺序", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "键盘排序只在桌面测试");
  const { profile, call } = await seedProfile(request, baseURL!, "排序测试");
  const second = await call<{ id: string }>("POST", "/api/v1/sources", { name: "第二个源", kind: "single", content: "ss://YWVzLTEyOC1nY206ZXhhbXBsZQ==@192.0.2.20:8388#second" });
  await login(page);
  await page.goto(`/profiles/${profile}`);
  await page.getByLabel("添加一个节点源…").selectOption(second.id);
  await page.getByRole("button", { name: "添加", exact: true }).click();
  const names = page.locator(".list .name");
  await expect(names).toHaveText(["排序测试 节点", "第二个源"]);
  await page.locator("#src-row-1 .handle").focus();
  await page.keyboard.press("Alt+ArrowUp");
  await expect(names).toHaveText(["第二个源", "排序测试 节点"]);
});

test("注册：默认关闭；凭一次性邀请注册，邀请不能复用", async ({ browser, request, baseURL }) => {
  const call = await apiAs(request, baseURL!);
  const inv = await call<{ url: string }>("POST", "/api/v1/admin/invites", { hours: 1, note: "e2e" });
  const hash = new URL(inv.url).hash;
  const user = "guest" + Date.now().toString(36);

  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  await page.goto("/register");
  await expect(page.getByText("当前不开放注册")).toBeVisible();

  await page.goto("/register" + hash);
  await page.getByLabel("用户名").fill(user);
  await page.getByLabel("口令", { exact: true }).fill("a long enough password");
  await page.getByLabel("确认口令").fill("a long enough password");
  await fillAnswers(page);
  await page.getByRole("button", { name: "注册" }).click();
  await expect(page).toHaveURL(/\/welcome$/);
  // 普通用户看不到管理
  await expect(page.getByRole("link", { name: "管理" })).toHaveCount(0);
  await ctx.close();

  const ctx2 = await browser.newContext();
  const p2 = await ctx2.newPage();
  await p2.goto("/register" + hash);
  await p2.getByLabel("用户名").fill(user + "x");
  await p2.getByLabel("口令", { exact: true }).fill("a long enough password");
  await p2.getByLabel("确认口令").fill("a long enough password");
  await fillAnswers(p2);
  await p2.getByRole("button", { name: "注册" }).click();
  await expect(p2.getByRole("alert")).toContainText("需要有效的邀请链接");
  await ctx2.close();
});

test("深色主题与伪 RTL", async ({ page, request, baseURL }) => {
  const { profile } = await seedProfile(request, baseURL!, "主题测试");
  await login(page);
  await page.goto(`/profiles/${profile}?section=dns&theme=dark`);
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  const bg = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  expect(bg).toBe("rgb(17, 17, 20)");

  await page.goto(`/profiles/${profile}?section=dns&lang=ar-XB`);
  await expect(page.locator("html")).toHaveAttribute("dir", "rtl");
  if (test.info().project.name === "desktop") {
    // 侧栏镜像到右侧；配置值保持 LTR
    const box = await page.locator("nav.sidebar").boundingBox();
    expect(box!.x).toBeGreaterThan(700);
  }
  await expect(page.locator(".yaml").first()).toHaveCSS("direction", "ltr");
});

test("手机视口：没有横向滚动", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name !== "mobile", "只在手机视口检查");
  const { profile, source } = await seedProfile(request, baseURL!, "手机测试");
  await login(page);
  for (const path of ["/", `/profiles/${profile}?section=dns`, `/profiles/${profile}/links`, `/sources`, `/sources/${source}`, `/rules`, `/admin`]) {
    await page.goto(path);
    await page.waitForLoadState("networkidle");
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow, path).toBeLessThanOrEqual(0);
  }
});

test("找回口令：答对 2 个问题后重设，旧会话失效", async ({ browser, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const call = await apiAs(request, baseURL!);
  await call("PUT", "/api/v1/admin/registration", { open: true });
  const user = "rc" + Date.now().toString(36);

  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  await page.goto("/register");
  await page.getByLabel("用户名").fill(user);
  await page.getByLabel("口令", { exact: true }).fill("a long enough password");
  await page.getByLabel("确认口令").fill("a long enough password");
  await fillAnswers(page);
  await page.getByRole("button", { name: "注册" }).click();
  await expect(page).toHaveURL(/\/welcome$/);
  await call("PUT", "/api/v1/admin/registration", { open: false });

  const other = await browser.newContext();
  const r = await other.newPage();
  await r.goto("/login");
  await r.getByRole("link", { name: "忘记口令？" }).click();
  await r.getByLabel("用户名").fill(user);
  await r.getByRole("button", { name: "下一步" }).click();
  await r.getByLabel("你出生在哪座城市？").fill("  ＢＥＩＪＩＮＧ ");
  await r.getByLabel("你的第一只宠物叫什么名字？").fill("mochi");
  await r.getByLabel("新口令").fill("another long password");
  await r.getByLabel("确认口令").fill("another long password");
  await r.getByRole("button", { name: "重设口令" }).click();
  await expect(r.getByText("口令已重设")).toBeVisible();

  // 原来的会话已失效
  await page.goto("/account");
  await expect(page).toHaveURL(/\/login/);
  await login(r, { username: user, password: "another long password" });
  await ctx.close();
  await other.close();
});

test("两步验证：扫码启用、显示恢复码；之后登录需要验证码", async ({ browser, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const user = await createUser(request, baseURL!, "mfa");
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  await login(page, { username: user, password: "a long enough password" });
  await page.goto("/account");
  await page.getByRole("button", { name: "启用" }).click();
  const secret = (await page.locator(".secret").textContent())!.replace(/\s+/g, "");
  await page.getByLabel("验证码").fill(totp(secret, step()));
  await page.getByRole("button", { name: "确认并启用" }).click();
  await expect(page.getByText("恢复码", { exact: true })).toBeVisible();
  await expect(page.locator(".code-list li")).toHaveCount(10);
  await page.getByRole("button", { name: "我已保存" }).click();
  await expect(page.getByText(/已启用 · 剩余 10 个恢复码/)).toBeVisible();
  await ctx.close();

  const ctx2 = await browser.newContext();
  const p2 = await ctx2.newPage();
  await p2.goto("/login");
  await p2.getByLabel("用户名").fill(user);
  await p2.getByLabel("口令", { exact: true }).fill("a long enough password");
  await p2.getByRole("button", { name: "登录" }).click();
  await expect(p2.getByRole("heading", { name: "两步验证" })).toBeVisible();
  // 启用时用过的时间步不能再用；下一个时间步（允许 ±1 的偏差）可以
  await p2.getByLabel("验证码").fill(totp(secret, step() + 1));
  await p2.getByRole("button", { name: "验证" }).click();
  await expect(p2).toHaveURL(/\/$/);
  await ctx2.close();
});
