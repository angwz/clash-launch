import { describe, expect, it } from "vitest";
import { flagOf, isLocalIP } from "./regions";

describe("regions", () => {
  it("旗帜", () => {
    expect(flagOf("cn")).toBe(String.fromCodePoint(0x1f1e8, 0x1f1f3));
    expect(flagOf("x")).toBe("");
  });
  it("本地网络", () => {
    for (const ip of ["127.0.0.1", "10.1.2.3", "172.20.0.1", "192.168.1.1", "100.64.0.1", "::1", "fd00::1", "fe80::1"]) expect(isLocalIP(ip), ip).toBe(true);
    for (const ip of ["8.8.8.8", "172.32.0.1", "2001:db8::1", "192.0.2.1"]) expect(isLocalIP(ip), ip).toBe(false);
  });
});
