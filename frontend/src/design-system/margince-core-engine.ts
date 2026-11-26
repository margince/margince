// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef, useState } from "react";
import type { MarginceCoreState } from "./margince-core";
import {
  dialsFrom,
  ease,
  readPalette,
  SETTLED,
  step,
} from "./margince-core-dials";
import type { CoreFrame, CoreRenderer } from "./margince-core-gl";
import {
  asHero,
  CALM_FRAME_MS,
  CALM_SPEED,
  type CoreBehaviour,
  CRAWL,
  CRAWL_FRAME_MS,
  CRAWL_SPEED,
  EASE,
  FEED_INGEST,
  FRAME_MS,
  MAX_STEP,
  PHASE_SEED,
  QUIET_AFTER_MS,
  rowFor,
  still,
} from "./margince-core-motion";
import {
  type PixelShift,
  pixelSnap,
  watchPlacement,
} from "./margince-core-snap";
import { usePrefersReducedMotion } from "./motion";
import {
  isWindowFocused,
  retainWindowFocusSignal,
  subscribeToWindowFocus,
} from "./window-focus";

/**
 * The Core's engine: it decides WHEN the sphere moves and how it gets from one
 * state to the next. What it looks like is the shader's business
 * (`margince-core-shader.ts`), and the numbers each state is made of are the
 * table's (`margince-core-motion.ts`).
 *
 * Three rules hold this file together.
 *
 * **Nothing snaps**, unless the reader asked for less motion. Every dial eases
 * toward the current state's target, so a state change reads as the material
 * changing. The phase is integrated, not derived from elapsed time, so `speed`
 * can change without the motion jumping.
 *
 * **The loop parks whenever nobody is watching, and only ever resumes on an
 * event.** It parks for an unfocused window, a settled state, a hidden tab or a
 * canvas out of view. Each condition must have an announced end, or the sphere
 * freezes for good.
 *
 * **It survives the machine.** No WebGL2, a refused compile, a context lost to
 * a GPU reset: the hook reports that it is not live and the component wears its
 * static dress instead.
 */

/** What the hook's callers can change under a running loop. */
export type Wanted = {
  behaviour: CoreBehaviour;
  paper: number;
  /** Reduced motion: every dial takes its target on the next frame. */
  snap?: boolean;
};

type Loop = Readonly<{
  wake: () => void;
  /** A reader is there: idle returns to its full drift. */
  poke: () => void;
  /** Parks the loop while the canvas is scrolled or clipped out of view. */
  setOnScreen: (onScreen: boolean) => void;
  /** Re-sizes the drawing buffer and draws once, whether parked or not. */
  remeasure: () => void;
  /** Re-lands the canvas on whole device pixels on the next frame, if seen. */
  resnap: () => void;
  stop: () => void;
}>;

/**
 * Where the pixels go.
 *
 * The GPU is the one true boundary this file has, and the loop's contract with
 * it (one advance per frame, one draw per budget, a draw on the frame that
 * parks) is a claim about timing that only a stand-in can be asked about: a
 * real context reports how it looks, never how often it was asked.
 */
export type CoreRendererFactory = (
  canvas: HTMLCanvasElement,
) => CoreRenderer | null;

