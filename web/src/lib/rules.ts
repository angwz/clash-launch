// 由规则源预设的变体生成新建请求：toml（groups + rulesets）或 ini（外部配置）。
import type { RuleSourceInput } from "./api/client";
import type { RulePreset, RulePresetVariant } from "./catalog.svelte";

export function presetInput(p: RulePreset, v: RulePresetVariant, name: string): RuleSourceInput {
  const preset = p.id + "/" + v.id;
  if (v.ini) return { name, kind: "ini", preset, iniUrl: v.ini };
  return {
    name,
    kind: "toml",
    preset,
    groupsUrl: v.groups || undefined,
    rulesetsUrl: v.rulesets || undefined,
    groupsText: v.groupsText || undefined,
    rulesetsText: v.rulesetsText || undefined,
  };
}
