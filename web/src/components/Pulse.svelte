<script lang="ts">
  // 在线指示灯（需求 R11 第一步）：一颗约 8 px 的圆点，用明暗与光晕表达与服务器的连接和发生的事。
  //
  // 多个状态同时成立时取优先级最高的（见 docs/development/backlog.md「R11 在线指示灯」）：
  //   断线 > 新版本 > 降级 > 未读消息（呼吸）> 一次性事件（启动、被拉取、恢复）> 操作中 > 静止。
  // 一次性事件叠加在持续状态之上播放一次，不改变持续状态。
  //
  // 约束：闪烁每秒不超过 3 次（WCAG 2.3.1）；动画只改 opacity 与 transform；系统要求减少动态效果时
  // 只保留常亮与熄灭；断线时形状也变（空心），不只靠颜色；读屏软件只播报断线与恢复。
  // 灯本身不响应点击：悬停、键盘聚焦或在手机上轻点时显示说明，3 秒后自动收起（仅轻点）。
  import { onMount } from "svelte";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { realtime } from "../lib/realtime.svelte";
  import { inbox } from "../lib/inbox.svelte";

  let { id }: { id: string } = $props();

  // 断线判定：连接断开超过 5 秒才熄灭，刷新、网络抖动造成的短暂重连不闪烁
  const DOWN_AFTER = 5_000;
  let down = $state(false);
  let everUp = $state(false);
  let downTimer: ReturnType<typeof setTimeout> | undefined;
  let announce = $state(""); // 读屏播报

  // 一次性事件：每次触发换一个序号，{#key} 重建元素让动画从头播放
  let flash = $state<{ kind: "start" | "pull" | "recover"; seq: number } | null>(null);
  let seq = 0;
  const play = (kind: "start" | "pull" | "recover") => (flash = { kind, seq: ++seq });

  $effect(() => {
    if (realtime.connected) {
      clearTimeout(downTimer);
      if (!everUp) play("start");
      else if (down) {
        play("recover");
        announce = t("pulse.recovered");
      }
      everUp = true;
      down = false;
    } else if (everUp && !down) {
      clearTimeout(downTimer);
      downTimer = setTimeout(() => {
        down = true;
        announce = t("pulse.down");
      }, DOWN_AFTER);
    }
  });

  let lastPull = $state<number | undefined>(undefined);
  $effect(() =>
    realtime.on(["sub"], () => {
      lastPull = Date.now();
      play("pull");
    }),
  );

  // 操作中：移动、点击、按键、滚动时亮度上扬，停止约 1.5 秒后回落
  let active = $state(false);
  let activeTimer: ReturnType<typeof setTimeout> | undefined;
  onMount(() => {
    const poke = () => {
      if (!active) active = true;
      clearTimeout(activeTimer);
      activeTimer = setTimeout(() => (active = false), 1_500);
    };
    const opts = { passive: true, capture: true } as const;
    const kinds = ["pointermove", "pointerdown", "keydown", "wheel", "scroll"] as const;
    for (const k of kinds) window.addEventListener(k, poke, opts);
    return () => {
      for (const k of kinds) window.removeEventListener(k, poke, opts);
      clearTimeout(activeTimer);
      clearTimeout(downTimer);
      clearTimeout(tipTimer);
    };
  });

  const newVersion = $derived(!!realtime.pageVersion && !!realtime.serverVersion && realtime.pageVersion !== realtime.serverVersion);
  const slow = $derived((realtime.rtt ?? 0) > 1_500);
  const mode = $derived<"down" | "starting" | "update" | "slow" | "unread" | "active" | "idle">(
    down ? "down" : !everUp ? "starting" : newVersion ? "update" : slow ? "slow" : inbox.unread > 0 ? "unread" : active ? "active" : "idle",
  );

  const label = $derived(
    {
      down: t("pulse.state_down"),
      starting: t("pulse.state_starting"),
      update: t("pulse.state_update"),
      slow: t("pulse.state_slow"),
      unread: t("pulse.state_unread", { n: inbox.unread }),
      active: t("pulse.state_idle"),
      idle: t("pulse.state_idle"),
    }[mode],
  );

  // 说明：悬停、聚焦时显示；手机上轻点显示 3 秒。打开时顺便测一次往返延迟
  // 说明用 fixed 定位：灯在可滚动的侧栏里（收起时只有 56 px 宽），绝对定位会被裁掉
  let tip = $state(false);
  let tipStyle = $state("");
  let el: HTMLSpanElement | undefined = $state();
  let tipTimer: ReturnType<typeof setTimeout> | undefined;
  function show(auto = false) {
    if (!tip) realtime.ping();
    if (el) {
      const r = el.getBoundingClientRect();
      const x = "left: " + Math.max(8, Math.min(r.left - 6, innerWidth - 248)) + "px;";
      // 靠近顶端（顶栏里的灯）时向下展开，否则向上
      tipStyle = r.top < innerHeight / 2 ? x + " top: " + (r.bottom + 8) + "px;" : x + " bottom: " + (innerHeight - r.top + 8) + "px;";
    }
    tip = true;
    clearTimeout(tipTimer);
    if (auto) tipTimer = setTimeout(() => (tip = false), 3_000);
  }
  function hide() {
    clearTimeout(tipTimer);
    tip = false;
  }
</script>

<!-- 灯不响应点击，但键盘用户要能聚焦它来查看说明（aria-describedby），所以可聚焦 -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<span
  bind:this={el}
  class="pulse"
  data-state={mode}
  role="img"
  tabindex="0"
  aria-label={label}
  aria-describedby={tip ? id + "-tip" : undefined}
  onmouseenter={() => show()}
  onmouseleave={hide}
  onfocus={() => show()}
  onblur={hide}
  ontouchstart={() => show(true)}
