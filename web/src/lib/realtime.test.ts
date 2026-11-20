import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// 假的 WebSocket：记录实例，测试里手动触发消息与关闭
class FakeWS {
  static all: FakeWS[] = [];
  onmessage: ((ev: { data: string }) => void) | null = null;
  onclose: ((ev: { code: number }) => void) | null = null;
  constructor(public url: string) {
    FakeWS.all.push(this);
  }
  send(e: object) {
    this.onmessage?.({ data: JSON.stringify(e) });
  }
  close(code = 1000) {
    this.onclose?.({ code });
  }
}

// 每个用例重新导入 .svelte.ts 模块（需要现场编译），机器繁忙时可能超过默认的 5 秒
describe("realtime", { timeout: 30_000 }, () => {
  beforeEach(() => {
    vi.useFakeTimers();
    FakeWS.all = [];
    vi.stubGlobal("WebSocket", FakeWS);
    vi.resetModules();
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("按事件类型分发，重连成功后让所有订阅者重读一次", async () => {
    const { realtime } = await import("./realtime.svelte");
    const sub = vi.fn();
    const msg = vi.fn();
    realtime.on(["sub"], sub);
    realtime.on(["message"], msg);
    realtime.start();
    const ws = FakeWS.all[0]!;
    expect(ws.url).toMatch(/^ws:\/\/.+\/api\/v1\/events$/);
    ws.send({ type: "hello" });
    expect(realtime.connected).toBe(true);
    expect(sub).not.toHaveBeenCalled(); // 第一次连上不需要补读
    ws.send({ type: "sub", id: "L1" });
    expect(sub).toHaveBeenCalledWith("L1");
    expect(msg).not.toHaveBeenCalled();

    ws.close(1006); // 异常断开：退避后重连
    expect(realtime.connected).toBe(false);
    vi.advanceTimersByTime(1_000);
    const ws2 = FakeWS.all[1]!;
    ws2.send({ type: "hello" });
    expect(msg).toHaveBeenCalledWith(undefined); // 重连后补读
    realtime.stop();
  });

  it("会话结束（1008）后不再重连", async () => {
    const { realtime } = await import("./realtime.svelte");
    realtime.start();
    FakeWS.all[0]!.close(1008);
    vi.advanceTimersByTime(60_000);
    expect(FakeWS.all).toHaveLength(1);
  });
});
