// 站内消息的端到端加密（需求 R46，决定 0009）：只用浏览器自带的 Web Crypto，不引入第三方库。
//
// - 每个用户一对 ECDH P-256 密钥。公钥（SPKI，Base64URL）存服务器；私钥只以包裹后的形式离开浏览器。
// - 包裹：PBKDF2-SHA256（登录密码，16 字节盐，600 000 次）→ AES-256-GCM；恢复密钥（32 字节随机数）经 HKDF-SHA256 得到第二把。
// - 本机：解开后的私钥以不可导出的 CryptoKey 存在 IndexedDB（按用户），退出登录时删除。
// - 消息：ECDH（自己的私钥，对方的公钥）→ HKDF-SHA256（信息串含双方密钥编号）→ AES-256-GCM，每条随机 12 字节 IV。

export const PBKDF2_ITER = 600_000;
const enc = new TextEncoder();
const dec = new TextDecoder();

export interface Wrapped {
  v: 1;
  kdf: "pbkdf2" | "hkdf";
  salt: string;
  iter?: number;
  iv: string;
  ct: string;
}

export interface EncText {
  v: 1;
  sk: string; // 发件人的密钥编号
  rk: string; // 收件人的密钥编号
  iv: string;
  ct: string;
}

// —— Base64URL（无填充，与服务端 base64.RawURLEncoding 一致）——

export function b64(buf: ArrayBuffer | Uint8Array): string {
  const bytes = buf instanceof Uint8Array ? buf : new Uint8Array(buf);
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export function unb64(s: string): Uint8Array<ArrayBuffer> {
  const t = s.replace(/-/g, "+").replace(/_/g, "/");
  const bin = atob(t + "=".repeat((4 - (t.length % 4)) % 4));
  const out = new Uint8Array(new ArrayBuffer(bin.length));
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function random(n: number): Uint8Array<ArrayBuffer> {
  return crypto.getRandomValues(new Uint8Array(new ArrayBuffer(n)));
}

// —— 密钥 ——

const EC = { name: "ECDH", namedCurve: "P-256" } as const;

/** 生成一对新密钥（私钥可导出：生成后要立即包裹上传）。 */
export async function generateKeyPair(): Promise<{ priv: CryptoKey; publicKey: string }> {
  const pair = await crypto.subtle.generateKey(EC, true, ["deriveBits"]);
  return { priv: pair.privateKey, publicKey: b64(await crypto.subtle.exportKey("spki", pair.publicKey)) };
}

export function importPublic(spki: string): Promise<CryptoKey> {
  return crypto.subtle.importKey("spki", unb64(spki), EC, false, []);
}

async function passwordKey(password: string, salt: Uint8Array<ArrayBuffer>, iter: number): Promise<CryptoKey> {
  const base = await crypto.subtle.importKey("raw", enc.encode(password), "PBKDF2", false, ["deriveKey"]);
  return crypto.subtle.deriveKey({ name: "PBKDF2", hash: "SHA-256", salt, iterations: iter }, base, { name: "AES-GCM", length: 256 }, false, ["encrypt", "decrypt"]);
}

async function recoveryKey(code: Uint8Array<ArrayBuffer>, salt: Uint8Array<ArrayBuffer>): Promise<CryptoKey> {
  const base = await crypto.subtle.importKey("raw", code, "HKDF", false, ["deriveKey"]);
  return crypto.subtle.deriveKey({ name: "HKDF", hash: "SHA-256", salt, info: enc.encode("clash-launch recovery v1") }, base, { name: "AES-GCM", length: 256 }, false, ["encrypt", "decrypt"]);
}

async function seal(priv: CryptoKey, key: CryptoKey): Promise<{ iv: string; ct: string }> {
  const iv = random(12);
  const pkcs8 = await crypto.subtle.exportKey("pkcs8", priv);
  return { iv: b64(iv), ct: b64(await crypto.subtle.encrypt({ name: "AES-GCM", iv }, key, pkcs8)) };
}

async function open(w: Wrapped, key: CryptoKey, extractable: boolean): Promise<CryptoKey> {
  const pkcs8 = await crypto.subtle.decrypt({ name: "AES-GCM", iv: unb64(w.iv) }, key, unb64(w.ct));
  return crypto.subtle.importKey("pkcs8", pkcs8, EC, extractable, ["deriveBits"]);
}

/** 用登录密码包裹私钥（私钥须可导出）。 */
export async function wrapWithPassword(priv: CryptoKey, password: string): Promise<Wrapped> {
  const salt = random(16);
  return { v: 1, kdf: "pbkdf2", salt: b64(salt), iter: PBKDF2_ITER, ...(await seal(priv, await passwordKey(password, salt, PBKDF2_ITER))) };
}

/** 用密码解开；密码不对时抛出（AES-GCM 认证失败）。extractable 为真时可以再次包裹（改密码、生成恢复密钥）。 */
export async function unwrapWithPassword(w: Wrapped, password: string, extractable = false): Promise<CryptoKey> {
  if (w.kdf !== "pbkdf2" || !w.iter) throw new Error("not a password wrap");
  return open(w, await passwordKey(password, unb64(w.salt), w.iter), extractable);
}

// —— 恢复密钥：32 字节随机数，以 Base32 显示（去掉易混的字符，每 4 位一组）——

const ALPHABET = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"; // 32 个，没有 I、O、0、1

export function newRecoveryCode(): { code: string; bytes: Uint8Array<ArrayBuffer> } {
  const bytes = random(32);
  let bits = "";
  for (const b of bytes) bits += b.toString(2).padStart(8, "0");
  let s = "";
  for (let i = 0; i < bits.length; i += 5) s += ALPHABET[parseInt(bits.slice(i, i + 5).padEnd(5, "0"), 2)];
  return { code: s.match(/.{1,4}/g)!.join("-"), bytes };
}

export function parseRecoveryCode(code: string): Uint8Array<ArrayBuffer> | null {
  const s = code.toUpperCase().replace(/[^A-Z0-9]/g, "");
  let bits = "";
  for (const c of s) {
    const i = ALPHABET.indexOf(c);
    if (i < 0) return null;
    bits += i.toString(2).padStart(5, "0");
  }
  if (bits.length < 256) return null;
  const out = new Uint8Array(new ArrayBuffer(32));
  for (let i = 0; i < 32; i++) out[i] = parseInt(bits.slice(i * 8, i * 8 + 8), 2);
  return out;
}

export async function wrapWithRecovery(priv: CryptoKey, code: Uint8Array<ArrayBuffer>): Promise<Wrapped> {
  const salt = random(16);
  return { v: 1, kdf: "hkdf", salt: b64(salt), ...(await seal(priv, await recoveryKey(code, salt))) };
}

export async function unwrapWithRecovery(w: Wrapped, code: Uint8Array<ArrayBuffer>, extractable = false): Promise<CryptoKey> {
  if (w.kdf !== "hkdf") throw new Error("not a recovery wrap");
  return open(w, await recoveryKey(code, unb64(w.salt)), extractable);
}

// —— 消息 ——

async function messageKey(priv: CryptoKey, peer: CryptoKey, sk: string, rk: string): Promise<CryptoKey> {
  const shared = await crypto.subtle.deriveBits({ name: "ECDH", public: peer }, priv, 256);
  const base = await crypto.subtle.importKey("raw", shared, "HKDF", false, ["deriveKey"]);
  // 信息串含双方的密钥编号：同一对用户换了密钥后得到不同的消息密钥
  const info = enc.encode("clash-launch message v1|" + sk + "|" + rk);
  return crypto.subtle.deriveKey({ name: "HKDF", hash: "SHA-256", salt: new Uint8Array(new ArrayBuffer(0)), info }, base, { name: "AES-GCM", length: 256 }, false, ["encrypt", "decrypt"]);
}

export async function encryptText(text: string, my: { keyId: string; priv: CryptoKey }, peer: { keyId: string; pub: CryptoKey }): Promise<EncText> {
  const key = await messageKey(my.priv, peer.pub, my.keyId, peer.keyId);
  const iv = random(12);
  const ct = await crypto.subtle.encrypt({ name: "AES-GCM", iv }, key, enc.encode(text));
  return { v: 1, sk: my.keyId, rk: peer.keyId, iv: b64(iv), ct: b64(ct) };
}

/**
 * 解开一条加密消息。mine 为真表示是自己发的：自己的密钥是 sk、对方的是 rk；否则相反。
 * 自己的私钥只有当前一把（重新生成后旧私钥已不存在），对应不上时抛出。
 */
export async function decryptText(e: EncText, mine: boolean, my: { keyId: string; priv: CryptoKey }, peerPub: (keyId: string) => Promise<CryptoKey | null>): Promise<string> {
  const myKid = mine ? e.sk : e.rk;
  const peerKid = mine ? e.rk : e.sk;
  if (myKid !== my.keyId) throw new Error("key replaced");
  const pub = await peerPub(peerKid);
  if (!pub) throw new Error("peer key missing");
  const key = await messageKey(my.priv, pub, e.sk, e.rk);
  return dec.decode(await crypto.subtle.decrypt({ name: "AES-GCM", iv: unb64(e.iv) }, key, unb64(e.ct)));
}

// —— 本机保存：IndexedDB 中不可导出的 CryptoKey ——

const DB = "clash-launch-e2e";
const STORE = "keys";

function idb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const r = indexedDB.open(DB, 1);
    r.onupgradeneeded = () => r.result.createObjectStore(STORE);
    r.onsuccess = () => resolve(r.result);
    r.onerror = () => reject(r.error);
  });
}

