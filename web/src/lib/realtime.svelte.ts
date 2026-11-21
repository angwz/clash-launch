// 实时通道（需求 R22）：登录后与服务器保持一条 WebSocket，收到「什么变了」的事件就让关心它的页面立即重读。
//
// - 事件只有类型与对象 ID（render、source、rules、message、sub、grant、typing、presence），数据仍经 API 读取；
// - 断线后按 1、2、4…最长 30 秒（带随机抖动）重连；重连成功时通知所有订阅者重读一次，补上断线期间的变化；
// - 服务器以 1008 关闭表示会话已结束：不再重连，交给下一次 API 请求走正常的登录失效流程；
// - 心跳：每 30 秒 ping 一次并测往返延迟；连续两次 10 秒内没有回应就断开重连（在线指示灯 R11 据此熄灭）；
// - 连不上（代理不支持升级、浏览器不支持）时页面照常靠 live() 的定时刷新，功能不受影响；指示灯改为每 15 秒
//   轮询 GET /api/v1/pulse（标签页隐藏时暂停），轮询成功就不算断线。
//
// 多个标签页共用一条连接：用 Web Locks 选出一个「主标签页」持有连接，它把收到的事件经 BroadcastChannel
// 转给其他标签页，其他标签页要发的消息也经它代发。主标签页关闭时锁自动释放，排队的下一个标签页接手并重连。
// 浏览器不支持这两个接口时，每个标签页各自连接（服务器按用户限制条数）。

export type Topic = "render" | "source" | "rules" | "message" | "sub" | "grant" | "typing" | "presence";

/** 服务器推来的事件（字段见 internal/events.Event）。 */
export interface RealtimeEvent {
  type: string;
  id?: string;
  state?: string;
  online?: number;
  version?: string;
  kernel?: string;
}
type Listener = (id: string | undefined, e?: RealtimeEvent) => void;
type Outgoing = { type: "typing"; to: string };

/** 标签页之间的消息。 */
type TabMsg =
  | { k: "event"; e: RealtimeEvent }
  | { k: "state"; connected: boolean; polled: boolean; rtt?: number; online?: number; version?: string; kernel?: boolean }
  | { k: "send"; msg: Outgoing }
  | { k: "ping" }
  | { k: "query" }
  | { k: "ended" };

const ALL = "*";
const CHANNEL = "clash-launch-realtime";
const PING_EVERY = 30_000;
const PONG_WAIT = 10_000;
// 断开后多久开始轮询、之后的间隔。首次轮询早于指示灯的断线判定（5 秒），实时通道不可用而服务器正常时不会误熄
const POLL_AFTER = 2_000;
const POLL_EVERY = 15_000;

interface PulseData {
  version: string;
  online?: number;
  unread: number;
  kernel: boolean;
}

interface LockManager {
  request(name: string, opts: { signal: AbortSignal }, cb: () => Promise<void>): Promise<unknown>;
}

class Realtime {
  /** 是否已连接（本标签页持有连接，或主标签页已连接） */
  connected = $state(false);
  /** 最近一次心跳的往返延迟（毫秒） */
  rtt = $state<number | undefined>(undefined);
  /** 在线人数；服务器只告诉管理员 */
  online = $state<number | undefined>(undefined);
  /** 服务器版本：页面第一次连上时的版本与最新的版本，不同时提示刷新（R11） */
  pageVersion = $state<string | undefined>(undefined);
  serverVersion = $state<string | undefined>(undefined);
  /** 服务端的内核检查是否可用；未知时为 undefined。不可用时指示灯降级（R11） */
  kernel = $state<boolean | undefined>(undefined);
  /** 实时通道断开期间，最近一次轮询是否成功 */
  polled = $state(false);

  /** 服务器是否可达：实时通道已连接，或轮询成功 */
  get reachable(): boolean {
    return this.connected || this.polled;
  }

  #ws: WebSocket | undefined;
  #listeners = new Map<string, Set<Listener>>();
  #retry = 0;
  #timer: ReturnType<typeof setTimeout> | undefined;
  #pingTimer: ReturnType<typeof setInterval> | undefined;
  #wanted = false;
  #everConnected = false;
  #leader = false;
  #channel: BroadcastChannel | undefined;
  #abort: AbortController | undefined;
  #release: (() => void) | undefined;
  #pending = new Map<string, number>(); // ping ID → 发出时间
  #missed = 0;
  #seq = 0;
  #pollTimer: ReturnType<typeof setTimeout> | undefined;