export function runCoreLoop(
  canvas: HTMLCanvasElement,
  wanted: { current: Wanted },
  makeRenderer: CoreRendererFactory,
): Loop | null {
  const renderer = makeRenderer(canvas);
  if (!renderer) {
    return null;
  }
  const dials = dialsFrom(wanted.current.behaviour);
  let phase = PHASE_SEED;
  let paper = wanted.current.paper;
  let palette = readPalette();
  let handle = 0;
  let last = 0;
  let drawnAt = 0;
  let stopped = false;
  let onScreen = true;
  let activeAt = performance.now();
  let calm = 1;
  let shift: PixelShift = [0, 0];
  let snapHandle = 0;
  const watched = () => onScreen && !document.hidden && isWindowFocused();

  const place = (box: DOMRect) => {
    shift = pixelSnap(box, canvas.offsetWidth, devicePixelRatio || 1, shift);
    canvas.style.translate = `${shift[0]}px ${shift[1]}px`;
  };

  const measure = () => {
    // The Core is a ball, so one side serves: whichever the box gives, the
    // square drawing buffer is sized from it.
    const box = canvas.getBoundingClientRect();
    renderer.resize(Math.max(box.width, box.height), devicePixelRatio || 1);
    place(box);
  };

  /** Advances every dial one frame and reports whether anything is still
   *  changing. False is the signal to park: the object has arrived and is not
   *  advancing under its own speed. */
  const advance = (now: number): boolean => {
    const target = wanted.current.behaviour;
    if (wanted.current.snap) {
      Object.assign(dials, dialsFrom(target));
      paper = wanted.current.paper;
    }
    const moving = step(dials, target);
    paper = ease(paper, wanted.current.paper, EASE.tint);
    const quiet = now - activeAt > QUIET_AFTER_MS && target.speed < CALM_SPEED;
    const calmTarget = quiet ? CRAWL : 1;
    calm = ease(calm, calmTarget, EASE.calm);
    const dt = last === 0 ? 0 : Math.min((now - last) / 1000, MAX_STEP);
    last = now;
    phase += dt * dials.speed * calm;
    return (
      moving ||
      dials.speed > SETTLED ||
      Math.abs(calm - calmTarget) > SETTLED ||
      Math.abs(paper - wanted.current.paper) > SETTLED
    );
  };

  const shown = (): CoreFrame => ({
    level: dials.level,
    form: dials.form,
    phase,
    pulse: dials.pulse,
    ingest: dials.ingest,
    tint: dials.tint,
    tintCol: dials.tintCol,
    paper,
    work: palette.work,
    body: palette.body,
  });

  const tick = (now: number) => {
    // A slow drift needs no draw on every frame, and a heavy shader at the
    // display's rate costs battery. So the budget spaces out draws only.
    // `advance` still runs every frame: the dials ease one step per call.
    handle = requestAnimationFrame(tick);
    const drifting = advance(now);
    const pace = dials.speed * calm;
    const budget =
      pace < CRAWL_SPEED
        ? CRAWL_FRAME_MS
        : pace < CALM_SPEED
          ? CALM_FRAME_MS
          : FRAME_MS;
    if (now - drawnAt >= budget || !drifting) {
      drawnAt = now;
      renderer.draw(shown());
    }
    // Parked only after the frame that decides it, so a park is a park: the
    // next callback is never already in flight when the loop stops wanting one.
    // Every condition but a settled state ends with an event that wakes it.
    if (!drifting || stopped || !watched()) {
      cancelAnimationFrame(handle);
      handle = 0;
    }
  };

  const wake = () => {
    // Before the early returns, so a theme flip lands while the loop runs too.
    palette = readPalette();
    if (stopped || handle !== 0 || !watched()) {
      return;
    }
    measure();
    last = 0;
    drawnAt = 0;
    handle = requestAnimationFrame(tick);
  };

  /**
   * A size change has to land whether the loop is running or not.
   *
   * `wake` cannot carry this: it returns early when a frame is already
   * scheduled, which is exactly the case while the orb is animating, so a
   * running Core would keep the buffer it was born with and the browser would
   * scale it up. That is not a subtle artefact, it is a visibly soft ball.
   */
  const remeasure = () => {
    if (stopped) {
      return;
    }
    measure();
    renderer.draw(shown());
  };

  // A move needs no draw. Unseen, it waits: the next wake measures anyway.
  const resnap = () => {
    if (stopped || snapHandle !== 0 || !watched()) {
      return;
    }
    snapHandle = requestAnimationFrame(() => {
      snapHandle = 0;
      if (!stopped) {
        place(canvas.getBoundingClientRect());
      }
    });
  };

  const stop = () => {
    stopped = true;
    if (handle !== 0) {
      cancelAnimationFrame(handle);
      handle = 0;
    }
    if (snapHandle !== 0) {
      cancelAnimationFrame(snapHandle);
      snapHandle = 0;
    }
    // A restored context or a remount measures a clean box.
    canvas.style.translate = "";
    renderer.dispose();
  };

  measure();
  // One frame straight away, so a Core that mounts parked (an unfocused window,
  // reduced motion) is drawn rather than showing an empty canvas.
  renderer.draw(shown());
  wake();
  const setOnScreen = (next: boolean) => {
    onScreen = next;
    wake();
  };

  const poke = () => {
    activeAt = performance.now();
    wake();
  };

  return { wake, poke, setOnScreen, remeasure, resnap, stop };
}

