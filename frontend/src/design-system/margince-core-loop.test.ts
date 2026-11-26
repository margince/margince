// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { afterEach, describe, expect, it, vi } from "vitest";
import {
  attachCoreLoop,
  runCoreLoop,
  type Wanted,
} from "./margince-core-engine";
import type { CoreFrame, CoreRenderer } from "./margince-core-gl";
import { FRAME_MS, rowFor, still } from "./margince-core-motion";
import { pixelSnap } from "./margince-core-snap";

// The draw loop's contract with the GPU, which is a contract about WHEN rather
// than about pixels. `margince-core-engine.test.tsx` covers the same engine from
// the outside, on a host with no WebGL2 at all, and can therefore say nothing
// about a running loop: jsdom refuses the context, so no loop is ever built.
// Here the context is stood in for, because a stand-in is the only thing that
// can be asked how often it was drawn to. What it is NOT asked is how anything
// looks: every expectation below is a count or a comparison between two runs of
// the real code.

/** A renderer that keeps every frame it was handed, and when. */
function recorder(clock: () => number): CoreRenderer & {
  frames: CoreFrame[];
  at: number[];
} {
  const frames: CoreFrame[] = [];
  const at: number[] = [];
  return {
    frames,
    at,
    resize: () => {},
    draw: (frame) => {
      // The engine hands out its own live dials object, so a stored reference
      // would read the LAST frame's numbers from every slot in this array.
      frames.push({ ...frame, tintCol: [...frame.tintCol] });
      at.push(clock());
    },
    dispose: () => {},
  };
}

/**
 * Runs a loop for a stretch of time, granting frames at a fixed interval.
 *
 * The interval is the display's, which is the variable the budget exists to be
 * independent of: a 60 Hz screen offers frames twice as often as a 30 Hz one and
 * must still get the same animation out of the same wall-clock time.
 */
function runFor(
  ms: number,
  everyMs: number,
  target: Wanted,
): { frames: CoreFrame[]; at: number[]; parked: boolean } {
  // jsdom's document reports no focus, and the loop parks on an unfocused
  // window: without this it never asks for a frame at all, and every count
  // below would be zero for a reason that has nothing to do with the budget.
  vi.spyOn(document, "hasFocus").mockReturnValue(true);
  const canvas = document.createElement("canvas");
  let now = 0;
  const renderer = recorder(() => now);
  // The frames nobody has spent yet. A queue rather than one slot because the
  // loop only ever holds one request at a time, and a queue that grows past one
  // would be a defect this harness should not hide.
  const asked: FrameRequestCallback[] = [];
  vi.spyOn(globalThis, "requestAnimationFrame").mockImplementation((cb) => {
    asked.push(cb);
    return asked.length;
  });
  vi.spyOn(globalThis, "cancelAnimationFrame").mockImplementation(() => {
    asked.length = 0;
  });

  const wanted = { current: target };
  const loop = runCoreLoop(canvas, wanted, () => renderer);
  if (!loop) {
    throw new Error("the loop refused a renderer that was handed to it");
  }
  // The mount draw is not part of what the budget governs, and counting it as
  // one would make every draw total off by one.
  const mounted = renderer.frames.length;

  for (now = everyMs; now <= ms; now += everyMs) {
    const frame = asked.shift();
    if (!frame) {
      break;
    }
    frame(now);
  }
  const parked = asked.length === 0;
  loop.stop();
  return {
    frames: renderer.frames.slice(mounted),
    at: renderer.at.slice(mounted),
    parked,
  };
}

/** A state that keeps drifting, so the loop under test never parks early. */
const drifting = (): Wanted => ({ behaviour: rowFor("ingest"), paper: 0 });

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("the Core's draw budget", () => {
  it("draws about thirty times a second, not once per offered frame", () => {
    // The whole point of the budget: a fragment-heavy shader on a 120 Hz panel
    // costs four times what the motion needs, forever, on every screen.
    const { frames } = runFor(1000, 1000 / 60, drifting());

    expect(frames.length).toBeLessThan(40);
    expect(frames.length).toBeGreaterThan(20);
  });

  it("never draws twice inside one budget window, at any offered rate", () => {
    // The rule rather than a total, because a total is only the rule plus
    // whichever rounding the offered interval happens to impose: a display that
    // offers frames every 8ms cannot land a draw exactly on the budget, so it
    // waits out the next whole offer. Both bounds together say the loop draws as
    // soon as it may and never sooner.
    // Whole milliseconds, so no case sits exactly on the budget: a 33.333ms
    // offer against a 33.333ms budget decides on the last bit of a float, which
    // is a property of the arithmetic rather than of the loop.
    for (const everyMs of [8, 16, 40]) {
      const { at } = runFor(1000, everyMs, drifting());
      const gaps = at.slice(1).map((moment, i) => moment - (at[i] ?? 0));

      expect(gaps.length).toBeGreaterThan(5);
      for (const gap of gaps) {
        expect(gap).toBeGreaterThanOrEqual(FRAME_MS);
        expect(gap).toBeLessThan(FRAME_MS + everyMs);
      }
    }
  });
});

