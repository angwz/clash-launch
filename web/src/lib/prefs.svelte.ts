// 用户的界面偏好（反馈 #37）：主菜单与内核设置分节的自定义顺序，按用户保存在服务端，换设备一致。
// 列表只表示顺序：按它排序，未列出的项保持默认顺序排在后面（新增的菜单项不需要迁移偏好）。
import { api } from "./api/client";

export interface PrefsData {
  nav?: string[];
  sections?: string[];
  /** 「说明与建议」的点击范围（反馈 #50、#55）：button 只有按钮 | text 说明文字 | card 整张卡片（默认）。
   *  早期版本存的是布尔值：false 视为 button。 */
  fieldClick?: FieldClickMode | boolean;
  /** 自动保存后自动做内核检查；默认开（反馈 #64） */
  autoCheck?: boolean;
}

export type FieldClickMode = "button" | "text" | "card";

export function fieldClickMode(v: PrefsData["fieldClick"]): FieldClickMode {
  if (v === false) return "button";
  return v === "button" || v === "text" ? v : "card";
}

/** 按 order 排序；不在 order 中的项保持原相对顺序排在后面。纯函数，便于测试。 */
export function ordered<T>(items: readonly T[], id: (x: T) => string, order: readonly string[] | undefined): T[] {
  if (!order?.length) return [...items];
  const rank = new Map(order.map((k, i) => [k, i]));
  return items
    .map((x, i) => ({ x, i, r: rank.get(id(x)) ?? order.length + i }))
    .sort((a, b) => a.r - b.r)
    .map((e) => e.x);
}

class Prefs {
  data = $state<PrefsData>({});
  loaded = false;

  async load(): Promise<void> {
    this.data = await api.get<PrefsData>("/api/v1/account/prefs");
    this.loaded = true;
  }

  async save(next: PrefsData): Promise<void> {
    this.data = await api.put<PrefsData>("/api/v1/account/prefs", next);
  }
}

export const prefs = new Prefs();
