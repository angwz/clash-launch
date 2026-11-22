import { expect, test, type Page } from "@playwright/test";
import { ADMIN, apiAs, createUser, login, seedProfile, step, totp } from "./helpers";

// 三个安全问题使用默认选择的内置问题 0、1、2
async function fillAnswers(page: Page, answers = ["Beijing", "Mochi", "Sunrise School"]) {
  for (let i = 0; i < 3; i++) await page.getByLabel(`第${i + 1}个问题的答案`).fill(answers[i]!);
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
  // 打开时显示与预览一致的完整配置；在文件头注释后另起一行写入新键，只有这一行被记下（反馈 #70）
  const editor = page.locator(".cm-content");
  await expect(editor).toContainText("mixed-port");
  await editor.click();
  await page.keyboard.press("Control+Home");
  await page.keyboard.press("End");
  await page.keyboard.press("Enter");
  await page.keyboard.type("x-e2e-test: true");
  await page.locator(".recorded summary").click();
  await expect(page.locator(".recorded pre")).toHaveText(/^x-e2e-test: true\s*$/);
  await page.screenshot({ path: "test-results/shot-raw.png" });
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
  test.skip(test.info().project.name === "mobile", "流程与视口无关；注册每 IP 每小时限 10 次，只在桌面运行一次");
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
  await page.getByLabel("密码", { exact: true }).fill("a long enough password");
  await page.getByLabel("确认密码").fill("a long enough password");
  await fillAnswers(page);
  await page.getByLabel("我已阅读并同意").check();
  await page.getByRole("button", { name: "注册" }).click();
  await expect(page).toHaveURL(/\/welcome$/);
  // 普通用户看不到管理
  await expect(page.getByRole("link", { name: "管理" })).toHaveCount(0);
  await ctx.close();

  const ctx2 = await browser.newContext();
  const p2 = await ctx2.newPage();
  await p2.goto("/register" + hash);
  await p2.getByLabel("用户名").fill(user + "x");
  await p2.getByLabel("密码", { exact: true }).fill("a long enough password");
  await p2.getByLabel("确认密码").fill("a long enough password");
  await fillAnswers(p2);
  await p2.getByLabel("我已阅读并同意").check();
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

test("找回密码：答对 2 个问题后重设，旧会话失效", async ({ browser, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const call = await apiAs(request, baseURL!);
  await call("PUT", "/api/v1/admin/registration", { open: true });
  const user = "rc" + Date.now().toString(36);

  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  await page.goto("/register");
  await page.getByLabel("用户名").fill(user);
  await page.getByLabel("密码", { exact: true }).fill("a long enough password");
  await page.getByLabel("确认密码").fill("a long enough password");
  await fillAnswers(page);
  await page.getByLabel("我已阅读并同意").check();
  await page.getByRole("button", { name: "注册" }).click();
  await expect(page).toHaveURL(/\/welcome$/);
  await call("PUT", "/api/v1/admin/registration", { open: false });

  const other = await browser.newContext();
  const r = await other.newPage();
  await r.goto("/login");
  await r.getByRole("link", { name: "忘记密码？" }).click();
  await r.getByLabel("用户名").fill(user);
  await r.getByRole("button", { name: "下一步" }).click();
  await r.getByLabel("你出生在哪座城市？").fill("  ＢＥＩＪＩＮＧ ");
  await r.getByLabel("你的第一只宠物叫什么名字？").fill("mochi");
  await r.getByLabel("新密码").fill("another long password");
  await r.getByLabel("确认密码").fill("another long password");
  await r.getByRole("button", { name: "重设密码" }).click();
  await expect(r.getByText("密码已重设")).toBeVisible();

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
  await expect(page.getByText(/已启用 · 剩余10个恢复码/)).toBeVisible();
  await ctx.close();

  const ctx2 = await browser.newContext();
  const p2 = await ctx2.newPage();
  await p2.goto("/login");
  await p2.getByLabel("用户名").fill(user);
  await p2.getByLabel("密码", { exact: true }).fill("a long enough password");
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
  // 用户菜单 → 设置 → 调整顺序（反馈 #57）
  await page.locator("header.topbar").getByRole("button", { name: "admin" }).click();
  await page.getByRole("menuitem", { name: "设置" }).click();
  await page.getByRole("button", { name: "调整顺序…" }).click();
  const dlg = page.locator("dialog[open]");
  // 把「帮助」上移到第一位（按钮到顶时不可用，不依赖菜单项的个数）
  const up = dlg.getByRole("button", { name: "上移 帮助" });
  while (await up.isEnabled()) await up.click();
  await dlg.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.locator("nav.sidebar li").first()).toContainText("帮助");
  await page.reload();
  await expect(page.locator("nav.sidebar li").first()).toContainText("帮助");
  // 恢复默认
  await page.getByRole("button", { name: "调整顺序…" }).click();
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

test("设置：「说明与建议」的点击范围", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  const { profile } = await seedProfile(request, baseURL!, "点击范围");
  await login(page);
  const card = () => page.locator("#f-allow-lan");
  const open = () => card().locator("button.guide");
  // 默认：点卡片空白处即可展开，点开关不会展开
  await page.goto(`/profiles/${profile}?section=inbound`);
  await card().click({ position: { x: 300, y: 70 } });
  await expect(open()).toHaveAttribute("aria-expanded", "true");
  await card().locator(".title").click();
  await expect(open()).toHaveAttribute("aria-expanded", "false");
  await card().getByRole("switch").click();
  await expect(open()).toHaveAttribute("aria-expanded", "false");
  // 改为只用按钮
  await page.goto("/settings");
  await page.getByLabel(/只点击/).check();
  await page.goto(`/profiles/${profile}?section=inbound`);
  await card().locator(".title").click();
  await expect(open()).toHaveAttribute("aria-expanded", "false");
  await open().click();
  await expect(open()).toHaveAttribute("aria-expanded", "true");
});

test("统计：配置链接页显示足迹与请求记录，管理页显示全站统计", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  const { profile } = await seedProfile(request, baseURL!, "统计");
  const call = await apiAs(request, baseURL!);
  const l = await call<{ url: string }>("POST", `/api/v1/profiles/${profile}/links`, { note: "手机" });
  await request.get(l.url, { headers: { "User-Agent": "clash-verge/v2.2.3" } });
  await login(page);
  await page.goto(`/profiles/${profile}/links`);
  await expect(page.getByText("最近一次被订阅")).toBeVisible();
  await page.getByRole("button", { name: "请求记录" }).first().click();
  await expect(page.locator(".lstats table").first()).toContainText("clash-verge");
  await page.getByRole("tab", { name: "逐条记录" }).click();
  await expect(page.locator(".lstats table").first()).toContainText("200");
  await page.goto("/admin");
  await expect(page.getByRole("heading", { name: "统计" })).toBeVisible();
  await expect(page.getByText("每天的订阅请求")).toBeVisible();
});

test("时区：输入时不区分大小写自动补全，保存到账号；配置链接可以导出", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  const { profile } = await seedProfile(request, baseURL!, "导出");
  const call = await apiAs(request, baseURL!);
  await call("POST", `/api/v1/profiles/${profile}/links`, { note: "电脑" });
  await login(page);
  await page.goto("/settings");
  const tz = page.locator("#st-tz");
  await tz.click();
  await page.keyboard.type("asia/sh");
  await expect(tz).toHaveValue("Asia/Shanghai"); // 自动补全为规范写法
  await page.keyboard.press("Enter");
  await expect(page.getByText("已保存时区")).toBeVisible();
  const me = await call<{ user: { timezone: string } }>("GET", "/api/v1/me");
  expect(me.user.timezone).toBe("Asia/Shanghai");
  await tz.click();
  await page.keyboard.type("TOKYO");
  await expect(page.getByRole("option", { name: /Asia\/Tokyo/ })).toBeVisible();
  await page.keyboard.press("Escape");
  await page.screenshot({ path: "test-results/shot-settings-tz.png" });

  await page.goto(`/profiles/${profile}/links`);
  // 导出在「导入」菜单里（反馈 #74）
  await page.getByRole("button", { name: "导入" }).first().click();
  const download = page.waitForEvent("download");
  await page.getByRole("menuitem", { name: "导出" }).click();
  expect((await download).suggestedFilename()).toBe("导出.yaml");
  // 不刷新页面：导出后自动刷新，记录里出现这次导出（反馈 #73）
  await page.getByRole("button", { name: "请求记录" }).first().click();
  await page.getByRole("tab", { name: "逐条记录" }).click();
  await expect(page.locator(".lstats table").first()).toContainText("导出");
  await page.screenshot({ path: "test-results/shot-links-export.png" });
  await call("PUT", "/api/v1/account/timezone", { timezone: "" });
});

test("分流规则：拖动手柄调整预设顺序，保存到账号；向导按此顺序默认选第一个", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  const call = await apiAs(request, baseURL!);
  await call("PUT", "/api/v1/account/prefs", {});
  await login(page);
  await page.goto("/rules");
  const cards = page.locator(".gallery > li h3");
  await expect(cards.first()).toHaveText("ACL4SSR"); // 默认第一位
  const third = await cards.nth(2).textContent();
  // 按住第三张卡片的手柄拖到第一张上
  const from = await page.locator(".gallery > li").nth(2).locator("[data-sort-handle]").boundingBox();
  const to = await page.locator(".gallery > li").nth(0).boundingBox();
  await page.mouse.move(from!.x + from!.width / 2, from!.y + from!.height / 2);
  await page.mouse.down();
  await page.mouse.move(to!.x + to!.width / 2, to!.y + to!.height / 2, { steps: 8 });
  await page.mouse.up();
  await expect(cards.first()).toHaveText(third!);
  await expect.poll(async () => (await call<{ rules?: string[] }>("GET", "/api/v1/account/prefs")).rules?.length ?? 0).toBeGreaterThan(0);
  await page.reload();
  await expect(cards.first()).toHaveText(third!);
  // 键盘：聚焦手柄后按方向键下移
  await page.locator(".gallery > li").first().locator("[data-sort-handle]").focus();
  await page.keyboard.press("ArrowRight");
  await expect(cards.nth(1)).toHaveText(third!);
  await page.waitForTimeout(400); // 等换位动画结束
  await page.screenshot({ path: "test-results/shot-rules-order.png" });
  await call("PUT", "/api/v1/account/prefs", {});
});

