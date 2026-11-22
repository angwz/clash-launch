// 生成安装为应用（R29）所需的 PNG 图标：与 favicon.svg 同一图形。
// any：圆角方块；maskable：满铺底色，图形缩在中间 60%（系统裁成圆形、圆角方等形状时不被裁掉）。
import { chromium } from "@playwright/test";
import { writeFileSync } from "node:fs";

const rocket = `<path d="M16 5.5c3.6 2.6 5.5 6.6 5.5 11v4.2h-11v-4.2c0-4.4 1.9-8.4 5.5-11Z" fill="#fff"/><circle cx="16" cy="14.4" r="2" fill="#4f4bc6"/><path d="M13.6 23.2c.5 1.5 1.3 2.6 2.4 3.3 1.1-.7 1.9-1.8 2.4-3.3" fill="none" stroke="#fff" stroke-width="1.6" stroke-linecap="round"/>`;
const any = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#4f4bc6"/>${rocket}</svg>`;
// apple-touch-icon：系统自己加圆角，图形满铺方块
const apple = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" fill="#4f4bc6"/><g transform="translate(2.4 2.4) scale(0.85)">${rocket}</g></svg>`;
const maskable = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" fill="#4f4bc6"/><g transform="translate(6.4 6.4) scale(0.6)">${rocket}</g></svg>`;

// 用法：npm run icons（输出到 public/icons；用本机的 Edge，与 e2e 相同，不下载浏览器）
const out = process.argv[2] ?? "public/icons";
const browser = await chromium.launch({ channel: "msedge" });
const page = await browser.newPage();
for (const [name, svg, size] of [["icon-192.png", any, 192], ["icon-512.png", any, 512], ["maskable-512.png", maskable, 512], ["apple-touch-icon.png", apple, 180]]) {
  await page.setViewportSize({ width: size, height: size });
  await page.setContent(`<html><body style="margin:0;background:transparent">${svg.replace("<svg ", `<svg width="${size}" height="${size}" `)}</body></html>`);
  const buf = await page.locator("svg").screenshot({ omitBackground: true });
  writeFileSync(`${out}/${name}`, buf);
}
await browser.close();
