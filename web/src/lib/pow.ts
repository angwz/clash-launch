// 工作量证明（第 14.1 节）：取挑战 → Worker 求解 → 随注册或找回请求提交。约 1 秒，不依赖第三方验证码服务。
import { api } from "./api/client";

export interface Proof {
  challenge: string;
  nonce: string;
}

/** onProgress 收到 0–1 的估计进度（按期望尝试次数 2^bits 估算，可能超过 1 后才完成）。 */
export async function proofOfWork(purpose: "register" | "recover", onProgress?: (p: number) => void): Promise<Proof> {
  const { challenge, bits } = await api.get<{ challenge: string; bits: number }>("/api/v1/pow", { query: { purpose } });
  const worker = new Worker(new URL("./pow.worker.ts", import.meta.url), { type: "module" });
  try {
    const nonce = await new Promise<string>((resolve, reject) => {
      worker.onmessage = (e: MessageEvent<{ progress?: number; nonce?: string }>) => {
        if (e.data.nonce !== undefined) resolve(e.data.nonce);
        else if (e.data.progress !== undefined) onProgress?.(Math.min(0.99, e.data.progress / 2 ** bits));
      };
      worker.onerror = (e) => reject(new Error(e.message));
      worker.postMessage({ challenge, bits });
    });
    onProgress?.(1);
    return { challenge, nonce };
  } finally {
    worker.terminate();
  }
}
