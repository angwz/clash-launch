// 本机的消息密钥状态（需求 R46，决定 0009）。
//
// - none：服务器上还没有密钥（用密码登录或在消息窗口输入密码时生成）；
// - locked：服务器上有密钥，本机没有可用的私钥（换了设备、清了数据、用通行密钥登录、或密钥已在别处重新生成）；
// - ready：本机有当前密钥的私钥，发出的文字会加密；
// - unsupported：浏览器没有 Web Crypto 或 IndexedDB（极少见），消息照旧不加密。
import { api } from "./api/client";
import * as e2e from "./e2e";

export type KeyState = "loading" | "none" | "locked" | "ready" | "unsupported";

/** 对方的公钥：全部（解开以前的消息）与当前的一把（加密新消息）。 */
interface PeerKeys {
  byId: Map<string, CryptoKey>;
  active: { keyId: string; pub: CryptoKey } | null;
}

interface ServerKey {
  keyId: string;
  publicKey: string;
  wrapped?: e2e.Wrapped;
  recovery?: e2e.Wrapped;
}

class MessageKeys {
  state = $state<KeyState>("loading");
  keyId = $state("");
  hasRecovery = $state(false);
  #local: e2e.LocalKey | null = null;
  #user = "";
  #peers = new Map<string, Promise<PeerKeys>>(); // 用户名（小写）→ 对方的公钥

  get supported(): boolean {
    return typeof crypto !== "undefined" && !!crypto.subtle && typeof indexedDB !== "undefined";
  }

