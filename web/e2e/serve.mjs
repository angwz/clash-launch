// 端到端测试的后端（第 16.6 节：使用真实二进制与临时数据目录）：
// 构建嵌入了 web/dist 的二进制 → 临时数据目录 → 创建管理员 → 启动服务并保持前台运行。
// 前提：已执行 npm run build。端口由 E2E_PORT 指定（默认 18181）。
import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

const root = resolve(import.meta.dirname, "..", "..");
const port = process.env.E2E_PORT ?? "18181";
const exe = join(root, ".cache", "bin", process.platform === "win32" ? "clash-launch-e2e.exe" : "clash-launch-e2e");
const data = mkdtempSync(join(tmpdir(), "clash-launch-e2e-"));

execFileSync("go", ["build", "-o", exe, "./cmd/clash-launch"], { cwd: root, stdio: "inherit" });
const env = { ...process.env, CLASH_LAUNCH_DATA_DIR: data, CLASH_LAUNCH_LISTEN: "127.0.0.1:" + port, CLASH_LAUNCH_PUBLIC_URL: "http://127.0.0.1:" + port, CLASH_LAUNCH_LOG_LEVEL: "warn" };
execFileSync(exe, ["admin", "create", "-username", "admin"], { env, input: "admin password 2026\n", stdio: ["pipe", "inherit", "inherit"] });

const child = spawn(exe, ["serve"], { env, stdio: "inherit" });
const stop = () => {
  child.kill();
  rmSync(data, { recursive: true, force: true });
};
process.on("SIGINT", stop);
process.on("SIGTERM", stop);
process.on("exit", stop);
child.on("exit", (code) => process.exit(code ?? 0));
