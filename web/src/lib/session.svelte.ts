// 会话：/api/v1/me 返回当前用户与 CSRF 令牌；未登录时 user 为 null。
// 登录可能分两步：密码正确且启用了第二因素时，服务端返回 {mfa, ticket} 而不建立会话。
import { api, ApiError, setCsrf, type Schemas, type Session, type User } from "./api/client";
import { i18n } from "./i18n/i18n.svelte";

export interface MFAChallenge {
  mfa: ("totp" | "passkey")[];
  ticket: string;
}

class SessionState {
  user = $state<User | null>(null);
  /** 当前设备的定时退出时间（需求 R8）；没有定时为 null */
  logoutAt = $state<string | null>(null);
  loaded = $state(false);
  /** 用户协议的同意状态（需求 R40）：pending 时页面先显示协议，其他接口在同意前返回 403 */
  terms = $state<Schemas["TermsState"] | null>(null);

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

  /** 密码登录。需要第二因素时返回挑战，否则返回 null（已登录）。 */
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

  /** 同意或拒绝当前版本的用户协议。版本过期时抛出 409（调用方重新读取协议）。 */
  async answerTerms(version: number, accept: boolean): Promise<void> {
    if (!accept) {
      await api.post("/api/v1/account/terms", { version, accept });
      return;
    }
    this.terms = await api.post<Schemas["TermsState"]>("/api/v1/account/terms", { version, accept });
  }

  async logout(): Promise<void> {
    try {
      await api.del("/api/v1/session");
    } finally {
      this.user = null;
      this.terms = null;
      i18n.timeZone = undefined;
      setCsrf("");
    }
  }

  /** 保存时区（反馈 #72）：空字符串表示跟随浏览器。界面上的时间立即按新时区显示。 */
  async setTimezone(tz: string): Promise<void> {
    const r = await api.put<{ timezone: string }>("/api/v1/account/timezone", { timezone: tz });
    if (this.user) this.user = { ...this.user, timezone: r.timezone };
    i18n.timeZone = r.timezone || undefined;
  }

  #apply(s: Session): void {
    this.user = s.user;
    this.logoutAt = s.logoutAt ?? null;
    this.terms = s.terms;
    i18n.timeZone = s.user?.timezone || undefined;
    setCsrf(s.csrf);
  }
}

export const session = new SessionState();
