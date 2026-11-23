import { describe, expect, it } from "vitest";
import { placeName } from "./place-names";
import { placeText } from "./regions";

// 地点名称按界面语言显示（反馈 #114）：收录的按语言显示，没收录的保留原文；运营商不翻译
describe("placeName", () => {
  it("中国的省市：英文与日文界面翻译，中文界面保留原文", () => {
    expect(placeName("上海市", "CN", "en")).toBe("Shanghai");
    expect(placeName("广东省", "CN", "ja")).toBe("広東省");
    expect(placeName("新疆维吾尔自治区", "CN", "en")).toBe("Xinjiang");
    expect(placeName("上海市", "CN", "zh-CN")).toBe("上海市");
  });
  it("美国各州与日本都道府县只在对应国家时翻译", () => {
    expect(placeName("California", "US", "zh-CN")).toBe("加利福尼亚州");
    expect(placeName("Georgia", "US", "ja")).toBe("ジョージア州");
    expect(placeName("Georgia", "GE", "zh-CN")).toBe("Georgia");
    expect(placeName("Tōkyō", "JP", "zh-CN")).toBe("东京都");
    expect(placeName("Osaka Prefecture", "JP", "ja")).toBe("大阪府");
  });
  it("常见城市不限国家；没收录的保留原文", () => {
    expect(placeName("Frankfurt am Main", "DE", "zh-CN")).toBe("法兰克福");
    expect(placeName("Bell Gardens", "US", "zh-CN")).toBe("Bell Gardens");
    expect(placeName("Los Angeles", "US", "en")).toBe("Los Angeles");
  });
  it("placeText：国家、省市按语言，运营商原样", () => {
    expect(placeText({ cc: "US", region: "California", city: "Los Angeles", isp: "Example ISP" }, "zh-CN")).toBe("美国 · 加利福尼亚州 · 洛杉矶 · Example ISP");
    expect(placeText({ cc: "CN", region: "上海市", city: "上海市", isp: "移动" }, "en")).toBe("China · Shanghai · 移动");
  });
});
