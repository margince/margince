// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useWarmRouteChunks } from "./App.warmchunks";

const chunks = vi.hoisted(() => ({
  loaders: [] as (() => Promise<unknown>)[],
}));

vi.mock("./App.screens", () => ({
  get ROUTE_CHUNKS() {
    return chunks.loaders;
  },
}));

type IdleCallback = () => void;

// A hand-run idle queue, so each spec says when the browser went idle.
function stubIdle() {
  const pending = new Map<number, IdleCallback>();
  let nextHandle = 1;
  const cancel = vi.fn((handle: number) => pending.delete(handle));
  vi.stubGlobal(
    "requestIdleCallback",
    vi.fn((callback: IdleCallback) => {
      pending.set(nextHandle, callback);
      nextHandle += 1;
      return nextHandle - 1;
    }),
  );
  vi.stubGlobal("cancelIdleCallback", cancel);
  return {
    cancel,
    goIdle: () => {
      for (const callback of pending.values()) {
        callback();
      }
    },
  };
}

beforeEach(() => {
  chunks.loaders = [];
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("useWarmRouteChunks", () => {
  it("waits for idle, then loads one chunk at a time", async () => {
    const idle = stubIdle();
    let finishFirst: (value?: unknown) => void = () => undefined;
    const first = vi.fn(
      () =>
        new Promise((resolve) => {
          finishFirst = resolve;
        }),
    );
    const second = vi.fn(() => Promise.resolve());
    chunks.loaders = [first, second];

    renderHook(() => useWarmRouteChunks());
    expect(first).not.toHaveBeenCalled();

    idle.goIdle();
    await waitFor(() => expect(first).toHaveBeenCalledOnce());
    expect(second).not.toHaveBeenCalled();

    finishFirst();
    await waitFor(() => expect(second).toHaveBeenCalledOnce());
  });

  it("logs a chunk that fails and goes on to the next", async () => {
    const idle = stubIdle();
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    const broken = new Error("chunk gone");
    const next = vi.fn(() => Promise.resolve());
    chunks.loaders = [() => Promise.reject(broken), next];

    renderHook(() => useWarmRouteChunks());
    idle.goIdle();

    await waitFor(() => expect(next).toHaveBeenCalledOnce());
    expect(warn).toHaveBeenCalledWith("route chunk warm-up failed", broken);
  });

  it("cancels the idle request when the shell unmounts first", () => {
    const idle = stubIdle();
    const load = vi.fn(() => Promise.resolve());
    chunks.loaders = [load];

    const { unmount } = renderHook(() => useWarmRouteChunks());
    unmount();
    idle.goIdle();

    expect(idle.cancel).toHaveBeenCalledOnce();
    expect(load).not.toHaveBeenCalled();
  });

  it("stops between chunks once unmounted", async () => {
    const idle = stubIdle();
    let finishFirst: (value?: unknown) => void = () => undefined;
    const first = vi.fn(
      () =>
        new Promise((resolve) => {
          finishFirst = resolve;
        }),
    );
    const second = vi.fn(() => Promise.resolve());
    chunks.loaders = [first, second];

    const { unmount } = renderHook(() => useWarmRouteChunks());
    idle.goIdle();
    await waitFor(() => expect(first).toHaveBeenCalledOnce());
    unmount();
    finishFirst();
    await Promise.resolve();
    await Promise.resolve();

    expect(second).not.toHaveBeenCalled();
  });

  it("falls back to a timer where the browser has no idle callback", async () => {
    vi.stubGlobal("requestIdleCallback", undefined);
    const load = vi.fn(() => Promise.resolve());
    chunks.loaders = [load];

    renderHook(() => useWarmRouteChunks());

    await waitFor(() => expect(load).toHaveBeenCalledOnce());
  });
});
