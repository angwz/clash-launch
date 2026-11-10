// 合并消息：node scripts/merge-messages.mjs <补丁.json>
// 补丁格式：{ "zh-CN": {键: 文字}, "en": {键: 文字}, "remove": [键] }。结果按键排序写回，便于审阅差异。
import { readFileSync, writeFileSync } from "node:fs";

const patch = JSON.parse(readFileSync(process.argv[2], "utf8"));
for (const lang of ["zh-CN", "en"]) {
  const f = `src/lib/i18n/messages/${lang}.json`;
  const m = JSON.parse(readFileSync(f, "utf8"));
  Object.assign(m, patch[lang] ?? {});
  for (const k of patch.remove ?? []) delete m[k];
  const sorted = Object.fromEntries(Object.keys(m).sort().map((k) => [k, m[k]]));
  writeFileSync(f, JSON.stringify(sorted, null, 2) + "\n");
}
