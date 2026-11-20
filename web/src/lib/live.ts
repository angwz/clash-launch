// 页面数据的轻量自动刷新（反馈 #73）：数据会在别处变化的页面（请求记录、订阅拉取状态、全站统计）
// 不必手动刷新浏览器。
//
// - 页面可见时每 interval 毫秒刷新一次；标签页隐藏时暂停，切回或窗口重新获得焦点时立即补一次；
// - 刷新进行中不叠加新的请求；刷新失败静默（下次再试），不打扰用户；
// - 自己刚做了会改变数据的操作时调用 soon()，稍等片刻后刷新（服务端统计每分钟落盘，但查询前会先落盘）。
//
// 用法：在 $effect 中调用并返回清理函数：
//   $effect(() => live(load, 30_000).stop);
// 实时通道（R22）：传入 topics 时，对应事件到达立即刷新；定时刷新保留为兜底（通道连不上时照常工作）。

import { realtime, type Topic } from "./realtime.svelte";

export interface Live {
  /** 稍后刷新一次（默认 1.5 秒后），用于自己刚做完操作 */
  soon: (delay?: number) => void;
  stop: () => void;
}

export function live(load: () => Promise<unknown>, interval: number, topics: readonly Topic[] = []): Live {
  let running = false;
  let again = false; // 刷新进行中又来了事件：结束后再刷新一次，不丢最后的变化
  let last = Date.now();
  let timer: ReturnType<typeof setInterval> | undefined;
  let once: ReturnType<typeof setTimeout> | undefined;

  const run = async () => {
    if (running) {
      again = true;
      return;
    }
    running = true;
    last = Date.now();
    try {
      await load();
    } catch {
      /* 静默：下一轮再试 */
    } finally {
      running = false;
      if (again) {
        again = false;
        void run();
      }
    }
  };
  const visible = () => typeof document === "undefined" || document.visibilityState === "visible";
  const tick = () => {
    if (visible()) void run();
  };
  // 切回页面时：距离上次刷新超过 5 秒才补一次，避免来回切换造成连续请求
  const wake = () => {
    if (visible() && Date.now() - last > 5_000) void run();
  };

  timer = setInterval(tick, interval);
  // 页面隐藏时也接收事件，但只在可见时刷新；切回时 wake() 会补上
  const off = topics.length ? realtime.on(topics, tick) : undefined;
  document.addEventListener("visibilitychange", wake);
  window.addEventListener("focus", wake);

  return {
    soon(delay = 1_500) {
      clearTimeout(once);
      once = setTimeout(() => void run(), delay);
    },
    stop() {
      clearInterval(timer);
      clearTimeout(once);
      timer = undefined;
      document.removeEventListener("visibilitychange", wake);
      window.removeEventListener("focus", wake);
      off?.();
    },
  };
}
