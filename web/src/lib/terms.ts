// 用户协议（需求 R40）：读取当前协议，按界面语言取文本，并拆出标题。
import { api } from "./api/client";
import type { Schemas } from "./api/client";
import { parse, type Block } from "./md";

export type Terms = Schemas["Terms"];

export interface TermsDoc {
  version: number;
  title: string;
  blocks: Block[];
  updatedAt?: string;
}

export function fetchTerms(): Promise<Terms> {
  return api.get<Terms>("/api/v1/terms");
}

/** 取界面语言的文本（没有时退回中文），第一个一级标题作为页面标题，其余为正文。 */
export function termsDoc(t: Terms, locale: string): TermsDoc {
  const text = t.texts[locale] ?? t.texts["zh-CN"] ?? Object.values(t.texts)[0] ?? "";
  const blocks = parse(text);
  const h = blocks.find((b) => b.k === "h" && b.level === 1);
  const title = h && h.k === "h" ? h.text.map((x) => x.s).join("") : "";
  return { version: t.version, title, blocks: blocks.filter((b) => b !== h), updatedAt: t.updatedAt };
}
