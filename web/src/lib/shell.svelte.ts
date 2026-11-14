// 外壳状态：页面向顶栏报告当前方案、检查状态与原始编辑提示（第 16.1 节顶栏）；侧栏折叠与手机抽屉。
export type CheckState = "idle" | "running" | "passed" | "failed" | "unavailable" | "error";

export interface ProfileStatus {
  id: string;
  name: string;
  check: CheckState;
  /** 方案中存在原始编辑内容时为 true：顶栏显示 ⚡（第 16.4 节） */
  raw: boolean;
  dirty: boolean;
  diagnostics: { errors: number; warnings: number };
  openRaw: () => void;
  runCheck: () => void;
  /** 预览未停靠时由顶栏提供开关：按钮不放在滚动区内，避免被滚动条遮住（反馈 #42） */
  preview: { available: boolean; open: boolean; toggle: () => void };
}

const RAIL_KEY = "cl.rail";

class ShellState {
  profile = $state<ProfileStatus | null>(null);
  rail = $state(false);
  drawer = $state(false);

  init(): void {
    try {
      this.rail = localStorage.getItem(RAIL_KEY) === "1";
    } catch {
      /* 默认展开 */
    }
  }

  toggleRail(): void {
    this.rail = !this.rail;
    try {
      localStorage.setItem(RAIL_KEY, this.rail ? "1" : "0");
    } catch {
      /* 只影响下次打开 */
    }
  }
}

export const shell = new ShellState();