  async #server(): Promise<ServerKey | null> {
    return (await api.get<{ key: ServerKey | null }>("/api/v1/account/message-key")).key;
  }

  /** 读取本机与服务器上的状态（打开消息窗口、账号页时调用）。 */
  async refresh(userId: string): Promise<void> {
    if (!this.supported) {
      this.state = "unsupported";
      return;
    }
    // 登录后的密钥处理还在进行（PBKDF2 需要一点时间）：等它完成，否则先读到的「还没有密钥」会盖掉它的结果
    if (this.#afterLogin) await this.#afterLogin;
    this.#user = userId;
    const [local, server] = await Promise.all([e2e.loadLocal(userId).catch(() => null), this.#server()]);
    this.hasRecovery = !!server?.recovery;
    this.keyId = server?.keyId ?? "";
    if (!server) {
      this.#local = null;
      this.state = "none";
    } else if (local && local.keyId === server.keyId) {
      this.#local = local;
      this.state = "ready";
    } else {
      this.#local = null;
      this.state = "locked";
    }
  }

  async #adopt(userId: string, keyId: string, priv: CryptoKey): Promise<void> {
    const local = { keyId, priv: priv.extractable ? await e2e.toLocal(priv) : priv };
    await e2e.saveLocal(userId, local);
    this.#local = local;
    this.#user = userId;
    this.keyId = keyId;
    this.state = "ready";
  }

  /** 生成新密钥并上传；replace 为真时替换现有的（此前的加密消息将无法再读）。 */
  async #create(userId: string, password: string, replace: boolean): Promise<void> {
    const { priv, publicKey } = await e2e.generateKeyPair();
    const wrapped = await e2e.wrapWithPassword(priv, password);
    const k = await api.put<{ keyId: string }>("/api/v1/account/message-key", { password, publicKey, wrapped, replace });
    this.hasRecovery = false;
    await this.#adopt(userId, k.keyId, priv);
  }

  /**
   * 用密码登录、注册后调用：没有密钥就生成，有就用这个密码解开。解不开（密码已被重置）时留在 locked，
   * 由消息窗口提示用恢复密钥或旧密码解锁。失败不影响登录本身。
   */
  #afterLogin: Promise<void> | null = null;

  afterPasswordLogin(userId: string, password: string): Promise<void> {
    if (!this.supported) return Promise.resolve();
    const p = this.#afterPasswordLogin(userId, password).finally(() => {
      if (this.#afterLogin === p) this.#afterLogin = null;
    });
    this.#afterLogin = p;
    return p;
  }

  async #afterPasswordLogin(userId: string, password: string): Promise<void> {
    try {
      const [local, server] = await Promise.all([e2e.loadLocal(userId).catch(() => null), this.#server()]);
      if (!server) return await this.#create(userId, password, false);
      // 本机已有这把密钥：不必再做一次 PBKDF2（同一设备再次登录不变慢）
      if (local && local.keyId === server.keyId) {
        this.#local = local;
        this.#user = userId;
        this.keyId = server.keyId;
        this.hasRecovery = !!server.recovery;
        this.state = "ready";
        return;
      }
      const priv = await e2e.unwrapWithPassword(server.wrapped!, password);
      this.hasRecovery = !!server.recovery;
      await this.#adopt(userId, server.keyId, priv);
    } catch {
      this.#afterLogin = null; // 下面的 refresh 不必再等自己
      await this.refresh(userId).catch(() => undefined);
    }
  }

  /** 在消息窗口或账号页输入密码：没有密钥时生成，有时解开。密码不对时抛出。 */
  async unlock(userId: string, password: string): Promise<void> {
    const server = await this.#server();
    if (!server) return this.#create(userId, password, false);
    const priv = await e2e.unwrapWithPassword(server.wrapped!, password);
    await this.#adopt(userId, server.keyId, priv);
  }

  /** 用恢复密钥解锁，并用当前登录密码重新包裹（之后用密码就能解锁）。 */
  async unlockWithRecovery(userId: string, code: string, password: string): Promise<void> {
    const bytes = e2e.parseRecoveryCode(code);
    const server = await this.#server();
    if (!bytes || !server?.recovery) throw new Error("recovery");
    const priv = await e2e.unwrapWithRecovery(server.recovery, bytes, true);
    const wrapped = await e2e.wrapWithPassword(priv, password);
    await api.put("/api/v1/account/message-key", { password, keyId: server.keyId, wrapped });
    await this.#adopt(userId, server.keyId, priv);
  }

  /** 重新生成密钥（忘记密码又没有恢复密钥时）。此前的加密消息无法再读。 */
  reset(userId: string, password: string): Promise<void> {
    return this.#create(userId, password, true);
  }

  /** 生成恢复密钥：用密码解开当前私钥，以新的恢复密钥再包裹一份。返回只显示一次的恢复密钥。 */
  async makeRecovery(password: string): Promise<string> {
    const server = await this.#server();
    if (!server?.wrapped) throw new Error("no key");
    const priv = await e2e.unwrapWithPassword(server.wrapped, password, true);
    const { code, bytes } = e2e.newRecoveryCode();
    const recovery = await e2e.wrapWithRecovery(priv, bytes);
    await api.put("/api/v1/account/message-key", { password, keyId: server.keyId, wrapped: server.wrapped, recovery });
    this.hasRecovery = true;
    return code;
  }

  /** 改密码时：用旧密码解开、新密码重新包裹，随改密码的请求一起提交。没有密钥时返回 undefined。 */
  async rewrapFor(current: string, next: string): Promise<{ keyId: string; wrapped: e2e.Wrapped } | undefined> {
    if (!this.supported) return undefined;
    const server = await this.#server();
    if (!server?.wrapped) return undefined;
    let priv: CryptoKey;
    try {
      priv = await e2e.unwrapWithPassword(server.wrapped, current, true);
    } catch {
      return undefined; // 现在的包裹不是用当前密码做的（密码曾被重置）：不改，仍可用恢复密钥解锁
    }
    return { keyId: server.keyId, wrapped: await e2e.wrapWithPassword(priv, next) };
  }

  /** 退出登录：删除本机的私钥。 */
  async forget(): Promise<void> {
    this.#local = null;
    this.#peers.clear();
    this.state = "loading";
    this.keyId = "";
    if (this.supported) await e2e.clearLocal().catch(() => undefined);
  }

  // —— 收发 ——

  #peerKeys(username: string): Promise<PeerKeys> {
    const key = username.toLowerCase();
    let p = this.#peers.get(key);
    if (!p) {
      p = api.get<{ items: { keyId: string; publicKey: string; active: boolean }[] }>("/api/v1/users/" + encodeURIComponent(username) + "/message-keys").then(async (r) => {
        const byId = new Map<string, CryptoKey>();
        let active: PeerKeys["active"] = null;
        for (const k of r.items) {
          const pub = await e2e.importPublic(k.publicKey);
          byId.set(k.keyId, pub);
          if (k.active) active = { keyId: k.keyId, pub };
        }
        return { byId, active };
      });
      // 失败或对方还没有当前密钥时不缓存：对方随时可能启用，下次打开会话重新查询
      p.then(
        (k) => !k.active && this.#peers.delete(key),
        () => this.#peers.delete(key),
      );
      this.#peers.set(key, p);
    }
    return p;
  }

  /** 对方的密钥可能刚换过：发送失败（密钥不是当前的）后清掉缓存重试。 */
  dropPeer(username: string): void {
    this.#peers.delete(username.toLowerCase());
  }

  /** 能否给对方发加密消息；不能时返回原因（界面显示）。 */
  async canEncrypt(username: string): Promise<"ok" | "self" | "peer"> {
    if (this.state !== "ready" || !this.#local) return "self";
    const k = await this.#peerKeys(username).catch(() => null);
    return k?.active ? "ok" : "peer";
  }

  async encrypt(username: string, text: string): Promise<e2e.EncText> {
    const { active } = await this.#peerKeys(username);
    if (!this.#local || !active) throw new Error("cannot encrypt");
    return e2e.encryptText(text, this.#local, active);
  }

  async decrypt(username: string, e: e2e.EncText, mine: boolean): Promise<string> {
    if (!this.#local) throw new Error("locked");
    return e2e.decryptText(e, mine, this.#local, async (kid) => (await this.#peerKeys(username)).byId.get(kid) ?? null);
  }

  get userId(): string {
    return this.#user;
  }
}

export const messageKeys = new MessageKeys();
