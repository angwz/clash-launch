// 把「字段路径 + 值」写成一小段 YAML，供说明面板展示写法。只处理目录里出现的值形状：
// 标量、标量列表、映射（流式）。字符串在可能被 YAML 误读时加双引号。

const plain = /^[A-Za-z0-9_./+-][A-Za-z0-9_./+:@-]*$/;
// 会被 YAML 读成布尔、空值或数字的字符串（IP 地址有多个点，不在其中）
const reserved = /^(true|false|yes|no|on|off|null|~|[-+]?\d[\d_]*(\.\d+)?([eE][-+]?\d+)?|[-+]?\.\d+|0x[0-9a-f]+|0o[0-7]+)$/i;

function scalar(v: unknown): string {
  if (typeof v === "string") return plain.test(v) && !reserved.test(v) && !v.includes(": ") ? v : JSON.stringify(v);
  if (v === null || v === undefined) return "null";
  if (typeof v === "object") return JSON.stringify(v).replace(/"([A-Za-z0-9_-]+)":/g, "$1: ").replace(/,/g, ", ");
  return String(v);
}

export function yamlSnippet(path: string[], value: unknown): string {
  const lines: string[] = [];
  path.forEach((k, i) => {
    const pad = "  ".repeat(i);
    if (i < path.length - 1) {
      lines.push(pad + k + ":");
      return;
    }
    if (Array.isArray(value) && value.length && value.every((x) => typeof x !== "object")) {
      lines.push(pad + k + ":");
      for (const x of value) lines.push(pad + "  - " + scalar(x));
    } else {
      lines.push(pad + k + ": " + scalar(value));
    }
  });
  return lines.join("\n");
}
