// 逐行差异（最长公共子序列）。只用于版本历史中比较两份方案文档（显示用途，不参与语义）。
// 行数乘积超过上限时退化为「整体替换」，避免大文档占用过多内存。
export interface DiffLine {
  op: " " | "-" | "+";
  text: string;
}

const LIMIT = 4_000_000;

export function lineDiff(a: string, b: string): DiffLine[] {
  const x = a.split("\n");
  const y = b.split("\n");
  // 去掉公共前后缀，缩小 LCS 规模
  let pre = 0;
  while (pre < x.length && pre < y.length && x[pre] === y[pre]) pre++;
  let suf = 0;
  while (suf < x.length - pre && suf < y.length - pre && x[x.length - 1 - suf] === y[y.length - 1 - suf]) suf++;
  const xs = x.slice(pre, x.length - suf);
  const ys = y.slice(pre, y.length - suf);
  const out: DiffLine[] = x.slice(0, pre).map((text) => ({ op: " ", text }));
  if (xs.length * ys.length > LIMIT) {
    out.push(...xs.map((text) => ({ op: "-" as const, text })), ...ys.map((text) => ({ op: "+" as const, text })));
  } else {
    const n = xs.length;
    const m = ys.length;
    const dp: Uint32Array[] = Array.from({ length: n + 1 }, () => new Uint32Array(m + 1));
    for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) dp[i]![j] = xs[i] === ys[j] ? dp[i + 1]![j + 1]! + 1 : Math.max(dp[i + 1]![j]!, dp[i]![j + 1]!);
    let i = 0;
    let j = 0;
    while (i < n && j < m) {
      if (xs[i] === ys[j]) {
        out.push({ op: " ", text: xs[i]! });
        i++;
        j++;
      } else if (dp[i + 1]![j]! >= dp[i]![j + 1]!) out.push({ op: "-", text: xs[i++]! });
      else out.push({ op: "+", text: ys[j++]! });
    }
    while (i < n) out.push({ op: "-", text: xs[i++]! });
    while (j < m) out.push({ op: "+", text: ys[j++]! });
  }
  out.push(...x.slice(x.length - suf).map((text) => ({ op: " " as const, text })));
  return out;
}
