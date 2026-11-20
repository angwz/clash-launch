import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { login, seedProfile } from "./helpers";

// 无障碍自动检查（R30）：主要页面在亮色与暗色下按 WCAG 2.1 A/AA 规则检查，问题清零。
// axe 只能发现一部分无障碍问题（键盘操作、读屏顺序、文字是否达意仍需人工检查），这里守住能自动发现的部分。
const TAGS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"];

test("无障碍：主要页面没有 axe 能发现的 WCAG 2.1 A/AA 问题", async ({ page, request, baseURL }) => {
  test.skip(test.info().project.name === "mobile", "桌面已覆盖全部页面；手机布局的差异由其他 e2e 检查");
  test.setTimeout(240_000);
  const { profile, source } = await seedProfile(request, baseURL!, "无障碍");
  const anon = ["/login", "/register"];
  const authed = ["/", `/profiles/${profile}`, `/profiles/${profile}?section=dns`, `/profiles/${profile}/links`, `/profiles/${profile}/history`, `/sources`, `/sources/${source}`, "/rules", "/short-links", "/analytics", "/settings", "/account", "/admin", "/help", "/help?tab=wiki"];
  const found: string[] = [];
  const scan = async (path: string, scheme: "light" | "dark") => {
    await page.emulateMedia({ colorScheme: scheme });
    await page.goto(path);
    await page.waitForLoadState("networkidle");
    const r = await new AxeBuilder({ page }).withTags(TAGS).analyze();
    for (const v of r.violations) {
      for (const n of v.nodes) {
        found.push(`${scheme} ${path} · ${v.id} · ${n.target.join(" ")} · ${n.failureSummary?.split("\n")[1]?.trim() ?? ""}`);
      }
    }
  };
  for (const scheme of ["light", "dark"] as const) for (const p of anon) await scan(p, scheme);
  await login(page);
  for (const scheme of ["light", "dark"] as const) for (const p of authed) await scan(p, scheme);
  expect(found, found.join("\n")).toEqual([]);
});
