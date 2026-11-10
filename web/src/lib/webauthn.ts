// 通行密钥（WebAuthn）的浏览器端：把服务端的 JSON 选项（base64url）转换为 API 需要的 ArrayBuffer，
// 并把结果编码回 base64url。验证全部在服务端完成（internal/auth/webauthn.go）。
import { api, type Session } from "./api/client";

const toBuf = (s: string): ArrayBuffer => {
  const b64 = s.replace(/-/g, "+").replace(/_/g, "/") + "===".slice((s.length + 3) % 4);
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out.buffer;
};

const toB64 = (b: ArrayBuffer | null): string => {
  if (!b) return "";
  let s = "";
  for (const c of new Uint8Array(b)) s += String.fromCharCode(c);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
};

/** 浏览器是否支持通行密钥。 */
export function passkeysSupported(): boolean {
  return typeof window !== "undefined" && "PublicKeyCredential" in window && !!navigator.credentials;
}

interface Begin {
  options: Record<string, unknown>;
  ticket: string;
}

type Desc = { type: "public-key"; id: string };

/** 注册一个通行密钥（需要已登录）。 */
export async function registerPasskey(name: string): Promise<void> {
  const { options: o, ticket } = await api.post<Begin>("/api/v1/account/passkeys/begin");
  const user = o.user as { id: string; name: string; displayName: string };
  const publicKey: PublicKeyCredentialCreationOptions = {
    ...(o as unknown as PublicKeyCredentialCreationOptions),
    challenge: toBuf(o.challenge as string),
    user: { ...user, id: toBuf(user.id) },
    excludeCredentials: (o.excludeCredentials as Desc[]).map((c) => ({ type: c.type, id: toBuf(c.id) })),
  };
  const cred = (await navigator.credentials.create({ publicKey })) as PublicKeyCredential | null;
  if (!cred) throw new Error("cancelled");
  const r = cred.response as AuthenticatorAttestationResponse;
  await api.post("/api/v1/account/passkeys", {
    ticket,
    name,
    credential: { clientDataJSON: toB64(r.clientDataJSON), attestationObject: toB64(r.attestationObject) },
  });
}

/** 用通行密钥登录。mfaTicket 为口令登录返回的第二步凭证；为空时是免密码登录。 */
export async function passkeyLogin(mfaTicket = ""): Promise<Session> {
  const { options: o, ticket } = await api.post<Begin>("/api/v1/session/passkey/begin", { ticket: mfaTicket || undefined });
  const publicKey: PublicKeyCredentialRequestOptions = {
    ...(o as unknown as PublicKeyCredentialRequestOptions),
    challenge: toBuf(o.challenge as string),
    allowCredentials: (o.allowCredentials as Desc[]).map((c) => ({ type: c.type, id: toBuf(c.id) })),
  };
  const cred = (await navigator.credentials.get({ publicKey })) as PublicKeyCredential | null;
  if (!cred) throw new Error("cancelled");
  const r = cred.response as AuthenticatorAssertionResponse;
  return api.post<Session>("/api/v1/session/passkey", {
    ticket,
    credential: {
      id: toB64(cred.rawId),
      clientDataJSON: toB64(r.clientDataJSON),
      authenticatorData: toB64(r.authenticatorData),
      signature: toB64(r.signature),
      userHandle: toB64(r.userHandle),
    },
  });
}

/** 用户取消或超时（DOMException NotAllowedError）时不显示为错误。 */
export function isCancelled(e: unknown): boolean {
  return (e instanceof DOMException && (e.name === "NotAllowedError" || e.name === "AbortError")) || (e instanceof Error && e.message === "cancelled");
}
