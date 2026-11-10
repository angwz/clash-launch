// 同步的 SHA-256（FIPS 180-4），只用于工作量证明：需要每秒上百万次，crypto.subtle 的异步调用太慢。
// 输出写入调用方提供的 Uint32Array(8)，避免在循环中分配内存。正确性由 sha256.test.ts 对照 node:crypto 检查。

const K = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5, 0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3,
  0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174, 0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967, 0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13,
  0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85, 0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3, 0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208,
  0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
]);

const W = new Uint32Array(64);
const BLOCK = new Uint8Array(64);

function compress(h: Uint32Array, b: Uint8Array, o = 0): void {
  for (let i = 0; i < 16; i++) W[i] = (b[o + i * 4]! << 24) | (b[o + i * 4 + 1]! << 16) | (b[o + i * 4 + 2]! << 8) | b[o + i * 4 + 3]!;
  for (let i = 16; i < 64; i++) {
    const w15 = W[i - 15]!;
    const w2 = W[i - 2]!;
    const s0 = ((w15 >>> 7) | (w15 << 25)) ^ ((w15 >>> 18) | (w15 << 14)) ^ (w15 >>> 3);
    const s1 = ((w2 >>> 17) | (w2 << 15)) ^ ((w2 >>> 19) | (w2 << 13)) ^ (w2 >>> 10);
    W[i] = (W[i - 16]! + s0 + W[i - 7]! + s1) | 0;
  }
  let a = h[0]!, bb = h[1]!, c = h[2]!, d = h[3]!, e = h[4]!, f = h[5]!, g = h[6]!, hh = h[7]!;
  for (let i = 0; i < 64; i++) {
    const S1 = ((e >>> 6) | (e << 26)) ^ ((e >>> 11) | (e << 21)) ^ ((e >>> 25) | (e << 7));
    const ch = (e & f) ^ (~e & g);
    const t1 = (hh + S1 + ch + K[i]! + W[i]!) | 0;
    const S0 = ((a >>> 2) | (a << 30)) ^ ((a >>> 13) | (a << 19)) ^ ((a >>> 22) | (a << 10));
    const maj = (a & bb) ^ (a & c) ^ (bb & c);
    const t2 = (S0 + maj) | 0;
    hh = g;
    g = f;
    f = e;
    e = (d + t1) | 0;
    d = c;
    c = bb;
    bb = a;
    a = (t1 + t2) | 0;
  }
  h[0] = (h[0]! + a) | 0;
  h[1] = (h[1]! + bb) | 0;
  h[2] = (h[2]! + c) | 0;
  h[3] = (h[3]! + d) | 0;
  h[4] = (h[4]! + e) | 0;
  h[5] = (h[5]! + f) | 0;
  h[6] = (h[6]! + g) | 0;
  h[7] = (h[7]! + hh) | 0;
}

const IV = new Uint32Array([0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19]);

/** 计算 msg[0:len] 的 SHA-256，结果写入 out（8 个大端 32 位字）。 */
export function sha256(msg: Uint8Array, len: number, out: Uint32Array): void {
  out.set(IV);
  let off = 0;
  for (; off + 64 <= len; off += 64) compress(out, msg, off);
  finish(out, msg, off, len - off, len);
}

/**
 * 从中间状态 out 继续：处理 msg[off:off+rest]（不足一块的剩余部分）并填充，total 为整条消息的字节数。
 * 求解时前缀中完整的块只压缩一次（中间状态），每个 nonce 只需处理尾部，速度约提高一倍。
 */
function finish(out: Uint32Array, msg: Uint8Array, off: number, rest: number, total: number): void {
  if (rest >= 64) {
    compress(out, msg, off);
    off += 64;
    rest -= 64;
  }
  BLOCK.fill(0);
  for (let i = 0; i < rest; i++) BLOCK[i] = msg[off + i]!;
  const len = total;
  BLOCK[rest] = 0x80;
  if (rest >= 56) {
    compress(out, BLOCK);
    BLOCK.fill(0);
  }
  const bits = len * 8;
  BLOCK[60] = (bits >>> 24) & 0xff;
  BLOCK[61] = (bits >>> 16) & 0xff;
  BLOCK[62] = (bits >>> 8) & 0xff;
  BLOCK[63] = bits & 0xff;
  BLOCK[59] = Math.floor(bits / 2 ** 32) & 0xff;
  compress(out, BLOCK);
}

/** 哈希的前导零位数（与后端 auth.LeadingZeroBits 一致）。 */
export function leadingZeroBits(h: Uint32Array): number {
  let n = 0;
  for (let i = 0; i < 8; i++) {
    const v = h[i]! >>> 0;
    if (v !== 0) return n + Math.clz32(v);
    n += 32;
  }
  return n;
}

/** 求解：找出最小的 nonce，使 SHA-256(challenge ":" nonce) 的前导零位数 ≥ bits。onProgress 每 65536 次回调一次。 */
export function solve(challenge: string, bits: number, onProgress?: (tried: number) => void): string {
  const prefix = new TextEncoder().encode(challenge + ":");
  const full = prefix.length - (prefix.length % 64); // 前缀中完整的块
  const mid = new Uint32Array(IV);
  for (let off = 0; off < full; off += 64) compress(mid, prefix, off);
  const tail = new Uint8Array(prefix.length - full + 20); // 前缀余下部分 + nonce 数字
  tail.set(prefix.subarray(full));
  const base = prefix.length - full;
  const out = new Uint32Array(8);
  for (let n = 0; ; n++) {
    const s = String(n);
    for (let i = 0; i < s.length; i++) tail[base + i] = s.charCodeAt(i);
    out.set(mid);
    finish(out, tail, 0, base + s.length, prefix.length + s.length);
    if (leadingZeroBits(out) >= bits) return s;
    if (onProgress && (n & 0xffff) === 0xffff) onProgress(n + 1);
  }
}
