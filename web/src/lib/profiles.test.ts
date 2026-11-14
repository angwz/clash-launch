import { describe, expect, it } from "vitest";
import { nodeInput } from "./profiles";

describe("nodeInput", () => {
  it("单行 http(s) 地址是订阅，名称取域名", () => {
    expect(nodeInput("  https://sub.example.com/api?token=example-token \n", "手动")).toEqual({ name: "sub.example.com", kind: "subscription", url: "https://sub.example.com/api?token=example-token" });
  });
  it("其余内容作为手动节点", () => {
    expect(nodeInput("ss://YWVzLTEyOC1nY206ZXhhbXBsZQ==@192.0.2.40:8388#x", "手动")).toMatchObject({ kind: "batch", name: "手动" });
    expect(nodeInput("https://a.example.com\nhttps://b.example.com", "手动")?.kind).toBe("batch");
  });
  it("空白不添加", () => {
    expect(nodeInput("  \n ", "手动")).toBeNull();
  });
});
