// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef } from "react";
import "./ambient-waves.css";
import { createWavesRenderer, type WavesFrame } from "./ambient-waves-gl";
import { readHue } from "./margince-core-gl";
import { usePrefersReducedMotion } from "./motion";

/**
 * The light in the room: a slow field of colour bands behind a full-viewport
 * moment.
 *
 * ONE KIND OF CALLER: a surface that is a PLACE a reader arrives in rather than
 * a screen they work on. Two of them today, and they are the same moment either
 * side of a password: the sign-in surface (`auth.css`) and the setup room
 * (`OnboardingStage`). Neither has anything on it to get through, and a still
 * ground behind an orb that is itself alive reads as a screenshot of one.
 *
 * It is NOT a background for working surfaces. A moving ground behind a record
 * somebody reads all day is something to look past, and the room stops being
 * the point the moment there is work in it.
 *
 * It claims nothing and cannot be interacted with: `aria-hidden`, no pointer
 * events, no text, no edge, no shape. See the shader for why that matters
 * (`ambient-waves-shader.ts`, "WHY IT SAYS NOTHING"): provenance colour is a
 * statement about who decided something, and atmosphere must not be mistaken
 * for one.
 *
 * A host without WebGL2 gets nothing from this component and no error. The
 * caller's own surface keeps a CSS ground underneath, so the screen is complete
 * either way and the canvas is what improves it.
 */

/**
 * The paper the bands sit on, and the three hues, palest first.
 *
 * ONE FAMILY, and the reason is not taste. Three hues from three families
 * (indigo, blue and the house green) is what a first cut used, and overlapping
 * bands of three different families is the recipe for camouflage: every
 * intersection is a fourth colour, and the eye reads patches with borders
 * instead of light. Neighbours on the wheel overlap into more of themselves,
 * which is what light does.
 *
 * The three are the orb's own glow, mid and bright tones, so the ground is the
 * light the Core standing in front of it would throw, not a second palette
 * arriving on the one screen whose subject is the Core.
 */
const PAPER_TOKEN = "--bgElevated";
const BAND_TOKENS = ["--orbGlow", "--orbMid", "--orbBright"];

/** Seconds the ground takes to rise out of the paper on arrival. */
const FADE_S = 1.6;

/**
 * How far the still frame is into the motion.
 *
 * A reduced-motion reader gets ONE frame rather than none: the field at t=0 is
 * the one arrangement of the noise nobody chose, and drawing it would make the
 * still ground the flattest version of itself. This is far enough in that the
 * three bands have separated.
 */
const STILL_T = 42;

/**
 * The largest step the clock is allowed to take, in seconds.
 *
 * A tab that was hidden for ten minutes hands back a ten-minute delta, and the
 * field would jump to somewhere unrelated in one frame. Clamped, it simply
 * carries on from where it was.
 */
const MAX_STEP_S = 1 / 15;

/** The palette part of a frame: everything read off the document's tokens. */
type Palette = Pick<WavesFrame, "paper" | "hues">;

/**
 * Reads the ground's palette off the document.
 *
 * `getComputedStyle` is a layout read, so this runs on loop start and on a
 * theme flip, never inside the per-frame draw call. The same rule
 * `margince-core-engine.ts` keeps for the orb's palette, and for the same
 * reason: at sixty frames a second it is a layout read per frame.
 */
function readPalette(): Palette {
  return {
    paper: readHue(PAPER_TOKEN),
    hues: BAND_TOKENS.map(readHue),
  };
}

/**
 * Runs the ground on one canvas until the returned function is called.
 *
 * Outside the component because it is a plain lifecycle: a renderer, a clock,
 * and three subscriptions that all end together. Inside an effect body it would
 * read as React state management, which none of it is.
 */
function runWaves(canvas: HTMLCanvasElement, still: boolean): () => void {
  const renderer = createWavesRenderer(canvas);
  if (!renderer) {
    return () => {};
  }
  let frame = 0;
  let last = 0;
  let time = still ? STILL_T : 0;
  // A still ground has no arrival to play, so it is drawn already there.
  let fade = still ? 1 : 0;

  let palette = readPalette();

  const paint = () => {
    renderer.resize(canvas.clientWidth, canvas.clientHeight);
    renderer.draw({ time, fade, ...palette });
  };

  const step = (now: number) => {
    // The first frame has no previous timestamp to measure against, and
    // treating `now` itself as the delta hands the clock the whole page load.
    const delta = last === 0 ? 0 : Math.min((now - last) / 1000, MAX_STEP_S);
    last = now;
    time += delta;
    fade = Math.min(1, fade + delta / FADE_S);
    paint();
    frame = requestAnimationFrame(step);
  };

  const start = () => {
    if (frame === 0) {
      // Dropped rather than carried across the gap: the clock resumes on the
      // next frame's timestamp instead of counting the time the tab was away.
      last = 0;
      frame = requestAnimationFrame(step);
    }
  };
  const stop = () => {
    cancelAnimationFrame(frame);
    frame = 0;
  };
  // A hidden tab still gets frames in some browsers and none in others, and
  // paying for a full-screen shader nobody is looking at is wrong in both.
  const visibility = () => {
    if (document.hidden) {
      stop();
    } else if (!still) {
      start();
    }
  };

  // Both of these change what the ground should look like without the loop
  // being involved: a resized viewport needs a new drawing buffer, and a theme
  // flip changes every token the shader is drawn from. While the loop runs it
  // picks both up on its next frame; while it does not, this is the redraw.
  const observer = new ResizeObserver(() => {
    if (still || frame === 0) {
      paint();
    }
  });
  observer.observe(canvas);
  const theme = new MutationObserver(() => {
    palette = readPalette();
    if (still || frame === 0) {
      paint();
    }
  });
  theme.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["data-theme"],
  });
  document.addEventListener("visibilitychange", visibility);

  paint();
  if (!still) {
    start();
  }

  return () => {
    stop();
    observer.disconnect();
    theme.disconnect();
    document.removeEventListener("visibilitychange", visibility);
    renderer.dispose();
  };
}

export function AmbientWaves({
  className = "",
}: Readonly<{ className?: string }>) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const still = usePrefersReducedMotion();

  useEffect(() => {
    const element = canvas.current;
    if (!element) {
      return;
    }
    return runWaves(element, still);
  }, [still]);

  return (
    // `tabIndex` -1 alongside `aria-hidden`: a canvas counts as focusable, and
    // hiding a focusable element from the accessibility tree while leaving it
    // in the tab order strands a keyboard on something a screen reader cannot
    // name. The same pair `crawl-canvas.tsx` uses on its decorative layer.
    <canvas
      className={`ambient-waves ${className}`.trim()}
      ref={canvas}
      aria-hidden="true"
      tabIndex={-1}
    />
  );
}
