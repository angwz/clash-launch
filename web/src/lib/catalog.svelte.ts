// 字段目录：表单元数据（/catalog/{pack}）、字段说明消息（/messages?lang=）与诊断消息（/diagnostics）。
// 元数据与语言无关，只取一次；消息随界面语言切换。说明文字只来自后端目录，前端不写任何字段说明。
import { api } from "./api/client";
import { i18n } from "./i18n/i18n.svelte";

export type FieldType = "int" | "bool" | "string" | "list" | "enum" | "object" | "map" | "anyOf" | "oneOf" | "any";
export type Importance = "core" | "recommended" | "optional" | "expert";

export interface MetaField {
  key: string;
  id: string;
  type: FieldType;
  importance?: Importance;
  stability?: "experimental" | "deprecated";
  since?: string;
  removed?: string;
  replacement?: string;
  default?: unknown;
  enum?: string[];
  examples?: unknown[];
  related?: string[];
  doc?: string;
  sensitive?: boolean;
  ui?: Record<string, string>;
  targets?: Record<string, string>;
  fields?: MetaField[];
  items?: MetaField;
  values?: MetaField;
  of?: MetaField[];
  discriminator?: string;
  variants?: { value: string; id: string; doc?: string; fields: MetaField[] }[];
  hasDetail?: boolean;
}

export interface NavSection {
  id: string;
  zh: string;
  en: string;
  generated?: "sources" | "rules";
  keys: string[];
}

export interface Preset {
  id: string;
  label: Record<string, string>;
  platform: string;
  core: string;
  docs: string;
  managed?: string[];
  notes: Record<string, string>;
  kernel: boolean;
  defaults?: Record<string, unknown>;
}

export interface RulePresetVariant {
  id: string;
  label: Record<string, string>;
  groups?: string;
  rulesets?: string;
  ini?: string;
  groupsText?: string;
  rulesetsText?: string;
}

export interface RulePreset {
  id: string;
  label: Record<string, string>;
  repo: string;
  license: string;
  checked: string;
  stars?: number;
  recommended?: boolean;
  notes: Record<string, string>;
  variants: RulePresetVariant[];
}

export interface CatalogMeta {
  pack: { id: string; label: string; docRoot: string; coreMin: string; coreMax: string; reference: string; order: string[]; nav: NavSection[] };
  digest: string;
  version: string;
  sections: MetaField[];
  presets: Preset[];
  ruleSources: RulePreset[];
  decor: string[];
}

type DiagInfo = { severity: "error" | "warning" | "info"; en: string; "zh-CN": string };

class CatalogState {
  meta = $state.raw<CatalogMeta | null>(null);
  messages = $state.raw<Record<string, string>>({});
  messagesLang = $state("");
  diags = $state.raw<Record<string, DiagInfo>>({});
  #byId = new Map<string, MetaField>();

  async load(): Promise<void> {
    if (!this.meta) {
      const [meta, diags] = await Promise.all([
        api.get<CatalogMeta>("/api/v1/catalog/mihomo"),
        api.get<Record<string, DiagInfo>>("/api/v1/diagnostics"),
      ]);
      this.#index(meta.sections);
      this.meta = meta;
      this.diags = diags;
    }
    await this.loadMessages();
  }

  async loadMessages(): Promise<void> {
    const lang = i18n.apiLang;
    if (this.messagesLang === lang) return;
    this.messages = await api.get<Record<string, string>>("/api/v1/catalog/mihomo/messages", { query: { lang } });
    this.messagesLang = lang;
  }

  #index(fields: MetaField[]): void {
    const walk = (f: MetaField) => {
      this.#byId.set(f.id, f);
      f.fields?.forEach(walk);
      if (f.items) walk(f.items);
      if (f.values) walk(f.values);
      f.of?.forEach(walk);
      f.variants?.forEach((v) => v.fields.forEach(walk));
    };
    fields.forEach(walk);
  }

  field(id: string): MetaField | undefined {
    return this.#byId.get(id);
  }

  top(key: string): MetaField | undefined {
    return this.meta?.sections.find((f) => f.key === key);
  }

  title(f: MetaField): string {
    return this.messages[`catalog.${f.id}.title`] ?? f.key;
  }

  summary(f: MetaField): string {
    return this.messages[`catalog.${f.id}.summary`] ?? "";
  }

  detail(f: MetaField): string {
    return this.messages[`catalog.${f.id}.detail`] ?? "";
  }

  enumLabel(f: MetaField, v: string): string {
    return this.messages[`catalog.${f.id}.enum.${v}`] ?? "";
  }

  exampleLabel(f: MetaField, i: number): string {
    return this.messages[`catalog.${f.id}.example.${i}`] ?? "";
  }

  navTitle(s: NavSection): string {
    return i18n.apiLang === "zh-CN" ? s.zh : s.en;
  }

  label(m: Record<string, string> | undefined): string {
    if (!m) return "";
    return (i18n.apiLang === "zh-CN" ? m.zh : m.en) ?? m.en ?? "";
  }

  /**
   * 诊断消息：与后端 diag.Message 相同的 {name} 替换。
   * names 把参数中的 ID（源、规则源）替换为可读的名称；setting 参数是整理设置的字段名列表，翻译为界面文字。
   */
  diagMessage(code: string, args?: Record<string, string>, names?: Map<string, string>): string {
    const info = this.diags[code];
    let msg = info ? info[i18n.apiLang] : code;
    for (const [k, raw] of Object.entries(args ?? {})) {
      let v = names?.get(raw) ?? raw;
      if (k === "setting") v = raw.split(", ").map((s) => (i18n.messages["proc.setting." + s] ? i18n.t("proc.setting." + s) : s)).join(i18n.apiLang === "zh-CN" ? "、" : ", ");
      msg = msg.split("{" + k + "}").join(v);
    }
    return msg;
  }
}

export const catalog = new CatalogState();
