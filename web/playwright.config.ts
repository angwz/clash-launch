// 端到端测试（第 16.6 节）：桌面、手机视口、深色主题与伪 RTL；后端是真实二进制。
// 浏览器：默认使用本机已安装的 Edge（channel: msedge），不下载浏览器；E2E_CHANNEL 可改为 chrome 或留空使用 Playwright 自带的 Chromium。
import { defineConfig, devices } from "@playwright/test";

const port = process.env.E2E_PORT ?? "18181";
const channel = process.env.E2E_CHANNEL ?? "msedge";

export default defineConfig({
  testDir: "e2e",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  use: {
    // localhost 而不是 127.0.0.1：WebAuthn 不接受 IP 地址作为 RP ID（通行密钥测试需要）
    baseURL: `http://localhost:${port}`,
    locale: "zh-CN",
    trace: "retain-on-failure",
    ...(channel ? { channel } : {}),
  },
  projects: [
    { name: "desktop", use: { viewport: { width: 1440, height: 900 } } },
    { name: "mobile", use: { ...devices["Pixel 7"], ...(channel ? { channel } : {}), locale: "zh-CN" } },
  ],
  webServer: {
    command: "node e2e/serve.mjs",
    url: `http://127.0.0.1:${port}/readyz`,
    timeout: 180_000,
    reuseExistingServer: false,
    env: { E2E_PORT: port },
  },
});
