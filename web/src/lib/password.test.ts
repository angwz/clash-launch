import { describe, expect, it } from "vitest";
import { passwordOk } from "./password";

// 与 internal/auth 的 TestCheckPassword 保持同样的边界
describe("口令策略", () => {
  it("拒绝", () => {
    for (const pw of ["ab1", "abcde", "12345", "abc12", "Abcde12", "abcdefgh", "一段中文口令"]) expect(passwordOk(pw), pw).toBe(false);
  });
  it("接受", () => {
    for (const pw of ["ab-12", "Ab-cd", "abcd1234", "Abcdefgh", "correct horse battery", "中文口令2026"]) expect(passwordOk(pw), pw).toBe(true);
  });
});
