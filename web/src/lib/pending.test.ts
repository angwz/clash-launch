import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const del = vi.fn<(path: string, opts?: { keepalive?: boolean }) => Promise<void>>();
vi.mock("./api/client", () => ({ ApiError: class extends Error {}, api: { del: (p: string, o?: { keepalive?: boolean }) => del(p, o) } }));

const { pending, UNDO_MS } = await import("./pending.svelte");
const { toasts } = await import("./toast.svelte");

describe("pending（可撤销的删除，R33）", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    del.mockReset();
    del.mockResolvedValue(undefined);
  });
  afterEach(() => vi.useRealTimers());

  it("立即隐藏，到期才删除；删掉后继续隐藏", async () => {
    const done = vi.fn();
    pending.remove([{ id: "a", path: "/x/a" }, { id: "b", path: "/x/b" }], "已删除", done);
    expect(pending.has("a") && pending.has("b")).toBe(true);
    expect(del).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(UNDO_MS);
    expect(del.mock.calls.map((c) => c[0])).toEqual(["/x/a", "/x/b"]);
    expect(done).toHaveBeenCalledOnce();
    expect(pending.has("a")).toBe(true); // 页面上删除前取到的列表不会让它重新出现
  });

  it("撤销后不删除并恢复显示", async () => {
    const done = vi.fn();
    pending.remove([{ id: "c", path: "/x/c" }], "已删除", done);
    const toast = toasts.items.find((x) => x.text === "已删除" && x.action)!;
    toast.action!.run();
    expect(pending.has("c")).toBe(false);
    await vi.advanceTimersByTimeAsync(UNDO_MS * 2);
    expect(del).not.toHaveBeenCalled();
    expect(done).toHaveBeenCalledOnce();
  });

  it("失败的条目恢复显示并报告；404 视为已删除", async () => {
    del.mockImplementation(async (p) => {
      if (p === "/x/e") throw Object.assign(new Error("boom"), { status: 500 });
      if (p === "/x/f") throw Object.assign(new Error("gone"), { status: 404 });
    });
    pending.remove([{ id: "d", path: "/x/d" }, { id: "e", path: "/x/e" }, { id: "f", path: "/x/f" }], "已删除3项");
    await vi.advanceTimersByTimeAsync(UNDO_MS);
    expect(pending.has("d")).toBe(true);
    expect(pending.has("e")).toBe(false);
    expect(pending.has("f")).toBe(true);
    expect(toasts.items.some((x) => x.kind === "err")).toBe(true);
  });

  it("页面关闭时立即以 keepalive 发出，之后不再重复发送", async () => {
    pending.remove([{ id: "g", path: "/x/g" }], "已删除");
    window.dispatchEvent(new Event("pagehide"));
    expect(del).toHaveBeenCalledWith("/x/g", { keepalive: true });
    await vi.advanceTimersByTimeAsync(UNDO_MS);
    expect(del).toHaveBeenCalledTimes(1);
  });
});
