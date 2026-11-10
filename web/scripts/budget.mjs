// 首屏 JavaScript 预算（第 15.4 节）：gzip 后默认 150 KiB。
// 首屏 = 入口及其静态依赖 + 较大的一个语言包 + 登录页（未登录用户的首屏）及其静态依赖。
// 按需加载的页面、CodeMirror 与 yaml 库不计入。用法：npm run build && npm run budget
import { readFileSync } from "node:fs";
import { gzipSync } from "node:zlib";

const LIMIT = 150 * 1024;
const manifest = JSON.parse(readFileSync("build-manifest.json", "utf8"));

const seen = new Set();
function collect(key) {
  const e = manifest[key];
  if (!e || seen.has(e.file)) return;
  seen.add(e.file);
  for (const i of e.imports ?? []) collect(i);
}

const entry = Object.keys(manifest).find((k) => manifest[k].isEntry);
collect(entry);
collect("src/pages/Login.svelte");

const gz = (file) => gzipSync(readFileSync("dist/" + file), { level: 9 }).length;
const langs = ["src/lib/i18n/messages/en.json", "src/lib/i18n/messages/zh-CN.json"].map((k) => manifest[k]?.file).filter(Boolean);
const langMax = Math.max(...langs.map(gz));

let total = langMax;
const rows = [];
for (const f of [...seen].filter((f) => f.endsWith(".js"))) {
  const n = gz(f);
  total += n;
  rows.push([f, n]);
}
rows.sort((a, b) => b[1] - a[1]);
for (const [f, n] of rows) console.log(`${(n / 1024).toFixed(1).padStart(7)} KiB  ${f}`);
console.log(`${(langMax / 1024).toFixed(1).padStart(7)} KiB  （语言包，取较大者）`);
console.log(`${(total / 1024).toFixed(1).padStart(7)} KiB  首屏合计（预算 ${LIMIT / 1024} KiB）`);
if (total > LIMIT) {
  console.error("超出首屏预算");
  process.exit(1);
}
