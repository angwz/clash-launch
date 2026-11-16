// 分流地图的粒子层（docs/design/routing.md 第 10 节，决定 0007 #12）：粒子是示意，不是流量。
// 每个环境粒子携带规则集真实内容中的一个样例，路线是服务器对这个样例的追踪结果（命中行）沿默认路径到出口；
// 追踪粒子按追踪结果逐条经过规则列。不依赖 Svelte；画布由组件创建并在布局变化时调用 setScene。
import type { Box, FlowKind } from "./layout";

export type ParticleMode = "all" | "focus" | "off";

export interface Scene {
  boxes: Map<string, Box>;
  /** 规则序号 → 方框键（折叠段内的规则共用段） */
  ruleBox: (index: number) => string;
  /** 从命中行出发沿默认路径经过的方框键（不含规则行本身）与流向 */
  pathOf: (hit: number) => { boxes: string[]; kind: FlowKind };
  /** 环境粒子的样例：服务器追踪得到的命中行；weight 按规则集条数 */
  samples: { host: string; hit: number; weight: number }[];
  /** 规则列左缘（追踪轨道在它左侧 16px） */
  rulesLeft: number;
  width: number;
  height: number;
  cap: number;
  labelProb: number;
}

export interface TraceAnim {
  host: string;
  steps: { index: number; result: string }[];
  hit: number;
  needsIp?: { at: number; candidates: number[] };
}

interface Seg {
  pts?: { x: number; y: number }[];
  wait?: number;
  mark?: { x: number; y: number; s: string; ok?: boolean };
  halo?: Box;
  end?: Box; // 到达这一段终点时在方框外沿画光圈
  burst?: { x: number; y: number };
  split?: { segs: Seg[]; kind: FlowKind; label: string; ids: Set<string> }[];
  neutral?: boolean;
  lane?: boolean;
  rail?: boolean;
  out?: boolean;
  _on?: boolean;
  _left?: number;
}

interface Particle {
  segs: Seg[];
  si: number;
  idx: number;
  x: number;
  y: number;
  trail: { x: number; y: number }[];
  r: number;
  speed: number;
  alive: boolean;
  mix: number;
  kind: FlowKind;
  ids: Set<string>;
  label: string | null;
  trace: boolean;
  ambient: boolean;
}

const SP = 4; // 路径预采样间距（px）
const rnd = (a: number, b: number) => a + Math.random() * (b - a);

export class ParticleField {
  private ctx: CanvasRenderingContext2D | null;
  private scene: Scene | null = null;
  private trace: TraceAnim | null = null;
  private mode: ParticleMode = "all";
  private focus: Set<string> | null = null;
  private parts: Particle[] = [];
  private halos: { box: Box; kind: FlowKind; born: number; strong: boolean }[] = [];
  private marks: { x: number; y: number; s: string; kind: FlowKind; born: number }[] = [];
  private bursts: { x: number; y: number; kind: FlowKind; born: number; big: boolean }[] = [];
  private colors: Record<string, string> = {};
  private rgb: Record<string, [number, number, number]> = {};
  private fonts = { mono: "monospace", sans: "sans-serif" };
  private spawnAcc = 0;
  private traceWait = 0;
  private now = 0;
  private last = 0;
  private raf = 0;
  private visible = true;
  private drawn = false;
  private totalWeight = 0;

  constructor(private canvas: HTMLCanvasElement) {
    this.ctx = canvas.getContext("2d");
    this.readColors();
  }

  /** 从 CSS 变量读取颜色（主题切换时再次调用）。 */
  readColors(): void {
    const cs = getComputedStyle(this.canvas);
    const v = (n: string) => cs.getPropertyValue(n).trim();
    this.colors = { proxy: v("--flow-proxy"), direct: v("--flow-direct"), reject: v("--flow-reject"), neutral: v("--flow-none"), fg: v("--fg-muted"), muted: v("--fg-subtle") };
    for (const k of ["proxy", "direct", "reject", "neutral"]) this.rgb[k] = hex(this.colors[k] || "#888888");
    this.fonts = { mono: v("--font-mono") || "monospace", sans: v("--font-sans") || "sans-serif" };
  }

  setScene(scene: Scene): void {
    this.scene = scene;
    this.totalWeight = scene.samples.reduce((s, x) => s + x.weight, 0);
    const dpr = Math.min(2, window.devicePixelRatio || 1); // 设备像素比上限 2
    this.canvas.width = Math.round(scene.width * dpr);
    this.canvas.height = Math.round(scene.height * dpr);
    this.ctx?.setTransform(dpr, 0, 0, dpr, 0, 0);
    this.reset();
  }