  start(): void {
    if (this.#wanted || typeof WebSocket === "undefined") return;
    this.#wanted = true;
    const locks = (globalThis.navigator as { locks?: LockManager } | undefined)?.locks;
    if (!locks || typeof BroadcastChannel === "undefined") {
      this.#becomeLeader();
      return;
    }
    const ch = new BroadcastChannel(CHANNEL);
    this.#channel = ch;
    ch.onmessage = (ev: MessageEvent<TabMsg>) => this.#onTab(ev.data);
    ch.postMessage({ k: "query" } satisfies TabMsg); // 已有主标签页时，它会回一次当前状态
    const abort = new AbortController();
    this.#abort = abort;
    locks
      .request(CHANNEL, { signal: abort.signal }, () => {
        if (!this.#wanted) return Promise.resolve();
        this.#becomeLeader();
        // 持有锁直到 stop()：标签页关闭时浏览器自动释放
        return new Promise<void>((resolve) => (this.#release = resolve));
      })
      .catch(() => {}); // stop() 取消排队时会以 AbortError 结束
  }

  stop(): void {
    this.#wanted = false;
    this.#leader = false;
    clearTimeout(this.#timer);
    clearInterval(this.#pingTimer);
    this.#stopPolling();
    this.#ws?.close(1000);
    this.#ws = undefined;
    this.#abort?.abort();
    this.#release?.();
    this.#release = undefined;
    this.#channel?.close();
    this.#channel = undefined;
    this.#pending.clear();
    this.connected = false;
    this.polled = false;
  }

  /** 向服务器发一条消息（目前只有「正在输入」）；没连上时丢弃，不排队。 */
  send(msg: Outgoing): void {
    if (this.#leader) this.#wsSend(msg);
    else this.#post({ k: "send", msg });
  }

  /** 立即测一次往返延迟（例如打开指示灯的说明时）。 */
  ping(): void {
    if (this.#leader) this.#ping();
    else this.#post({ k: "ping" });
  }

  /** 订阅一组事件类型；返回取消函数。 */
  on(topics: readonly Topic[], fn: Listener): () => void {
    for (const t of topics) {
      let set = this.#listeners.get(t);
      if (!set) this.#listeners.set(t, (set = new Set()));
      set.add(fn);
    }
    return () => {
      for (const t of topics) this.#listeners.get(t)?.delete(fn);
    };
  }

  #emit(type: string, e?: RealtimeEvent): void {
    if (type === "resync" || type === ALL) {
      // 重读类的订阅者才需要补读；「正在输入」与在线状态是瞬时信号，不补
      const seen = new Set<Listener>();
      for (const [t, set] of this.#listeners) if (t !== "typing" && t !== "presence") for (const fn of set) if (!seen.has(fn)) (seen.add(fn), fn(undefined));
      return;
    }
    for (const fn of this.#listeners.get(type) ?? []) fn(e?.id, e);
  }

  /** 处理一条服务器事件：主标签页从 WebSocket 收到，其他标签页从广播收到。 */
  #handle(e: RealtimeEvent): void {
    if (e.type === "hello") {
      this.connected = true;
      this.polled = false;
      this.kernel = e.kernel !== "unavailable";
      this.#noteVersion(e.version);
      if (e.online) this.online = e.online;
      // 重连成功或换了主标签页：断线期间可能错过事件，让所有页面重读一次
      if (this.#everConnected) this.#emit(ALL);
      this.#everConnected = true;
      return;
    }
    if (e.type === "pong") return;
    this.#emit(e.type, e);
  }

  #noteVersion(v: string | undefined): void {
    if (!v) return;
    this.pageVersion ??= v;
    this.serverVersion = v;
  }

  #onTab(m: TabMsg): void {
    switch (m.k) {
      case "event":
        if (!this.#leader) this.#handle(m.e);
        break;
      case "state":
        if (this.#leader) break;
        if (m.connected && !this.connected) this.#everConnected = true; // 加入时主标签页已连接：之后的 hello 才算重连
        this.connected = m.connected;
        this.polled = m.polled;
        this.kernel = m.kernel;
        this.rtt = m.rtt;
        this.online = m.online;
        this.#noteVersion(m.version);
        break;
      case "send":
        if (this.#leader) this.#wsSend(m.msg);
        break;
      case "ping":
        if (this.#leader) this.#ping();
        break;
      case "query":
        if (this.#leader) this.#broadcastState();
        break;
      case "ended":
        if (!this.#leader) this.stop();
        break;
    }
  }

  #post(m: TabMsg): void {
    try {
      this.#channel?.postMessage(m);
    } catch {
      // 页面卸载中：忽略
    }
  }

  #broadcastState(): void {
    this.#post({ k: "state", connected: this.connected, polled: this.polled, rtt: this.rtt, online: this.online, version: this.serverVersion, kernel: this.kernel });
  }

  #becomeLeader(): void {
    this.#leader = true;
    this.#retry = 0;
    this.#connect();
  }

  #wsSend(msg: object): void {
    if (this.#ws?.readyState === 1) this.#ws.send(JSON.stringify(msg));
  }

  #ping(): void {
    if (this.#ws?.readyState !== 1) return;
    const id = String(++this.#seq);
    this.#pending.set(id, performance.now());
    this.#wsSend({ type: "ping", id });
    setTimeout(() => {
      if (!this.#pending.delete(id)) return;
      // 连续两次没有回应：连接多半已经断了（休眠、切换网络），主动断开以触发重连
      if (++this.#missed >= 2) this.#ws?.close(4000);
    }, PONG_WAIT);
  }

  #connect(): void {
    const url = (location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/api/v1/events";
    let ws: WebSocket;
    try {
      ws = new WebSocket(url);
    } catch {
      this.#schedule();
      this.#startPolling();
      return;
    }
    this.#ws = ws;
    ws.onmessage = (ev) => {
      let e: RealtimeEvent;
      try {
        e = JSON.parse(String(ev.data)) as RealtimeEvent;
      } catch {
        return;
      }
      if (!e.type) return;
      if (e.type === "pong") {
        const sent = e.id ? this.#pending.get(e.id) : undefined;
        if (sent !== undefined) {
          this.#pending.delete(e.id!);
          this.rtt = Math.round(performance.now() - sent);
        }
        this.#missed = 0;
        if (e.online) this.online = e.online;
        this.#broadcastState();
        return;
      }
      if (e.type === "hello") {
        this.#retry = 0;
        this.#missed = 0;
        this.#stopPolling();
        clearInterval(this.#pingTimer);
        this.#pingTimer = setInterval(() => this.#ping(), PING_EVERY);
      }
      this.#handle(e);
      this.#post({ k: "event", e });
      if (e.type === "hello") {
        this.#broadcastState();
        this.#ping();
      }
    };
    ws.onclose = (ev) => {
      if (this.#ws === ws) this.#ws = undefined;
      clearInterval(this.#pingTimer);
      this.#pending.clear();
      this.connected = false;
      this.rtt = undefined;
      if (ev.code === 1008) {
        // 会话已结束：所有标签页都停下
        this.#post({ k: "ended" });
        this.#wanted = false;
      }
      this.#broadcastState();
      if (this.#wanted) {
        this.#schedule();
        this.#startPolling();
      }
    };
  }

  // —— 轮询兜底（只有持有连接的标签页轮询，结果随 state 广播给其他标签页）——

  #startPolling(): void {
    if (this.#pollTimer !== undefined) return;
    this.#pollTimer = setTimeout(() => void this.#poll(), POLL_AFTER);
  }

  #stopPolling(): void {
    clearTimeout(this.#pollTimer);
    this.#pollTimer = undefined;
  }

  async #poll(): Promise<void> {
    if (!this.#wanted || this.connected) return this.#stopPolling();
    // 标签页隐藏时不发请求，到点再看
    if (typeof document === "undefined" || !document.hidden) {
      const t0 = performance.now();
      try {
        const r = await fetch("/api/v1/pulse", { credentials: "same-origin", headers: { Accept: "application/json" } });
        if (!r.ok) throw new Error(String(r.status));
        const p = (await r.json()) as PulseData;
        if (this.connected) return this.#stopPolling(); // 等待期间实时通道已恢复，以它为准
        this.polled = true;
        this.rtt = Math.round(performance.now() - t0);
        this.online = p.online;
        this.kernel = p.kernel;
        this.#noteVersion(p.version);
      } catch {
        // 401（会话结束）也按不可达处理：交给下一次 API 请求走登录失效流程
        if (this.connected) return this.#stopPolling();
        this.polled = false;
        this.rtt = undefined;
      }
      this.#broadcastState();
    }
    this.#pollTimer = this.#wanted && !this.connected ? setTimeout(() => void this.#poll(), POLL_EVERY) : undefined;
  }

  #schedule(): void {
    clearTimeout(this.#timer);
    const base = Math.min(30_000, 1_000 * 2 ** this.#retry);
    this.#retry = Math.min(this.#retry + 1, 5);
    this.#timer = setTimeout(() => this.#wanted && this.#connect(), base / 2 + Math.random() * (base / 2));
  }
}

export const realtime = new Realtime();