test("短链接与订阅分析：生成短链接、访问计入统计，分析页显示趋势、热力图与 IP 明细", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  const { profile } = await seedProfile(request, baseURL!, "短链");
  const call = await apiAs(request, baseURL!);
  await call("POST", `/api/v1/profiles/${profile}/links`, { note: "路由器" });
  await login(page);
  await page.goto("/short-links");
  await page.getByPlaceholder("留空则随机生成").fill("e2e-" + Date.now().toString(36));
  await page.getByRole("button", { name: "生成短链接" }).click();
  await expect(page.getByText("短链接已生成")).toBeVisible();
  const code = (await page.locator(".code-big").first().textContent())!.replace("/", "");
  // 短链接直接在根路径下（反馈 #75）
  for (const ua of ["mihomo/1.19.33", "clash-verge/v2.2.3", "FlClash/0.8"]) await request.get("/" + code, { headers: { "User-Agent": ua } });
  await page.reload();
  await expect(page.locator(".item").first()).toContainText("3次请求");
  await expect(page.locator(".item").first()).toContainText("上次请求"); // 默认折叠，只显示次数与上次请求时间（反馈 #78）
  await expect(page.locator(".item").first().locator(".detail")).toHaveCount(0);
  await page.waitForTimeout(300);
  await page.screenshot({ path: "test-results/shot-short-links.png" });
  // 导入菜单列出客户端
  await page.getByRole("button", { name: "导入" }).first().click();
  await expect(page.getByRole("menuitem", { name: /Clash Verge Rev/ })).toHaveAttribute("href", /^clash:\/\/install-config\?url=/);
  await page.keyboard.press("Escape");

  await page.locator(".item").first().getByRole("button", { name: "详情" }).click();
  await page.getByRole("link", { name: "查看分析" }).first().click();
  await expect(page).toHaveURL(/\/analytics\?short=/);
  await expect(page.locator(".tile").first()).toContainText("3");
  await expect(page.locator(".heat .cell:not(.zero)")).not.toHaveCount(0);
  await expect(page.getByRole("heading", { name: "IP明细" })).toBeVisible();
  await page.screenshot({ path: "test-results/shot-analytics.png", fullPage: true });
});

test("站内消息：发送文字与节点，对方的节点页自动加入，未读角标与已读", async ({ browser, page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  const bob = await createUser(request, baseURL!, "bob");
  await login(page);
  await page.goto("/messages");
  await page.getByPlaceholder("输入用户名，开始新会话").fill(bob);
  await page.keyboard.press("Enter");
  await expect(page.locator(".window .peer")).toHaveText(bob); // 悬浮窗内切换到会话（反馈 #79）
  const box = page.getByPlaceholder("输入消息，回车发送，Shift + 回车换行");
  await box.fill("送你一组节点 ");
  await page.getByRole("button", { name: "表情" }).click();
  await page.getByRole("option", { name: "🎁" }).click();
  await page.getByRole("button", { name: "附上节点" }).click();
  await page.locator(".pick").first().getByRole("button", { name: "分享" }).click();
  await expect(page.locator(".chip")).toBeVisible();
  await page.getByRole("button", { name: "发送" }).click();
  await expect(page.locator(".msg.mine .text").last()).toHaveText("送你一组节点 🎁");
  await expect(page.locator(".msg.mine .card")).toBeVisible();
  await page.screenshot({ path: "test-results/shot-messages.png" });

  // 收件人：角标、自动加入、已读
  const ctx = await browser.newContext();
  const p2 = await ctx.newPage();
  await login(p2, { username: bob, password: "a long enough password" });
  await expect(p2.locator(".msg-btn .dot")).toHaveText("1"); // 右上角的消息图标（反馈 #75）
  await p2.goto("/messages/admin");
  await expect(p2.getByRole("link", { name: "已在节点页" })).toBeVisible();
  await expect(p2.locator(".msg-btn .dot")).toHaveCount(0);
  await p2.goto("/sources");
  await expect(p2.getByText(/来自 admin/)).toBeVisible();
  await ctx.close();
});

test("授权与回收站：联想收件人，授权节点给好友，对方只读使用；删除对话收回授权，可从回收站恢复", async ({ browser, page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  const bob = await createUser(request, baseURL!, "gr");
  await seedProfile(request, baseURL!, "授权");
  await login(page);
  await page.goto("/messages");
  await page.getByPlaceholder("输入用户名，开始新会话").fill(bob.slice(0, 4));
  await expect(page.locator(".hits").getByText(bob)).toBeVisible();
  await expect(page.locator(".hits").getByText("可以发消息").first()).toBeVisible();
  await page.screenshot({ path: "test-results/shot-messages-hits.png" });
  await page.locator(".hits").getByText(bob).click();
  await expect(page.locator(".window .peer")).toHaveText(bob); // 悬浮窗内切换到会话（反馈 #79）
  await page.getByRole("button", { name: "附上节点" }).click();
  await page.locator(".pick").first().getByRole("button", { name: "授权" }).click();
  await expect(page.locator(".chip")).toContainText("授权");
  await page.getByRole("button", { name: "发送" }).click();
  await expect(page.locator(".msg.mine .card")).toContainText("有效");
  await page.screenshot({ path: "test-results/shot-messages-grant.png" });

  const ctx = await browser.newContext();
  const p2 = await ctx.newPage();
  await login(p2, { username: bob, password: "a long enough password" });
  await p2.goto("/sources");
  await expect(p2.getByRole("heading", { name: "别人授权给我的节点" })).toBeVisible();
  await expect(p2.locator(".granted li")).toHaveCount(1);
  await p2.screenshot({ path: "test-results/shot-sources-granted.png" });

  // 删除对话：放进回收站，授权收回
  page.once("dialog", (d) => void d.accept());
  await page.getByRole("button", { name: "删除对话" }).click();
  await expect(page.getByText("对话在回收站中")).toBeVisible();
  await p2.reload();
  await expect(p2.locator(".granted li")).toHaveCount(0);
  await page.getByRole("button", { name: /回收站/ }).click();
  await expect(page.locator(".th.bin")).toContainText(bob);
  await page.screenshot({ path: "test-results/shot-messages-bin.png" });
  await page.locator(".th.bin").getByRole("button", { name: "恢复" }).click();
  await expect(page.locator(".msg.mine .card")).toContainText("已收回");
  await ctx.close();
});

test("管理：审计日志分页", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  // 先产生足够多的审计记录（每新建一条配置链接记一条），保证不止一页
  const { profile } = await seedProfile(request, baseURL!, "审计");
  const call = await apiAs(request, baseURL!);
  for (let i = 0; i < 32; i++) await call("POST", `/api/v1/profiles/${profile}/links`, { note: "a" + i });
  await login(page);
  await page.goto("/admin");
  const pager = page.getByRole("navigation", { name: "审计日志分页" });
  await expect(pager).toContainText("第1 /");
  await pager.getByRole("button", { name: "下一页" }).click();
  await expect(pager).toContainText("第2 /");
});

test("删除配置：从列表卡片、编辑器的「更多」或命令面板删除，订阅链接随之失效", async ({ page, request, baseURL }) => {
  const suffix = " " + test.info().project.name;
  const a = await seedProfile(request, baseURL!, "删除甲" + suffix);
  const b = await seedProfile(request, baseURL!, "删除乙" + suffix);
  const link = await b.call<{ url: string }>("POST", `/api/v1/profiles/${a.profile}/links`, { note: "手机" }); // 用最后一次登录的会话（每次登录换新的 CSRF）
  await login(page);

  // 列表：卡片右端的删除按钮 → 对话框列出会一起删除的内容 → 确认
  await page.getByRole("button", { name: `删除配置「删除甲${suffix}」` }).click();
  const dialog = page.getByRole("dialog", { name: "删除配置" });
  await expect(dialog).toContainText("1条订阅链接");
  await dialog.getByRole("button", { name: "删除", exact: true }).click();
  await expect(page.getByRole("link", { name: "删除甲" + suffix })).toHaveCount(0);
  // 可撤销（R33）：10 秒后才真正删除，订阅链接随之失效
  await expect.poll(async () => (await request.get(new URL(link.url).pathname)).status(), { timeout: 20_000 }).toBe(404);

  // 编辑器：桌面在「更多」里，手机（分节导航是下拉框）经命令面板；删除后回到列表
  await page.goto(`/profiles/${b.profile}`);
  await page.locator("#editor-main").waitFor();
  if (test.info().project.name === "mobile") {
    await page.keyboard.press("Control+K");
    await expect(page.locator("dialog.palette input")).toBeFocused();
    await page.locator("dialog.palette input").fill(">删除配置");
    await page.keyboard.press("Enter");
  } else {
    await page.getByText("更多", { exact: true }).click();
    await page.getByRole("button", { name: "删除配置" }).click();
  }
  await page.getByRole("dialog", { name: "删除配置" }).getByRole("button", { name: "删除", exact: true }).click();
  // 回到配置列表（首页），而不是不存在的 /profiles
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole("heading", { name: "配置", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "删除乙" + suffix })).toHaveCount(0);
});

