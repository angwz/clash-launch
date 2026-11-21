// 动效偏好：系统的「减少动态效果」，或账户设置里关闭动效（R11，根元素 data-motion="off"，由 AppShell 按偏好设置）。
// 关闭动效时 base.css 把所有动画与过渡压到瞬时；这里供脚本驱动的动效（Svelte 过渡、分流地图粒子）查询。
const systemReduced = typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;

/** 是否应减少动效。每次调用重新读取根元素，偏好改变后立即生效。 */
export function reducedMotion(): boolean {
  return systemReduced || (typeof document !== "undefined" && document.documentElement.dataset.motion === "off");
}

/** Svelte 过渡的时长：减少动效时缩短而不取消（与 tokens.css 的约定一致）。 */
export function dur(ms: number): number {
  return reducedMotion() ? Math.min(ms, 120) : ms;
}
