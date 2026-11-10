<script lang="ts" module>
  export interface QuestionInput {
    builtin: number; // -1 表示自定义
    text: string;
    answer: string;
  }

  export const BUILTIN_COUNT = 10;

  export function blankQuestions(): QuestionInput[] {
    return [0, 1, 2].map((i) => ({ builtin: i, text: "", answer: "" }));
  }

  /** 与后端 auth.CheckQuestions 的规则一致的即时检查（后端仍会再校验）。 */
  export function questionsValid(qs: QuestionInput[]): boolean {
    const keys = qs.map((q) => (q.builtin >= 0 ? "b" + q.builtin : "c" + q.text.trim()));
    if (new Set(keys).size !== qs.length) return false;
    return qs.every((q) => (q.builtin >= 0 || [...q.text.trim()].length >= 2) && [...q.answer.trim()].length >= 2);
  }

  export function toPayload(qs: QuestionInput[]) {
    return {
      questions: qs.map((q) => (q.builtin >= 0 ? { builtin: q.builtin } : { builtin: -1, text: q.text.trim() })),
      answers: qs.map((q) => q.answer),
    };
  }
</script>

<script lang="ts">
  // 三个安全问题（第 14.1 节）：每个可选内置问题或自定义；答案规范化后比较（大小写、全半角、首尾空白不影响）。
  import { t } from "../lib/i18n/i18n.svelte";

  let { questions = $bindable() }: { questions: QuestionInput[] } = $props();

  const taken = (i: number) => new Set(questions.filter((_, j) => j !== i && questions[j]!.builtin >= 0).map((q) => q.builtin));
</script>

<fieldset class="qs">
  <legend class="label">{t("questions.title")}</legend>
  <p class="hint">{t("questions.hint")}</p>
  {#each questions as q, i (i)}
    <div class="q">
      <select class="select" aria-label={t("questions.question_n", { n: i + 1 })} bind:value={q.builtin}>
        {#each Array.from({ length: BUILTIN_COUNT }, (_, k) => k) as k (k)}
          <option value={k} disabled={taken(i).has(k)}>{t("question.builtin." + k)}</option>
        {/each}
        <option value={-1}>{t("questions.custom")}</option>
      </select>
      {#if q.builtin < 0}
        <input class="input" bind:value={q.text} maxlength="100" placeholder={t("questions.custom_placeholder")} aria-label={t("questions.custom_n", { n: i + 1 })} />
      {/if}
      <input class="input" bind:value={q.answer} maxlength="200" autocomplete="off" placeholder={t("questions.answer")} aria-label={t("questions.answer_n", { n: i + 1 })} />
    </div>
  {/each}
</fieldset>

<style>
  .qs {
    display: grid;
    gap: var(--space-3);
    margin: 0;
    padding: 0;
    border: 0;
  }
  .qs legend {
    padding: 0;
  }
  .q {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
  }
</style>
