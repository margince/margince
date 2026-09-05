// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  createEdgeRenderer,
  EDGE_REGISTERS,
  type EdgeDress,
  type EdgeHues,
  type EdgeRenderer,
} from "./agent-edge-gl";
import type { AgentEdgeRegister } from "./agent-edge-signal";

/**
 * The edge's draw loop, separated from the component that mounts it.
 *
 * Its own module because everything here is a decision about TIME, and none of it
 * needs a GPU to be checked: how often to draw, how the light arrives, when to
 * remeasure, what to tear down. Left inside the effect it was unreachable by any
 * test in this tree, since jsdom answers `getContext("webgl2")` with null and the
 * effect returns before any of it runs.
 *
 * The renderer arrives as a factory so a test can hand over a stand-in. That is
 * the same seam the Core's loop uses, and for the same reason: the GPU is a true
 * boundary, and how often it was drawn to is a question only a stand-in can
 * answer. Nothing here asserts what anything LOOKS like.
 */

/** Thirty a second. The waves are slow and the shader is the most expensive thing
 *  on the surface; at a display's own rate this is the same picture drawn twice. */
export const FRAME_MS = 1000 / 30;

/** How long the light takes to arrive, in seconds. Nothing here appears or cuts:
 *  an edge that snapped on would read as a fault rather than as an agent. */
export const FADE = 0.45;

export type EdgeLoop = Readonly<{
  /** Stops the loop and releases the GPU objects. Safe to call twice. */
  stop: () => void;
  /**
   * Asks for the light, or asks for it back.
   *
   * Separate from `stop` because leaving is a picture and not just a teardown: a
   * caller that unmounted the canvas the moment the work finished would cut the
   * edge off mid-glow, and a light that vanishes reads as a fault where a light
   * that dims reads as finishing. The caller keeps this mounted until `onDark`.
   */
  setLit: (lit: boolean) => void;
  /**
   * Changes which register the light is in.
   *
   * Eased while lit, at the light's own pace: a run that starts mid-import
   * thickens the rim rather than swapping it, and swapping is exactly the cut
   * the fade exists to avoid. Taken at once while dark, because a dark edge has
   * nothing to ease from and the first frame a reader sees should already be in
   * the right register.
   */
  setRegister: (register: AgentEdgeRegister) => void;
}>;

export type EdgeLoopOptions = Readonly<{
  /** Holds the waves still, for a reader who asked for less movement. The light
   *  stays: it is the part that carries the reading. */
  reduced: boolean;
  /** Told when the context is lost, so the caller can wear the static rim. */
  onLost: () => void;
  /** Told once the light has finished going out, so the caller may unmount. */
  onDark?: () => void;
  /** Swapped in tests. The GPU is the one true boundary this file has. */
  makeRenderer?: (
    canvas: HTMLCanvasElement,
    hues: EdgeHues,
  ) => EdgeRenderer | null;
}>;

/** One step toward a target, never past it. Shared by the light and the register
 *  so the two move at the same pace, which is what makes a register change read
 *  as the same light changing rather than as a second one arriving. */
function approach(value: number, target: number, step: number): number {
  return value < target
    ? Math.min(value + step, target)
    : Math.max(value - step, target);
}

/** The dress at a point between the two registers: 0 is the agent's, 1 the import's. */
function dressAt(weight: number): EdgeDress {
  const { agent, capture } = EDGE_REGISTERS;
  // Two products rather than one difference, so both ends are exact: a rim
  // asked for the import's register should draw the import's numbers, not a
  // float a rounding error away from them.
  const mix = (from: number, to: number) => from * (1 - weight) + to * weight;
  return {
    thick: mix(agent.thick, capture.thick),
    beam: mix(agent.beam, capture.beam),
    wave: mix(agent.wave, capture.wave),
  };
}

/**
 * Starts drawing, and returns null when the host cannot.
 *
 * Null is not a failure to handle so much as an answer to report: a locked-down
 * browser, a refused compile and a GPU with no WebGL2 all mean the same thing to
 * a decoration, and the caller has a static rim for exactly this.
 */