function tx<T>(mode: IDBTransactionMode, fn: (s: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  return idb().then(
    (db) =>
      new Promise<T>((resolve, reject) => {
        const t = db.transaction(STORE, mode);
        const r = fn(t.objectStore(STORE));
        t.oncomplete = () => {
          db.close();
          resolve(r.result);
        };
        t.onerror = () => {
          db.close();
          reject(t.error);
        };
      }),
  );
}

export interface LocalKey {
  keyId: string;
  priv: CryptoKey; // 不可导出
}

export async function saveLocal(userId: string, k: LocalKey): Promise<void> {
  await tx("readwrite", (s) => s.put(k, userId));
}

export async function loadLocal(userId: string): Promise<LocalKey | null> {
  return ((await tx("readonly", (s) => s.get(userId))) as LocalKey | undefined) ?? null;
}

/** 退出登录时删除本机的全部私钥。 */
export async function clearLocal(): Promise<void> {
  await tx("readwrite", (s) => s.clear());
}

/** 把可导出的私钥转为不可导出的副本再保存（本机只留不可导出的）。 */
export async function toLocal(priv: CryptoKey): Promise<CryptoKey> {
  const pkcs8 = await crypto.subtle.exportKey("pkcs8", priv);
  return crypto.subtle.importKey("pkcs8", pkcs8, EC, false, ["deriveBits"]);
}
