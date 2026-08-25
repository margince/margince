// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MarginceCoreState } from "./margince-core";

/**
 * What each Core state DOES, as the six numbers the shader is driven by.
 *
 * This table is the whole of WDS-CORE-3: state is motion first, colour second.
 * The five states share one object and one palette, so what tells them apart
 * has to be how the material behaves. Six dials do that:
 *
 *  - `level`   how much energy is in the ribbons: width, exposure, core heat.
 *  - `speed`   how fast the phase advances. Integrated host-side, never
 *              multiplied into elapsed time, so a change bends the motion
 *              instead of teleporting it.
 *  - `pulse`   how far the ball breathes in and out.
 *  - `ingest`  the whirlpool: ribbons migrating from the glass to the centre,
 *              dissolving as they arrive. This is the one state that reads as
 *              material going IN rather than the object being busy.
 *  - `tint`    how far the indigo palette collapses to `tintCol`. Only the two
 *              states that STOP leave the green, which is what keeps the three
 *              live ones readable as one run.
 *  - `tintCol` the colour it collapses to, looked at only when `tint` is above
 *              zero. The states that never tint still declare one, so the eased
 *              colour has somewhere to rest.
 *
 * Every value is a TARGET, never a cut: the engine eases toward whichever row
 * is current, so a state change reads as the material changing rather than as a
 * switch being thrown.
 */
export type CoreBehaviour = Readonly<{
  level: number;
  speed: number;
  pulse: number;
  ingest: number;
  tint: number;
  tintCol: readonly [number, number, number];
}>;

/** Amber for a thing that wants a look, red for a thing that broke. */
// Emissive, so it runs hotter than the token it mirrors (--orbAmber). Walked
// down from gold: at hue 40 the warning state read as a colour somebody chose
// for looks, and the point of it is that a person is being asked for something.
// Chosen from where it LANDS, not from where it looks amber in a swatch.
//
// This triple is multiplied by a ribbon's GAIN (up to 3.3) and then run through
// the exposure tonemap, which clips red long before green and so walks the hue
// upward on the way to the screen. A swatch-plausible amber at [0.98, 0.55,
// 0.13] arrives at hue 47deg, which is gold, and two rounds of nudging the
// swatch moved the rendered hue by five degrees in total.
//
// Solved backwards instead, and pinned by TWO constraints rather than one: it
// has to read amber, and it has to stay clear of RED below. A first pass at
// green 0.19 rendered 25deg on the brightest ribbon and 18deg on the faintest,
// and red renders 17deg: on half the ribbons the warning state and the failed
// state were the same colour. Green 0.38 lands 42deg and 34deg against red's
// 17deg and 12deg, which holds the gap at every gain in the set while staying
// under the 47deg that read as gold.
//
// Nothing in the tree checks this, and a swatch comparison cannot: these two
// only collide AFTER the gain and the tonemap. Move either constant and the
// arithmetic above is what has to be redone.
const AMBER: readonly [number, number, number] = [0.98, 0.38, 0.03];
const RED: readonly [number, number, number] = [1.0, 0.24, 0.14];

export const BEHAVIOUR: Readonly<Record<MarginceCoreState, CoreBehaviour>> = {
  /* At rest, and it has to survive being looked at all day on every screen:
     slow, dim, breathing. An idle state with energy in it reads as work nobody
     asked for. A finished run settles back here rather than into a state of
     its own, so `idle` is both where the agent starts and where it lands. */
  idle: {
    level: 0.08,
    speed: 0.3,
    pulse: 0.3,
    ingest: 0,
    tint: 0,
    tintCol: RED,
  },
  /* Taking something in. The only state where the ribbons travel inward. */
  ingest: {
    level: 0.55,
    speed: 1.15,
    pulse: 0.22,
    ingest: 1,
    tint: 0,
    tintCol: RED,
  },
  /* Working on it: fast, hot, barely breathing. Thought looks like speed. */
  working: {
    level: 0.85,
    speed: 1.9,
    pulse: 0.12,
    ingest: 0,
    tint: 0,
    tintCol: RED,
  },
  /* Stopped and waiting for a person: a contradiction, an unreachable source, a
     licence it lacks. Amber, slow, and breathing hard enough to be caught out
     of the corner of an eye. */
  warning: {
    level: 0.45,
    speed: 0.5,
    pulse: 0.6,
    ingest: 0,
    tint: 1,
    tintCol: AMBER,
  },
  /* Broke. Red, and the hardest breath of the five. */
  error: {
    level: 0.52,
    speed: 0.45,
    pulse: 0.9,
    ingest: 0,
    tint: 1,
    tintCol: RED,
  },
};

