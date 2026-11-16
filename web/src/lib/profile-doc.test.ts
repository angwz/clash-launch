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
  it("默认多行；切到单行写入 proxies-flow，切回时移除，并清理旧的 proxies-block", () => {
    const d = new ProfileDoc('clash-launch/profile: v1\nname: x\ntarget: { pack: mihomo, preset: mihomo, core: "1.19.33" }\noutput: { timezone: Asia/Shanghai, proxies-block: true }\n');
    expect(d.proxiesBlock).toBe(true);
    d.proxiesBlock = false;
    expect(d.proxiesBlock).toBe(false);
    expect(d.toString()).toContain("proxies-flow: true");
    expect(d.toString()).not.toContain("proxies-block");
    d.proxiesBlock = true;
    expect(d.proxiesBlock).toBe(true);
    expect(d.toString()).toContain("Asia/Shanghai");
    expect(d.toString()).not.toContain("proxies-flow");
  });
});

describe("链式代理", () => {
  it("chains 读写往返；空条件去掉，未知选择器键原样保留", () => {
    const d = new ProfileDoc('clash-launch/profile: v1\nname: x\ntarget: { pack: mihomo, preset: mihomo, core: "1.19.33" }\nchains:\n  - via: VPS\n    select: { region: [HK], protocol: [ss] }\n');
    expect(d.chains).toEqual([{ via: "VPS", select: { region: ["HK"], protocol: ["ss"] }, disabled: undefined }]);
    d.chains = [{ via: "VPS", select: { region: ["HK"], protocol: ["ss"], source: [], name: "" } }, { via: "G", disabled: true }];
    const s = d.toString();
    expect(s).toContain("select: { region: [ HK ], protocol: [ ss ] }");
    expect(s).not.toContain("source");
    expect(d.chains[1]).toEqual({ via: "G", select: undefined, disabled: true });
    d.chains = [];
    expect(d.toString()).not.toContain("chains");
  });
});

// 直接编辑 YAML 是独立的一层（反馈 #32）：表单、源、链式代理、规则与输出选项的任何操作都不改动它，
// 即使操作的字段与原始编辑写的是同一个键；只有在直接编辑中修改才会变化。
describe("直接编辑不被其他操作改动", () => {
  it("逐项操作后 raw 原文逐字节不变", () => {
    const d = new ProfileDoc(sample + "");
    const before = d.raw;
    d.setForm(["rules"], ["MATCH,DIRECT"]); // 与 raw 中的 rules 同名
    d.setForm(["dns", "ipv6"], false);
    d.deleteForm(["dns", "enable"]);
    d.deleteForm(["mixed-port"]);
    d.deleteForm(["rules"]);
    d.sources = [{ ref: "src_b" }];
    d.sources = [];
    d.chains = [{ via: "VPS" }];
    d.chains = [];
    d.rules = "rs_x";
    d.rules = undefined;
    d.proxiesBlock = false;
    d.proxiesBlock = true;
    expect(d.raw).toBe(before);
    expect(d.toString()).not.toContain("output");
    expect(d.raw).toContain("DOMAIN,intranet.example.com,DIRECT");
  });
  it("清空表单后 raw 仍在", () => {
    const d = new ProfileDoc(sample);
    for (const p of d.formPaths()) d.deleteForm(p);
    expect(d.hasRaw).toBe(true);
  });
});

// 新建的空配置上设置嵌套字段（反馈 #59）：form 原本不存在，dns 也不存在，需要逐层创建。
describe("新建配置上设置嵌套字段", () => {
  it("dns.enable 等嵌套路径可以设置，且多次设置互不影响", () => {
    const d = ProfileDoc.create("空白", { pack: "mihomo", preset: "mihomo", core: "1.19.33" });
    d.setForm(["dns", "enable"], true);
    d.setForm(["dns", "ipv6"], false);
    d.setForm(["tun", "stack", "x"], "y");
    expect(d.getForm(["dns", "enable"])).toBe(true);
    expect(d.getForm(["dns", "ipv6"])).toBe(false);
    expect(new ProfileDoc(d.toString()).getForm(["dns", "enable"])).toBe(true);
  });
});

describe("routing 段", () => {
  it("写入后规则集每条一行、组的 rule 一行；读回相同；删除后回到跟随上游", () => {
    const d = new ProfileDoc("clash-launch/profile: v1\nname: x\nrules: { ref: rs1 }\n");
    expect(d.routing).toBeNull();
    const r = {
      from: { rules: "rs1", digest: "sha256:00", at: "2026-11-15T00:00:00Z" },
      groups: [{ name: "🚀 P", type: "select", rule: ["[]DIRECT", ".*"] }],
      rulesets: [
        { group: "🚀 P", ruleset: "https://raw.githubusercontent.com/x/y/release/clash-domain/A.yaml", type: "clash-domain", interval: 86400, bundle: "A" },
        { group: "DIRECT", ruleset: "[]MATCH" },
      ],
    };
    d.routing = r;
    const text = d.toString();
    expect(text).toContain(`rule: [ "[]DIRECT", .* ]`);
    expect(text).toContain(`- { group: DIRECT, ruleset: "[]MATCH" }`);
    expect(new ProfileDoc(text).routing).toEqual(r);
    d.routing = null;
    expect(d.toString()).not.toContain("routing");
  });
});

describe("流式集合中的指示符", () => {
  it("含 ? 等字符的正则在流式列表与映射中加引号（后端的 go-yaml 不接受未加引号的写法）", () => {
    const d = new ProfileDoc("clash-launch/profile: v1\nname: x\n");
    d.routing = { groups: [{ name: "日本", type: "select", rule: ["(^(?=.*(日|JP))(?!.*尼日利亚).*$)", "(港|HK)", "[]DIRECT"] }], rulesets: [] };
    d.chains = [{ via: "前置", select: { name: "(?i)hk|港" } }];
    const text = d.toString();
    expect(text).toContain(`rule: [ "(^(?=.*(日|JP))(?!.*尼日利亚).*$)", (港|HK), "[]DIRECT" ]`);
    expect(text).toContain(`name: "(?i)hk|港"`);
    expect(new ProfileDoc(text).routing!.groups[0]!.rule![0]).toBe("(^(?=.*(日|JP))(?!.*尼日利亚).*$)");
  });
});