test("节点页：显示节点概况，直接分享或授权给好友；注册时安全问题可选并逐个出现", async ({ browser, page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  const bob = await createUser(request, baseURL!, "nd");
  await seedProfile(request, baseURL!, "概况");
  await login(page);
  await page.goto("/sources");
  await expect(page.locator("tbody tr").first()).toContainText("个可用节点");
  await page.locator("tbody tr").first().getByRole("button", { name: /^授权「/ }).click();
  const dlg = page.locator("dialog[open]");
  await dlg.getByPlaceholder("输入对方的用户名").fill(bob);
  await expect(dlg.locator('input[name="share-mode"][value="grant"]')).toBeChecked();
  await dlg.getByRole("button", { name: "发送" }).click();
  await expect(page.getByText("已授权给" + bob)).toBeVisible();
  await page.screenshot({ path: "test-results/shot-sources-share.png" });

  // 注册：先只显示一个安全问题，答了才出现下一个；不答也能注册
  const admin = await apiAs(request, baseURL!);
  await admin("PUT", "/api/v1/admin/registration", { open: true });
  const ctx = await browser.newContext();
  const p2 = await ctx.newPage();
  await p2.goto("/register");
  await expect(p2.getByLabel("第1个问题的答案")).toBeVisible();
  await expect(p2.getByLabel("第2个问题的答案")).toHaveCount(0);
  await p2.getByLabel("第1个问题的答案").fill("Beijing");
  await expect(p2.getByLabel("第2个问题的答案")).toBeVisible();
  await p2.getByLabel("第1个问题的答案").fill("");
  await p2.screenshot({ path: "test-results/shot-register.png" });
  await admin("PUT", "/api/v1/admin/registration", { open: false });
  await ctx.close();
});

test("隐私说明：节点页提示加密保存，链接到帮助中的「隐私与安全」", async ({ page }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  await login(page);
  await page.goto("/sources");
  await expect(page.locator(".privacy")).toContainText("加密保存");
  await page.locator(".privacy").getByRole("link", { name: "了解详情" }).click();
  await expect(page.getByRole("heading", { name: "隐私与安全" }).first()).toBeVisible();
  await expect(page.getByText("在技术上可以绕过网站去解密")).toBeVisible();
  await page.screenshot({ path: "test-results/shot-privacy.png", fullPage: true });
});

test("消息是悬浮窗：在当前页面上打开，不离开页面，Esc 关闭，大小随窗口变化", async ({ page }) => {
  test.skip(test.info().project.name === "mobile", "手机上铺满屏幕，另见下方");
  await login(page);
  await page.goto("/sources");
  await page.locator(".msg-btn").click();
  const win = page.locator(".window");
  await expect(win).toBeVisible();
  await expect(page).toHaveURL(/\/sources$/); // 不离开当前页面
  const big = (await win.boundingBox())!;
  await page.setViewportSize({ width: 900, height: 640 });
  const small = (await win.boundingBox())!;
  expect(small.width).toBeLessThan(big.width);
  expect(small.height).toBeLessThanOrEqual(640);
  await page.screenshot({ path: "test-results/shot-chat-window.png" });
  await page.keyboard.press("Escape");
  await expect(win).toHaveCount(0);
  // 点击窗口外的页面也会关闭；点击窗口内不会；消息图标再点一次是收起
  await page.locator(".msg-btn").click();
  await expect(win).toBeVisible();
  await win.locator(".win-head h2").click();
  await expect(win).toBeVisible();
  await page.mouse.click(40, 600);
  await expect(win).toHaveCount(0);
  await page.locator(".msg-btn").click();
  await expect(win).toBeVisible();
  await page.locator(".msg-btn").click();
  await expect(win).toHaveCount(0);
});

test("讲解图层：从分流规则旁的「?」打开，逐步切换，Esc 关闭", async ({ page, request, baseURL }) => {
  const problems = watch(page);
  const { profile } = await seedProfile(request, baseURL!, "讲解测试");
  await login(page);
  await page.goto(`/profiles/${profile}`);
  await page.getByRole("button", { name: "流量是怎么走的" }).click();
  const dlg = page.getByRole("dialog", { name: "流量是怎么走的" });
  await expect(dlg).toBeVisible();
  const panel = dlg.getByRole("tabpanel");
  await expect(panel.locator("h3")).toHaveText("应用发出请求");
  await expect(dlg.getByRole("button", { name: "上一步" })).toBeDisabled();
  // 「下一步」逐步前进，步骤按钮直接跳转，最后一步变为「完成」
  await dlg.getByRole("button", { name: "下一步" }).click();
  await expect(panel.locator("h3")).toHaveText("客户端接住");
  await dlg.getByRole("tab", { name: "5. 从出口离开" }).click();
  await expect(dlg.getByRole("tab", { name: "5. 从出口离开" })).toHaveAttribute("aria-selected", "true");
  await expect(dlg.getByRole("button", { name: "完成" })).toBeVisible();
  await page.screenshot({ path: `test-results/shot-explain-${test.info().project.name}.png` });
  await dlg.getByRole("tab", { name: "3. 规则从上往下比对" }).click();
  await page.emulateMedia({ colorScheme: "dark" });
  await page.screenshot({ path: `test-results/shot-explain-dark-${test.info().project.name}.png` });
  await page.emulateMedia({ colorScheme: "light" });
  await dlg.getByRole("tab", { name: "5. 从出口离开" }).click();
  // 页面本身没有横向滚动（窄屏时图在自己的容器里滚动）
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.keyboard.press("Escape");
  await expect(dlg).toBeHidden();
  // 再次打开从第一步开始；「完成」关闭
  await page.getByRole("button", { name: "流量是怎么走的" }).click();
  await expect(panel.locator("h3")).toHaveText("应用发出请求");
  await dlg.getByRole("tab", { name: "5. 从出口离开" }).click();
  await dlg.getByRole("button", { name: "完成" }).click();
  await expect(dlg).toBeHidden();
  expect(problems).toEqual([]);
});

test("分流：地图与列表、追踪的逐步卡片、清空、粒子模式（不联网：内联规则）", async ({ page, request, baseURL }) => {
  const problems = watch(page);
  const call = await apiAs(request, baseURL!);
  const src = await call<{ id: string }>("POST", "/api/v1/sources", { name: "分流 e2e 节点", kind: "batch", content: "- {name: 香港 01, type: ss, server: 192.0.2.61, port: 8388, cipher: aes-128-gcm, password: example-pw}\n- {name: 日本 02, type: ss, server: 192.0.2.62, port: 8388, cipher: aes-128-gcm, password: example-pw}\n" });
  const groups = '[[custom_groups]]\nname = "PROXY"\ntype = "select"\nrule = [".*", "[]DIRECT"]\n';
  const rulesets = '[[rulesets]]\ngroup = "PROXY"\nruleset = "[]DOMAIN-SUFFIX,example.com"\n[[rulesets]]\ngroup = "DIRECT"\nruleset = "[]MATCH"\n';
  const rs = await call<{ id: string }>("POST", "/api/v1/rule-sources", { name: "分流 e2e 规则", kind: "toml", groupsText: groups, rulesetsText: rulesets });
  const doc = [
    "clash-launch/profile: v1",
    "name: 分流 e2e",
    'target: { pack: mihomo, preset: mihomo, core: "1.19.33" }',
    "sources:",
    `  - ref: ${src.id}`,
    `rules: { ref: ${rs.id} }`,
    "raw: |",
    "  rule-providers:",
    "    Ads: {type: inline, behavior: domain, payload: ['+.ads.example.net']}",
    "  rules: !prepend",
    "    - RULE-SET,Ads,REJECT",
    "",
  ].join("\n");
  const p = await call<{ id: string }>("POST", "/api/v1/profiles", { doc, note: "" });
  await login(page);
  await page.goto(`/profiles/${p.id}?section=routing`);
  await expect(page.getByRole("heading", { name: "分流", exact: true })).toBeVisible();
  await expect(page.locator(".viewnote")).toContainText("3条规则逐条显示");
  const mobile = test.info().project.name === "mobile";
  if (mobile) {
    // 窄屏：列表模式，写明默认从哪里出去
    await expect(page.locator("ol.list")).toContainText("PROXY → 🇭🇰 香港 01");
  } else {
    await expect(page.locator(".map [data-box='rule:0']")).toContainText("Ads");
    await expect(page.locator(".map [data-box='group:PROXY']")).toBeVisible();
    // 粒子模式保存到账号偏好
    await page.getByRole("radio", { name: "聚焦" }).click();
    await expect(page.getByRole("radio", { name: "聚焦" })).toHaveAttribute("aria-checked", "true");
    await page.getByRole("radio", { name: "全部", exact: true }).click();
  }
  // 示例来自规则集的真实条目；追踪给出逐步卡片
  await page.getByRole("button", { name: "ads.example.net" }).click();
  await expect(page.locator(".hops")).toContainText("第1条「Ads」命中");
  await expect(page.locator(".hops")).toContainText("结束：已拦截");
  const input = page.getByLabel("输入域名或IP，例如www.example.com");
  await input.fill("www.example.com");
  await page.getByRole("button", { name: "追踪", exact: true }).click();
  await expect(page.locator(".hops")).toContainText("第2条「DOMAIN-SUFFIX,example.com」命中");
  await expect(page.locator(".hops")).toContainText("交给PROXY");
  await expect(page.locator(".hops")).toContainText("命中：DOMAIN-SUFFIX,example.com");
  // 追踪后地图滚到可见（粒子动画在地图上，反馈 #88）
  await expect(page.locator(".routing .card").first()).toBeInViewport();
  if (!mobile) {
    // 重置视图：取消选中、清空追踪
    await page.locator(".map [data-box='group:PROXY']").click();
    await expect(page.locator(".detail")).toBeVisible();
    const scrolled = () => page.evaluate(() => document.getElementById("editor-main")!.scrollTop);
    const top = await scrolled();
    await page.getByRole("button", { name: "重置视图" }).click();
    // 重置不移动页面，也不把光标放到追踪框（反馈 #89）
    await expect(page.getByLabel("输入域名或IP，例如www.example.com")).not.toBeFocused();
    // 内容变短时浏览器会把滚动位置往回收，但不会往下跳
    expect(await scrolled()).toBeLessThanOrEqual(top + 1);
    await expect(page.locator(".routing .card").first()).toBeInViewport();
    await expect(page.locator(".detail")).toHaveCount(0);
    await expect(page.locator(".hops")).toHaveCount(0);
    await page.getByLabel("输入域名或IP，例如www.example.com").fill("www.example.com");
    await page.getByRole("button", { name: "追踪", exact: true }).click();
    await expect(page.locator(".hops")).toBeVisible();
  }
  await page.screenshot({ path: `test-results/shot-routing-${test.info().project.name}.png`, fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  // × 清空并结束追踪
  await input.hover();
  await page.getByRole("button", { name: "清空" }).click();
  await expect(input).toHaveValue("");
  await expect(page.locator(".hops")).toHaveCount(0);
  expect(problems).toEqual([]);
});

test("分流编辑：拖动改走向、合并与拆出、改组类型、拒绝循环；应用后写入配置，可撤销", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "拖动编辑只在桌面（手机用列表模式）");
  const problems = watch(page);
  const call = await apiAs(request, baseURL!);
  const src = await call<{ id: string }>("POST", "/api/v1/sources", { name: "编辑 e2e 节点", kind: "batch", content: "- {name: 香港 01, type: ss, server: 192.0.2.71, port: 8388, cipher: aes-128-gcm, password: example-pw}\n" });
  // 规则地址指向本机的关闭端口：服务器的 SSRF 防护直接拒绝，不联网
  const u = (kind: string, f: string) => `http://127.0.0.1:9/${kind}/${f}`;
  const groups = ['[[custom_groups]]\nname = "PROXY"\ntype = "select"\nrule = [".*", "[]DIRECT"]', '[[custom_groups]]\nname = "AUTO"\ntype = "url-test"\nurl = "https://www.gstatic.com/generate_204"\ninterval = 300\nrule = ["(?i)^(?!.*example).*$"]', '[[custom_groups]]\nname = "ALT"\ntype = "select"\nrule = ["[]PROXY"]'].join("\n\n");
  const rulesets = [
    ["PROXY", u("clash-domain", "A.yaml"), "clash-domain"],
    ["PROXY", u("clash-classic", "A-classic.yaml"), "clash-classic"],
    ["AUTO", u("clash-domain", "B.yaml"), "clash-domain"],
  ].map(([g, r, ty]) => `[[rulesets]]\ngroup = "${g}"\nruleset = "${r}"\ntype = "${ty}"`).join("\n") + '\n[[rulesets]]\ngroup = "PROXY"\nruleset = "[]DOMAIN-SUFFIX,example.com"\n[[rulesets]]\ngroup = "DIRECT"\nruleset = "[]MATCH"\n';
  const rs = await call<{ id: string }>("POST", "/api/v1/rule-sources", { name: "编辑 e2e 规则", kind: "toml", groupsText: groups, rulesetsText: rulesets });
  const doc = ["clash-launch/profile: v1", "name: 分流编辑 e2e", 'target: { pack: mihomo, preset: mihomo, core: "1.19.33" }', "sources:", `  - ref: ${src.id}`, `rules: { ref: ${rs.id} }`, "output: { empty-groups: keep }", ""].join("\n");
  const p = await call<{ id: string }>("POST", "/api/v1/profiles", { doc, note: "" });
  await login(page);
  await page.goto(`/profiles/${p.id}?section=routing`);
  const box = (k: string) => page.locator(`.map [data-box='${k}']`);
  // A 与 A-classic 是 domainrouter_v2 约定的三件套，自动识别为合集
  await expect(box("bundle:0")).toContainText("合集");

  async function dragTo(from: string, to: string, where: "mid" | "top" | "bottom" = "mid") {
    const a = (await box(from).boundingBox())!;
    const b = (await box(to).boundingBox())!;
    await page.mouse.move(a.x + a.width / 2, a.y + a.height / 2);
    await page.mouse.down();
    await page.mouse.move(a.x + a.width / 2 + 12, a.y + a.height / 2 + 12, { steps: 3 });
    const y = where === "top" ? b.y + b.height * 0.1 : where === "bottom" ? b.y + b.height * 0.9 : b.y + b.height / 2;
    await page.mouse.move(b.x + b.width / 2, y, { steps: 10 });
    await page.mouse.up();
  }
  const bar = page.getByRole("region", { name: "未应用的改动" });

  // 合并：B 拖到合集中间 → 并入合集；取消
  await dragTo("rule:2", "bundle:0");
  await expect(bar).toContainText("B：并入合集A");
  await page.getByRole("button", { name: "取消" }).click();
  await expect(bar).toHaveCount(0);
  // 拆出：选中合集，在详情里拆出 A-classic
  await box("bundle:0").click();
  await page.getByRole("button", { name: "拆出" }).last().click();
  await expect(bar).toContainText("A-classic：离开合集A");
  await page.getByRole("button", { name: "取消" }).click();

  // 循环：ALT 引用 PROXY，把 ALT 加入 PROXY 会成环，被拒绝
  await dragTo("group:ALT", "group:PROXY");
  await expect(page.locator(".edit-err")).toContainText("循环");
  await expect(bar).toHaveCount(0);

  // 改走向：内联规则拖到「直连」；改组类型：AUTO 改为故障转移
  await dragTo("rule:3", "exit:builtin:DIRECT");
  await expect(bar).toContainText("DOMAIN-SUFFIX,example.com：PROXY → DIRECT");
  await box("group:AUTO").click();
  await page.getByRole("radio", { name: "故障转移" }).click();
  await expect(bar).toContainText("AUTO：url-test → fallback");
  await expect(bar).toContainText("应用后这个配置会自己管理分流");
  await page.screenshot({ path: "test-results/shot-routing-edit.png", fullPage: true });
  // 应用前对比与检查（反馈 #88）：e2e 服务器没有 mihomo，检查说明无法预先检查，仍可应用
  await page.getByRole("button", { name: "应用", exact: true }).click();
  const review = page.getByRole("dialog", { name: "应用前对比与检查" });
  await expect(review).toContainText("行变化");
  await expect(review).toContainText("无法预先检查");
  await page.screenshot({ path: "test-results/shot-routing-apply.png" });
  await review.getByRole("button", { name: "确认应用" }).click();
  await expect(bar).toHaveCount(0);
  await expect(page.getByText("已自动保存")).toBeVisible();
  const saved = await call<{ doc: string }>("GET", `/api/v1/profiles/${p.id}`);
  expect(saved.doc).toContain("routing:");
  const pv = await call<{ yaml: string }>("POST", `/api/v1/profiles/${p.id}/preview`, { doc: saved.doc });
  expect(pv.yaml).toContain("DOMAIN-SUFFIX,example.com,DIRECT");
  expect(pv.yaml).toMatch(/name: AUTO\n\s+type: fallback/);
  // 撤销：回到跟随上游
  await page.getByRole("button", { name: "撤销" }).click();
  await expect(page.getByText("已撤销到这次编辑之前")).toBeVisible();
  await expect.poll(async () => (await call<{ doc: string }>("GET", `/api/v1/profiles/${p.id}`)).doc).not.toContain("routing:");
  expect(problems).toEqual([]);
});

test("分流：规则源更新后提示并能采纳；从追踪固定期望", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "桌面覆盖即可");
  const problems = watch(page);
  const call = await apiAs(request, baseURL!);
  const src = await call<{ id: string }>("POST", "/api/v1/sources", { name: "同步 e2e 节点", kind: "batch", content: "- {name: 香港 01, type: ss, server: 192.0.2.81, port: 8388, cipher: aes-128-gcm, password: example-pw}\n" });
  const groups = '[[custom_groups]]\nname = "PROXY"\ntype = "select"\nrule = [".*", "[]DIRECT"]\n';
  const rulesets = '[[rulesets]]\ngroup = "PROXY"\nruleset = "[]DOMAIN-SUFFIX,example.com"\n[[rulesets]]\ngroup = "DIRECT"\nruleset = "[]MATCH"\n';
  const rs = await call<{ id: string; version: number }>("POST", "/api/v1/rule-sources", { name: "同步 e2e 规则", kind: "toml", groupsText: groups, rulesetsText: rulesets });
  const doc = ["clash-launch/profile: v1", "name: 分流同步 e2e", 'target: { pack: mihomo, preset: mihomo, core: "1.19.33" }', "sources:", `  - ref: ${src.id}`, `rules: { ref: ${rs.id} }`, ""].join("\n");
  const p = await call<{ id: string; version: number }>("POST", "/api/v1/profiles", { doc, note: "" });
  // 开始自己管理：fork 得到的 routing 段写入文档
  const fk = await call<{ routing: string }>("POST", `/api/v1/profiles/${p.id}/routing/fork`, { doc });
  const own = doc + "routing:\n" + fk.routing.trimEnd().split("\n").map((l) => (l ? "  " + l : l)).join("\n") + "\n";
  await call("PUT", `/api/v1/profiles/${p.id}`, { doc: own }, p.version);
  // 规则源更新：新增一条规则
  await call("PUT", `/api/v1/rule-sources/${rs.id}`, { name: "同步 e2e 规则", kind: "toml", groupsText: groups, rulesetsText: '[[rulesets]]\ngroup = "PROXY"\nruleset = "[]DOMAIN,new.example.org"\n' + rulesets }, rs.version);

  await login(page);
  await page.goto(`/profiles/${p.id}?section=routing`);
  await expect(page.getByText("规则源有1处更新还没有处理。")).toBeVisible();
  await page.getByRole("button", { name: "查看" }).click();
  const dlg = page.getByRole("dialog", { name: "规则源的更新" });
  await expect(dlg).toContainText("新增规则DOMAIN,new.example.org");
  await dlg.getByRole("button", { name: "采纳勾选的1处，其余忽略" }).click();
  const bar = page.getByRole("region", { name: "未应用的改动" });
  await expect(bar).toContainText("新增DOMAIN,new.example.org → PROXY");
  await expect(bar).toContainText("处理了规则源的更新");

  // 固定期望：追踪 www.example.com，命中交给 PROXY
  await page.getByLabel("输入域名或IP，例如www.example.com").fill("www.example.com");
  await page.getByRole("button", { name: "追踪", exact: true }).click();
  await page.getByRole("button", { name: "固定为期望：始终经过PROXY" }).click();
  await expect(bar).toContainText("期望：www.example.com经过PROXY");
  // 应用前对比与检查（反馈 #88）：e2e 服务器没有 mihomo，检查说明无法预先检查，仍可应用
  await page.getByRole("button", { name: "应用", exact: true }).click();
  const review = page.getByRole("dialog", { name: "应用前对比与检查" });
  await expect(review).toContainText("行变化");
  await expect(review).toContainText("无法预先检查");
  await review.getByRole("button", { name: "确认应用" }).click();
  await expect(bar).toHaveCount(0);
  await expect(page.getByText("规则源有1处更新还没有处理。")).toHaveCount(0);
  await expect(page.locator(".checks")).toContainText("✓");
  await expect(page.getByText("已自动保存")).toBeVisible();
  const saved = await call<{ doc: string }>("GET", `/api/v1/profiles/${p.id}`);
  expect(saved.doc).toContain("new.example.org");
  expect(saved.doc).toContain("host: www.example.com");
  expect(problems).toEqual([]);
});

