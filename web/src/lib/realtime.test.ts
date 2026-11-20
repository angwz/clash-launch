import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// 假的 WebSocket：deliver 模拟服务器推送，send 记录浏览器发出的消息，close 模拟断开
class FakeWS {
  static all: FakeWS[] = [];
  onmessage: ((ev: { data: string }) => void) | null = null;
  onclose: ((ev: { code: number }) => void) | null = null;
  readyState = 1;
  sent: string[] = [];
  constructor(public url: string) {
    FakeWS.all.push(this);
  }
  deliver(e: object) {
    this.onmessage?.({ data: JSON.stringify(e) });
  }
  send(s: string) {
    this.sent.push(s);
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
    ws.deliver({ type: "hello" });
    expect(realtime.connected).toBe(true);
    expect(sub).not.toHaveBeenCalled(); // 第一次连上不需要补读
    ws.deliver({ type: "sub", id: "L1" });
    expect(sub).toHaveBeenCalledWith("L1");
    expect(msg).not.toHaveBeenCalled();

    ws.close(1006); // 异常断开：退避后重连
    expect(realtime.connected).toBe(false);
    vi.advanceTimersByTime(1_000);
    const ws2 = FakeWS.all[1]!;
    ws2.deliver({ type: "hello" });
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

  it("「正在输入」：连上后才发送；补读不会触发它的订阅者", async () => {
    const { realtime } = await import("./realtime.svelte");
    const typing = vi.fn();
    realtime.on(["typing"], typing);
    realtime.start();
    const ws = FakeWS.all[0]!;
    ws.readyState = 0; // 连接中：丢弃，不排队
    realtime.send({ type: "typing", to: "bobby" });
    expect(ws.sent).toEqual([]);
    ws.readyState = 1;
    realtime.send({ type: "typing", to: "bobby" });
    expect(JSON.parse(ws.sent[0]!)).toEqual({ type: "typing", to: "bobby" });
    ws.deliver({ type: "hello" });
    ws.deliver({ type: "resync" });
    expect(typing).not.toHaveBeenCalled();
    ws.deliver({ type: "typing", id: "alice" });
    expect(typing).toHaveBeenCalledWith("alice");
    realtime.stop();
  });
});