  setTrace(t: TraceAnim | null): void {
    this.trace = t;
    this.reset();
  }

  setMode(mode: ParticleMode): void {
    this.mode = mode;
    this.reset();
  }

  /** 悬停或选中时聚焦的方框键；focus 模式下其余粒子变淡。不清空粒子。 */
  setFocus(ids: Set<string> | null): void {
    this.focus = ids;
  }

  setVisible(v: boolean): void {
    this.visible = v;
  }

  start(): void {
    if (!this.raf) this.raf = requestAnimationFrame(this.frame);
  }

  stop(): void {
    cancelAnimationFrame(this.raf);
    this.raf = 0;
    this.clear();
  }

  private reset(): void {
    this.parts = [];
    this.halos = [];
    this.marks = [];
    this.bursts = [];
    this.traceWait = 200;
    this.clear();
  }

  private clear(): void {
    this.ctx?.clearRect(0, 0, this.canvas.width, this.canvas.height);
    this.drawn = false;
  }

  private frame = (ts: number): void => {
    this.raf = requestAnimationFrame(this.frame);
    const dt = Math.min(48, ts - (this.last || ts));
    this.last = ts;
    const sc = this.scene;
    if (this.mode === "off" || !this.visible || document.hidden || !sc || !this.ctx) {
      if (this.drawn) this.clear();
      return;
    }
    this.now = ts;
    this.spawnAcc += dt;
    const every = this.trace && this.mode === "focus" ? 560 : 190;
    while (this.spawnAcc > every) {
      this.spawnAcc -= every;
      if (this.parts.filter((p) => p.ambient).length < sc.cap) this.spawnAmbient();
    }
    if (this.trace && !this.parts.some((p) => p.trace)) {
      this.traceWait -= dt;
      if (this.traceWait <= 0) {
        this.spawnTrace();
        this.traceWait = 900; // 整条结束后 900ms 重播
      }
    }
    for (const p of this.parts) this.advance(p, dt);
    this.parts = this.parts.filter((p) => p.alive);
    this.draw(dt);
    this.drawn = true;
  };

  // —— 路线 ——

  private route(hit: number): { segs: Seg[]; kind: FlowKind; ids: Set<string> } | null {
    const sc = this.scene!;
    const rk = sc.ruleBox(hit);
    const Rb = sc.boxes.get(rk);
    if (!Rb) return null;
    const segs: Seg[] = [{ pts: seg({ x: Rb.l, y: Rb.y }, { x: Rb.r, y: Rb.y }), neutral: true }];
    const ids = new Set([rk]);
    let prev = { x: Rb.r, y: Rb.y };
    const path = sc.pathOf(hit);
    for (const k of path.boxes) {
      const box = sc.boxes.get(k);
      if (!box) break;
      ids.add(k);
      segs.push({ pts: seg(prev, { x: box.l, y: box.y }, true), end: box });
      if (path.kind === "reject" && k === path.boxes[path.boxes.length - 1]) {
        segs.push({ burst: { x: box.l, y: box.y } });
        return { segs, kind: path.kind, ids };
      }
      segs.push({ pts: seg({ x: box.l, y: box.y }, { x: box.r, y: box.y }) });
      prev = { x: box.r, y: box.y };
    }
    const outY = Math.max(12, Math.min(sc.height - 12, prev.y + rnd(-34, 34)));
    segs.push({ pts: seg(prev, { x: sc.width - 6, y: outY }, true), out: true });
    return { segs, kind: path.kind, ids };
  }

  private pickSample(): { host: string; hit: number } | null {
    const sc = this.scene!;
    if (!sc.samples.length) return null;
    let x = Math.random() * this.totalWeight;
    for (const s of sc.samples) {
      x -= s.weight;
      if (x <= 0) return s;
    }
    return sc.samples[0]!;
  }

  private spawnAmbient(): void {
    const sc = this.scene!;
    const s = this.pickSample();
    if (!s || s.hit < 0) return;
    const Rb = sc.boxes.get(sc.ruleBox(s.hit));
    const rt = this.route(s.hit);
    if (!Rb || !rt) return;
    const start = { x: 4, y: rnd(16, sc.height - 16) };
    this.parts.push(
      particle({
        segs: [{ pts: seg(start, { x: Rb.l, y: Rb.y }, true), end: Rb, lane: true, neutral: true }, ...rt.segs],
        kind: rt.kind,
        ids: rt.ids,
        label: Math.random() < sc.labelProb ? s.host : null,
        ambient: true,
        x: start.x,
        y: start.y,
      }),
    );
  }