test("定时退出：其他设备一起定时，对方页面顶部提示；可以逐台取消", async ({ browser, page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  // 不新建账号（注册有每 IP 每小时的次数限制，整套 e2e 已接近上限）：管理员在两个浏览器中登录
  const ctx2 = await browser.newContext();
  const other = await ctx2.newPage();
  await login(other);
  await login(page);

  await page.goto("/account");
  await page.getByRole("button", { name: "其他设备定时退出" }).click();
  const dialog = page.getByRole("dialog", { name: "其他设备定时退出" });
  await expect(dialog.getByRole("radio", { name: /1小时后/ })).toBeChecked();
  await dialog.getByRole("button", { name: "定时退出" }).click();
  const current = page.locator(".sessions li").filter({ hasText: "当前设备" });
  await expect(current).not.toContainText("将于");
  await expect(page.locator(".sessions li").filter({ hasText: "将于" }).first()).toBeVisible();

  await other.goto("/account");
  await expect(other.getByRole("status").filter({ hasText: "此设备将于" })).toBeVisible();
  // 在被定时的设备上取消自己的定时
  await other.locator(".sessions li").filter({ hasText: "当前设备" }).getByRole("button", { name: "取消定时" }).click();
  await expect(other.getByText("此设备将于")).toHaveCount(0);
  await ctx2.close();
  // 清理：取消其余设备的定时（之前的测试留下的管理员会话）
  await (await apiAs(request, baseURL!))("PUT", "/api/v1/account/sessions/others/logout-at", { at: null });
});

test("昵称与邀请：设置昵称；账户页生成可用多次的邀请，注册页显示邀请人", async ({ browser, page }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  await login(page);
  await page.goto("/account");
  await page.getByLabel("昵称", { exact: true }).fill("Ad Min");
  await page.locator("section").filter({ hasText: "昵称与用户名" }).getByRole("button", { name: "保存" }).click();
  await expect(page.getByText("昵称已保存")).toBeVisible();

  const panel = page.locator("section").filter({ has: page.getByRole("heading", { name: "邀请朋友" }) });
  await panel.getByLabel("可用次数").selectOption({ label: "可用5次" });
  await panel.getByRole("button", { name: "生成邀请" }).click();
  const url = (await panel.locator("code.url").textContent())!;
  await panel.getByRole("button", { name: "二维码" }).click();
  await expect(panel.getByRole("img", { name: "二维码" })).toBeVisible();
  await expect(panel.locator("tbody tr").first()).toContainText("0/5");

  const ctx = await browser.newContext();
  const guest = await ctx.newPage();
  await guest.goto(new URL(url).pathname + new URL(url).hash);
  await expect(guest.getByRole("note")).toContainText("Ad Min（@admin）邀请你加入");
  await ctx.close();

  // 清理：作废这个邀请，昵称还原
  await panel.locator("tbody tr").first().getByRole("button", { name: "撤销" }).click();
  await expect(panel.locator("tbody tr").first()).toContainText("已作废");
  await page.getByLabel("昵称", { exact: true }).fill("");
  await page.locator("section").filter({ hasText: "昵称与用户名" }).getByRole("button", { name: "保存" }).click();
});

test("节点源：保存后返回列表，列表中可直接删除（R48、R50）", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  const call = await apiAs(request, baseURL!);
  const name = "可删除的节点 " + Date.now();
  const src = await call<{ id: string }>("POST", "/api/v1/sources", {
    name, kind: "batch", content: "- {name: 测试 01, type: ss, server: 192.0.2.31, port: 8388, cipher: aes-128-gcm, password: example-pw}\n",
  });
  await login(page);
  await page.goto(`/sources/${src.id}`);
  await page.getByRole("button", { name: "保存" }).click();
  await expect(page).toHaveURL(/\/sources$/);
  await page.getByRole("button", { name: `删除节点源「${name}」` }).click();
  await expect(page.getByRole("link", { name })).toHaveCount(0);
});

