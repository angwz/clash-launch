import { describe, expect, it } from "vitest";
import { pulse } from "./pulse.svelte";

describe("pulse", () => {
  it("track 在工作期间计数，成败都会注销；finish 每次换一个序号", async () => {
    let done!: () => void;
    const a = pulse.track(new Promise<void>((r) => (done = r)));
    const b = pulse.track(Promise.reject(new Error("x")));
    expect(pulse.working).toBe(2);
    await expect(b).rejects.toThrow("x");
    expect(pulse.working).toBe(1);
    done();
    await a;
    expect(pulse.working).toBe(0);

    pulse.finish(true);
    const first = pulse.outcome!;
    pulse.finish(true);
    expect(pulse.outcome!.ok).toBe(true);
    expect(pulse.outcome!.seq).toBe(first.seq + 1);
  });
});
