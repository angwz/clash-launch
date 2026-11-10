// 口令策略的前端即时判断，与后端 auth.CheckPassword 一致（常见口令表只在后端检查）：
// 至少 5 个字符、至少 2 类字符（ASCII 小写字母、ASCII 大写字母、数字、特殊字符）；
// 少于 8 个字符时，其中一类必须是特殊字符。
export const MIN_PASSWORD_LEN = 5;
export const MIN_PASSWORD_CLASSES = 2;
export const LONG_PASSWORD_LEN = 8;

function kind(c: string): "lower" | "upper" | "digit" | "special" {
  if (c >= "a" && c <= "z") return "lower";
  if (c >= "A" && c <= "Z") return "upper";
  if (c >= "0" && c <= "9") return "digit";
  return "special";
}

export function passwordClasses(s: string): number {
  return new Set([...s].map(kind)).size;
}

export function passwordOk(s: string): boolean {
  const chars = [...s];
  if (chars.length < MIN_PASSWORD_LEN || passwordClasses(s) < MIN_PASSWORD_CLASSES) return false;
  return chars.length >= LONG_PASSWORD_LEN || chars.some((c) => kind(c) === "special");
}