  // 追踪粒子：沿规则列左侧的轨道逐条下行（未命中 ✗，无法判断 ?），在命中处停下（✓）并染色，
  // 再沿默认路径离开；需要 IP 时在那一行分成两路：A 为第一条可能因 IP 命中的行，B 继续到实际命中行。
  private spawnTrace(): void {
    const sc = this.scene!;
    const t = this.trace!;
    if (t.hit < 0 || !t.steps.length) return;
    const rail = sc.rulesLeft - 16;
    const yOf = (i: number) => sc.boxes.get(sc.ruleBox(i))?.y;
    const y0 = yOf(t.steps[0]!.index);
    if (y0 === undefined) return;
    const segs: Seg[] = [{ pts: seg({ x: 4, y: y0 }, { x: rail, y: y0 }, true), lane: true, neutral: true }];
    const result = new Map(t.steps.map((s) => [s.index, s.result]));
    // 折叠段内的规则共用一行：同一行只停一次
    const scan = (from: number, to: number, list: Seg[]) => {
      let lastY: number | undefined;
      for (let i = from; i < to; i++) {
        const y = yOf(i);
        if (y === undefined) continue;
        if (lastY !== undefined && lastY !== y) list.push({ pts: seg({ x: rail, y: lastY }, { x: rail, y }), rail: true, neutral: true });
        const nextY = yOf(i + 1);
        if (nextY !== y) list.push({ wait: 160, mark: { x: rail, y, s: result.get(i) === "miss" ? "✗" : "?" }, rail: true, neutral: true });
        lastY = y;
      }
      const yt = yOf(to);
      if (lastY !== undefined && yt !== undefined && lastY !== yt) list.push({ pts: seg({ x: rail, y: lastY }, { x: rail, y: yt }), rail: true, neutral: true });
    };
    const base = { label: t.host, trace: true, r: 3, speed: 200, x: 4, y: y0 };
    const branchAt = t.needsIp?.at ?? -1;
    const branchTo = t.needsIp?.candidates.find((c) => c > branchAt && c !== t.hit) ?? -1;
    if (branchAt < 0 || branchTo < 0) {
      scan(t.steps[0]!.index, t.hit, segs);
      const Mb = sc.boxes.get(sc.ruleBox(t.hit));
      const rt = this.route(t.hit);
      if (!Mb || !rt) return;
      rt.segs[0]!.neutral = false;
      segs.push({ wait: 340, mark: { x: rail, y: Mb.y, s: "✓", ok: true }, halo: Mb, rail: true }, { pts: seg({ x: rail, y: Mb.y }, { x: Mb.l, y: Mb.y }) });
      this.parts.push(particle({ ...base, segs: [...segs, ...rt.segs], kind: rt.kind, ids: rt.ids }));
      return;
    }
    scan(t.steps[0]!.index, branchAt, segs);
    const Qb = sc.boxes.get(sc.ruleBox(branchAt));
    if (!Qb) return;
    segs.push({ wait: 380, mark: { x: rail, y: Qb.y, s: "?" }, halo: Qb, rail: true, neutral: true });
    const arm = (to: number, label: string) => {
      const list: Seg[] = [];
      scan(branchAt, to, list);
      const Tb = sc.boxes.get(sc.ruleBox(to));
      const rt = this.route(to);
      if (!Tb || !rt) return null;
      rt.segs[0]!.neutral = false;
      list.push({ wait: 260, mark: { x: rail, y: Tb.y, s: "✓", ok: true }, halo: Tb, rail: true }, { pts: seg({ x: rail, y: Tb.y }, { x: Tb.l, y: Tb.y }) });
      return { segs: [...list, ...rt.segs], kind: rt.kind, label, ids: rt.ids };
    };
    const a = arm(branchTo, t.host + " · IP");
    const b = arm(t.hit, t.host);
    const split = [a, b].filter((x): x is NonNullable<typeof x> => !!x);
    segs.push({ split });
    this.parts.push(particle({ ...base, segs, kind: "neutral", ids: new Set(split.flatMap((x) => [...x.ids])) }));
  }

  // —— 运动与绘制 ——

