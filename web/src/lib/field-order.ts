// 设置项按热度排序（反馈 #68）。热度来自服务端每周重算一次的快照，所以两次重算之间顺序不变。
// 规则：
//   - 一组字段的热度合计不足 minTotal 时保持默认顺序（数据太少时「热度」没有意义）；
//   - 否则按热度从高到低稳定排序，热度相同保持原顺序；
//   - 对象字段的热度是它和全部子字段之和。
// 总开关不在这里处理：调用方先把它取出，固定放在最前。

/** 某个路径的热度：自身与子路径之和。 */
export function heat(path: string, scores: Record<string, number>): number {
  let n = 0;
  for (const [k, v] of Object.entries(scores)) {
    if (k === path || k.startsWith(path + ".")) n += v;
  }
  return n;
}

export function byHeat<T>(items: readonly T[], pathOf: (x: T) => string, scores: Record<string, number>, minTotal = 30): T[] {
  const h = items.map((x, i) => ({ x, i, n: heat(pathOf(x), scores) }));
  const total = h.reduce((a, b) => a + b.n, 0);
  if (total < minTotal) return [...items];
  return h.sort((a, b) => b.n - a.n || a.i - b.i).map((e) => e.x);
}
