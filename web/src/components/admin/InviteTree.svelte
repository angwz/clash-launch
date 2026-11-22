<script lang="ts">
  // 邀请树（O11）：谁邀请了谁。没有邀请人（或邀请人已不在）的账号是根；子节点按加入时间排列，
  // 每个节点显示直接邀请与全部下级的人数；有下级的节点可以折叠。只有名字、头像与加入时间。
  import { api } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { shownName } from "../../lib/names";
  import Avatar from "../Avatar.svelte";

  interface Node {
    id: string;
    username: string;
    nickname?: string;
    avatar?: string;
    invitedBy?: string;
    joinedAt: string;
    disabled?: boolean;
    deleted?: boolean;
  }
  interface Tree {
    node: Node;
    children: Tree[];
    total: number; // 全部下级人数
  }

  let roots = $state<Tree[] | null>(null);
  let error = $state("");
  let onlyInvited = $state(true); // 默认只看有邀请关系的人，单独注册的账号很多时不刷屏

  function build(items: Node[]): Tree[] {
    const byId = new Map(items.map((n) => [n.id, n]));
    const kids = new Map<string, Node[]>();
    const top: Node[] = [];
    for (const n of items) {
      if (n.invitedBy && byId.has(n.invitedBy) && n.invitedBy !== n.id) {
        if (!kids.has(n.invitedBy)) kids.set(n.invitedBy, []);
        kids.get(n.invitedBy)!.push(n);
      } else top.push(n);
    }
    const byJoin = (a: Node, b: Node) => a.joinedAt.localeCompare(b.joinedAt);
    const seen = new Set<string>(); // 邀请关系不会成环；防御性地只访问一次
    const grow = (n: Node): Tree => {
      seen.add(n.id);
      const children = (kids.get(n.id) ?? []).filter((c) => !seen.has(c.id)).sort(byJoin).map(grow);
      return { node: n, children, total: children.reduce((s, c) => s + 1 + c.total, 0) };
    };
    return top.sort(byJoin).map(grow);
  }

  $effect(() => {
    api.get<{ items: Node[] }>("/api/v1/admin/invite-tree").then(
      (r) => (roots = build(r.items)),
      (e) => (error = errorText(e)),
    );
  });
  const shown = $derived((roots ?? []).filter((r) => !onlyInvited || r.children.length > 0));
</script>

{#snippet who(n: Node)}
  <Avatar user={n} size={22} />
  <strong class:gone={n.deleted || n.disabled}>{shownName(n, t("deletion.deleted_user"))}</strong>
  {#if n.nickname && !n.deleted}<span class="muted">@{n.username}</span>{/if}
  <span class="muted when">{i18n.date(n.joinedAt)}</span>
  {#if n.disabled && !n.deleted}<span class="tag">{t("admin.disabled")}</span>{/if}
{/snippet}

{#snippet branch(list: Tree[])}
  <ul>
    {#each list as x (x.node.id)}
      <li>
        {#if x.children.length}
          <details open={x.total <= 20}>
            <summary>{@render who(x.node)}<span class="count">{t("invite_tree.count", { direct: x.children.length, total: x.total })}</span></summary>
            {@render branch(x.children)}
          </details>
        {:else}
          <div class="leaf">{@render who(x.node)}</div>
        {/if}
      </li>
    {/each}
  </ul>
{/snippet}

<section class="panel box" aria-labelledby="a-tree">
  <div class="head">
    <h2 id="a-tree">{t("invite_tree.title")}</h2>
    <label class="choice"><input type="checkbox" bind:checked={onlyInvited} />{t("invite_tree.only_invited")}</label>
  </div>
  {#if error}
    <p class="err">{error}</p>
  {:else if roots === null}
    <p class="muted">{t("common.loading")}</p>
  {:else if shown.length === 0}
    <p class="muted">{t("invite_tree.empty")}</p>
  {:else}
    <div class="tree">{@render branch(shown)}</div>
  {/if}
</section>

<style>
  .box {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-5);
    margin-block-end: var(--space-4);
  }
  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }
  h2 {
    font-size: var(--text-lg);
  }
  .choice {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-size: var(--text-sm);
  }
  .tree {
    overflow-x: auto;
  }
  .tree :global(ul) {
    display: grid;
    gap: 2px;
  }
  .tree :global(ul ul) {
    margin-inline-start: 11px;
    padding-inline-start: var(--space-4);
    border-inline-start: 1px solid var(--border);
  }
  summary,
  .leaf {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    padding-block: 3px;
  }
  summary {
    cursor: pointer;
  }
  .leaf {
    padding-inline-start: 18px; /* 与带折叠箭头的行对齐 */
  }
  .gone {
    color: var(--fg-muted);
    text-decoration: line-through;
  }
  .when,
  .count {
    font-size: var(--text-xs);
  }
  .count {
    color: var(--fg-muted);
  }
  .tag {
    padding-inline: 5px;
    border-radius: var(--radius-sm);
    background: var(--bg-active);
    font-size: var(--text-xs);
  }
  .err {
    color: var(--err);
  }
</style>