test("字段：恢复默认后可在原位撤销；敏感字段不被浏览器自动填充（R52、R51）", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  const { profile } = await seedProfile(request, baseURL!, "恢复默认");
  await login(page);
  await page.goto(`/profiles/${profile}?section=inbound`);
  const card = page.locator("#f-mixed-port");
  await expect(page.locator("#ctl-mixed-port")).toHaveValue("7890");
  await card.getByRole("button", { name: "恢复默认" }).click();
  await expect(card.getByRole("status")).toHaveText("已恢复默认");
  await card.getByRole("button", { name: "撤销" }).click();
  await expect(page.locator("#ctl-mixed-port")).toHaveValue("7890");
  await expect(card.getByRole("button", { name: "恢复默认" })).toBeVisible();
  await page.goto(`/profiles/${profile}?section=controller`);
  await expect(page.locator("#ctl-secret")).toHaveAttribute("autocomplete", "new-password");
});

test("节点源：内容失焦后下方节点立即更新，不必先保存（R49）", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  const { source } = await seedProfile(request, baseURL!, "即时预览");
  await login(page);
  await page.goto(`/sources/${source}`);
  const box = page.locator("#sd-content");
  await box.fill("- {name: 预览专用 07, type: ss, server: 192.0.2.37, port: 8388, cipher: aes-128-gcm, password: example-pw}\n");
  await page.getByRole("heading", { level: 1 }).click(); // 点到别处，触发失焦
  await expect(page.getByText("预览专用 07")).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/sources/${source}$`)); // 没有保存、仍在原页
});

test("实时通道：客户端拉取订阅后，配置链接页立即更新，不等定时刷新（R22）", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  const problems = watch(page);
  const { profile } = await seedProfile(request, baseURL!, "实时");
  const call = await apiAs(request, baseURL!);
  const l = await call<{ url: string }>("POST", `/api/v1/profiles/${profile}/links`, { note: "实时" });
  await login(page);
  await page.goto(`/profiles/${profile}/links`);
  const total = page.locator(".moment").filter({ hasText: "累计被请求" }).locator("strong");
  await expect(total).toHaveText("0");
  await page.waitForTimeout(500); // 等实时通道连上（hello）
  await request.get(l.url, { headers: { "User-Agent": "mihomo/1.19.33" } });
  // 定时刷新是 30 秒一次：5 秒内更新只能来自实时通道
  await expect(total).toHaveText("1", { timeout: 5_000 });
  expect(problems).toEqual([]);
});

test("关闭动效与在线人数设置（R11）", async ({ page }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  const problems = watch(page);
  await login(page);
  await page.goto("/settings");
  const motion = page.getByLabel("关闭动效");
  await motion.check();
  await expect(page.locator("html")).toHaveAttribute("data-motion", "off");
  await page.reload(); // 偏好存在服务端：重新打开仍然生效
  await expect(page.locator("html")).toHaveAttribute("data-motion", "off");
  await page.getByLabel("关闭动效").uncheck();
  await expect(page.locator("html")).not.toHaveAttribute("data-motion", "off");

  await page.goto("/admin");
  const sw = page.getByRole("switch", { name: "向普通用户显示在线人数" });
  await expect(sw).toHaveAttribute("aria-checked", "false");
  await sw.click();
  await expect(sw).toHaveAttribute("aria-checked", "true");
  await sw.click(); // 恢复默认，不影响其他用例
  await expect(sw).toHaveAttribute("aria-checked", "false");
  expect(problems).toEqual([]);
});

test("在线指示灯与多标签页共用一条实时连接（R11、R22）", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "与桌面相同");
  const problems = watch(page);
  const { profile } = await seedProfile(request, baseURL!, "共用连接");
  const call = await apiAs(request, baseURL!);
  const l = await call<{ url: string }>("POST", `/api/v1/profiles/${profile}/links`, { note: "共用连接" });
  // 第二个标签页建立的 WebSocket 数：主标签页还在时应为 0（页面重新加载时 Playwright 不报告旧连接关闭，所以不统计第一个页面）
  let secondSockets = 0;
  page.context().on("page", (p) => p.on("websocket", () => secondSockets++));
  await login(page);
  await page.goto(`/profiles/${profile}/links`);

  // 连上后灯常亮、随操作起伏或因未读而呼吸（前面的用例可能留下未读），不是「正在连接」或「断线」；
  // 端到端环境没有 mihomo，服务端内核检查不可用，灯降级为琥珀色并在说明里写明原因
  const pulse = page.locator(".sidebar-foot .pulse");
  await expect(pulse).toHaveAttribute("data-state", /^(idle|active|unread|degraded)$/, { timeout: 5_000 });
  await pulse.hover();
  const tip = page.getByRole("tooltip");
  await expect(tip).toContainText("往返延迟");
  await expect(tip).toContainText(/\d+ ms/);
  await expect(tip).toContainText("在线人数"); // 管理员能看到在线人数
  await expect(tip).toContainText("内核不可用");
  await page.mouse.move(600, 400);
  await expect(tip).toBeHidden();

  // 第二个标签页：不新建连接，事件经主标签页转发
  const second = await page.context().newPage();
  await second.goto(`/profiles/${profile}/links`);
  await expect(second.locator(".sidebar-foot .pulse")).toHaveAttribute("data-state", /^(idle|active|unread|degraded)$/, { timeout: 5_000 });
  const totals = [page, second].map((p) => p.locator(".moment").filter({ hasText: "累计被请求" }).locator("strong"));
  for (const x of totals) await expect(x).toHaveText("0");
  await request.get(l.url, { headers: { "User-Agent": "mihomo/1.19.33" } });
  for (const x of totals) await expect(x).toHaveText("1", { timeout: 5_000 });
  expect(secondSockets).toBe(0);

  // 主标签页关闭：第二个标签页接手连接，之后的事件照常到达
  await page.close();
  await expect.poll(() => secondSockets, { timeout: 10_000 }).toBe(1);
  await expect(second.locator(".sidebar-foot .pulse")).toHaveAttribute("data-state", /^(idle|active|unread|degraded)$/, { timeout: 10_000 });
  await request.get(l.url, { headers: { "User-Agent": "mihomo/1.19.33" } });
  await expect(totals[1]!).toHaveText("2", { timeout: 5_000 });
  expect(problems).toEqual([]);
});

test("用户协议：注册时同意，管理员发布新版本后普通用户先同意才能继续（R40）", async ({ browser, page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const problems = watch(page);
  // 公开页面：未登录也能读
  const anon = await browser.newContext();
  const pub = await anon.newPage();
  await pub.goto("/terms");
  await expect(pub.getByRole("heading", { name: "用户协议" })).toBeVisible();
  await expect(pub.getByText("节点与配置不能端到端加密")).toBeVisible();
  await expect(pub.getByText("{{")).toHaveCount(0); // 保留期占位符已替换
  // 注册页：不勾选不能提交；可在页内展开阅读
  const call = await apiAs(request, baseURL!);
  await call("PUT", "/api/v1/admin/registration", { open: true });
  await pub.goto("/register");
  const user = "terms" + Date.now().toString(36);
  await pub.getByLabel("用户名").fill(user);
  await pub.getByLabel("密码", { exact: true }).fill("a long enough password");
  await pub.getByLabel("确认密码").fill("a long enough password");
  await expect(pub.getByRole("button", { name: "注册" })).toBeDisabled();
  await pub.getByText("在这里阅读").click();
  await expect(pub.getByText("你的责任")).toBeVisible();
  await pub.getByLabel("我已阅读并同意").check();
  await expect(pub.getByRole("button", { name: "注册" })).toBeEnabled();
  await pub.getByRole("button", { name: "注册" }).click(); // 用这个账号继续测试新版本的拦截，不再另外注册
  await expect(pub).toHaveURL(/\/welcome$/);
  await call("PUT", "/api/v1/admin/registration", { open: false });
  await anon.close();

  // 普通用户在新版本发布后被引导先同意
  await login(page);
  await page.goto("/admin");
  const editor = page.getByRole("textbox", { name: /用户协议 · zh-CN/ });
  await editor.fill((await editor.inputValue()) + "\n\n- 端到端测试追加的一条。");
  await page.getByRole("button", { name: "发布新版本" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "发布新版本" }).click();
  await expect(page.getByText(/用户协议已更新为第\d+版/)).toBeVisible();

  const ctx = await browser.newContext();
  const u = await ctx.newPage();
  await login(u, { username: user, password: "a long enough password" });
  await expect(u.getByRole("heading", { name: "用户协议已更新" })).toBeVisible();
  await expect(u.getByText("端到端测试追加的一条")).toBeVisible();
  await expect(u.locator("nav.sidebar")).toHaveCount(0); // 同意前不显示主界面
  await u.getByRole("button", { name: "同意并继续" }).click();
  await expect(u.locator("nav.sidebar")).toBeVisible();
  await ctx.close();

  // 恢复默认文本，不影响之后的用例
  await page.getByRole("button", { name: "恢复默认文本" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "恢复默认文本" }).click();
  await expect(page.getByText(/第\d+版 · 默认文本/)).toBeVisible();
  expect(problems).toEqual([]);
});

test("访问令牌：生成后只显示一次，脚本可用它读取配置，吊销后失效（R25）", async ({ page, request }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const problems = watch(page);
  await login(page);
  await page.goto("/account");
  const box = page.locator("section", { has: page.getByRole("heading", { name: "访问令牌" }) });
  await box.getByLabel("名称").fill("e2e 脚本");
  await box.getByRole("button", { name: "生成令牌" }).click();
  const tok = (await box.locator("code.tok").textContent())!.trim();
  expect(tok).toMatch(/^clt_/);
  const auth = { headers: { Authorization: "Bearer " + tok } };
  expect((await request.get("/api/v1/profiles", auth)).status()).toBe(200);
  expect((await request.post("/api/v1/sources", { ...auth, data: { name: "x", kind: "batch", content: "" } })).status()).toBe(403); // 只读
  await box.getByRole("button", { name: "我已保存" }).click();
  await expect(box.locator("code.tok")).toHaveCount(0);
  await box.getByRole("button", { name: "吊销 e2e 脚本" }).click();
  await box.getByRole("button", { name: "确认吊销" }).click();
  await expect(box.getByText("e2e 脚本")).toHaveCount(0);
  expect((await request.get("/api/v1/profiles", auth)).status()).toBe(401);
  expect(problems).toEqual([]);
});

test("站内提醒：订阅链接被新的客户端拉取时提醒，可在提醒里直接更换链接令牌（R23）", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const problems = watch(page);
  const { profile, call } = await seedProfile(request, baseURL!, "提醒");
  const l = await call<{ url: string }>("POST", `/api/v1/profiles/${profile}/links`, { note: "提醒测试" });
  await request.get(l.url, { headers: { "User-Agent": "clash-verge/v2.0" } }); // 第一个客户端：基线，不提醒
  await request.get(l.url, { headers: { "User-Agent": "Stash/2.5" } });
  await call("GET", "/api/v1/analytics?days=1"); // 读取分析时写入内存中的统计，立即产生提醒（否则最迟约一分钟）

  await login(page);
  const bell = page.getByRole("button", { name: /条未读提醒/ });
  await expect(bell).toBeVisible();
  await bell.click();
  const panel = page.getByRole("dialog", { name: "提醒" });
  await expect(panel.getByText("订阅链接「提醒测试」第一次被Stash拉取。")).toBeVisible();
  page.once("dialog", (d) => void d.accept());
  await panel.getByRole("button", { name: "更换链接令牌" }).first().click();
  await expect(page.getByText("已生成新链接")).toBeVisible();
  expect((await request.get(l.url)).status()).toBe(404); // 旧链接失效
  await page.keyboard.press("Escape");
  await expect(panel).toBeHidden();
  await expect(page.getByRole("button", { name: "提醒", exact: true })).toBeVisible(); // 打开后已读，角标消失
  expect(problems).toEqual([]);
});

test("订阅链接访问限制：只允许指定客户端，每天次数用完返回 429（R42）", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const problems = watch(page);
  const { profile, call } = await seedProfile(request, baseURL!, "访问限制");
  const l = await call<{ url: string }>("POST", `/api/v1/profiles/${profile}/links`, { note: "限制测试" });
  await login(page);
  await page.goto(`/profiles/${profile}/links`);
  await page.getByRole("button", { name: "访问限制" }).first().click();
  const dlg = page.getByRole("dialog", { name: "访问限制" });
  await dlg.getByRole("button", { name: "+ Stash" }).click();
  await dlg.getByLabel("每天最多拉取次数").fill("1");
  await dlg.getByRole("button", { name: "保存" }).click();
  await expect(page.getByText("已限制：Stash · 每天1次")).toBeVisible();

  expect((await request.get(l.url, { headers: { "User-Agent": "clash-verge/v2.0" } })).status()).toBe(403);
  expect((await request.get(l.url, { headers: { "User-Agent": "Stash/2.5" } })).status()).toBe(200);
  const over = await request.get(l.url, { headers: { "User-Agent": "Stash/2.5" } });
  expect(over.status()).toBe(429);
  expect(Number(over.headers()["retry-after"])).toBeGreaterThan(0);

  // 清空限制
  await page.getByRole("button", { name: "访问限制" }).first().click();
  await dlg.getByRole("button", { name: "移除Stash" }).click();
  await dlg.getByLabel("每天最多拉取次数").fill("0");
  await dlg.getByRole("button", { name: "保存" }).click();
  await expect(page.getByText(/已限制：/)).toHaveCount(0);
  expect((await request.get(l.url, { headers: { "User-Agent": "clash-verge/v2.0" } })).status()).toBe(200);
  expect(problems).toEqual([]);
});

test("提醒渠道：内网地址被拒绝，测试失败时显示原因，修改后回到未验证（R31）", async ({ page }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const problems = watch(page);
  await login(page);
  await page.goto("/account");
  const box = page.locator("section", { has: page.getByRole("heading", { name: "提醒渠道" }) });
  await box.getByRole("button", { name: "添加渠道" }).click();
  // Webhook 指向本机：保存时被 SSRF 防护拒绝
  await box.getByLabel("Webhook地址").fill("https://127.0.0.1/hook");
  await box.getByRole("button", { name: "保存" }).click();
  await expect(page.getByText(/推送失败：.*安全策略/)).toBeVisible();
  // ntfy：保存后未验证；测试时连接不上，原因记在渠道上
  await box.getByRole("radio", { name: "ntfy" }).click();
  await expect(box.getByLabel("服务器地址")).toHaveValue("https://ntfy.sh");
  await box.getByLabel("服务器地址").fill("https://ntfy.example.com");
  await box.getByLabel("主题").fill("e2e-alerts");
  await box.getByLabel("名称").fill("手机");
  await box.getByRole("button", { name: "保存" }).click();
  const item = box.locator("li", { hasText: "手机" });
  await expect(item.getByText("未验证")).toBeVisible();
  await expect(item.getByText("e2e-alerts · ntfy.example.com")).toBeVisible();
  await item.getByRole("button", { name: "发送测试消息" }).click();
  await expect(item.getByText(/最近一次发送失败：/)).toBeVisible({ timeout: 20_000 });
  await expect(item.getByText("未验证")).toBeVisible();
  await item.getByRole("button", { name: "修改" }).click();
  await box.getByLabel("主题").fill("e2e-alerts-2");
  await box.getByRole("button", { name: "保存" }).click();
  await expect(item.getByText("e2e-alerts-2 · ntfy.example.com")).toBeVisible();
  await expect(item.getByText(/最近一次发送失败/)).toHaveCount(0);
  await item.getByRole("button", { name: "删除" }).click();
  await expect(box.getByText("还没有提醒渠道。")).toBeVisible();
  expect(problems.filter((p) => !p.includes("422"))).toEqual([]);
});

test("删除保护、可撤销的删除与批量操作（R55、R33、R35）", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const problems = watch(page);
  const a = await seedProfile(request, baseURL!, "保护甲");
  const b = await seedProfile(request, baseURL!, "保护乙");
  await login(page);

  // 编辑器「更多」里开启删除保护：删除入口禁用，服务端同样拒绝
  await page.goto(`/profiles/${a.profile}`);
  await page.locator("#editor-main").waitFor();
  await page.getByText("更多", { exact: true }).click();
  await page.getByRole("switch", { name: /删除保护/ }).click();
  await expect(page.getByText("已开启删除保护")).toBeVisible();
  await expect(page.getByRole("button", { name: "删除配置" })).toBeDisabled();

  // 列表：卡片上的删除按钮禁用；批量删除跳过受保护的
  await page.goto("/");
  await expect(page.getByRole("button", { name: "删除配置「保护甲」" })).toBeDisabled();
  await page.getByRole("checkbox", { name: "选择保护甲" }).check();
  await page.getByRole("checkbox", { name: "选择保护乙" }).check();
  await expect(page.getByRole("toolbar", { name: "批量操作" })).toContainText("已选2项");
  await page.getByRole("toolbar", { name: "批量操作" }).getByRole("button", { name: "删除" }).click();
  await expect(page.getByText("1个开启了删除保护的配置没有删除")).toBeVisible();
  await expect(page.getByRole("link", { name: "保护乙" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "保护甲" })).toBeVisible();

  // 撤销：恢复显示，服务器上没有删除
  await page.getByRole("button", { name: "撤销" }).click();
  await expect(page.getByRole("link", { name: "保护乙" })).toBeVisible();
  await page.waitForTimeout(11_000);
  expect((await b.call<{ id: string }>("GET", `/api/v1/profiles/${b.profile}`)).id).toBe(b.profile);

  // 不撤销：10 秒后真正删除
  await page.getByRole("checkbox", { name: "选择保护乙" }).check();
  await page.getByRole("toolbar", { name: "批量操作" }).getByRole("button", { name: "删除" }).click();
  await expect(page.getByRole("link", { name: "保护乙" })).toHaveCount(0);
  await expect.poll(async () => (await request.get(`/api/v1/profiles/${b.profile}`)).status(), { timeout: 20_000 }).toBe(404);
  await page.reload();
  await expect(page.getByRole("link", { name: "保护乙" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "保护甲" })).toBeVisible();
  expect(problems).toEqual([]);
});

test("导出配置：含节点时与订阅内容相同，不含节点时得到骨架并列出去掉的内容（R17）", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const problems = watch(page);
  const { profile } = await seedProfile(request, baseURL!, "导出测试");
  await login(page);
  await page.goto(`/profiles/${profile}`);
  await page.locator("#editor-main").waitFor();
  await page.getByText("更多", { exact: true }).click();
  await page.getByRole("button", { name: "导出配置" }).click();
  const dlg = page.getByRole("dialog", { name: "导出配置" });

  const full = page.waitForEvent("download");
  await dlg.getByRole("button", { name: "导出并下载" }).click();
  const f = await full;
  expect(f.suggestedFilename()).toBe("导出测试.yaml");
  const fullText = await (await f.createReadStream()).toArray().then((c) => Buffer.concat(c).toString("utf8"));
  expect(fullText).toContain("proxies:");
  expect(fullText).toContain("香港 01");

  await dlg.getByLabel(/含节点/).uncheck();
  await dlg.getByLabel(/含规则集/).uncheck();
  const bare = page.waitForEvent("download");
  await dlg.getByRole("button", { name: "导出并下载" }).click();
  const b = await bare;
  expect(b.suggestedFilename()).toBe("导出测试.skeleton.yaml");
  const text = await (await b.createReadStream()).toArray().then((c) => Buffer.concat(c).toString("utf8"));
  expect(text).not.toContain("香港 01");
  expect(text).not.toMatch(/^proxies:/m);
  await expect(dlg.getByText(/导出时不含节点：去掉了 2 个节点/)).toBeVisible();
  expect(problems).toEqual([]);
});

test("导入配置：预览将要创建的内容与去掉的设置，导入后节点名与策略组原样保留（R17）", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const problems = watch(page);
  await login(page);
  await page.goto("/");
  await page.getByRole("button", { name: "导入配置" }).click();
  const dlg = page.getByRole("dialog", { name: "导入配置" });
  await dlg.getByLabel("配置内容").fill([
    "mixed-port: 7890",
    "external-controller: 127.0.0.1:9090",
    "proxies:",
    "  - {name: 首尔 01, type: ss, server: 192.0.2.41, port: 8388, cipher: aes-128-gcm, password: example-pw}",
    "proxy-groups:",
    "  - {name: 导入的组, type: select, proxies: [首尔 01, DIRECT]}",
    "rules:",
    "  - MATCH,导入的组",
  ].join("\n"));
  await dlg.getByLabel("配置名称").fill("e2e 导入");
  await dlg.getByRole("button", { name: "预览" }).click();
  await expect(dlg.getByText("1个节点（一个手动节点源）")).toBeVisible();
  await expect(dlg.getByText("1个策略组")).toBeVisible();
  await expect(dlg.getByText(/external-controller 是面板或外部控制设置，没有导入/)).toBeVisible();
  await dlg.getByRole("button", { name: "导入", exact: true }).click();
  await expect(page).toHaveURL(/\/profiles\/[0-9a-f-]+$/);
  const id = page.url().split("/").pop()!;

  const call = await apiAs(request, baseURL!);
  const l = await call<{ url: string }>("POST", `/api/v1/profiles/${id}/links`, { note: "导入" });
  const y = await (await request.get(new URL(l.url).pathname)).text();
  expect(y).toContain("name: 首尔 01");
  expect(y).toContain("name: 导入的组");
  expect(y).toContain("MATCH,导入的组");
  expect(y).not.toContain("🇰🇷");
  expect(y).not.toContain("external-controller");
  expect(problems).toEqual([]);
});

test("备份与还原：下载加密备份，还原后整体回到备份时的状态，可以撤销（R21）", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const problems = watch(page);
  await login(page);
  await page.goto("/account");
  const box = page.locator("section", { has: page.getByRole("heading", { name: "备份与还原" }) });
  const pass = "e2e backup passphrase";
  await box.getByLabel("当前密码").first().fill(ADMIN.password);
  await box.getByLabel("备份口令", { exact: true }).first().fill(pass);
  await box.getByLabel("再输入一次备份口令").fill(pass);
  const dl = page.waitForEvent("download");
  await box.getByRole("button", { name: "下载备份" }).click();
  const file = await (await dl).path();
  await expect(page.getByText("备份已下载")).toBeVisible();

  // 备份之后新增的节点源：还原后应消失，撤销后应回来
  const call = await apiAs(request, baseURL!);
  const name = "还原后应消失 " + Date.now();
  await call("POST", "/api/v1/sources", { name, kind: "batch", content: "- {name: 测试 51, type: ss, server: 192.0.2.51, port: 8388, cipher: aes-128-gcm, password: example-pw}\n" });

  await box.getByLabel("备份文件").setInputFiles(file);
  await box.getByLabel("备份口令", { exact: true }).nth(1).fill(pass);
  await box.getByRole("button", { name: "检查备份" }).click();
  await expect(box.getByRole("table")).toContainText("节点源");
  await box.getByLabel(/我明白/).check();
  await box.getByLabel("当前密码").nth(1).fill(ADMIN.password);
  await box.getByRole("button", { name: "还原（整体替换）" }).click();
  await expect(box.getByRole("status")).toContainText("已从备份还原");
  const names = async () => ((await call<{ items: { name: string }[] }>("GET", "/api/v1/sources?limit=100")).items.map((s) => s.name));
  expect(await names()).not.toContain(name);

  await expect(box.getByRole("heading", { name: "撤销最近一次还原" })).toBeVisible();
  await box.getByLabel("当前密码").last().fill(ADMIN.password);
  await box.getByRole("button", { name: "撤销还原" }).click();
  await expect(page.getByText("已撤销还原")).toBeVisible();
  expect(await names()).toContain(name);
  expect(problems).toEqual([]);
});

test("注销账号：需要确认与当前密码，最后一个管理员不能注销（R20）", async ({ page }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const problems = watch(page);
  await login(page);
  await page.goto("/account");
  const box = page.locator("section", { has: page.getByRole("heading", { name: "注销账号" }) });
  await expect(box).toContainText("7天冷静期");
  await box.getByLabel("当前密码").fill(ADMIN.password);
  const run = box.getByRole("button", { name: "注销账号" });
  await expect(run).toBeDisabled(); // 未勾选确认
  await box.getByLabel(/我明白/).check();
  await run.click();
  await expect(page.getByText("这是最后一个管理员，不能注销。")).toBeVisible();
  await expect(page).toHaveURL(/\/account$/); // 仍然登录
  expect(problems.filter((p) => !p.includes("409"))).toEqual([]);
});

test("规则源编辑器：高亮编辑、策略组预览与带行号的问题（O2）", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const problems = watch(page);
  const { call } = await seedProfile(request, baseURL!, "规则预览");
  const groups = '[[custom_groups]]\nname = "代理"\ntype = "select"\nrule = ["[]DIRECT", ".*"]\n';
  const rulesets = '[[rulesets]]\ngroup = "代理"\nruleset = "[]MATCH"\n';
  const rs = await call<{ id: string }>("POST", "/api/v1/rule-sources", { name: "预览测试 " + Date.now(), kind: "toml", groupsText: groups, rulesetsText: rulesets });
  await login(page);
  await page.goto("/rules/" + rs.id);
  const pv = page.locator("section", { has: page.getByRole("heading", { name: "策略组预览" }) });
  await expect(pv.getByText(/1个策略组、1条规则/)).toBeVisible();
  await expect(pv.getByText("代理", { exact: true })).toBeVisible();
  await expect(pv.locator("li[data-kind=builtin]", { hasText: "DIRECT" })).toBeVisible();
  await expect(pv.locator("li[data-kind=node]").first()).toBeVisible();

  // 在 groups 编辑器末尾追加一个缺少 name 的组：问题列表指向它所在的第 5 行（保存的文本末尾没有换行）
  const editor = page.locator("#rd-gt .cm-content");
  await editor.click();
  await page.keyboard.press("Control+End");
  await page.keyboard.insertText('\n[[custom_groups]]\ntype = "select"\nrule = [".*"]\n');
  await expect(pv.getByRole("list", { name: "问题" })).toContainText("groups.toml:5");
  await pv.getByRole("list", { name: "问题" }).getByRole("button").first().click();
  await expect(page.locator("#rd-gt .cm-activeLine")).toContainText("[[custom_groups]]");
  expect(problems).toEqual([]);
});

test("运行指标与后台任务：管理页列出任务，/metrics 在本机可读（R26、R36）", async ({ page, request }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const problems = watch(page);
  const m = await request.get("/metrics");
  expect(m.status()).toBe(200);
  const text = await m.text();
  expect(text).toContain("clash_launch_http_requests_total");
  expect(text).toContain('clash_launch_job_runs_total{job="fetch"}');
  expect((await request.get("/metrics", { headers: { "X-Forwarded-For": "203.0.113.9" } })).status()).toBe(404);
  await login(page);
  await page.goto("/admin");
  const box = page.locator("section", { has: page.getByRole("heading", { name: "后台任务" }) });
  await expect(box.getByRole("row", { name: /拉取订阅与规则源/ })).toBeVisible();
  await expect(box.getByRole("row", { name: /清理审计日志/ })).toBeVisible();
  expect(problems).toEqual([]);
});

test("数据保留与订阅分析导出：设置保留期、统计数据量、导出 CSV（R43、R39）", async ({ page }) => {
  test.skip(test.info().project.name === "mobile", "流程与视口无关，只在桌面运行一次");
  const problems = watch(page);
  await login(page);
  await page.goto("/admin");
  const box = page.locator("section", { has: page.getByRole("heading", { name: "数据保留" }) });
  await box.getByLabel("审计日志保留天数").fill("365");
  await box.getByLabel("订阅请求明细保留天数").fill("60");
  await box.getByRole("button", { name: "保存" }).click();
  await expect(box.getByText(/已在网页上设置/).first()).toBeVisible();
  await box.getByRole("button", { name: "统计" }).click();
  await expect(box.getByRole("cell", { name: "users", exact: true })).toBeVisible();
  await box.getByRole("button", { name: "恢复为环境变量" }).click();
  await expect(box.getByText(/当前沿用环境变量/).first()).toBeVisible();

  await page.goto("/analytics");
  const dl = page.waitForEvent("download");
  await page.getByRole("link", { name: "导出明细" }).click();
  const f = await dl;
  expect(f.suggestedFilename()).toMatch(/^subscription-requests-\d{8}-\d{8}\.csv$/);
  const text = await (await f.createReadStream()).toArray().then((c) => Buffer.concat(c).toString("utf8"));
  expect(text.replace(/^﻿/, "").split("\n")[0]).toBe("time,config,link,short_link,friend,client,ip,country,asn,org,status,kind");
  expect(problems).toEqual([]);
});

test("命令面板搜数据与快捷键一览：搜到节点并定位到所在行，按 ? 打开快捷键（R34、R45）", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "键盘快捷键只在桌面测试");
  const problems = watch(page);
  const call = await apiAs(request, baseURL!);
  const tag = "R34" + Date.now().toString(36);
  const src = await call<{ id: string }>("POST", "/api/v1/sources", {
    name: "搜索测试 " + tag,
    kind: "batch",
    content: `- {name: 定位节点 ${tag}, type: ss, server: 192.0.2.10, port: 8388, cipher: aes-128-gcm, password: example-pw}\n`,
  });
  await login(page);
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "配置" })).toBeVisible();
  // 没有输入时不列出数据，只有页面与命令
  await page.keyboard.press("Control+K");
  await expect(page.locator("dialog.palette input")).toBeFocused();
  await expect(page.getByRole("option", { name: new RegExp(tag) })).toHaveCount(0);
  await page.keyboard.type("定位节点 " + tag);
  const opt = page.getByRole("option", { name: new RegExp("定位节点 " + tag) });
  await expect(opt).toContainText("节点");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL((u) => u.pathname === "/sources/" + src.id && u.searchParams.has("node"));
  await expect(page.locator("tr.hit")).toContainText("定位节点 " + tag);
  await expect(page.locator("tr.hit")).toBeInViewport();

  // 节点源本身也能搜到
  await page.keyboard.press("Control+K");
  await expect(page.locator("dialog.palette input")).toBeFocused();
  await page.keyboard.type("搜索测试 " + tag);
  // 节点也会因所在节点源的名称命中；节点源本身排在前面
  await expect(page.locator("#palette-list [role=option]").first()).toContainText("节点源");
  await page.screenshot({ path: "test-results/shot-palette-data.png" });
  await page.keyboard.press("Escape");

  // ?：快捷键一览；在输入框中照常输入问号
  await page.locator("body").click({ position: { x: 5, y: 300 } });
  await page.keyboard.press("?");
  const dlg = page.getByRole("dialog", { name: "快捷键" });
  await expect(dlg).toBeVisible();
  await expect(dlg.getByText("打开命令面板")).toBeVisible();
  await page.screenshot({ path: "test-results/shot-shortcuts.png" });
  await page.keyboard.press("Escape");
  await expect(dlg).toBeHidden();
  const name = page.locator("#sd-name");
  await name.focus();
  await page.keyboard.press("?");
  await expect(dlg).toBeHidden();
  expect(problems).toEqual([]);
});

test("浏览器返回：编辑器分节逐个返回；未保存时确认；保存后返回不回到详情；滚动位置还原（R3）", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "历史记录与视口无关，只在桌面运行一次");
  const problems = watch(page);
  const { profile, source } = await seedProfile(request, baseURL!, "返回测试 " + Date.now().toString(36));
  const srcURL = (u: URL) => u.pathname === "/sources/" + source;
  await login(page);

  // 编辑器：切换分节进入历史，返回回到上一个分节，再返回离开编辑器
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "配置" })).toBeVisible();
  await page.goto("/profiles/" + profile);
  const secnav = page.locator("nav.secnav");
  await secnav.getByRole("button", { name: "分流", exact: true }).click();
  await expect(page).toHaveURL(/section=routing/);
  await secnav.locator("button").nth(3).click();
  await expect(page).not.toHaveURL(/section=routing/);
  await page.goBack();
  await expect(page).toHaveURL(/section=routing/);
  await expect(secnav.getByRole("button", { name: "分流", exact: true })).toHaveAttribute("aria-current", "true");
  await page.goBack();
  await expect(secnav.getByRole("button", { name: /节点与分流/ })).toHaveAttribute("aria-current", "true");
  await page.goBack();
  await expect(page).toHaveURL((u) => u.pathname === "/");

  // 节点源：未保存时离开需要确认；取消则留在原页（站内链接与浏览器返回都一样）
  await page.getByRole("link", { name: "节点", exact: true }).first().click();
  await expect(page).toHaveURL((u) => u.pathname === "/sources");
  await page.locator('a[href="/sources/' + source + '"]').first().click();
  await expect(page.locator("#sd-name")).toHaveValue(/返回测试/);
  await page.locator("#sd-name").fill("返回测试 改名");
  let asked = 0;
  page.on("dialog", (d) => {
    asked++;
    void d.dismiss();
  });
  await page.getByRole("link", { name: "分流规则" }).first().click();
  await expect(page).toHaveURL(srcURL);
  // 浏览器返回：拒绝后撤销这次返回，仍在详情页
  await page.evaluate(() => history.back());
  await expect.poll(() => asked).toBe(2);
  await expect(page).toHaveURL(srcURL);
  await expect(page.locator("#sd-name")).toHaveValue("返回测试 改名");
  page.removeAllListeners("dialog");

  // 保存后回到列表：上一页就是列表时后退一步；再返回到列表之前的页面，不回到详情
  await page.getByRole("button", { name: "保存" }).click();
  await expect(page).toHaveURL((u) => u.pathname === "/sources");
  await page.goBack();
  await expect(page).toHaveURL((u) => u.pathname === "/");
  await page.goForward();
  await expect(page).toHaveURL((u) => u.pathname === "/sources");

  // 滚动位置：离开后返回，回到原来的位置
  await page.getByRole("link", { name: "帮助", exact: true }).first().click();
  await page.getByRole("tab", { name: /百科/ }).click();
  await expect(page).toHaveURL(/tab=wiki/);
  await page.waitForLoadState("networkidle");
  const y = await page.evaluate(() => {
    const m = document.getElementById("main")!;
    const el = m.scrollHeight > m.clientHeight + 10 ? m : document.scrollingElement!;
    el.scrollTop = 1200;
    return el.scrollTop;
  });
  expect(y).toBeGreaterThan(600);
  await page.getByRole("link", { name: "节点", exact: true }).first().click();
  await expect(page).toHaveURL((u) => u.pathname === "/sources");
  await page.goBack();
  await expect(page).toHaveURL(/tab=wiki/);
  await expect.poll(() => page.evaluate(() => Math.max(document.getElementById("main")!.scrollTop, document.scrollingElement!.scrollTop))).toBeGreaterThan(y - 10);
  // 帮助的标签也在历史中
  await page.goBack();
  await expect(page).not.toHaveURL(/tab=wiki/);
  expect(problems).toEqual([]);
});
