// 在渲染出的 YAML 文本中找到某个字段路径（如 "dns.enhanced-mode"）占据的行范围，供预览高亮。
// 只处理本项目渲染器的输出形状：块式映射、两格缩进、键不带引号或带引号均可；
// 路径中途遇到列表或流式写法就停止，返回已找到的最深一层（exact=false）。
// 复杂度 O(行数)：每一层只在父范围内向下扫描一次。

export interface LineRange {
  start: number; // 含
  end: number; // 不含
  exact: boolean; // 是否精确找到整条路径
}

const indentOf = (s: string) => s.length - s.trimStart().length;
const blank = (s: string) => s.trim() === "" || s.trimStart().startsWith("#");

function keyOf(line: string): string | null {
  const m = /^\s*("([^"]*)"|'([^']*)'|[^\s#'"][^:]*?):(\s|$)/.exec(line);
  if (!m) return null;
  return m[2] ?? m[3] ?? m[1]!;
}

export function yamlRange(lines: string[], path: string): LineRange | null {
  const segs = path.split(".").filter(Boolean);
  if (!segs.length) return null;
  let lo = 0;
  let hi = lines.length;
  let indent = 0;
  let found: LineRange | null = null;
  for (const seg of segs) {
    let hit = -1;
    for (let i = lo; i < hi; i++) {
      const l = lines[i]!;
      if (blank(l) || indentOf(l) !== indent) continue;
      if (keyOf(l) === seg) {
        hit = i;
        break;
      }
    }
    if (hit < 0) return found && { ...found, exact: false };
    // 范围延伸到下一个缩进不大于本层的非空行；末尾的空行不计入
    let end = hit + 1;
    while (end < hi && (blank(lines[end]!) || indentOf(lines[end]!) > indent)) end++;
    while (end > hit + 1 && blank(lines[end - 1]!)) end--;
    found = { start: hit, end, exact: true };
    // 下一层的缩进取第一个子行；没有子行（标量值）时后续段必然找不到
    const child = lines.slice(hit + 1, end).find((l) => !blank(l));
    if (!child || child.trimStart().startsWith("- ")) {
      if (seg !== segs[segs.length - 1]) return { ...found, exact: false };
      break;
    }
    lo = hit + 1;
    hi = end;
    indent = indentOf(child);
  }
  return found;
}
