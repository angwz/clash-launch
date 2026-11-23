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

// @replit/codemirror-minimap@0.5.2 取样式时给临时元素写 style 属性（style: "display: none"），
// 违反 CSP 的 style-src 'self'（反馈 #114 的缩略图）。该元素在同一个同步过程中插入、读取样式后立即移除，
// 不会被绘制，去掉这个属性不影响结果。只改这一处；升级该包时确认此处仍然存在（找不到即构建失败，避免悄悄失效）。
const minimapCsp: Plugin = {
  name: "minimap-csp",
  transform(code, id) {
    if (!id.includes("@replit/codemirror-minimap")) return null;
    const from = `crelt("div", { class: "cm-line", style: "display: none" }, mockToken)`;
    if (!code.includes(from)) {
      if (code.includes("cm-minimap")) throw new Error("minimap-csp：@replit/codemirror-minimap 的代码已变化，请检查 vite.config.ts 中的补丁");
      return null;
    }
    return code.replace(from, `crelt("div", { class: "cm-line" }, mockToken)`);
  },
};

export default defineConfig({
  plugins: [svelte(), keepDist, minimapCsp],
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
