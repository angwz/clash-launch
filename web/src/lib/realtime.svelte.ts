// 实时通道（需求 R22）：登录后与服务器保持一条 WebSocket，收到「什么变了」的事件就让关心它的页面立即重读。
//
// - 事件只有类型与对象 ID（render、source、rules、message、sub、grant），数据仍经 API 读取；
// - 断线后按 1、2、4…最长 30 秒（带随机抖动）重连；重连成功时通知所有订阅者重读一次，补上断线期间的变化；
// - 服务器以 1008 关闭表示会话已结束：不再重连，交给下一次 API 请求走正常的登录失效流程；
// - 连不上（代理不支持升级、浏览器不支持）时页面照常靠 live() 的定时刷新，功能不受影响。
// 同一浏览器的多个标签页各自一条连接（服务器按用户限制条数）；共用连接留待后续。

export type Topic = "render" | "source" | "rules" | "message" | "sub" | "grant";
type Listener = (id: string | undefined) => void;

const ALL = "*";

class Realtime {
  /** 是否已连接（在线指示灯 R11 使用） */
  connected = $state(false);
  #ws: WebSocket | undefined;
  #listeners = new Map<string, Set<Listener>>();
  #retry = 0;
  #timer: ReturnType<typeof setTimeout> | undefined;
  #wanted = false;
  #everConnected = false;

  start(): void {
    if (this.#wanted || typeof WebSocket === "undefined") return;
    this.#wanted = true;
    this.#connect();
  }

  stop(): void {
    this.#wanted = false;
    clearTimeout(this.#timer);
    this.#ws?.close(1000);
    this.#ws = undefined;
    this.connected = false;
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

  #emit(type: string, id?: string): void {
    if (type === "resync" || type === ALL) {
      const seen = new Set<Listener>();
      for (const set of this.#listeners.values()) for (const fn of set) if (!seen.has(fn)) (seen.add(fn), fn(undefined));
      return;
    }
    for (const fn of this.#listeners.get(type) ?? []) fn(id);
  }

  #connect(): void {
    const url = (location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/api/v1/events";
    let ws: WebSocket;
    try {
      ws = new WebSocket(url);
    } catch {
      this.#schedule();
      return;
    }
    this.#ws = ws;
    ws.onmessage = (ev) => {
      let e: { type?: string; id?: string };
      try {
        e = JSON.parse(String(ev.data)) as { type?: string; id?: string };
      } catch {
        return;
      }
      if (e.type === "hello") {
        this.connected = true;
        this.#retry = 0;
        // 重连成功：断线期间可能错过事件，让所有页面重读一次
        if (this.#everConnected) this.#emit(ALL);
        this.#everConnected = true;
        return;
      }
      if (e.type) this.#emit(e.type, e.id);
    };
    ws.onclose = (ev) => {
      if (this.#ws === ws) this.#ws = undefined;
      this.connected = false;
      if (ev.code === 1008) this.#wanted = false; // 会话已结束
      if (this.#wanted) this.#schedule();
    };
  }

  #schedule(): void {
    clearTimeout(this.#timer);
    const base = Math.min(30_000, 1_000 * 2 ** this.#retry);
    this.#retry = Math.min(this.#retry + 1, 5);
    this.#timer = setTimeout(() => this.#wanted && this.#connect(), base / 2 + Math.random() * (base / 2));
  }
}

export const realtime = new Realtime();
