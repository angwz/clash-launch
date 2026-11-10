import { describe, expect, it } from "vitest";
import { ProfileDoc } from "./profile-doc";

const sample = `clash-launch/profile: v1
name: 测试
target: { pack: mihomo, preset: mihomo, core: "1.19.33" }
sources:
  - { ref: src_a }
  - { ref: src_b, mode: provider }
form:
  mixed-port: 7890   # 常用端口
  dns:
    enable: true
raw: |
  # 原始编辑：注释保留
  rules: !prepend
    - DOMAIN,intranet.example.com,DIRECT
`;

describe("ProfileDoc", () => {
  it("读取基本信息", () => {
    const d = new ProfileDoc(sample);
    expect(d.name).toBe("测试");
    expect(d.target).toEqual({ pack: "mihomo", preset: "mihomo", core: "1.19.33" });
    expect(d.sources).toEqual([{ ref: "src_a", mode: undefined, disabled: undefined }, { ref: "src_b", mode: "provider", disabled: undefined }]);
    expect(d.getForm(["dns", "enable"])).toBe(true);
    expect(d.getForm(["dns", "ipv6"])).toBeUndefined();
    expect(d.formPaths()).toEqual([["mixed-port"], ["dns", "enable"]]);
  });

  it("未改动时原样输出；改动后注释与 raw 块保留", () => {
    expect(new ProfileDoc(sample).toString()).toBe(sample);
    const d = new ProfileDoc(sample);
    d.setForm(["log-level"], "info");
    const out = d.toString();
    expect(out).toContain("# 常用端口");
    expect(out).toContain(["raw: |", "  # 原始编辑：注释保留", "  rules: !prepend", "    - DOMAIN,intranet.example.com,DIRECT", ""].join("\n"));
  });

  it("设置与删除路径，清理空的上级", () => {
    const d = new ProfileDoc(sample);
    d.setForm(["dns", "enhanced-mode"], "fake-ip");
    d.setForm(["tun", "enable"], true);
    expect(d.getForm(["tun"])).toEqual({ enable: true });
    d.deleteForm(["tun", "enable"]);
    expect(d.hasForm(["tun"])).toBe(false);
    d.deleteForm(["dns", "enable"]);
    d.deleteForm(["dns", "enhanced-mode"]);
    d.deleteForm(["mixed-port"]);
    expect(d.toString()).not.toContain("form:");
    expect(d.raw).toContain("# 原始编辑：注释保留");
  });

  it("短的标量列表写成流式", () => {
    const d = new ProfileDoc(sample);
    d.setForm(["dns", "nameserver"], ["https://doh.example.com/dns-query", "tls://192.0.2.53"]);
    expect(d.toString()).toContain("nameserver: [ https://doh.example.com/dns-query, tls://192.0.2.53 ]");
  });

  it("调整源的顺序与写法", () => {
    const d = new ProfileDoc(sample);
    d.sources = [{ ref: "src_b" }, { ref: "src_a", mode: "provider", disabled: true }];
    const back = new ProfileDoc(d.toString());
    expect(back.sources).toEqual([
      { ref: "src_b", mode: undefined, disabled: undefined },
      { ref: "src_a", mode: "provider", disabled: true },
    ]);
  });

  it("raw 用块字面量保存原文；清空时删除", () => {
    const d = new ProfileDoc(sample);
    d.raw = "dns:\n  ipv6: true # 注释\n\n";
    expect(new ProfileDoc(d.toString()).raw).toBe("dns:\n  ipv6: true # 注释\n\n");
    d.raw = "  \n";
    expect(d.hasRaw).toBe(false);
    expect(d.toString()).not.toContain("raw:");
  });

  it("新建文档", () => {
    const d = ProfileDoc.create("新方案", { pack: "mihomo", preset: "mihomo", core: "1.19.33" }, { "mixed-port": 7890 });
    const s = d.toString();
    expect(s.startsWith("clash-launch/profile: v1\nname: 新方案\n")).toBe(true);
    expect(new ProfileDoc(s).getForm(["mixed-port"])).toBe(7890);
  });

  it("拒绝重复键", () => {
    expect(() => new ProfileDoc("name: a\nname: b\n")).toThrow();
  });
});

describe("输出格式", () => {
  it("proxies-block 写入 output 并可移除，不影响其他输出设置", () => {
    const d = new ProfileDoc('clash-launch/profile: v1\nname: x\ntarget: { pack: mihomo, preset: mihomo, core: "1.19.33" }\noutput: { timezone: Asia/Shanghai }\n');
    expect(d.proxiesBlock).toBe(false);
    d.proxiesBlock = true;
    expect(d.proxiesBlock).toBe(true);
    expect(d.toString()).toContain("proxies-block: true");
    d.proxiesBlock = false;
    expect(d.proxiesBlock).toBe(false);
    expect(d.toString()).toContain("Asia/Shanghai");
    expect(d.toString()).not.toContain("proxies-block");
  });
});
