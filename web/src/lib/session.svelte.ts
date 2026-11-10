// 会话：/api/v1/me 返回当前用户与 CSRF 令牌；未登录时 user 为 null。
import { api, ApiError, setCsrf, type Session, type User } from "./api/client";

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

  async login(username: string, password: string): Promise<void> {
    this.#apply(await api.post<Session>("/api/v1/session", { username, password }));
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
