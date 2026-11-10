// 工作量证明的求解在 Worker 中进行，不阻塞界面。消息：{challenge, bits} → {progress} … {nonce}
import { solve } from "./sha256";

// tsconfig 使用 DOM 类型库：这里把 self 视为 Worker 的全局作用域
const ctx = self as unknown as {
  onmessage: ((e: MessageEvent<{ challenge: string; bits: number }>) => void) | null;
  postMessage(message: unknown): void;
};

ctx.onmessage = (e) => {
  const { challenge, bits } = e.data;
  const nonce = solve(challenge, bits, (tried) => ctx.postMessage({ progress: tried }));
  ctx.postMessage({ nonce });
};
