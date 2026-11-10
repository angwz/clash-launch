<script lang="ts">
  // 二维码：在本地生成（qrcode-generator），渲染为 SVG 路径；不调用任何二维码服务。
  import qrcode from "qrcode-generator";

  let { text, size = 168, label }: { text: string; size?: number; label: string } = $props();

  const model = $derived.by(() => {
    const qr = qrcode(0, "M");
    qr.addData(text, "Byte");
    qr.make();
    const n = qr.getModuleCount();
    let d = "";
    for (let r = 0; r < n; r++) for (let c = 0; c < n; c++) if (qr.isDark(r, c)) d += `M${c + 4} ${r + 4}h1v1h-1z`;
    return { n: n + 8, d };
  });
</script>

<svg class="qr" width={size} height={size} viewBox={`0 0 ${model.n} ${model.n}`} role="img" aria-label={label} shape-rendering="crispEdges">
  <rect width={model.n} height={model.n} fill="#fff" />
  <path d={model.d} fill="#000" />
</svg>

<style>
  .qr {
    display: block;
    border-radius: var(--radius-md);
  }
</style>
