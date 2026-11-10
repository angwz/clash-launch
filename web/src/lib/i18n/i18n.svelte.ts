// 多语言（第 15 节）：界面文字全部走消息键，ICU MessageFormat；语言包按需加载。
// 协商顺序：用户设置 → 浏览器语言（等同 Accept-Language）→ en。
// ar-XB 是伪 RTL 测试语言：由 en 生成，每条消息包进 RLO…PDF，<html dir="rtl">；只用于端到端测试与开发，
// 不出现在语言选择器中。
import { IntlMessageFormat } from "intl-messageformat";

export type Locale = "en" | "zh-CN" | "ar-XB";
export const LOCALES: { id: Exclude<Locale, "ar-XB">; label: string }[] = [
  { id: "zh-CN", label: "简体中文" },
  { id: "en", label: "English" },
];

type Messages = Record<string, string>;

const loaders: Record<"en" | "zh-CN", () => Promise<{ default: Messages }>> = {
  en: () => import("./messages/en.json"),
  "zh-CN": () => import("./messages/zh-CN.json"),
};

const RLO = String.fromCharCode(0x202e);
const PDF = String.fromCharCode(0x202c);
const STORAGE_KEY = "cl.lang";

class I18n {
  locale = $state<Locale>("en");
  messages = $state.raw<Messages>({});
  ready = $state(false);
  #cache = new Map<string, IntlMessageFormat>();

  get dir(): "ltr" | "rtl" {
    return this.locale === "ar-XB" ? "rtl" : "ltr";
  }

  /** 后端只认识 en 与 zh-CN（目录说明与诊断消息）。 */
  get apiLang(): "en" | "zh-CN" {
    return this.locale === "zh-CN" ? "zh-CN" : "en";
  }

  async set(locale: Locale, persist = true): Promise<void> {
    const base = locale === "ar-XB" ? "en" : locale;
    let msgs = (await loaders[base]()).default;
    if (locale === "ar-XB") msgs = pseudoRTL(msgs);
    this.#cache.clear();
    this.messages = msgs;
    this.locale = locale;
    this.ready = true;
    document.documentElement.lang = locale;
    document.documentElement.dir = this.dir;
    if (persist) {
      try {
        localStorage.setItem(STORAGE_KEY, locale);
      } catch {
        /* 隐私模式等：只影响下次打开时的默认语言 */
      }
    }
  }

  /** 用户设置优先；其次是本机记住的选择；再其次是浏览器语言。 */
  negotiate(userLang?: string): Locale {
    const forced = new URLSearchParams(location.search).get("lang");
    if (forced === "ar-XB" || forced === "en" || forced === "zh-CN") return forced;
    if (userLang) return pick(userLang);
    try {
      const saved = localStorage.getItem(STORAGE_KEY);
      if (saved === "en" || saved === "zh-CN") return saved;
    } catch {
      /* 读不到就继续协商 */
    }
    for (const l of navigator.languages ?? [navigator.language]) {
      const p = pick(l);
      if (p !== "en" || l.toLowerCase().startsWith("en")) return p;
    }
    return "en";
  }

  t = (key: string, values?: Record<string, unknown>): string => {
    const src = this.messages[key];
    if (src === undefined) {
      if (import.meta.env.DEV && this.ready) console.warn("缺少消息", key);
      return key;
    }
    if (!values && !src.includes("{")) return src;
    let f = this.#cache.get(key);
    if (!f) {
      f = new IntlMessageFormat(src, this.locale === "ar-XB" ? "en" : this.locale);
      this.#cache.set(key, f);
    }
    return String(f.format(values as Record<string, string | number>));
  };

  // —— 显示用的格式化：配置值本身不经过 Intl（第 15.3 节）——
  date = (iso: string | undefined, style: "short" | "long" = "short"): string => {
    if (!iso) return "—";
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return "—";
    return new Intl.DateTimeFormat(this.intlLocale, style === "short" ? { dateStyle: "medium", timeStyle: "short" } : { dateStyle: "long", timeStyle: "medium" }).format(d);
  };

  relative = (iso: string | undefined): string => {
    if (!iso) return "—";
    const diff = (new Date(iso).getTime() - Date.now()) / 1000;
    const abs = Math.abs(diff);
    const rtf = new Intl.RelativeTimeFormat(this.intlLocale, { numeric: "auto" });
    if (abs < 60) return rtf.format(Math.round(diff), "second");
    if (abs < 3600) return rtf.format(Math.round(diff / 60), "minute");
    if (abs < 86400) return rtf.format(Math.round(diff / 3600), "hour");
    return rtf.format(Math.round(diff / 86400), "day");
  };

  number = (n: number): string => new Intl.NumberFormat(this.intlLocale).format(n);

  get intlLocale(): string {
    return this.locale === "ar-XB" ? "en" : this.locale;
  }
}

function pick(tag: string): Locale {
  return tag.toLowerCase().startsWith("zh") ? "zh-CN" : "en";
}

// 伪 RTL：把每条消息的可见文字放进 RLO…PDF，占位符 {x} 与 ICU 结构保持原样，以检查布局能否镜像。
function pseudoRTL(m: Messages): Messages {
  const out: Messages = {};
  for (const [k, v] of Object.entries(m)) out[k] = RLO + v + PDF;
  return out;
}

export const i18n = new I18n();
export const t = i18n.t;
