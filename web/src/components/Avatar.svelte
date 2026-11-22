<script lang="ts">
  // 头像（需求 R19）：预置图形（内联 SVG，只用属性着色，符合 style-src 'self'）、上传的图片，或首字母。
  // 头像是装饰：旁边总有昵称或用户名，因此对读屏软件隐藏。
  import { avatarView, type AvatarUser } from "../lib/avatars";

  let { user, size = 32 }: { user: AvatarUser; size?: number } = $props();
  const v = $derived(avatarView(user));
  const W = "#fff";
</script>

{#if v.kind === "image"}
  <img class="avatar" src={v.src} alt="" width={size} height={size} loading="lazy" decoding="async" aria-hidden="true" />
{:else if v.kind === "preset"}
  <svg class="avatar" viewBox="0 0 48 48" width={size} height={size} aria-hidden="true">
    <rect width="48" height="48" fill={v.color} />
    {#if v.shape === "rocket"}
      <path d="M24 8c5.5 4.5 7.5 10.5 7.5 17.5V32h-15v-6.5C16.5 18.5 18.5 12.5 24 8z" fill={W} />
      <circle cx="24" cy="20" r="3" fill={v.color} />
      <path d="M16.5 26 12 32.5V37l4.5-2.5zM31.5 26 36 32.5V37l-4.5-2.5z" fill={W} opacity="0.75" />
      <path d="M20.5 33.5h7l-1.5 5-2 3-2-3z" fill={W} opacity="0.6" />
    {:else if v.shape === "planet"}
      <circle cx="24" cy="24" r="9" fill={W} />
      <ellipse cx="24" cy="24" rx="16" ry="5" transform="rotate(-20 24 24)" fill="none" stroke={W} stroke-width="2.5" opacity="0.75" />
      <circle cx="21" cy="21" r="2" fill={v.color} opacity="0.35" />
    {:else if v.shape === "comet"}
      <g stroke={W} stroke-linecap="round" fill="none">
        <path d="M27 21 11 37" stroke-width="3.5" opacity="0.8" />
        <path d="M30 24 19 39" stroke-width="2" opacity="0.5" />
        <path d="M24 18 9 27" stroke-width="2" opacity="0.5" />
      </g>
      <circle cx="30" cy="18" r="6" fill={W} />
    {:else if v.shape === "satellite"}
      <rect x="19.5" y="19.5" width="9" height="9" rx="1.5" fill={W} />
      <rect x="7" y="20.5" width="10" height="7" rx="1" fill={W} opacity="0.75" />
      <rect x="31" y="20.5" width="10" height="7" rx="1" fill={W} opacity="0.75" />
      <path d="M17 24h2.5M28.5 24H31M24 19.5V13" stroke={W} stroke-width="2" stroke-linecap="round" />
      <circle cx="24" cy="11.5" r="2.5" fill={W} />
    {:else if v.shape === "constellation"}
      <path d="M12 31 19 18l9 6 8-11M28 24l6 11" fill="none" stroke={W} stroke-width="1.5" stroke-linejoin="round" opacity="0.55" />
      <circle cx="12" cy="31" r="2.5" fill={W} />
      <circle cx="19" cy="18" r="2.5" fill={W} />
      <circle cx="28" cy="24" r="3.5" fill={W} />
      <circle cx="36" cy="13" r="2.5" fill={W} />
      <circle cx="34" cy="35" r="2.5" fill={W} />
    {:else if v.shape === "launchpad"}
      <rect x="11" y="11" width="3" height="26" fill={W} opacity="0.75" />
      <path d="M14 16h6M14 23h6M14 30h6" stroke={W} stroke-width="2" opacity="0.75" />
      <path d="M28 10c3.5 3 5 7.5 5 12.5V31h-10v-8.5c0-5 1.5-9.5 5-12.5z" fill={W} />
      <circle cx="28" cy="19" r="2" fill={v.color} />
      <rect x="7" y="36" width="34" height="3.5" rx="1.75" fill={W} />
    {:else if v.shape === "moon"}
      <path d="M27 9a15 15 0 1 0 12 22.5A12 12 0 0 1 27 9z" fill={W} />
      <circle cx="35" cy="12" r="1.6" fill={W} opacity="0.75" />
      <circle cx="39" cy="20" r="1.1" fill={W} opacity="0.6" />
    {:else if v.shape === "sun"}
      <circle cx="24" cy="24" r="7.5" fill={W} />
      <g stroke={W} stroke-width="2.5" stroke-linecap="round" opacity="0.8">
        <path d="M24 9v4M24 35v4M9 24h4M35 24h4M13.4 13.4l2.8 2.8M31.8 31.8l2.8 2.8M13.4 34.6l2.8-2.8M31.8 16.2l2.8-2.8" />
      </g>
    {:else if v.shape === "telescope"}
      <path d="M9 23.5 29 15l3 7-20 8.5z" fill={W} />
      <path d="M29 15l5-2 3 7-5 2z" fill={W} opacity="0.75" />
      <path d="M21 27l-5 12M21 27l6 12" stroke={W} stroke-width="2.2" stroke-linecap="round" opacity="0.75" />
      <circle cx="38" cy="9" r="1.8" fill={W} />
    {:else if v.shape === "orbit"}
      <g fill="none" stroke={W} stroke-width="2" opacity="0.7">
        <ellipse cx="24" cy="24" rx="16" ry="6.5" transform="rotate(30 24 24)" />
        <ellipse cx="24" cy="24" rx="16" ry="6.5" transform="rotate(-30 24 24)" />
      </g>
      <circle cx="24" cy="24" r="4.5" fill={W} />
      <circle cx="37.9" cy="32" r="2.5" fill={W} />
    {/if}
  </svg>
{:else}
  <span class="avatar initial" style:--s={size + "px"} aria-hidden="true">{v.letter}</span>
{/if}

<style>
  .avatar {
    display: inline-block;
    flex: none;
    border-radius: 50%;
    object-fit: cover;
    vertical-align: middle;
  }
  .initial {
    display: inline-grid;
    place-items: center;
    inline-size: var(--s);
    block-size: var(--s);
    background: var(--bg-active);
    color: var(--fg-muted);
    font-size: calc(var(--s) * 0.45);
    font-weight: 600;
  }
</style>
