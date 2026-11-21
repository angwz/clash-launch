// 在线指示灯（R11）的「工作中」与「完成、失败」信号。页面在自动保存、内核检查时登记，指示灯据此脉动或闪一下。
//
// 只登记用户能感知到的后台工作（自动保存、mihomo 检查）；编辑时的草稿预览很频繁，不算，避免灯一直在动。

class PulseBus {
  /** 进行中的工作数（计数而不是布尔值：保存与检查可能重叠） */
  working = $state(0);
  /** 最近一次完成或失败；seq 每次递增，让指示灯重播一次闪烁 */
  outcome = $state<{ ok: boolean; seq: number } | null>(null);
  #seq = 0;

  /** 登记一项工作，结束（不论成败）时注销。 */
  async track<T>(p: Promise<T>): Promise<T> {
    this.working++;
    try {
      return await p;
    } finally {
      this.working--;
    }
  }

  /** 报告一轮工作的结果：成功时一闪确认，失败时闪一下警示色。 */
  finish(ok: boolean): void {
    this.outcome = { ok, seq: ++this.#seq };
  }
}

export const pulse = new PulseBus();
