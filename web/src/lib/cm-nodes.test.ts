import { describe, expect, it } from "vitest";
import { contextAt, looksLikeYaml, yamlErrors } from "./cm-nodes";

// 节点编辑器的上下文推断（反馈 #114）：光标处是键还是值、所在的嵌套路径与节点的 type
const at = (s: string) => contextAt(s.replace("|", ""), s.indexOf("|"));

describe("contextAt", () => {
  it("行内花括号：键与值", () => {
    expect(at("proxies:\n  - { name: A, type: ss, ci|")).toMatchObject({ mode: "key", word: "ci", type: "ss", path: [], present: ["name", "type"] });
    expect(at("proxies:\n  - { name: A, type: ss, cipher: aes|")).toMatchObject({ mode: "value", key: "cipher", word: "aes", type: "ss", path: [] });
    expect(at("proxies:\n  - { name: A, type: |")).toMatchObject({ mode: "value", key: "type", word: "" });
  });

  it("行内花括号：嵌套映射", () => {
    expect(at("  - { name: A, type: vmess, ws-opts: { pa|")).toMatchObject({ mode: "key", path: ["ws-opts"], word: "pa", type: "vmess" });
    expect(at("  - { name: A, type: vmess, ws-opts: { path: /x }, net|")).toMatchObject({ mode: "key", path: [], word: "net" });
  });

  it("type 写在光标之后也能读出", () => {
    expect(contextAt("  - { name: A, ci, type: trojan }", 17)).toMatchObject({ mode: "key", type: "trojan" });
  });

  it("块写法：键、值与按缩进的父键", () => {
    const doc = "proxies:\n  - name: A\n    type: vless\n    reality-opts:\n      pub|";
    expect(at(doc)).toMatchObject({ mode: "key", path: ["reality-opts"], word: "pub", type: "vless" });
    expect(at("proxies:\n  - name: A\n    type: ss\n    cipher: ch|")).toMatchObject({ mode: "value", key: "cipher", word: "ch", path: [] });
    expect(at("proxies:\n  - name: A\n    type: ss\n    pa|")).toMatchObject({ mode: "key", path: [], present: ["name", "type"] });
  });

  it("字符串里与列表外不提示", () => {
    expect(at('  - { name: "A, b|')).toBeNull();
    expect(at("proxies:\n  mixed|")).toBeNull();
  });
});

describe("yaml 检查", () => {
  it("分享链接与 base64 不当作 YAML", () => {
    expect(looksLikeYaml("ss://YWVz@192.0.2.1:8388#A\n")).toBe(false);
    expect(looksLikeYaml("proxies:\n  - {name: A}\n")).toBe(true);
    expect(yamlErrors("ss://YWVz@192.0.2.1:8388#A\n")).toEqual([]);
  });
  it("语法错误给出位置", () => {
    const e = yamlErrors("proxies:\n  - { name: A, type: ss\n");
    expect(e.length).toBeGreaterThan(0);
  });
});
