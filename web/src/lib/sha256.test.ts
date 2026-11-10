import { createHash } from "node:crypto";
import { describe, expect, it } from "vitest";
import { leadingZeroBits, sha256, solve } from "./sha256";

const hex = (h: Uint32Array) => [...h].map((x) => (x >>> 0).toString(16).padStart(8, "0")).join("");

describe("sha256", () => {
  it("与 node:crypto 一致（覆盖 0–200 字节，含跨块边界 55/56/63/64）", () => {
    const out = new Uint32Array(8);
    for (let n = 0; n <= 200; n++) {
      const msg = new Uint8Array(n).map((_, i) => (i * 31 + n) & 0xff);
      sha256(msg, n, out);
      expect(hex(out), String(n)).toBe(createHash("sha256").update(msg).digest("hex"));
    }
  });

  it("求解结果满足难度，且与后端的验证方式一致", () => {
    const ch = "eyJ0ZXN0IjoxfQ.sig";
    const nonce = solve(ch, 10);
    const h = createHash("sha256").update(ch + ":" + nonce).digest();
    const out = new Uint32Array(8);
    for (let i = 0; i < 8; i++) out[i] = h.readUInt32BE(i * 4);
    expect(leadingZeroBits(out)).toBeGreaterThanOrEqual(10);
  });
});
