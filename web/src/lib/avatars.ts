// 预置头像（需求 R19）：「启动」主题的 10 种图形 × 6 种颜色。图形是原创的简单几何，深浅主题下都清晰。
// SHAPES 与 COLORS 的数量与后端 internal/service/avatar.go 一致（TestAvatarPresetsMatchFrontend 锁定）。

export const SHAPES = ["rocket", "planet", "comet", "satellite", "constellation", "launchpad", "moon", "sun", "telescope", "orbit"] as const;
export type Shape = (typeof SHAPES)[number];

/** 背景色，依次是颜色 1–6。白色图形在每种背景上的对比度都不低于 3:1。 */
export const COLORS = ["#4f46e5", "#0f766e", "#b45309", "#be123c", "#0369a1", "#7c3aed"] as const;

export interface AvatarUser {
  id?: string;
  username: string;
  nickname?: string;
  avatar?: string;
}

export type AvatarView = { kind: "preset"; shape: Shape; color: string } | { kind: "image"; src: string } | { kind: "initial"; letter: string };

/** 由头像标识得出显示方式：预置图形、上传的图片，或（尚未分配、标识无效时）名字的首字母。 */
export function avatarView(u: AvatarUser): AvatarView {
  const a = u.avatar ?? "";
  const m = /^p:([a-z]+)-([1-6])$/.exec(a);
  if (m && (SHAPES as readonly string[]).includes(m[1]!)) return { kind: "preset", shape: m[1] as Shape, color: COLORS[Number(m[2]) - 1]! };
  if (a.startsWith("u:") && u.id) return { kind: "image", src: "/api/v1/avatars/" + encodeURIComponent(u.id) + "?v=" + encodeURIComponent(a.slice(2)) };
  const name = u.nickname || u.username || "?";
  return { kind: "initial", letter: [...name][0]!.toUpperCase() };
}

/** 全部预置头像的标识，按图形再按颜色排列（账号页的选择网格）。 */
export const PRESETS: string[] = SHAPES.flatMap((s) => COLORS.map((_, i) => `p:${s}-${i + 1}`));
