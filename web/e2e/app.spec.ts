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

test("登录后看到配置列表，各页面没有错误与 CSP 违规", async ({ page, request, baseURL }) => {
  const problems = watch(page);
  // 两个项目共用同一个后端：名称带上项目名，避免重名
  const name = "列表测试 " + test.info().project.name;
  const { profile, source } = await seedProfile(request, baseURL!, name);
  await login(page);
  await expect(page.getByRole("heading", { name: "配置" })).toBeVisible();
  await expect(page.getByRole("link", { name })).toBeVisible();
  for (const path of [`/profiles/${profile}`, `/profiles/${profile}?section=dns`, `/profiles/${profile}/links`, `/profiles/${profile}/history`, `/sources`, `/sources/${source}`, `/rules`, `/admin`, `/account`, `/welcome`, `/help`, `/help?tab=wiki`]) {
    await page.goto(path);
    await page.waitForLoadState("networkidle");
  }
  expect(problems).toEqual([]);
});

test("编辑字段：预览实时更新，自动保存，可撤销", async ({ page, request, baseURL }) => {
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
  // 不点保存：停止编辑后自动保存，提示条提供撤销（反馈 #35）
  await expect(page.getByText("已自动保存")).toBeVisible();
  await page.reload();
  await expect(page.locator("#ctl-mixed-port")).toHaveValue("7891");

  await page.locator("#ctl-mixed-port").fill("7892");
  await page.locator("#ctl-mixed-port").press("Enter");
  await page.getByRole("button", { name: "撤销" }).click();
  await expect(page.getByText("已撤销到这次编辑之前")).toBeVisible();
  await expect(page.locator("#ctl-mixed-port")).toHaveValue("7891");
  await page.reload();
  await expect(page.locator("#ctl-mixed-port")).toHaveValue("7891");
});

test("命令面板：搜索字段并定位；命令模式进入直接编辑 YAML", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "键盘快捷键只在桌面测试");
  const { profile } = await seedProfile(request, baseURL!, "面板测试");
  await login(page);
  await page.goto(`/profiles/${profile}`);
  await page.locator("#editor-main").waitFor();
  // 「源与规则」默认不显示预览（反馈 #28）
  await expect(page.locator("aside.preview .yaml")).toBeHidden();

  await page.keyboard.press("Control+K");
  await expect(page.locator("dialog.palette input")).toBeFocused(); // 打开动画期间先等输入框获得焦点
  await page.keyboard.type("增强模式");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/section=dns/);
  await expect(page.locator("#f-dns\\.enhanced-mode")).toBeVisible();
  // 设置分区在宽屏上停靠预览；悬停已设置的字段时高亮对应行
  await expect(page.locator("aside.preview .yaml")).toBeVisible();

  // 「原始编辑」不出现在普通搜索中，只在 > 命令模式中出现（第 16.4 节）
  await page.keyboard.press("Control+K");
  await expect(page.locator("dialog.palette input")).toBeFocused(); // 打开动画期间先等输入框获得焦点
  await page.keyboard.type("直接编辑");
  await expect(page.getByRole("option", { name: /直接编辑/ })).toHaveCount(0);
  await page.locator("dialog.palette input").fill(">直接编辑");
  await page.keyboard.press("Enter");
  const editor = page.locator(".cm-content");
  await editor.click();
  await page.keyboard.type("ipv6: true\n");
  await expect(page.locator(".raw-flag")).toBeVisible(); // 顶栏 ⚡
  await page.getByRole("button", { name: "回到表单" }).click();
  await page.goto(`/profiles/${profile}?section=general`);
  await page.goto(`/profiles/${profile}?section=inbound`);
  await page.locator("#f-mixed-port").hover();
  await expect(page.locator(".yaml tr.mark").first()).toContainText("mixed-port");
  await page.screenshot({ path: "test-results/shot-docked.png" });

  // 「源与规则」：点击后以浮层显示，Esc 关闭
  await page.goto(`/profiles/${profile}?section=sources`);
  await page.locator(".preview-toggle").click();
  await expect(page.locator("aside.preview .yaml")).toBeVisible();
  await page.screenshot({ path: "test-results/shot-floating.png" });
  await page.keyboard.press("Escape");
  await expect(page.locator("aside.preview .yaml")).toBeHidden();
});

test("源排序：用键盘调整顺序", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "键盘排序只在桌面测试");
  const { profile, call } = await seedProfile(request, baseURL!, "排序测试");
  const second = await call<{ id: string }>("POST", "/api/v1/sources", { name: "第二个源", kind: "single", content: "ss://YWVzLTEyOC1nY206ZXhhbXBsZQ==@192.0.2.20:8388#second" });
  await login(page);
  await page.goto(`/profiles/${profile}`);
  await page.getByLabel("添加订阅或手动节点…").selectOption(second.id);
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

