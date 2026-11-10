// Svelte 过渡的时长：系统要求减少动画时缩短而不取消（与 tokens.css 的约定一致）。
const reduced = typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;

export function dur(ms: number): number {
  return reduced ? Math.min(ms, 120) : ms;
}