/** Wakes the live loop on what says a reader may be looking: a resize, the
 *  window's focus, a pointer or focus on the orb, the canvas in view. */
function watchCanvas(
  canvas: HTMLCanvasElement,
  loopRef: { current: Loop | null },
  seen: (onScreen: boolean) => void,
): () => void {
  const poke = () => loopRef.current?.poke();
  const wake = () => loopRef.current?.wake();
  const observer = new ResizeObserver(() => {
    loopRef.current?.remeasure();
    wake();
  });
  observer.observe(canvas);
  const unplace = watchPlacement(
    () => loopRef.current?.resnap(),
    () => {
      loopRef.current?.remeasure();
      wake();
    },
  );
  const releaseFocus = subscribeToWindowFocus((focused) => {
    if (focused) {
      poke();
    }
  });
  // The row a caller marks with `data-core-wake` counts as the orb: a
  // pointer or focus there is a reader looking at it.
  const host =
    canvas.closest<HTMLElement>("[data-core-wake]") ?? canvas.parentElement;
  host?.addEventListener("pointerenter", poke);
  host?.addEventListener("focusin", poke);
  const sight = new IntersectionObserver((entries) => {
    for (const entry of entries) {
      seen(entry.isIntersecting);
    }
  });
  sight.observe(canvas);
  document.addEventListener("visibilitychange", wake);
  return () => {
    host?.removeEventListener("pointerenter", poke);
    host?.removeEventListener("focusin", poke);
    document.removeEventListener("visibilitychange", wake);
    unplace();
    sight.disconnect();
    releaseFocus();
    observer.disconnect();
  };
}

/** Runs the loop on a canvas through resize, focus, scroll, a hidden tab and a
 *  GPU reset: a restored context is empty, so the loop is built again. */
export function attachCoreLoop(
  canvas: HTMLCanvasElement,
  wanted: { current: Wanted },
  makeRenderer: CoreRendererFactory,
  loopRef: { current: Loop | null },
  setLive: (live: boolean) => void,
): () => void {
  let onScreen = true;
  let unwatch: (() => void) | undefined;
  const seen = (next: boolean) => {
    onScreen = next;
    loopRef.current?.setOnScreen(onScreen);
  };
  const begin = () => {
    const loop = runCoreLoop(canvas, wanted, makeRenderer);
    loop?.setOnScreen(onScreen);
    loopRef.current = loop;
    setLive(loop !== null);
    // Watched from the first loop that runs, so a host that cannot draw yet
    // keeps no observers until a restore lets it.
    if (loop && !unwatch) {
      unwatch = watchCanvas(canvas, loopRef, seen);
    }
  };
  // Default prevented, or the browser never offers the context back.
  const onLost = (event: Event) => {
    event.preventDefault();
    loopRef.current?.stop();
    loopRef.current = null;
    setLive(false);
  };
  // Before the first renderer is asked for, so a context lost at birth still
  // starts the loop once it is restored.
  canvas.addEventListener("webglcontextlost", onLost);
  canvas.addEventListener("webglcontextrestored", begin);
  begin();
  return () => {
    canvas.removeEventListener("webglcontextlost", onLost);
    canvas.removeEventListener("webglcontextrestored", begin);
    unwatch?.();
    loopRef.current?.stop();
    loopRef.current = null;
  };
}