describe("the Core's motion, against the budget", () => {
  it("reaches the same point in the same time on a faster display", () => {
    // The defect this case exists for: easing once per DRAW rather than once per
    // frame made the orb take twice as long to reach a state on a 60 Hz screen
    // as the table is written for. The two runs cover the same second of wall
    // clock, so the last frame each drew has to describe the same object.
    const slow = runFor(1000, 1000 / 30, drifting()).frames.at(-1);
    const fast = runFor(1000, 1000 / 120, drifting()).frames.at(-1);
    if (!slow || !fast) {
      throw new Error("a drifting Core drew nothing over a whole second");
    }

    // Not equality: the two runs land their last frame at different moments
    // inside the same second, and one eased step apart is the honest tolerance.
    expect(fast.level).toBeCloseTo(slow.level, 2);
    expect(fast.ingest).toBeCloseTo(slow.ingest, 2);
    expect(fast.phase).toBeCloseTo(slow.phase, 1);
  });

  it("advances the phase under its own speed rather than by frame count", () => {
    const { frames } = runFor(1000, 1000 / 60, drifting());
    const first = frames[0];
    const last = frames.at(-1);
    if (!first || !last) {
      throw new Error("a drifting Core drew nothing over a whole second");
    }

    expect(last.phase).toBeGreaterThan(first.phase);
  });
});

describe("the Core's parking", () => {
  it("draws the frame it parks on, so a settled orb is never a stale one", () => {
    // A budget that skipped this frame would leave the last drawn position one
    // step short of where the object came to rest, and nothing would ever
    // correct it: the loop is parked, by definition.
    const settled: Wanted = { behaviour: rowFor("idle"), paper: 0 };
    const still: Wanted = {
      behaviour: { ...settled.behaviour, speed: 0 },
      paper: 0,
    };
    const { frames, parked } = runFor(1000, 1000 / 60, still);

    expect(parked).toBe(true);
    expect(frames.length).toBeGreaterThan(0);
  });

  it("keeps running while the state it was given still drifts", () => {
    expect(runFor(1000, 1000 / 60, drifting()).parked).toBe(false);
  });
});

describe("the Core under reduced motion", () => {
  /** The first frame drawn after a still Core turns from idle to error. */
  function firstFrameAfterTurning(snap: boolean): CoreFrame {
    vi.spyOn(document, "hasFocus").mockReturnValue(true);
    const asked: FrameRequestCallback[] = [];
    vi.spyOn(globalThis, "requestAnimationFrame").mockImplementation((cb) => {
      asked.push(cb);
      return asked.length;
    });
    vi.spyOn(globalThis, "cancelAnimationFrame").mockImplementation(() => {
      asked.length = 0;
    });
    const renderer = recorder(() => 0);
    const wanted = { current: { behaviour: still("idle"), paper: 0, snap } };
    const loop = runCoreLoop(
      document.createElement("canvas"),
      wanted,
      () => renderer,
    );
    asked.shift()?.(FRAME_MS);
    wanted.current.behaviour = still("error");
    loop?.poke();
    asked.shift()?.(FRAME_MS * 2);
    loop?.stop();
    const frame = renderer.frames.at(-1);
    if (!frame) {
      throw new Error("the Core drew nothing after its state changed");
    }
    return frame;
  }

  it("takes the new state's colour on the next frame", () => {
    expect(firstFrameAfterTurning(true).tint).toBe(1);
    expect(firstFrameAfterTurning(true).tintCol).toEqual(
      still("error").tintCol,
    );
  });

  it("eases into it otherwise", () => {
    expect(firstFrameAfterTurning(false).tint).toBeLessThan(0.5);
  });
});

