// 强调色（需求 R28）：几种预设，按钮、链接、焦点环、指示灯与选中态随之变化。
// 色值在 styles/accents.css；这里只列出名称（设置页的选项与偏好的合法值）。默认 indigo 即 tokens.css 中的原值。
// 分流图的流向色（经代理、直连、拦截）是独立的语义色（决定 0007 #13），不随强调色变化：
// 直连本身是青色，「经代理」若跟着变成青色或绿色会与直连混淆。

export const ACCENTS = ["indigo", "blue", "teal", "green", "amber", "rose", "violet"] as const;
export type Accent = (typeof ACCENTS)[number];

/** 设置页色块的预览色（与 accents.css 中浅色主题的 --accent 一致）。 */
export const SWATCH: Record<Accent, string> = {
  indigo: "#4f4bc6",
  blue: "#1d5fd0",
  teal: "#0f766e",
  green: "#2e7a3b",
  amber: "#a8510b",
  rose: "#be123c",
  violet: "#7c3aed",
};

const KEY = "cl.accent";

/** 把强调色应用到根元素；indigo（默认）不设属性。同时记在本机，下次打开时在加载偏好前就生效，避免闪一下默认色。 */
export function applyAccent(a: string | undefined): void {
  const v = (ACCENTS as readonly string[]).includes(a ?? "") && a !== "indigo" ? a : undefined;
  if (v) document.documentElement.dataset.accent = v;
  else delete document.documentElement.dataset.accent;
  try {
    if (v) localStorage.setItem(KEY, v);
    else localStorage.removeItem(KEY);
  } catch {
    // 无痕模式等不能写入：只是下次打开时先显示默认色
  }
}

/** 启动时（main.ts）按本机记下的值先应用。 */
export function applyStoredAccent(): void {
  try {
    const v = localStorage.getItem(KEY);
    if (v && (ACCENTS as readonly string[]).includes(v)) document.documentElement.dataset.accent = v;
  } catch {
    // 同上
  }
}