/**
 * The target row for a Core, given everything outside the state table that has
 * a say in it.
 *
 * `feed` is the caller telling us context is ARRIVING, which is a fact about the
 * surface rather than about the agent's state: a dock that is being fed while
 * the agent rests is a real combination. It raises the intake floor and leaves
 * everything else the state's own.
 */
function wants(
  state: MarginceCoreState,
  reduced: boolean,
  feed: boolean,
  hero: boolean,
): CoreBehaviour {
  const row = reduced ? still(state) : rowFor(state);
  const base = hero && !reduced ? asHero(row) : row;
  if (!feed || reduced) {
    return base;
  }
  return { ...base, ingest: Math.max(base.ingest, FEED_INGEST) };
}

/**
 * Mounts the engine on a canvas and reports whether it is live.
 *
 * False means the host cannot run it, and the component owes the reader its
 * static dress. It is false on the first render by definition: a context cannot
 * be made before the canvas is in the document.
 *
 * The window-focus signal is held for the orb's whole life rather than inside
 * the loop, because the stylesheet's half of the same stillness is driven by the
 * `data-window-blurred` attribute that signal maintains, and that half runs even
 * where this loop is parked.
 */
export function useCoreEngine(
  canvasRef: React.RefObject<HTMLCanvasElement | null>,
  state: MarginceCoreState,
  options: Readonly<{
    paper: boolean;
    feed: boolean;
    hero: boolean;
    /**
     * Whether the page the Core sits on is dark right now.
     *
     * `paper` alone misses a `surface="dark"` Core (the workspace rail): that
     * one holds `paper` at a constant 0 regardless of theme, so a flip would
     * never touch the effect below and the palette would sit unread until the
     * loop next woke for some other reason. Optional so a caller mid-migration
     * (or a test driving the hook directly) still compiles without it; the
     * loop simply re-reads a theme-invariant palette in that case.
     */
    dark?: boolean;
  }>,
): boolean {
  const reduced = usePrefersReducedMotion();
  const [live, setLive] = useState(false);
  const { paper, feed, hero, dark } = options;
  const wanted = useRef<Wanted>({
    behaviour: wants(state, reduced, feed, hero),
    paper: paper ? 1 : 0,
    snap: reduced,
  });

  useEffect(retainWindowFocusSignal, []);

  // Targets are written through a ref, so a state change bends a running loop
  // instead of tearing the GL context down and building another one. The
  // dependencies are the primitives the target is DERIVED from, not the derived
  // object: a fresh table row every render would rerun this every render.
  const loopRef = useRef<Loop | null>(null);
  // dark is not read below; it exists only to retrigger this effect on a theme
  // flip that paper alone would miss (a surface="dark" Core holds paper
  // constant), and the wake() call inside is what actually re-reads the palette.
  // biome-ignore lint/correctness/useExhaustiveDependencies(dark): see above.
  useEffect(() => {
    wanted.current.behaviour = wants(state, reduced, feed, hero);
    wanted.current.paper = paper ? 1 : 0;
    wanted.current.snap = reduced;
    loopRef.current?.poke();
  }, [state, reduced, feed, paper, hero, dark]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) {
      return;
    }
    let cancelled = false;
    let release: (() => void) | undefined;
    void import("./margince-core-gl")
      .then(({ createCoreRenderer }) => {
        if (!cancelled) {
          release = attachCoreLoop(
            canvas,
            wanted,
            createCoreRenderer,
            loopRef,
            setLive,
          );
        }
      })
      .catch((error: unknown) =>
        console.error("Core animation could not be loaded", error),
      );
    return () => {
      cancelled = true;
      release?.();
    };
    // Only the canvas: the loop reads everything else through the ref above, so
    // a state change bends the running loop instead of rebuilding the GL context
    // under it.
  }, [canvasRef]);

  return live;
}
