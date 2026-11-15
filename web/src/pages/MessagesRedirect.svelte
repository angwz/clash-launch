<script lang="ts">
  // 旧地址 /messages、/messages/<用户名>（反馈 #79）：消息改为悬浮窗后，这两个地址打开悬浮窗并回到首页。
  import { onMount } from "svelte";
  import { chat } from "../lib/chat.svelte";
  import { router } from "../lib/router.svelte";

  let { params }: { params: Record<string, string> } = $props();

  onMount(() => {
    chat.show(params.username ? decodeURIComponent(params.username) : "");
    router.go("/", true);
  });
</script>
