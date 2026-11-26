// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readHue } from "./margince-core-color";
import { type CoreBehaviour, EASE } from "./margince-core-motion";

/** The eased state of the object, as opposed to where it is heading. */
export type Dials = {
  level: number;
  form: number;
  speed: number;
  pulse: number;
  ingest: number;
  tint: number;
  tintCol: [number, number, number];
};

/** Close enough that another frame would not change a pixel. */
export const SETTLED = 1e-4;

const CHANNELS = [0, 1, 2] as const;

// In the order the shader's uWork and uBody arrays expect: ribbon stops A to D,
// amber the one warm note, then the highlight tint.
const WORK_TOKENS = [
  "--ai",
  "--orbGlow",
  "--orbMid",
  "--orbAmber",
  "--orbBright",
];
const BODY_TOKENS = ["--orbDeep", "--orbInk"];

export type Palette = Readonly<{
  work: readonly (readonly [number, number, number])[];
  body: readonly (readonly [number, number, number])[];
}>;

/**
 * Reads the orb's palette off the document.
 *
 * `getComputedStyle` is a layout read, so this runs on loop start and on a
 * theme flip, never inside the per-frame draw call.
 */
export function readPalette(): Palette {
  return {
    work: WORK_TOKENS.map(readHue),
    body: BODY_TOKENS.map(readHue),
  };
}

export function dialsFrom(behaviour: CoreBehaviour): Dials {
  return {
    level: behaviour.level,
    form: behaviour.form,
    speed: behaviour.speed,
    pulse: behaviour.pulse,
    ingest: behaviour.ingest,
    tint: behaviour.tint,
    tintCol: [...behaviour.tintCol],
  };
}

/** Moves one dial a fraction of the way to its target. */
export function ease(from: number, to: number, rate: number): number {
  return from + (to - from) * rate;
}

/** Eases every dial one frame toward `target`, and says if any still moves. */
export function step(dials: Dials, target: CoreBehaviour): boolean {
  dials.level = ease(dials.level, target.level, EASE.level);
  dials.form = ease(dials.form, target.form, EASE.form);
  dials.speed = ease(dials.speed, target.speed, EASE.speed);
  dials.pulse = ease(dials.pulse, target.pulse, EASE.pulse);
  dials.ingest = ease(dials.ingest, target.ingest, EASE.ingest);
  dials.tint = ease(dials.tint, target.tint, EASE.tint);
  for (const i of CHANNELS) {
    dials.tintCol[i] = ease(dials.tintCol[i], target.tintCol[i], EASE.tintCol);
  }
  return (
    Math.abs(dials.level - target.level) > SETTLED ||
    Math.abs(dials.form - target.form) > SETTLED ||
    Math.abs(dials.speed - target.speed) > SETTLED ||
    Math.abs(dials.pulse - target.pulse) > SETTLED ||
    Math.abs(dials.ingest - target.ingest) > SETTLED ||
    Math.abs(dials.tint - target.tint) > SETTLED ||
    CHANNELS.some(
      (i) => Math.abs(dials.tintCol[i] - target.tintCol[i]) > SETTLED,
    )
  );
}
