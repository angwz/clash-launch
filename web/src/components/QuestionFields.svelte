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

  /** 填写了答案的问题（安全问题可选，反馈 #76：没有答案的行不提交） */
  export function filled(qs: QuestionInput[]): QuestionInput[] {
    return qs.filter((q) => q.answer.trim() !== "");
  }

  /** 与后端 auth.CheckQuestions 的规则一致的即时检查（后端仍会再校验）。min 是至少要填的个数：注册为 0，修改为 1。 */
  export function questionsValid(qs: QuestionInput[], min = 0): boolean {
    const fs = filled(qs);
    if (fs.length < min) return false;
    const keys = fs.map((q) => (q.builtin >= 0 ? "b" + q.builtin : "c" + q.text.trim()));
    if (new Set(keys).size !== fs.length) return false;
    return fs.every((q) => (q.builtin >= 0 || [...q.text.trim()].length >= 2) && [...q.answer.trim()].length >= 2);
  }

  export function toPayload(qs: QuestionInput[]) {
    const fs = filled(qs);
    return {
      questions: fs.map((q) => (q.builtin >= 0 ? { builtin: q.builtin } : { builtin: -1, text: q.text.trim() })),
      answers: fs.map((q) => q.answer),
    };
  }
</script>

<script lang="ts">
  // 安全问题（第 14.1 节；反馈 #76 改为可选）：最多三个，每个可选内置问题或自定义；答案规范化后比较
  // （大小写、全半角、首尾空白不影响）。先只显示一个，答了上一个才显示下一个；没有答案的行不提交。
  import { t } from "../lib/i18n/i18n.svelte";

  let { questions = $bindable() }: { questions: QuestionInput[] } = $props();

  // 显示的问题数：第一个总是显示；前一个已经作答（至少 2 个字符）时再显示下一个
  const visible = $derived.by(() => {
    let n = 1;
    while (n < questions.length && [...questions[n - 1]!.answer.trim()].length >= 2) n++;
    return n;
  });
  const taken = (i: number) => new Set(questions.filter((_, j) => j !== i && questions[j]!.builtin >= 0).map((q) => q.builtin));
</script>

<fieldset class="qs">
  <legend class="label">{t("questions.title")} <span class="muted opt">{t("questions.optional")}</span></legend>
  <p class="hint">{t("questions.hint")}</p>
  {#each questions.slice(0, visible) as q, i (i)}
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
  .opt {
    font-weight: 400;
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
