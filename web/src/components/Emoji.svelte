<script lang="ts" module>
  // 状态 emoji（反馈 #44）：Fluent Emoji Flat（MIT），随站点打包为独立的 SVG 文件，按需由浏览器加载。
  // 只用于表达状态的少数位置（徽章、提示条、空状态、庆祝），装饰性：文字本身已说明含义，读屏器忽略图片。
  const files = import.meta.glob<string>("../assets/emoji/*.svg", { eager: true, query: "?url", import: "default" });
  const urls = Object.fromEntries(Object.entries(files).map(([p, u]) => [p.slice(p.lastIndexOf("/") + 1, -4), u]));

  export type EmojiName =
    | "check" | "sparkles" | "party" | "confetti" | "trophy" | "rocket"
    | "prohibited" | "no-entry" | "warning" | "thinking" | "cross" | "mailbox";
</script>

<script lang="ts">
  let { name, size = 16 }: { name: EmojiName; size?: number } = $props();
</script>

<img class="emoji" src={urls[name]} width={size} height={size} alt="" aria-hidden="true" draggable="false" />

<style>
  .emoji {
    flex: none;
    display: inline-block;
    vertical-align: -0.15em;
    user-select: none;
  }
</style>
