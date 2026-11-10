// 口令策略的前端即时判断，与后端 auth.CheckPassword 一致（常见口令表只在后端检查）：
// 至少 5 个字符，至少 2 类字符（ASCII 小写字母、ASCII 大写字母、数字、其他字符）。
export const MIN_PASSWORD_LEN = 5;
export const MIN_PASSWORD_CLASSES = 2;

export function passwordClasses(s: string): number {
  let lower = false;
  let upper = false;
  let digit = false;
  let other = false;
  for (const c of s) {
    if (c >= "a" && c <= "z") lower = true;
    else if (c >= "A" && c <= "Z") upper = true;
    else if (c >= "0" && c <= "9") digit = true;
    else other = true;
  }
  return [lower, upper, digit, other].filter(Boolean).length;
}

export function passwordOk(s: string): boolean {
  return [...s].length >= MIN_PASSWORD_LEN && passwordClasses(s) >= MIN_PASSWORD_CLASSES;
}
