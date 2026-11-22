// 站内消息端到端加密（R46）的加密库：包裹与解开、恢复密钥、双方都能解开、换了密钥后解不开。
// 只测纯加密部分（IndexedDB 的保存由 e2e 测试覆盖）。
import { describe, expect, it } from "vitest";
import * as e2e from "./e2e";

describe("e2e", () => {
  it("密码包裹：正确的密码解开，错误的密码失败", async () => {
    const { priv } = await e2e.generateKeyPair();
    const w = await e2e.wrapWithPassword(priv, "correct horse battery");
    expect(w.iter).toBe(e2e.PBKDF2_ITER);
    await expect(e2e.unwrapWithPassword(w, "correct horse battery")).resolves.toBeTruthy();
    await expect(e2e.unwrapWithPassword(w, "wrong password")).rejects.toThrow();
    const k = await e2e.unwrapWithPassword(w, "correct horse battery");
    expect(k.extractable).toBe(false); // 默认不可导出
  }, 20_000);

  it("恢复密钥：显示形式往返一致，容忍小写、空格与连字符", async () => {
    const { code, bytes } = e2e.newRecoveryCode();
    expect(code).toMatch(/^([A-Z2-9]{4}-){12}[A-Z2-9]{4}$/);
    expect(e2e.parseRecoveryCode(code.toLowerCase().replace(/-/g, " "))).toEqual(bytes);
    expect(e2e.parseRecoveryCode("not a code")).toBeNull();
    const { priv } = await e2e.generateKeyPair();
    const w = await e2e.wrapWithRecovery(priv, bytes);
    await expect(e2e.unwrapWithRecovery(w, bytes)).resolves.toBeTruthy();
    await expect(e2e.unwrapWithRecovery(w, e2e.newRecoveryCode().bytes)).rejects.toThrow();
  });

  it("消息：收发双方都能解开；密文不含原文；密钥编号对不上时失败", async () => {
    const a = await e2e.generateKeyPair();
    const b = await e2e.generateKeyPair();
    const aPub = await e2e.importPublic(a.publicKey);
    const bPub = await e2e.importPublic(b.publicKey);
    const text = "今晚 8 点见 🚀";
    const m = await e2e.encryptText(text, { keyId: "ka", priv: a.priv }, { keyId: "kb", pub: bPub });
    expect(m.ct).not.toContain(e2e.b64(new TextEncoder().encode("今晚")));
    const peerOf = (pub: CryptoKey, kid: string) => async (k: string) => (k === kid ? pub : null);
    await expect(e2e.decryptText(m, true, { keyId: "ka", priv: a.priv }, peerOf(bPub, "kb"))).resolves.toBe(text);
    await expect(e2e.decryptText(m, false, { keyId: "kb", priv: b.priv }, peerOf(aPub, "ka"))).resolves.toBe(text);
    // 收件人换了密钥：旧消息解不开（预期行为）
    const b2 = await e2e.generateKeyPair();
    await expect(e2e.decryptText(m, false, { keyId: "kb2", priv: b2.priv }, peerOf(aPub, "ka"))).rejects.toThrow();
    // 密文被改动：认证失败
    const tampered = { ...m, ct: m.ct.slice(0, -2) + (m.ct.endsWith("AA") ? "BB" : "AA") };
    await expect(e2e.decryptText(tampered, true, { keyId: "ka", priv: a.priv }, peerOf(bPub, "kb"))).rejects.toThrow();
  });
});
