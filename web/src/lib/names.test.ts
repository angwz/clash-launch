import { describe, expect, it } from "vitest";
import { hostOf, uniqueName } from "./names";

describe("默认名称", () => {
  it("不重复时原样使用，重复时追加序号", () => {
    expect(uniqueName("手动节点", [])).toBe("手动节点");
    expect(uniqueName("手动节点", ["手动节点", "手动节点 2"])).toBe("手动节点 3");
  });
  it("取地址的域名", () => {
    expect(hostOf("https://sub.example.com/api?token=example-token")).toBe("sub.example.com");
    expect(hostOf("not a url")).toBeNull();
  });
});
