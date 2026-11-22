import { describe, expect, it } from "vitest";
import { emptySelect, listGroups, readSelect, writeSelect } from "./toml-select";

// 选择器标签的读写（O2）：只改动用户改过的键，其余原文逐字保留。
const doc = `# 我的分组
[[custom_groups]]
name = "🚀 节点选择"
type = "select"
rule = ["[]DIRECT", ".*"]

[[custom_groups]]
name = "🇭🇰 香港"
type = "url-test"
rule = ["@select"]

[custom_groups.select]
region   = ["HK"]                   # 落地地区
protocol = [
  "hysteria2",
  "vless",
]
name     = '(?i)premium\\d'
exclude  = { region = ["CN"], name = "(?i)测试" }
custom   = "保留"

[[custom_groups]]
name = "内联"
type = "select"
select = { region = ["JP"] }
`;

describe("toml-select", () => {
  it("列出组与是否可编辑", () => {
    expect(listGroups(doc)).toEqual([
      { name: "🚀 节点选择", readonly: "" },
      { name: "🇭🇰 香港", readonly: "" },
      { name: "内联", readonly: "inline" },
    ]);
    expect(listGroups(`[[custom_groups]]\nname = "A"\n[custom_groups.select]\nregion.x = 1\n`)[0]!.readonly).toBe("complex");
    expect(listGroups(`[[custom_groups]]\nname = "A"\n[custom_groups.select]\n[custom_groups.select.exclude]\nregion = ["CN"]\n`)[0]!.readonly).toBe("complex");
  });

  it("读取条件：多行数组、字面量字符串、内联表", () => {
    const s = readSelect(doc, 1);
    expect(s.region).toEqual(["HK"]);
    expect(s.protocol).toEqual(["hysteria2", "vless"]);
    expect(s.name).toBe("(?i)premium\\d");
    expect(s.exclude.region).toEqual(["CN"]);
    expect(s.exclude.name).toBe("(?i)测试");
    expect(readSelect(doc, 0)).toEqual(emptySelect());
    expect(() => readSelect(doc, 2)).toThrow();
  });

  it("条件不变时原文不变", () => {
    expect(writeSelect(doc, 1, readSelect(doc, 1))).toBe(doc);
  });

  it("只改动改过的键，保留行尾注释与其他行", () => {
    const s = readSelect(doc, 1);
    s.region = ["HK", "TW"];
    const out = writeSelect(doc, 1, s);
    expect(out).toBe(doc.replace(`region   = ["HK"]                   # 落地地区`, `region = ["HK", "TW"] # 落地地区`));
  });

  it("多行数组改动后收为一行；删除键连同所在行删除；新增键追加在最后", () => {
    const s = readSelect(doc, 1);
    s.protocol = ["vless"];
    s.exclude = { ...s.exclude, region: [], name: "" };
    s.tags = ["IEPL"];
    s.rateMax = 1.5;
    const out = writeSelect(doc, 1, s);
    expect(out).toContain(`protocol = ["vless"]\nname     = '(?i)premium\\d'\ncustom   = "保留"\ntags = ["IEPL"]\nrate = { max = 1.5 }\n\n[[custom_groups]]`);
    expect(out).not.toContain("exclude");
    expect(readSelect(out, 1)).toMatchObject({ protocol: ["vless"], tags: ["IEPL"], rateMax: 1.5, rateMin: null });
  });

  it("没有 select 表时在组末尾新建，组间空行保持", () => {
    const s = emptySelect();
    s.source = ["机场 A", "#1"];
    s.name = "it's";
    const out = writeSelect(doc, 0, s);
    expect(out).toContain(`rule = ["[]DIRECT", ".*"]\n\n[custom_groups.select]\nsource = ["机场 A", "#1"]\nname = "it's"\n\n[[custom_groups]]\nname = "🇭🇰 香港"`);
    expect(readSelect(out, 0).source).toEqual(["机场 A", "#1"]);
  });

  it("条件全部清空时删除整个 select 表（空表会选中全部节点）", () => {
    const one = `[[custom_groups]]\nname = "A"\ntype = "select"\n\n[custom_groups.select]\nregion = ["HK"]\n\n[[custom_groups]]\nname = "B"\n`;
    expect(writeSelect(one, 0, emptySelect())).toBe(`[[custom_groups]]\nname = "A"\ntype = "select"\n\n[[custom_groups]]\nname = "B"\n`);
    // 新建再清空，回到原文
    const s = emptySelect();
    s.region = ["JP"];
    const base = `[[custom_groups]]\nname = "A"\ntype = "select"\n`;
    expect(writeSelect(writeSelect(base, 0, s), 0, emptySelect())).toBe(base);
  });

  it("字符串转义：引号、反斜杠与控制字符", () => {
    const s = emptySelect();
    s.source = ['a"b\\c'];
    s.name = "x\ty";
    const out = writeSelect(`[[custom_groups]]\nname = "A"\n`, 0, s);
    expect(readSelect(out, 0).source).toEqual(['a"b\\c']);
    expect(readSelect(out, 0).name).toBe("x\ty");
  });

  it("名称在其他表头之后的组也能识别；非 custom_groups 的表结束组", () => {
    const t = `[[custom_groups]]\nname = "A"\n[custom_groups.select]\nregion = ["HK"]\n[other]\nregion = ["JP"]\n`;
    expect(readSelect(t, 0).region).toEqual(["HK"]);
    const s = readSelect(t, 0);
    s.region = [];
    expect(writeSelect(t, 0, s)).toBe(`[[custom_groups]]\nname = "A"\n[other]\nregion = ["JP"]\n`);
  });
});
