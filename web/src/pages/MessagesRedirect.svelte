<script lang="ts">
  // 旧地址 /messages、/messages/<用户名>（反馈 #79）：消息改为悬浮窗后，这两个地址打开悬浮窗并回到首页。
  // 对方改过用户名时（O10），先把旧名解析为现在的用户名；解析不到就按原样打开（悬浮窗里会提示找不到）。
  import { onMount } from "svelte";
  import { api } from "../lib/api/client";
  import { chat } from "../lib/chat.svelte";
  import { router } from "../lib/router.svelte";

  let { params }: { params: Record<string, string> } = $props();

  onMount(() => {
    const name = params.username ? decodeURIComponent(params.username) : "";
    router.go("/", true);
    if (!name) {
      chat.show("");
      return;
    }
    api.get<{ username: string }>("/api/v1/messages/resolve/" + encodeURIComponent(name)).then(
      (r) => chat.show(r.username),
      () => chat.show(name),
    );
  });
</script>
