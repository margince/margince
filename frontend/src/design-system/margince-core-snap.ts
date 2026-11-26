// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * Keeps the Core's canvas on whole device pixels.
 *
 * A canvas between pixels is resampled by the compositor on every frame, which
 * blurs the whole ball. These are the pure nudge and the events that move it.
 */

/** The canvas's nudge onto the device-pixel grid, in its own CSS px. */
export type PixelShift = readonly [number, number];

/**
 * The nudge that lands a canvas's corner on whole device pixels.
 *
 * `box` already carries the previous nudge; an ancestor's scale divides it.
 */
export function pixelSnap(
  box: Readonly<{ left: number; top: number; width: number }>,
  layoutWidth: number,
  dpr: number,
  previous: PixelShift,
): PixelShift {
  const scale = layoutWidth > 0 ? box.width / layoutWidth : 1;
  const x = (box.left - previous[0] * scale) * dpr;
  const y = (box.top - previous[1] * scale) * dpr;
  return [(Math.round(x) - x) / dpr / scale, (Math.round(y) - y) / dpr / scale];
}

/**
 * Calls `moved` when the canvas may have moved without resizing, and
 * `rescaled` when the device pixel ratio changes. Returns the release.
 */
export function watchPlacement(
  moved: () => void,
  rescaled: () => void,
): () => void {
  // A resolution query matches one ratio only, so each change arms the next.
  let media: MediaQueryList | null = null;
  const onRatio = () => {
    arm();
    rescaled();
  };
  const arm = () => {
    media?.removeEventListener("change", onRatio);
    media =
      typeof matchMedia === "function"
        ? matchMedia(`(resolution: ${devicePixelRatio || 1}dppx)`)
        : null;
    media?.addEventListener("change", onRatio);
  };
  arm();
  // Any animation can push the canvas, even a sibling's, so every end counts.
  window.addEventListener("resize", moved);
  document.addEventListener("animationend", moved, true);
  document.addEventListener("transitionend", moved, true);
  return () => {
    window.removeEventListener("resize", moved);
    document.removeEventListener("animationend", moved, true);
    document.removeEventListener("transitionend", moved, true);
    media?.removeEventListener("change", onRatio);
    media = null;
  };
}
