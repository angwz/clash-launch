/// <reference types="vitest/config" />
import { renameSync, rmSync, writeFileSync } from "node:fs";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { defineConfig, type Plugin } from "vite";

// 构建后：
// - 补回 dist/.gitkeep：Go 的 //go:embed all:dist 需要目录在未构建前端时也存在；
// - 把构建清单移出 dist（all: 会嵌入点目录，清单不应由服务端提供），供 scripts/budget.mjs 使用。
const keepDist: Plugin = {
  name: "keep-dist",
  apply: "build",
  closeBundle() {
    writeFileSync("dist/.gitkeep", "");
    renameSync("dist/.vite/manifest.json", "build-manifest.json");
    rmSync("dist/.vite", { recursive: true, force: true });
  },
};

export default defineConfig({
  plugins: [svelte(), keepDist],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "es2022",
    // CSP 为 script-src 'self'：不内联任何脚本，资源全部带内容哈希。
    assetsInlineLimit: 0,
    modulePreload: { polyfill: false },
    sourcemap: false,
    // 预算检查（scripts/budget.mjs）根据清单计算首屏的静态依赖
    manifest: true,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:18080",
      "/s/": "http://127.0.0.1:18080",
      "/p/": "http://127.0.0.1:18080",
    },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.ts"],
  },
});