  private advance(p: Particle, dt: number): void {
    let budget = (p.speed * dt) / 1000;
    while (p.alive && p.si < p.segs.length) {
      const s = p.segs[p.si]!;
      if (s.wait != null) {
        if (!s._on) {
          s._on = true;
          s._left = s.wait;
          if (s.mark) this.marks.push({ ...s.mark, born: this.now, kind: s.mark.ok ? p.kind : "neutral" });
          if (s.halo) this.halos.push({ box: s.halo, kind: s.mark?.ok ? p.kind : "neutral", born: this.now, strong: true });
        }
        s._left! -= dt;
        if (s._left! > 0) return;
        p.si++;
        continue;
      }
      if (s.split) {
        for (const c of s.split) this.parts.push(particle({ segs: c.segs, kind: c.kind, ids: c.ids, label: c.label, trace: true, r: p.r, speed: p.speed, x: p.x, y: p.y, trail: p.trail.slice() }));
        p.alive = false;
        return;
      }
      if (s.burst) {
        this.bursts.push({ ...s.burst, kind: p.kind, born: this.now, big: p.trace });
        p.alive = false;
        return;
      }
      if (budget <= 0 || !s.pts) break;
      const rem = s.pts.length - 1 - p.idx;
      const step = budget / SP;
      if (step < rem) {
        p.idx += step;
        budget = 0;
      } else {
        budget -= rem * SP;
        p.idx = 0;
        p.si++;
        if (s.end && (p.trace || Math.random() < 0.2)) this.halos.push({ box: s.end, kind: s.neutral ? "neutral" : p.kind, born: this.now, strong: p.trace });
        const last = s.pts[s.pts.length - 1]!;
        p.x = last.x;
        p.y = last.y;
        continue;
      }
      const i = Math.floor(p.idx);
      const f = p.idx - i;
      const a = s.pts[i]!;
      const b = s.pts[Math.min(i + 1, s.pts.length - 1)]!;
      p.x = a.x + (b.x - a.x) * f;
      p.y = a.y + (b.y - a.y) * f;
    }
    if (p.si >= p.segs.length) p.alive = false;
  }

  private tint(kind: FlowKind, mix: number): string {
    const a = this.rgb.neutral!;
    const b = this.rgb[kind] ?? a;
    const c = (i: 0 | 1 | 2) => Math.round(a[i] + (b[i] - a[i]) * mix);
    return `rgb(${c(0)} ${c(1)} ${c(2)})`;
  }