describe("the Core through a GPU reset", () => {
  it("drops to its still dress on a lost context and goes live again on restore", () => {
    vi.spyOn(globalThis, "requestAnimationFrame").mockReturnValue(1);
    vi.spyOn(globalThis, "cancelAnimationFrame").mockImplementation(() => {});
    const canvas = document.createElement("canvas");
    const built: ReturnType<typeof recorder>[] = [];
    const live: boolean[] = [];
    const release = attachCoreLoop(
      canvas,
      { current: { behaviour: rowFor("idle"), paper: 0 } },
      () => {
        const renderer = recorder(() => 0);
        built.push(renderer);
        return renderer;
      },
      { current: null },
      (now) => live.push(now),
    );

    const lost = new Event("webglcontextlost", { cancelable: true });
    canvas.dispatchEvent(lost);
    canvas.dispatchEvent(new Event("webglcontextrestored"));
    release();

    // Not prevented, and the browser would never offer the context back.
    expect(lost.defaultPrevented).toBe(true);
    expect(live).toEqual([true, false, true]);
    expect(built).toHaveLength(2);
    expect(built[1]?.frames.length).toBeGreaterThan(0);
  });

  it("goes live on restore when the context it was born with was already lost", () => {
    vi.spyOn(globalThis, "requestAnimationFrame").mockReturnValue(1);
    vi.spyOn(globalThis, "cancelAnimationFrame").mockImplementation(() => {});
    const canvas = document.createElement("canvas");
    const renderer = recorder(() => 0);
    const offered = [null, renderer];
    const live: boolean[] = [];
    const release = attachCoreLoop(
      canvas,
      { current: { behaviour: rowFor("idle"), paper: 0 } },
      () => offered.shift() ?? null,
      { current: null },
      (now) => live.push(now),
    );

    canvas.dispatchEvent(new Event("webglcontextrestored"));
    release();

    expect(live).toEqual([false, true]);
    expect(renderer.frames.length).toBeGreaterThan(0);
  });
});

describe("the Core out of sight", () => {
  it("parks once its canvas reports it is not in view", () => {
    vi.spyOn(document, "hasFocus").mockReturnValue(true);
    const asked: FrameRequestCallback[] = [];
    vi.spyOn(globalThis, "requestAnimationFrame").mockImplementation((cb) => {
      asked.push(cb);
      return asked.length;
    });
    vi.spyOn(globalThis, "cancelAnimationFrame").mockImplementation(() => {
      asked.length = 0;
    });
    // A hidden element has no box, and the browser reports it out of view.
    let report: (entries: { isIntersecting: boolean }[]) => void = () => {};
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        constructor(callback: typeof report) {
          report = callback;
        }
        observe() {}
        disconnect() {}
      },
    );
    const canvas = document.createElement("canvas");
    const renderer = recorder(() => 0);
    const release = attachCoreLoop(
      canvas,
      { current: drifting() },
      () => renderer,
      { current: null },
      () => {},
    );
    expect(asked).toHaveLength(1);

    report([{ isIntersecting: false }]);
    asked.shift()?.(FRAME_MS);
    const drawn = renderer.frames.length;
    release();

    expect(asked).toHaveLength(0);
    expect(drawn).toBe(renderer.frames.length);
  });
});

describe("the Core on whole device pixels", () => {
  it("nudges a canvas that sits between pixels onto the grid", () => {
    const box = { left: 643.875, top: 119.875, width: 152 };
    expect(pixelSnap(box, 152, 2, [0, 0])).toEqual([0.125, 0.125]);
  });

  it("keeps the same nudge when measured with that nudge applied", () => {
    const nudged = { left: 644, top: 120, width: 152 };
    expect(pixelSnap(nudged, 152, 2, [0.125, 0.125])).toEqual([0.125, 0.125]);
  });

  it("leaves a canvas already on the grid where it is", () => {
    expect(pixelSnap({ left: 12.5, top: 3, width: 50 }, 50, 2, [0, 0])).toEqual(
      [0, 0],
    );
  });

  it("scales the nudge into the canvas's own CSS px under a scaled ancestor", () => {
    // At ratio 2, a quarter device px is 0.125 CSS px on screen, 0.25 inside.
    const halved = { left: 10.125, top: 0, width: 50 };
    expect(pixelSnap(halved, 100, 2, [0, 0])).toEqual([-0.25, 0]);
  });
});

