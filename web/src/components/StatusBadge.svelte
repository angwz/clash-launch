<script lang="ts">
  // 状态徽章：颜色与图标同时表达状态（第 16.5 节可访问性）。
  import Emoji, { type EmojiName } from "./Emoji.svelte";
  import Icon, { type IconName } from "./Icon.svelte";

  let { tone, label, icon }: { tone: "ok" | "err" | "warn" | "info" | "muted" | "raw"; label: string; icon?: IconName } = $props();
  const fallback: Record<string, IconName> = { ok: "ok", err: "error", warn: "warning", info: "info", muted: "info", raw: "bolt" };
  // 成功、错误、警告用 Fluent emoji（反馈 #44）；其余保持线性图标。传入 icon 时以调用方为准。
  const emoji: Record<string, EmojiName> = { ok: "sparkles", err: "no-entry", warn: "warning" };
</script>

<span class="badge" data-tone={tone}>{#if !icon && emoji[tone]}<Emoji name={emoji[tone]!} size={13} />{:else}<Icon name={icon ?? fallback[tone]!} size={13} />{/if}{label}</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    block-size: 22px;
    padding-inline: 7px 8px;
    border-radius: var(--radius-full);
    font-size: var(--text-xs);
    font-weight: 560;
    white-space: nowrap;
  }
  [data-tone="ok"] {
    background: var(--ok-soft);
    color: var(--ok);
  }
  [data-tone="err"] {
    background: var(--err-soft);
    color: var(--err);
  }
  [data-tone="warn"] {
    background: var(--warn-soft);
    color: var(--warn);
  }
  [data-tone="info"] {
    background: var(--info-soft);
    color: var(--info);
  }
  [data-tone="muted"] {
    background: var(--bg-sunken);
    color: var(--fg-muted);
  }
  [data-tone="raw"] {
    background: var(--raw-soft);
    color: var(--raw);
  }
</style>