export function runEdgeLoop(
  canvas: HTMLCanvasElement,
  hues: EdgeHues,
  options: EdgeLoopOptions,
): EdgeLoop | null {
  const make = options.makeRenderer ?? createEdgeRenderer;
  const started = make(canvas, hues);
  if (!started) {
    return null;
  }
  // Bound after the guard, so the closures below hold a renderer rather than a
  // maybe-renderer: `stop` is hoisted above its own narrowing, and reading the
  // union in there would be a null check on a value that cannot be null.
  const renderer: EdgeRenderer = started;

  let handle = 0;
  let drawnAt = 0;
  let born = 0;
  let lastAt = 0;
  let stopped = false;
  // The light is integrated toward a target rather than read off the loop's age,
  // which is what lets it come back DOWN. Reading it off age can only ever go one
  // way, and the way it could not go is the one a reader notices.
  let level = 0;
  let target = 1;
  // Where between the two registers the light is, and where it is going. The
  // same integration as the light, for the same reason: reading it off the
  // register alone could only ever cut.
  let worn = 0;
  let wanted = 0;
  let wentDark = false;

  const measure = () => {
    renderer.resize(
      window.innerWidth,
      window.innerHeight,
      window.devicePixelRatio || 1,
    );
  };

  /**
   * Device pixel ratio changes without the page changing size.
   *
   * Drag a window between a laptop panel and an external monitor and the CSS
   * viewport is identical either side of the move, so no box changed and a
   * ResizeObserver has nothing to report — but the buffer this loop asked for is
   * now the wrong scale, and the rim renders soft or gritty until something else
   * happens to resize it.
   *
   * A media query on the CURRENT ratio is the one thing that does fire: it stops
   * matching the moment the ratio moves. It has to be re-armed each time, because
   * the query names the ratio it was built for.
   */
  let ratioWatch: MediaQueryList | null = null;
  const onRatio = () => {
    measure();
    watchRatio();
  };
  function watchRatio() {
    ratioWatch?.removeEventListener("change", onRatio);
    ratioWatch = window.matchMedia(
      `(resolution: ${window.devicePixelRatio || 1}dppx)`,
    );
    ratioWatch.addEventListener("change", onRatio);
  }

  const tick = (now: number) => {
    handle = requestAnimationFrame(tick);
    if (born === 0) {
      born = now;
      lastAt = now;
    }
    // The budget covers the DRAW. The wave's phase is a function of the clock
    // rather than of a step count, so a skipped frame costs a frame and never
    // changes the motion; the light moves by ELAPSED time for the same reason,
    // which is what keeps the fade the same length on any display.
    if (now - drawnAt < FRAME_MS) {
      return;
    }
    const step = (now - lastAt) / 1000 / FADE;
    lastAt = now;
    drawnAt = now;
    level = approach(level, target, step);
    worn = approach(worn, wanted, step);
    const dress = dressAt(worn);
    renderer.draw({
      time: options.reduced ? 0 : (now - born) / 1000,
      level,
      thick: dress.thick,
      wave: dress.wave,
      // Holding the clock still parks the travelling head in a corner rather
      // than stopping it, so the head has to be turned off explicitly.
      beam: options.reduced ? 0 : dress.beam,
    });
    if (target === 0 && level === 0 && !wentDark) {
      // Once. The caller unmounts on this, and a second call would arrive after
      // the canvas it refers to is gone.
      wentDark = true;
      options.onDark?.();
    }
  };

  const onContextLost = (event: Event) => {
    // Prevented, or the browser will not let the context come back for anyone.
    event.preventDefault();
    // Stop as well as report: reporting alone leaves the loop drawing into a
    // context that no longer exists, once per frame, for as long as the page is
    // open.
    stop();
    options.onLost();
  };

  function stop() {
    if (stopped) {
      return;
    }
    stopped = true;
    if (handle !== 0) {
      cancelAnimationFrame(handle);
      handle = 0;
    }
    observer.disconnect();
    window.removeEventListener("resize", measure);
    ratioWatch?.removeEventListener("change", onRatio);
    canvas.removeEventListener("webglcontextlost", onContextLost);
    renderer.dispose();
  }

  // The root rather than the canvas: the canvas is sized FROM the window, so
  // watching it would be watching this loop's own output.
  const observer = new ResizeObserver(measure);
  observer.observe(document.documentElement);
  // Three ways the buffer can go stale, and they catch different things: the
  // observer sees layout change the root's box, `resize` sees the window itself
  // change, and the ratio watch above sees a move to a display with a different
  // pixel density, which changes neither box.
  window.addEventListener("resize", measure);
  watchRatio();
  canvas.addEventListener("webglcontextlost", onContextLost);
  measure();
  handle = requestAnimationFrame(tick);

  return {
    stop,
    setLit(lit) {
      target = lit ? 1 : 0;
      // Asking for the light back after it has gone out is a fresh arrival, so
      // the report is armed again rather than being spent for the loop's life.
      if (lit) {
        wentDark = false;
      }
    },
    setRegister(register) {
      wanted = register === "capture" ? 1 : 0;
      if (level === 0) {
        worn = wanted;
      }
    },
  };
}
