// 会话：/api/v1/me 返回当前用户与 CSRF 令牌；未登录时 user 为 null。
// 登录可能分两步：口令正确且启用了第二因素时，服务端返回 {mfa, ticket} 而不建立会话。
import { api, ApiError, setCsrf, type Session, type User } from "./api/client";

export interface MFAChallenge {
  mfa: ("totp" | "passkey")[];
  ticket: string;
}

class SessionState {
  user = $state<User | null>(null);
  loaded = $state(false);

  get isAdmin(): boolean {
    return this.user?.role === "admin";
  }

  async load(): Promise<void> {
    try {
      const s = await api.get<Session>("/api/v1/me");
      this.#apply(s);
    } catch (e) {
      if (!(e instanceof ApiError) || e.status !== 401) throw e;
      this.user = null;
    } finally {
      this.loaded = true;
    }
  }

  /** 口令登录。需要第二因素时返回挑战，否则返回 null（已登录）。 */
  async login(username: string, password: string): Promise<MFAChallenge | null> {
    const r = await api.post<Session | MFAChallenge>("/api/v1/session", { username, password });
    if ("mfa" in r) return r;
    this.#apply(r);
    return null;
  }

  /** 第二步：TOTP 验证码或恢复码。 */
  async completeTOTP(ticket: string, code: string): Promise<void> {
    this.#apply(await api.post<Session>("/api/v1/session/totp", { ticket, code }));
  }

  /** 由其他方式（如通行密钥）完成登录后，直接采用返回的会话。 */
  adopt(s: Session): void {
    this.#apply(s);
  }

  async logout(): Promise<void> {
    try {
      await api.del("/api/v1/session");
    } finally {
      this.user = null;
      setCsrf("");
    }
  }

  #apply(s: Session): void {
    this.user = s.user;
    setCsrf(s.csrf);
  }
}

export const session = new SessionState();
