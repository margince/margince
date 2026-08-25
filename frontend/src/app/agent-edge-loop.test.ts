// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

import { afterEach, describe, expect, it, vi } from "vitest";
import type { EdgeFrame, EdgeHues, EdgeRenderer } from "./agent-edge-gl";
import { FADE, FRAME_MS, runEdgeLoop } from "./agent-edge-loop";

// The loop's contract with the GPU, which is a contract about WHEN and not about
// pixels. jsdom answers `getContext("webgl2")` with null, so the real renderer
// cannot be built here and never runs; a stand-in is the only thing that can be
// asked how often it was drawn to, and how often is the whole question. Nothing
// below asserts what anything looks like.

const HUES: EdgeHues = [
  [0, 0, 0],
  [0, 0, 0],
  [0, 0, 0],
  [0, 0, 0],
  [0, 0, 0],
];

/** A renderer that records what it was asked to do, and nothing else. */
function recorder(): EdgeRenderer & {
  frames: EdgeFrame[];
  sizes: number[][];
  disposed: () => number;
} {
  const frames: EdgeFrame[] = [];
  const sizes: number[][] = [];
  let disposed = 0;
  return {
    frames,
    sizes,
    disposed: () => disposed,
    resize: (w, h, dpr) => {
      sizes.push([w, h, dpr]);
    },
    draw: (frame) => {
      frames.push({ ...frame });
    },
    dispose: () => {
      disposed += 1;
    },
  };
}

/** Drives rAF by hand, so a "frame" is something this test decides to grant. */
function clock() {
  let pending: FrameRequestCallback | null = null;
  vi.spyOn(globalThis, "requestAnimationFrame").mockImplementation((cb) => {
    pending = cb;
    return 1;
  });
  vi.spyOn(globalThis, "cancelAnimationFrame").mockImplementation(() => {
    pending = null;
  });
  return {
    /** Offers frames at a fixed interval, the way a display does. */
    run(untilMs: number, everyMs: number) {
      for (let now = everyMs; now <= untilMs; now += everyMs) {
        const frame = pending;
        if (!frame) {
          return;
        }
        pending = null;
        frame(now);
      }
    },
    parked: () => pending === null,
  };
}

function start(
  renderer: EdgeRenderer | null,
  over: Partial<{ reduced: boolean; onLost: () => void }> = {},
) {
  const canvas = document.createElement("canvas");
  const loop = runEdgeLoop(canvas, HUES, {
    reduced: over.reduced ?? false,
    onLost: over.onLost ?? (() => {}),
    makeRenderer: () => renderer,
  });
  return { canvas, loop };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("the edge's draw loop", () => {
  it("reports the host cannot draw, rather than throwing", () => {
    // Every way this fails is the same answer to the caller: a locked-down
    // browser, a refused compile, a GPU with no WebGL2. The caller has a static
    // rim for exactly this, and it only gets to show it if nothing throws.
    clock();
    expect(start(null).loop).toBeNull();
  });

  it("sizes the buffer from the window before the first frame", () => {
    // Drawing into a buffer of the wrong size is the one thing that cannot be
    // corrected later by a frame: the first picture a reader sees is stretched.
    const paint = clock();
    const renderer = recorder();
    start(renderer);

    expect(renderer.sizes.length).toBe(1);
    expect(renderer.sizes[0]?.[0]).toBe(window.innerWidth);
    expect(paint.parked()).toBe(false);
  });

  it("draws about thirty times a second, not once per offered frame", () => {
    // The budget is why a fullscreen window stays smooth. At a display's own
    // rate this shader draws the same picture twice.
    const paint = clock();
    const renderer = recorder();
    start(renderer);
    paint.run(1000, 1000 / 60);

    expect(renderer.frames.length).toBeLessThan(40);
    expect(renderer.frames.length).toBeGreaterThan(20);
  });

  it("spends the same draws whether the display offers 60 or 120 frames", () => {
    // Rate independence, stated as the property rather than as a total: the
    // budget exists to make the draw count a function of TIME and not of the
    // panel somebody happens to own.
    const paint60 = clock();
    const sixty = recorder();
    start(sixty);
    paint60.run(1000, 1000 / 60);
    vi.restoreAllMocks();

    const paint120 = clock();
    const oneTwenty = recorder();
    start(oneTwenty);
    paint120.run(1000, 1000 / 120);

    expect(
      Math.abs(sixty.frames.length - oneTwenty.frames.length),
    ).toBeLessThanOrEqual(4);
  });

  it("never draws twice inside one budget window", () => {
    const paint = clock();
    const renderer = recorder();
    start(renderer);
    paint.run(1000, 8);
    const times = renderer.frames.map((frame) => frame.time);
    const gaps = times.slice(1).map((t, i) => (t - (times[i] ?? 0)) * 1000);

    expect(gaps.length).toBeGreaterThan(5);
    for (const gap of gaps) {
      expect(gap).toBeGreaterThanOrEqual(FRAME_MS - 0.001);
    }
  });

  it("brings the light up rather than switching it on", () => {
    // An edge that appeared at full strength would read as a fault. The fade is
    // in the shader's own level uniform, not in a CSS opacity, so it cannot
    // fight the canvas it is drawn on.
    const paint = clock();
    const renderer = recorder();
    start(renderer);
    paint.run(FADE * 1000 * 2, 1000 / 60);
    const first = renderer.frames[0];
    const last = renderer.frames.at(-1);
    if (!first || !last) {
      throw new Error("the loop drew nothing over two fade lengths");
    }

    expect(first.level).toBeLessThan(0.5);
    expect(last.level).toBe(1);
  });

  it("advances the wave with the clock, so the crests travel", () => {
    const paint = clock();
    const renderer = recorder();
    start(renderer);
    paint.run(1000, 1000 / 60);
    const times = renderer.frames.map((frame) => frame.time);

    expect(times.at(-1) ?? 0).toBeGreaterThan(times[0] ?? 0);
  });

  it("holds the waves still for a reader who asked for less movement", () => {
    // The light survives and the travel does not: the reading is carried by the
    // edge being lit at all, so reduced motion must not cost it.
    const paint = clock();
    const renderer = recorder();
    start(renderer, { reduced: true });
    paint.run(1000, 1000 / 60);

    expect(renderer.frames.length).toBeGreaterThan(5);
    for (const frame of renderer.frames) {
      expect(frame.time).toBe(0);
    }
    expect(renderer.frames.at(-1)?.level).toBe(1);
  });

  it("gives the GPU objects back when it stops, once", () => {
    const paint = clock();
    const renderer = recorder();
    const { loop } = start(renderer);
    loop?.stop();
    loop?.stop();

    expect(renderer.disposed()).toBe(1);
    expect(paint.parked()).toBe(true);
  });

  it("stops drawing when the context is lost, and says so", () => {
    // Reporting alone would leave the loop drawing into a context that no longer
    // exists, once a frame, for as long as the page is open.
    const paint = clock();
    const renderer = recorder();
    const lost = vi.fn();
    const { canvas } = start(renderer, { onLost: lost });
    paint.run(200, 1000 / 60);
    const drawn = renderer.frames.length;

    canvas.dispatchEvent(new Event("webglcontextlost", { cancelable: true }));
    paint.run(1000, 1000 / 60);

    expect(lost).toHaveBeenCalledOnce();
    expect(renderer.frames.length).toBe(drawn);
    expect(renderer.disposed()).toBe(1);
  });
});