>
  <span class="halo" aria-hidden="true"></span>
  <span class="core" aria-hidden="true"></span>
  {#if flash && mode !== "down"}
    {#key flash.seq}<span class="flash" data-kind={flash.kind} aria-hidden="true"></span>{/key}
  {/if}
  {#if tip}
    <span class="tip" id={id + "-tip"} role="tooltip" style={tipStyle}>
      <strong>{label}</strong>
      {#if mode === "update"}<span>{t("pulse.update_hint")}</span>{/if}
      {#if mode === "slow"}<span>{t("pulse.slow_hint")}</span>{/if}
      <span class="rows">
        {#if realtime.online !== undefined}<span>{t("pulse.online")}</span><span>{i18n.number(realtime.online)}</span>{/if}
        <span>{t("pulse.rtt")}</span><span>{realtime.rtt !== undefined && realtime.connected ? realtime.rtt + " ms" : "—"}</span>
        <span>{t("pulse.last_pull")}</span><span>{lastPull ? i18n.relative(new Date(lastPull).toISOString()) : t("pulse.not_yet")}</span>
        <span>{t("pulse.unread")}</span><span>{i18n.number(inbox.unread)}</span>
      </span>
    </span>
  {/if}
</span>
<span class="visually-hidden" role="status">{announce}</span>

<style>
  .pulse {
    --c: var(--accent);
    position: relative;
    display: inline-grid;
    place-items: center;
    inline-size: 20px;
    block-size: 20px;
    flex: none;
    border-radius: 50%;
    outline-offset: 2px;
    cursor: default;
    -webkit-tap-highlight-color: transparent;
  }
  .pulse > span:not(.tip) {
    grid-area: 1 / 1;
    border-radius: 50%;
    pointer-events: none;
  }
  .core {
    inline-size: 8px;
    block-size: 8px;
    background: var(--c);
    opacity: 0.85;
    transition:
      opacity 0.6s ease,
      background 0.4s ease;
  }
  /* 柔光：浅色主题下收敛，深色主题下更明显（--pulse-glow 在 tokens 之外按主题取值） */
  .halo {
    inline-size: 8px;
    block-size: 8px;
    background: var(--c);
    opacity: var(--pulse-glow, 0.22);
    transform: scale(2);
    transition: opacity 0.6s ease;
  }
  :global(:root[data-theme="dark"]) .halo {
    --pulse-glow: 0.38;
  }
  @media (prefers-color-scheme: dark) {
    :global(:root:not([data-theme="light"])) .halo {
      --pulse-glow: 0.38;
    }
  }
  .flash {
    inline-size: 8px;
    block-size: 8px;
    background: var(--c);
    opacity: 0;
  }

  [data-state="active"] .core {
    opacity: 1;
  }
  [data-state="active"] .halo {
    opacity: calc(var(--pulse-glow, 0.22) * 1.6);
  }
  [data-state="update"] {
    --c: var(--info);
  }
  [data-state="slow"] {
    --c: var(--warn);
  }
  /* 断线：灯芯熄灭，只留一圈细边框（空心），不只靠颜色表达 */
  [data-state="down"] .core {
    background: transparent;
    box-shadow: inset 0 0 0 1.5px var(--fg-subtle);
    opacity: 1;
  }
  [data-state="down"] .halo {
    opacity: 0;
  }
  [data-state="starting"] .core {
    opacity: 0.35;
  }
  [data-state="starting"] .halo {
    opacity: 0;
  }

  @media (prefers-reduced-motion: no-preference) {
    /* 未读：约 5 秒一个周期的呼吸 */
    [data-state="unread"] .core {
      animation: breathe 5s ease-in-out infinite;
    }
    [data-state="unread"] .halo {
      animation: breathe-halo 5s ease-in-out infinite;
    }
    /* 启动：连闪三下（每秒 2.5 次）后渐亮 */
    .flash[data-kind="start"] {
      animation: blink 0.4s ease-in-out 3;
    }
    /* 被拉取、恢复：快闪两下（每秒 2.5 次） */
    .flash[data-kind="pull"],
    .flash[data-kind="recover"] {
      animation: blink 0.4s ease-in-out 2;
    }
  }
  @keyframes breathe {
    0%,
    100% {
      opacity: 0.35;
    }
    50% {
      opacity: 1;
    }
  }
  @keyframes breathe-halo {
    0%,
    100% {
      opacity: 0;
      transform: scale(1.4);
    }
    50% {
      opacity: calc(var(--pulse-glow, 0.22) * 1.8);
      transform: scale(2.2);
    }
  }
  @keyframes blink {
    0%,
    100% {
      opacity: 0;
      transform: scale(1);
    }
    45% {
      opacity: 1;
      transform: scale(2.4);
    }
  }

  .tip {
    position: fixed;
    z-index: 60;
    display: grid;
    gap: 6px;
    min-inline-size: 200px;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-md);
    color: var(--fg);
    font-size: var(--text-sm);
    line-height: 1.4;
    white-space: nowrap;
  }
  .tip > span:not(.rows) {
    color: var(--fg-muted);
    white-space: normal;
  }
  .rows {
    display: grid;
    grid-template-columns: auto auto;
    justify-content: space-between;
    gap: 2px 16px;
    color: var(--fg-muted);
  }
  .rows > span:nth-child(even) {
    color: var(--fg);
    text-align: end;
    font-variant-numeric: tabular-nums;
  }
</style>