/**
 * Where the phase starts.
 *
 * Not zero: at phase zero every ribbon sits at its undisturbed base angle and
 * the four of them are close to symmetric, so the first frame of a Core that
 * has just mounted is the one frame that looks designed rather than alive.
 */
export const PHASE_SEED = 11.3;

/**
 * How fast each dial closes the distance to its target, per frame at 60Hz.
 *
 * Split, because the numbers do not want the same easing. Colour moves slowest,
 * since a hue sliding is the most visible kind of change and the one most worth
 * spending time on; `level` follows a little quicker so a state change is felt
 * promptly.
 */
export const EASE = {
  level: 0.05,
  speed: 0.04,
  pulse: 0.04,
  ingest: 0.045,
  tint: 0.05,
  tintCol: 0.06,
} as const;

/**
 * The least time between two drawn frames, in milliseconds.
 *
 * Thirty a second. The Core's fastest motion is a ribbon drifting across its own
 * shell over seconds, so the frames between these are frames nobody can tell
 * apart, and the loop runs for as long as the app is open on every screen. The
 * phase is integrated from real elapsed time, so drawing half as often slows
 * nothing down: it draws the same motion with half the work.
 */
export const FRAME_MS = 1000 / 30;

/** The largest step the phase may take, in seconds of motion per frame. */
export const MAX_STEP = 0.05;

/**
 * The intake floor a surface reporting arriving context puts under any state.
 *
 * Below `ingest`, deliberately: the state means the agent is taking evidence
 * in as its whole activity, and a dock being fed while the agent rests is a
 * quieter fact than that.
 */
export const FEED_INGEST = 0.45;

/**
 * How much more alive the Core is when it is the SUBJECT of a page.
 *
 * The same state, at a different distance from the reader. In the chrome the
 * Core is a status light somebody sits beside all day, and its resting dress is
 * quiet on purpose: energy there reads as work nobody asked for. On the sign-in
 * screen and in onboarding it is the thing the page is about, there is no work
 * for it to be quiet about, and the quiet dress reads as a downgrade of the
 * object rather than as calm.
 *
 * A multiplier rather than a second table: the state has to remain the SAME
 * state, or a reader who learns the object in one place has to learn it again in
 * the other.
 */
const HERO = {
  level: 2.4,
  speed: 1.5,
  pulse: 1.35,
} as const;

/** The hero dress of a row, with the state itself unchanged. */
export function asHero(row: CoreBehaviour): CoreBehaviour {
  return {
    ...row,
    level: Math.min(1, row.level * HERO.level),
    speed: row.speed * HERO.speed,
    pulse: Math.min(1, row.pulse * HERO.pulse),
  };
}

/**
 * The reduced-motion still, per state.
 *
 * Somebody who has asked for less motion still needs the Core to say which of
 * the five things it is. So the answer is not a frozen frame of the animation,
 * it is the same object with the phase held: full colour, full tint, the level
 * the state carries, and nothing advancing. `pulse` and `ingest` go to zero
 * because both are motion by definition.
 */
export function still(state: MarginceCoreState): CoreBehaviour {
  return { ...rowFor(state), speed: 0, pulse: 0, ingest: 0 };
}

/**
 * The row for a state, and never undefined.
 *
 * The type says a caller cannot ask for a state that is not in the table, and at
 * runtime that is one module version's word against another's: a hot reload mid
 * edit, or a bundle split where a screen shipped a state this table has not heard
 * of. The Core is decoration, so the honest failure is to keep drawing at rest
 * and say so in the console, not to throw and take the whole shell down with a
 * component nobody was looking at.
 */
export function rowFor(state: MarginceCoreState): CoreBehaviour {
  const row = BEHAVIOUR[state];
  if (row) {
    return row;
  }
  console.error("Margince Core asked for a state it has no behaviour for", {
    state,
  });
  return BEHAVIOUR.idle;
}