test("没有横向滚动（页面与主区）", async ({ page, request, baseURL }) => {
  const { profile, source } = await seedProfile(request, baseURL!, "横向滚动测试");
  await login(page);
  for (const path of ["/", `/profiles/${profile}`, `/profiles/${profile}?section=dns`, `/profiles/${profile}/links`, `/sources`, `/sources/${source}`, `/rules`, `/admin`, `/help`]) {
    await page.goto(path);
    await page.waitForLoadState("networkidle");
    // 主区是滚动容器：只看 documentElement 会漏掉主区内的横向滚动条（反馈 #41）
    const overflow = await page.evaluate(() => {
      const m = document.querySelector("main")!;
      return Math.max(document.documentElement.scrollWidth - window.innerWidth, m.scrollWidth - m.clientWidth);
    });
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
  await p2.getByRole("button", { name: "登录", exact: true }).click();
  await expect(p2.getByRole("heading", { name: "两步验证" })).toBeVisible();
  // 启用时用过的时间步不能再用；下一个时间步（允许 ±1 的偏差）可以
  await p2.getByLabel("验证码").fill(totp(secret, step() + 1));
  await p2.getByRole("button", { name: "验证" }).click();
  await expect(p2).toHaveURL(/\/$/);
  await ctx2.close();
});

test("通行密钥：添加后可以免密码登录", async ({ browser, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "虚拟认证器只在桌面项目中配置");
  const user = await createUser(request, baseURL!, "pk");
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  // Chromium 的虚拟认证器：支持可发现凭据与用户验证
  const cdp = await ctx.newCDPSession(page);
  await cdp.send("WebAuthn.enable");
  await cdp.send("WebAuthn.addVirtualAuthenticator", {
    options: { protocol: "ctap2", transport: "internal", hasResidentKey: true, hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true },
  });
  await login(page, { username: user, password: "a long enough password" });
  await page.goto("/account");
  await page.getByLabel("名称", { exact: true }).last().fill("虚拟认证器");
  await page.getByRole("button", { name: "添加通行密钥" }).click();
  await expect(page.getByText("通行密钥已添加")).toBeVisible();
  await expect(page.locator(".keys li")).toHaveCount(1);

  // 退出后用通行密钥登录（同一浏览器上下文中的认证器）
  await page.evaluate(() => fetch("/api/v1/me").then((r) => r.json()).then((s) => fetch("/api/v1/session", { method: "DELETE", headers: { "X-CSRF-Token": s.csrf } })));
  await page.goto("/login");
  await page.getByRole("button", { name: "使用通行密钥登录" }).click();
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole("heading", { name: "配置", exact: true })).toBeVisible();
  await ctx.close();
});

test("帮助：教程切换、百科搜索与深链接", async ({ page }) => {
  await login(page);
  await page.goto("/help");
  await expect(page.getByRole("heading", { name: "快速上手" })).toBeVisible();
  await page.getByRole("button", { name: "链式代理" }).click();
  await expect(page).toHaveURL(/topic=05-chains/);
  await expect(page.locator(".doc pre")).toContainText("dialer-proxy");

  // 深链接直接展开字段，面板与编辑器共用
  await page.goto("/help?tab=wiki&f=dns.enhanced-mode");
  const entry = page.locator("[id='w-dns.enhanced-mode']");
  await expect(entry).toHaveAttribute("open", "");
  await expect(entry.locator(".guide")).toContainText("enhanced-mode: fake-ip");

  // 搜索按名称、键名与说明过滤
  await page.getByRole("searchbox").fill("dns.respect-rules");
  await expect(page.locator("details.entry")).toHaveCount(1);
});

test("调整菜单顺序：保存到账号，刷新后保持", async ({ page }) => {
  test.skip(test.info().project.name === "mobile", "侧栏在手机上是抽屉");
  await login(page);
  await page.goto("/");
  await page.locator("header.topbar").getByRole("button", { name: "admin" }).click();
  await page.getByRole("menuitem", { name: "调整菜单顺序" }).click();
  const dlg = page.locator("dialog[open]");
  // 把「帮助」上移到第一位
  for (let i = 0; i < 3; i++) await dlg.getByRole("button", { name: "上移 帮助" }).click();
  await dlg.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.locator("nav.sidebar li").first()).toContainText("帮助");
  await page.reload();
  await expect(page.locator("nav.sidebar li").first()).toContainText("帮助");
  // 恢复默认
  await page.locator("header.topbar").getByRole("button", { name: "admin" }).click();
  await page.getByRole("menuitem", { name: "调整菜单顺序" }).click();
  await page.locator("dialog[open]").getByRole("button", { name: "恢复默认顺序" }).click();
  await expect(page.locator("nav.sidebar li").first()).toContainText("配置");
});

test("存为模板，新建配置时从默认模板开始并粘贴节点", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "流程与桌面相同");
  const { profile } = await seedProfile(request, baseURL!, "模板来源");
  await login(page);
  await page.goto(`/profiles/${profile}?section=inbound`);
  await page.locator(".adv summary").click();
  await page.getByRole("button", { name: "存为模板…" }).click();
  const dlg = page.locator("dialog[open]");
  await dlg.getByLabel("模板名称").fill("家里路由器");
  await dlg.getByLabel("说明").fill("OpenClash · mihomo 1.19.33");
  await dlg.getByRole("button", { name: "保存" }).click();
  await expect(page.getByText("已存为模板「家里路由器」")).toBeVisible();

  await page.goto("/");
  await page.getByRole("button", { name: "新建配置" }).click();
  const nd = page.locator("dialog[open]");
  await expect(nd.locator("#np-start")).toHaveValue(/^t:/);
  await nd.getByLabel("名称").fill("从模板新建");
  await nd.locator("#np-nodes").fill("ss://YWVzLTEyOC1nY206ZXhhbXBsZQ==@192.0.2.40:8388#模板测试节点");
  await expect(nd.getByText("识别为手动节点")).toBeVisible();
  await nd.getByRole("button", { name: "创建" }).click();
  await page.waitForURL(/\/profiles\/[^/?]+$/);
  // 模板中的设置（mixed-port）与新添加的节点都在
  if (test.info().project.name === "mobile") await page.getByRole("tab", { name: /预览/ }).click();
  else await page.locator(".preview-toggle").click();
  await expect(page.locator(".yaml").first()).toContainText("模板测试节点");
  await expect(page.locator(".yaml").first()).toContainText("mixed-port");
});
