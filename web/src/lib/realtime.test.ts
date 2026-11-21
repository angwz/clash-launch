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
    this.readyState = 3;
    this.onclose?.({ code });
  }
  types() {
    return this.sent.map((s) => (JSON.parse(s) as { type: string }).type);
  }
}

// 假的 BroadcastChannel：同名频道之间同步投递，不投给自己（与浏览器一致）
class FakeChannel {
  static all: FakeChannel[] = [];
  onmessage: ((ev: { data: unknown }) => void) | null = null;
  constructor(public name: string) {
    FakeChannel.all.push(this);
  }
  postMessage(data: unknown) {
    for (const c of FakeChannel.all) if (c !== this && c.name === this.name) c.onmessage?.({ data: structuredClone(data) });
  }
  close() {
    FakeChannel.all = FakeChannel.all.filter((c) => c !== this);
  }
}

// 假的 Web Locks：同一时刻只有一个持有者，其余排队；可以用 signal 取消排队
class FakeLocks {
  held = false;
  queue: (() => void)[] = [];
  request(_name: string, opts: { signal: AbortSignal }, cb: () => Promise<void>) {
    return new Promise((resolve, reject) => {
      const run = () => {
        this.held = true;
        void cb().then(() => {
          this.held = false;
          resolve(undefined);
          this.queue.shift()?.();
        });
      };
      if (!this.held) return run();
      this.queue.push(run);
      opts.signal.addEventListener("abort", () => {
        const i = this.queue.indexOf(run);
        if (i >= 0) this.queue.splice(i, 1);
        reject(new DOMException("aborted", "AbortError"));
      });
    });
  }
}

const flush = () => new Promise<void>((r) => queueMicrotask(r));

// 每个用例重新导入 .svelte.ts 模块（需要现场编译），机器繁忙时可能超过默认的 5 秒
describe("realtime", { timeout: 30_000 }, () => {
  beforeEach(() => {
    vi.useFakeTimers();
    FakeWS.all = [];
    FakeChannel.all = [];
    vi.stubGlobal("WebSocket", FakeWS);
    vi.stubGlobal("BroadcastChannel", undefined); // 默认：不支持共用连接，每个标签页各自连接
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
    ws.deliver({ type: "hello", version: "v1" });
    expect(realtime.connected).toBe(true);
    expect(sub).not.toHaveBeenCalled(); // 第一次连上不需要补读
    ws.deliver({ type: "sub", id: "L1" });
    expect(sub).toHaveBeenCalledWith("L1", { type: "sub", id: "L1" });
    expect(msg).not.toHaveBeenCalled();

    ws.close(1006); // 异常断开：退避后重连
    expect(realtime.connected).toBe(false);
    vi.advanceTimersByTime(1_000);
    const ws2 = FakeWS.all[1]!;
    ws2.deliver({ type: "hello", version: "v2" });
    expect(msg).toHaveBeenCalledWith(undefined); // 重连后补读
    expect(realtime.pageVersion).toBe("v1"); // 服务器换了版本：指示灯提示刷新
    expect(realtime.serverVersion).toBe("v2");
    realtime.stop();
  });

  it("会话结束（1008）后不再重连", async () => {
    const { realtime } = await import("./realtime.svelte");
    realtime.start();
    FakeWS.all[0]!.close(1008);
    vi.advanceTimersByTime(60_000);
    expect(FakeWS.all).toHaveLength(1);
  });

  it("「正在输入」：连上后才发送；补读不会触发它与在线状态的订阅者", async () => {
    const { realtime } = await import("./realtime.svelte");
    const typing = vi.fn();
    const presence = vi.fn();
    realtime.on(["typing"], typing);
    realtime.on(["presence"], presence);
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
    expect(presence).not.toHaveBeenCalled();
    ws.deliver({ type: "typing", id: "alice" });
    expect(typing).toHaveBeenCalledWith("alice", { type: "typing", id: "alice" });
    ws.deliver({ type: "presence", id: "alice", state: "off" });
    expect(presence).toHaveBeenCalledWith("alice", { type: "presence", id: "alice", state: "off" });
    realtime.stop();
  });

  it("心跳：测往返延迟；连续两次没有回应就断开重连", async () => {
    const { realtime } = await import("./realtime.svelte");
    realtime.start();
    const ws = FakeWS.all[0]!;
    ws.deliver({ type: "hello", online: 3 });
    expect(realtime.online).toBe(3);
    expect(ws.types()).toEqual(["ping"]); // 连上立即测一次
    const id = (JSON.parse(ws.sent[0]!) as { id: string }).id;
    vi.advanceTimersByTime(40);
    ws.deliver({ type: "pong", id, online: 4 });
    expect(realtime.rtt).toBeGreaterThanOrEqual(0);
    expect(realtime.online).toBe(4);

    vi.advanceTimersByTime(30_000 + 10_000); // 第一次没回应
    expect(FakeWS.all).toHaveLength(1);
    vi.advanceTimersByTime(30_000); // 第二次也没回应：断开
    expect(realtime.connected).toBe(false);
    vi.advanceTimersByTime(1_000);
    expect(FakeWS.all).toHaveLength(2);
    realtime.stop();
  });

  it("多个标签页共用一条连接：主标签页转发事件、代发消息，关闭后由下一个接手", async () => {
    const locks = new FakeLocks();
    vi.stubGlobal("BroadcastChannel", FakeChannel);
    vi.stubGlobal("navigator", { locks });
    const a = (await import("./realtime.svelte")).realtime;
    vi.resetModules();
    const b = (await import("./realtime.svelte")).realtime;
    const subB = vi.fn();
    const msgB = vi.fn();
    b.on(["sub"], subB);
    b.on(["message"], msgB);

    a.start();
    b.start();
    await flush();
    expect(FakeWS.all).toHaveLength(1); // 只有主标签页连接
    const ws = FakeWS.all[0]!;
    ws.deliver({ type: "hello", version: "v1" });
    expect(a.connected).toBe(true);
    expect(b.connected).toBe(true); // 状态同步到其他标签页
    ws.deliver({ type: "sub", id: "L1" });
    expect(subB).toHaveBeenCalledWith("L1", { type: "sub", id: "L1" });
    b.send({ type: "typing", to: "bobby" }); // 经主标签页代发
    expect(ws.types()).toEqual(["ping", "typing"]);
    expect(msgB).not.toHaveBeenCalled();

    a.stop(); // 主标签页关闭：锁释放，b 接手并重新连接
    await flush();
    await flush();
    expect(FakeWS.all).toHaveLength(2);
    FakeWS.all[1]!.deliver({ type: "hello", version: "v1" });
    expect(b.connected).toBe(true);
    expect(msgB).toHaveBeenCalledWith(undefined); // 交接期间可能错过事件：补读
    b.stop();
  });
});
