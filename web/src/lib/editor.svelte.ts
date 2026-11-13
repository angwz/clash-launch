// 方案编辑器的状态：文档、草稿预览、保存与内核检查。
// 文档对象（ProfileDoc）本身不是响应式的；每次修改递增 rev，读取方法都依赖 rev，界面随之更新。
// 预览：修改后 450 ms 去抖调用 /preview（不保存、不做内核检查）；「检查」按钮额外执行 mihomo -t。
import { api, ApiError, listAll, type Preview, type Profile, type RuleSource, type Source } from "./api/client";
import { errorText } from "./errors";
import { t } from "./i18n/i18n.svelte";
import { ProfileDoc, type SourceRef } from "./profile-doc";
import type { CheckState } from "./shell.svelte";
import { toasts } from "./toast.svelte";

export class EditorState {
  profile = $state.raw<Profile | null>(null);
  rev = $state(0);
  savedText = $state("");
  preview = $state.raw<Preview | null>(null);
  previewing = $state(false);
  check = $state<CheckState>("idle");
  saving = $state(false);
  error = $state("");
  sources = $state.raw<Source[]>([]);
  ruleSources = $state.raw<RuleSource[]>([]);
  /** 需要定位的字段路径（点分）：折叠的组与「更多」据此展开。 */
  focus = $state("");
  doc: ProfileDoc | null = null;

  #timer: ReturnType<typeof setTimeout> | undefined;
  #abort: AbortController | undefined;

  async load(id: string): Promise<void> {
    const [p, sources, rules] = await Promise.all([
      api.get<Profile>("/api/v1/profiles/" + encodeURIComponent(id)),
      listAll<Source>("/api/v1/sources"),
      listAll<RuleSource>("/api/v1/rule-sources"),
    ]);
    this.sources = sources;
    this.ruleSources = rules;
    this.#adopt(p);
    void this.runPreview(false);
  }

  #adopt(p: Profile): void {
    this.profile = p;
    this.doc = new ProfileDoc(p.doc);
    this.savedText = p.doc;
    this.rev++;
  }

  get text(): string {
    void this.rev;
    return this.doc?.toString() ?? "";
  }

  get dirty(): boolean {
    return this.text !== this.savedText;
  }

  // —— 表单 ——

  value(path: string[]): unknown {
    void this.rev;
    return this.doc?.getForm(path);
  }

  isSet(path: string[]): boolean {
    void this.rev;
    return this.doc?.hasForm(path) ?? false;
  }

  set(path: string[], v: unknown): void {
    this.doc?.setForm(path, v);
    this.#changed();
  }

  reset(path: string[]): void {
    this.doc?.deleteForm(path);
    this.#changed();
  }

  get sourceRefs(): SourceRef[] {
    void this.rev;
    return this.doc?.sources ?? [];
  }

  setSources(list: SourceRef[]): void {
    if (!this.doc) return;
    this.doc.sources = list;
    this.#changed();
  }

  get rulesRef(): string | undefined {
    void this.rev;
    return this.doc?.rules;
  }

  setRules(ref: string | undefined): void {
    if (!this.doc) return;
    this.doc.rules = ref;
    this.#changed();
  }

  get proxiesBlock(): boolean {
    void this.rev;
    return this.doc?.proxiesBlock ?? true;
  }

  setProxiesBlock(v: boolean): void {
    if (!this.doc) return;
    this.doc.proxiesBlock = v;
    this.#changed();
  }

  get raw(): string {
    void this.rev;
    return this.doc?.raw ?? "";
  }

  setRaw(text: string): void {
    if (!this.doc) return;
    this.doc.raw = text;
    this.#changed();
  }

  /** 原始编辑写入或修改过的路径（来自上一次预览）。 */
  touched(path: string[]): boolean {
    return this.preview?.touched.includes(path.join(".")) ?? false;
  }

  #changed(): void {
    this.rev++;
    this.check = "idle";
    clearTimeout(this.#timer);
    this.#timer = setTimeout(() => void this.runPreview(false), 450);
  }

  // —— 预览与检查 ——

  async runPreview(kernel: boolean): Promise<void> {
    if (!this.profile) return;
    this.#abort?.abort();
    const ac = new AbortController();
    this.#abort = ac;
    this.previewing = true;
    if (kernel) this.check = "running";
    try {
      const res = await api.post<Preview>(
        "/api/v1/profiles/" + this.profile.id + "/preview",
        { doc: this.text },
        { query: { check: kernel ? "true" : undefined }, signal: ac.signal },
      );
      this.preview = res;
      this.error = "";
      if (kernel) {
        const s = res.fatal ? "failed" : res.kernel.status;
        this.check = s === "passed" ? "passed" : s === "failed" || s === "timeout" ? "failed" : s === "unavailable" ? "unavailable" : "idle";
      }
    } catch (e) {
      if (e instanceof DOMException && e.name === "AbortError") return;
      this.error = errorText(e);
      if (kernel) this.check = "error";
    } finally {
      if (this.#abort === ac) this.previewing = false;
    }
  }

  async save(note = ""): Promise<boolean> {
    if (!this.profile || !this.dirty) return true;
    this.saving = true;
    try {
      const p = await api.put<Profile>("/api/v1/profiles/" + this.profile.id, { doc: this.text, note }, { ifMatch: this.profile.version });
      // 服务端保存后重新渲染；保留本地文档对象（内容相同），只更新版本与状态。
      this.profile = p;
      this.savedText = this.text;
      toasts.push("ok", t("editor.saved", { n: p.version }));
      void this.runPreview(false);
      return true;
    } catch (e) {
      if (e instanceof ApiError && e.status === 412) toasts.push("err", t("editor.conflict"));
      else if (e instanceof ApiError && e.status === 422) toasts.push("err", t("editor.invalid", { detail: e.detail }));
      else toasts.push("err", errorText(e));
      return false;
    } finally {
      this.saving = false;
    }
  }

  /** 放弃未保存的修改：重新读取服务端的当前版本。 */
  async discard(): Promise<void> {
    if (!this.profile) return;
    this.#adopt(await api.get<Profile>("/api/v1/profiles/" + this.profile.id));
    void this.runPreview(false);
  }

  dispose(): void {
    clearTimeout(this.#timer);
    this.#abort?.abort();
  }

  get counts(): { errors: number; warnings: number } {
    const ds = this.preview?.diagnostics ?? [];
    return { errors: ds.filter((d) => d.severity === "error").length, warnings: ds.filter((d) => d.severity === "warning").length };
  }
}