  private draw(dt: number): void {
    const ctx = this.ctx!;
    const now = this.now;
    const tracing = !!this.trace;
    ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
    // 行外沿的光圈：柔和、短促（600ms）
    this.halos = this.halos.filter((h) => now - h.born < 600);
    for (const h of this.halos) {
      const a = (now - h.born) / 600;
      const e = 1 + a * 5;
      ctx.globalAlpha = (1 - a) * (h.strong ? 0.7 : 0.18);
      ctx.strokeStyle = this.colors[h.kind] || this.colors.neutral!;
      ctx.lineWidth = h.strong ? 2 : 1.2;
      roundRect(ctx, h.box.l - e, h.box.t - e, h.box.r - h.box.l + 2 * e, h.box.b - h.box.t + 2 * e, 7 + e);
      ctx.stroke();
    }
    ctx.lineCap = "round";
    const w = this.canvas.width / Math.min(2, window.devicePixelRatio || 1);
    for (const p of this.parts) {
      const s = p.segs[Math.min(p.si, p.segs.length - 1)];
      // 命中规则之前保持未分类色；之后 0.35 秒内过渡到流向色
      if (s && !s.neutral && p.kind !== "neutral") p.mix = Math.min(1, p.mix + dt / 350);
      const col = this.tint(p.kind, p.mix);
      let alpha = p.trace ? 0.95 : 0.62;
      if (this.mode === "focus") {
        if (tracing && !p.trace) alpha = 0.1;
        if (this.focus && ![...p.ids].some((x) => this.focus!.has(x))) alpha = 0.1;
      }
      p.trail.push({ x: p.x, y: p.y });
      if (p.trail.length > (p.trace ? 14 : 8)) p.trail.shift();
      for (let k = 1; k < p.trail.length; k++) {
        const q = k / p.trail.length;
        ctx.globalAlpha = alpha * q * 0.4;
        ctx.strokeStyle = col;
        ctx.lineWidth = p.r * q;
        ctx.beginPath();
        ctx.moveTo(p.trail[k - 1]!.x, p.trail[k - 1]!.y);
        ctx.lineTo(p.trail[k]!.x, p.trail[k]!.y);
        ctx.stroke();
      }
      ctx.fillStyle = col;
      if (p.trace) {
        ctx.globalAlpha = 0.14;
        ctx.beginPath();
        ctx.arc(p.x, p.y, p.r * 2.6, 0, Math.PI * 2);
        ctx.fill();
      }
      ctx.globalAlpha = alpha;
      ctx.beginPath();
      ctx.arc(p.x, p.y, p.r, 0, Math.PI * 2);
      ctx.fill();
      if (p.label && s && (s.lane || s.out || (p.trace && s.rail))) {
        let la = alpha * (p.trace ? 1 : 0.75);
        if (s.pts && !p.trace) {
          const prog = p.idx / Math.max(1, s.pts.length - 1);
          la *= s.lane ? Math.min(1, (1 - prog) * 2.2) : Math.min(1, prog * 3, (1 - prog) * 3);
        }
        ctx.globalAlpha = Math.max(0, la);
        ctx.font = (p.trace ? "600 11px " : "10px ") + this.fonts.mono;
        ctx.fillStyle = (p.trace ? this.colors.fg : this.colors.muted) || "#888";
        const tw = ctx.measureText(p.label).width;
        let tx = s.rail ? p.x - tw - 10 : p.x + 7;
        tx = Math.max(4, Math.min(w - tw - 4, tx));
        ctx.fillText(p.label, tx, p.y - 7);
      }
    }
    this.marks = this.marks.filter((m) => now - m.born < 1800);
    ctx.font = "700 12px " + this.fonts.sans;
    for (const m of this.marks) {
      const a = (now - m.born) / 1800;
      ctx.globalAlpha = (1 - a * a) * 0.9;
      ctx.fillStyle = (m.kind === "neutral" ? this.colors.muted : this.colors[m.kind]) || "#888";
      ctx.fillText(m.s, m.x - 4, m.y + 4);
    }
    // 拦截：在「拦截」行左缘画扩散圆环
    this.bursts = this.bursts.filter((b) => now - b.born < 600);
    for (const b of this.bursts) {
      const a = (now - b.born) / 600;
      const R = (b.big ? 5 : 2.5) + a * (b.big ? 18 : 9);
      ctx.globalAlpha = (1 - a) * (this.mode === "focus" && tracing && !b.big ? 0.1 : b.big ? 0.8 : 0.45);
      ctx.strokeStyle = this.colors[b.kind] || "#888";
      ctx.lineWidth = 1.2;
      ctx.beginPath();
      ctx.arc(b.x, b.y, R, 0, Math.PI * 2);
      ctx.stroke();
    }
    ctx.globalAlpha = 1;
  }
}

function particle(o: Partial<Particle> & { segs: Seg[]; kind: FlowKind; ids: Set<string> }): Particle {
  return { si: 0, idx: 0, x: 0, y: 0, trail: [], r: 1.8, speed: rnd(140, 210), alive: true, mix: 0, label: null, trace: false, ambient: false, ...o };
}

/** 两点间按 4px 间距预采样；curve 时为水平切线的三次贝塞尔（与连线相同）。 */
export function seg(a: { x: number; y: number }, b: { x: number; y: number }, curve = false): { x: number; y: number }[] {
  const dist = Math.hypot(b.x - a.x, b.y - a.y) * (curve ? 1.12 : 1);
  const n = Math.max(2, Math.ceil(dist / SP));
  const mx = (a.x + b.x) / 2;
  const pts: { x: number; y: number }[] = [];
  for (let i = 0; i <= n; i++) {
    const t = i / n;
    const u = 1 - t;
    if (curve) pts.push({ x: u * u * u * a.x + 3 * u * u * t * mx + 3 * u * t * t * mx + t * t * t * b.x, y: u * u * u * a.y + 3 * u * u * t * a.y + 3 * u * t * t * b.y + t * t * t * b.y });
    else pts.push({ x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t });
  }
  return pts;
}

function hex(h: string): [number, number, number] {
  const m = /^#?([0-9a-f]{6})$/i.exec(h.trim());
  const n = m ? parseInt(m[1]!, 16) : 0x888888;
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

function roundRect(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number): void {
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + w, y, x + w, y + h, r);
  ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r);
  ctx.arcTo(x, y, x + w, y, r);
  ctx.closePath();
}