describe("the Core's placement through the page's events", () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  /** A parked Core on a fractional box, with frames and ratio queries held. */
  function placed(layoutWidth = 152) {
    vi.spyOn(document, "hasFocus").mockReturnValue(true);
    const frames = new Map<number, FrameRequestCallback>();
    let ids = 0;
    vi.spyOn(globalThis, "requestAnimationFrame").mockImplementation((cb) => {
      ids += 1;
      frames.set(ids, cb);
      return ids;
    });
    vi.spyOn(globalThis, "cancelAnimationFrame").mockImplementation((id) => {
      frames.delete(id);
    });
    vi.stubGlobal("devicePixelRatio", 2);
    const ratio = { asked: 0, added: 0, removed: 0, change: () => {} };
    vi.stubGlobal("matchMedia", () => {
      ratio.asked += 1;
      return {
        addEventListener: (_type: string, listener: () => void) => {
          ratio.added += 1;
          ratio.change = listener;
        },
        removeEventListener: () => {
          ratio.removed += 1;
        },
      };
    });
    const host = document.createElement("div");
    const canvas = document.createElement("canvas");
    host.append(canvas);
    document.body.append(host);
    const box = { left: 643.875, width: 152 };
    vi.spyOn(canvas, "offsetWidth", "get").mockReturnValue(layoutWidth);
    // Like a browser, the measured box carries the nudge, scaled by the ancestor.
    const nudge = () => Number.parseFloat(canvas.style.translate) || 0;
    const scale = box.width / layoutWidth;
    vi.spyOn(canvas, "getBoundingClientRect").mockImplementation(
      () => new DOMRect(box.left + nudge() * scale, 120, box.width, box.width),
    );
    const work = { sized: 0, drawn: 0 };
    const loopRef: Parameters<typeof attachCoreLoop>[3] = { current: null };
    const release = attachCoreLoop(
      canvas,
      { current: { behaviour: still("idle"), paper: 0, snap: true } },
      () => ({
        ...recorder(() => 0),
        resize: () => {
          work.sized += 1;
        },
        draw: () => {
          work.drawn += 1;
        },
      }),
      loopRef,
      () => {},
    );
    // Runs the frames asked for so far, so the loop parks before a case begins.
    const flush = () => {
      for (const [id, frame] of [...frames]) {
        frames.delete(id);
        frame(0);
      }
    };
    flush();
    const settled = { ...work };
    return {
      canvas,
      host,
      box,
      work,
      settled,
      frames,
      flush,
      ratio,
      release,
      loopRef,
    };
  }

  it("re-lands on whole pixels after a resize, in one frame, without drawing", () => {
    const core = placed();
    expect(core.canvas.style.translate).toBe("0.125px 0px");

    core.box.left = 300.25;
    window.dispatchEvent(new Event("resize"));
    window.dispatchEvent(new Event("resize"));
    expect(core.frames.size).toBe(1);
    core.flush();

    expect(core.canvas.style.translate).toBe("0.25px 0px");
    expect(core.work).toEqual(core.settled);
    core.release();
  });

  it("ignores a transition that ends outside the canvas and its ancestors", () => {
    const core = placed();
    const elsewhere = document.createElement("div");
    document.body.append(elsewhere);
    core.box.left = 300.25;

    elsewhere.dispatchEvent(new Event("transitionend", { bubbles: true }));
    expect(core.frames.size).toBe(0);

    core.host.dispatchEvent(new Event("animationend", { bubbles: true }));
    core.flush();
    expect(core.canvas.style.translate).toBe("0.25px 0px");
    expect(core.work).toEqual(core.settled);
    core.release();
  });

  it("waits while off screen, and lands again on the next wake", () => {
    const core = placed();
    core.loopRef.current?.setOnScreen(false);
    core.box.left = 300.25;
    window.dispatchEvent(new Event("resize"));
    expect(core.frames.size).toBe(0);

    core.loopRef.current?.setOnScreen(true);
    expect(core.canvas.style.translate).toBe("0.25px 0px");
    core.release();
  });

  it("re-measures, draws and listens again when the pixel ratio changes", () => {
    const core = placed();
    core.ratio.change();

    expect(core.work.sized).toBeGreaterThan(core.settled.sized);
    expect(core.work.drawn).toBeGreaterThan(core.settled.drawn);
    expect(core.ratio.asked).toBe(2);
    expect(core.ratio.removed).toBe(1);
    core.release();
  });

  it("follows an ancestor's scale through the canvas's own layout width", () => {
    const core = placed(304);
    // Drawn at half scale, so the quarter device px nudge is 0.25 CSS px inside.
    expect(core.canvas.style.translate).toBe("0.25px 0px");
    core.release();
  });

  it("removes every listener it added, and the nudge, on unmount", () => {
    const counted = ["resize", "animationend", "transitionend"];
    const tally = (target: EventTarget) => {
      const added = vi.spyOn(target, "addEventListener");
      const removed = vi.spyOn(target, "removeEventListener");
      return () =>
        counted.map((type) => [
          added.mock.calls.filter(([name]) => name === type).length,
          removed.mock.calls.filter(([name]) => name === type).length,
        ]);
    };
    const onWindow = tally(window);
    const onDocument = tally(document);
    const core = placed();
    core.release();

    for (const [added, removed] of [...onWindow(), ...onDocument()]) {
      expect(removed).toBe(added);
    }
    expect(onWindow()[0]?.[0]).toBe(1);
    expect(core.ratio.removed).toBe(core.ratio.added);
    expect(core.canvas.style.translate).toBe("");
  });
});
