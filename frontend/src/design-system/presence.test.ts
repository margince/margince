/** @vitest-environment happy-dom */
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { usePresence } from "./presence";

// The contract is "stay mounted until the exit is OVER", and the only thing
// that knows when it is over is the element. So every case here is written
// against an element whose `getAnimations` answers something specific — a
// running exit, nothing at all, a cancelled one — and never against a clock.

afterEach(cleanup);

/**
 * An element that reports exactly the animations a case is about.
 *
 * Only `finished` is ever read, so the doubles are exactly that. A fuller
 * `Animation` would be this file keeping its own copy of the browser's.
 */
function elementRunning(...finished: Promise<unknown>[]) {
  return elementReporting(finished.map((promise) => animation(promise, 1)));
}

/** One animation, with the iteration count that decides whether it can end. */
function animation(finished: Promise<unknown>, iterations: number): Animation {
  return {
    finished,
    effect: { getComputedTiming: () => ({ iterations }) },
  } as unknown as Animation;
}

function elementReporting(animations: Animation[]) {
  const element = document.createElement("div");
  element.getAnimations = () => animations;
  return { current: element };
}

/** An environment with no Web Animations API at all, which jsdom is. */
function elementWithoutAnimations() {
  const element = document.createElement("div");
  Object.defineProperty(element, "getAnimations", { value: undefined });
  return { current: element };
}

function deferred() {
  let settle: () => void = () => undefined;
  let refuse: () => void = () => undefined;
  const promise = new Promise<void>((resolve, reject) => {
    settle = resolve;
    refuse = () => reject(new DOMException("cancelled", "AbortError"));
  });
  return { promise, settle, refuse };
}

describe("a dismissed surface stays mounted while its exit plays", () => {
  it("is mounted and open while open", () => {
    const element = elementRunning();
    const { result } = renderHook(() => usePresence({ open: true, element }));
    expect(result.current).toEqual({ mounted: true, state: "open" });
  });

  it("was never mounted, so a surface that starts closed never closes", () => {
    const element = elementRunning();
    const { result } = renderHook(() => usePresence({ open: false, element }));
    expect(result.current).toEqual({ mounted: false, state: "open" });
  });

  it("holds the surface in a closing state until the exit finishes", async () => {
    const exit = deferred();
    const element = elementRunning(exit.promise);
    const { result, rerender } = renderHook(
      ({ open }) => usePresence({ open, element }),
      { initialProps: { open: true } },
    );

    rerender({ open: false });
    expect(result.current).toEqual({ mounted: true, state: "closing" });

    await act(async () => {
      exit.settle();
      await exit.promise;
    });
    expect(result.current.mounted).toBe(false);
  });

  it("waits for the LAST of several animations", async () => {
    const scrim = deferred();
    const panel = deferred();
    const element = elementRunning(scrim.promise, panel.promise);
    const { result, rerender } = renderHook(
      ({ open }) => usePresence({ open, element }),
      { initialProps: { open: true } },
    );

    rerender({ open: false });
    await act(async () => {
      scrim.settle();
      await scrim.promise;
    });
    // The scrim is gone and the panel is still travelling. Unmounting here
    // would cut the panel off halfway, which is the defect a duration-in-JS
    // implementation produces whenever the two lengths differ.
    expect(result.current).toEqual({ mounted: true, state: "closing" });

    await act(async () => {
      panel.settle();
      await panel.promise;
    });
    expect(result.current.mounted).toBe(false);
  });

  it("treats a cancelled animation as finished rather than waiting forever", async () => {
    const exit = deferred();
    const element = elementRunning(exit.promise);
    const { result, rerender } = renderHook(
      ({ open }) => usePresence({ open, element }),
      { initialProps: { open: true } },
    );

    rerender({ open: false });
    // `finished` rejects with AbortError when the animation is cancelled — a
    // rule replaced its keyframes, or the reader re-opened the surface. A
    // rejection left to propagate would strand the overlay on the page.
    await act(async () => {
      exit.refuse();
      await exit.promise.catch(() => undefined);
    });
    expect(result.current.mounted).toBe(false);
  });

  it("unmounts in the same effect when nothing is animating", () => {
    // Reduced motion, and the same path `animation: none` takes: the rules that
    // animate are behind `prefers-reduced-motion: no-preference`, so there is
    // nothing to wait for and the surface must go at once rather than after a
    // frame of empty scrim.
    const element = elementRunning();
    const { result, rerender } = renderHook(
      ({ open }) => usePresence({ open, element }),
      { initialProps: { open: true } },
    );

    rerender({ open: false });
    expect(result.current).toEqual({ mounted: false, state: "open" });
  });

  it("does not wait on a loop, which is not an exit", async () => {
    // A skeleton shimmer, a spinner, the pulse under a write still out — the
    // subtree walk finds every one of them, and none of them ever finishes.
    // Waiting would leave the dialog on the screen for the rest of the session.
    const forever = new Promise<void>(() => undefined);
    const exit = deferred();
    const element = elementReporting([
      animation(forever, Number.POSITIVE_INFINITY),
      animation(exit.promise, 1),
    ]);
    const { result, rerender } = renderHook(
      ({ open }) => usePresence({ open, element }),
      { initialProps: { open: true } },
    );

    rerender({ open: false });
    expect(result.current.state).toBe("closing");
    await act(async () => {
      exit.settle();
      await exit.promise;
    });
    expect(result.current.mounted).toBe(false);
  });

  it("unmounts at once where the environment has no getAnimations", () => {
    const element = elementWithoutAnimations();
    const { result, rerender } = renderHook(
      ({ open }) => usePresence({ open, element }),
      { initialProps: { open: true } },
    );

    rerender({ open: false });
    expect(result.current).toEqual({ mounted: false, state: "open" });
  });

  it("returns to open when the reader re-opens mid-exit", async () => {
    const exit = deferred();
    const element = elementRunning(exit.promise);
    const { result, rerender } = renderHook(
      ({ open }) => usePresence({ open, element }),
      { initialProps: { open: true } },
    );

    rerender({ open: false });
    expect(result.current.state).toBe("closing");

    rerender({ open: true });
    expect(result.current).toEqual({ mounted: true, state: "open" });

    // The exit's promise rejecting late must not then unmount the surface the
    // reader has re-opened.
    await act(async () => {
      exit.refuse();
      await exit.promise.catch(() => undefined);
    });
    expect(result.current).toEqual({ mounted: true, state: "open" });
  });
});
