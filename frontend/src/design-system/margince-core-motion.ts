// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MarginceCoreState } from "./margince-core";

/**
 * What each Core state does, as the seven numbers the shader is driven by.
 *
 * This table is the whole of WDS-CORE-3: state is motion first, colour second.
 * The five states share one object and one palette, so what tells them apart
 * has to be how the material behaves. Seven dials do that:
 *
 *  - `level`   exposure and core heat.
 *  - `form`    the figure: 0 is the calm idle ribbons, 1 the thinking ones,
 *              wider, more twisted, more folded and more saturated.
 *  - `speed`   how fast the phase advances. Integrated host-side, never
 *              multiplied into elapsed time, so a change bends the motion
 *              instead of teleporting it. Zero lets the loop park.
 *  - `pulse`   how far the ribbons breathe in and out.
 *  - `ingest`  the intake: grain leaving the ribbons and streaming to the
 *              centre, fading as it arrives. This is the one state that reads
 *              as material going IN rather than the object being busy.
 *  - `tint`    how far the indigo palette collapses to `tintCol`. Only the two
 *              states that STOP leave the indigo, which is what keeps the three
 *              live ones readable as one run.
 *  - `tintCol` the colour it collapses to, looked at only when `tint` is above
 *              zero. The states that never tint still declare one, so the eased
 *              colour has somewhere to rest.
 *
 * Every value is a target, never a cut: the engine eases toward the current
 * row, so a state change reads as the material changing. Under reduced motion
 * it cuts, since the ease is motion.
 */
export type CoreBehaviour = Readonly<{
  level: number;
  form: number;
  speed: number;
  pulse: number;
  ingest: number;
  tint: number;
  tintCol: readonly [number, number, number];
}>;

/** Amber for a thing that wants a look, red for a thing that broke. */
// Emissive, so hotter than --orbAmber. The tonemap clips red first and walks
// the hue toward gold, so the pair is solved from where it lands on screen.
// Amber must read amber and stay clear of RED on bright and faint grain alike:
// render both states side by side after moving either.
const AMBER: readonly [number, number, number] = [0.98, 0.38, 0.03];
const RED: readonly [number, number, number] = [1.0, 0.24, 0.14];

export const BEHAVIOUR: Readonly<Record<MarginceCoreState, CoreBehaviour>> = {
  /* At rest, and it has to survive being looked at all day on every screen:
     the calm figure on a slow drift, drawn at the calm frame rate. A finished
     run settles back here rather than into a state of its own. */
  idle: {
    level: 0.3,
    form: 0,
    speed: 0.5,
    pulse: 0.3,
    ingest: 0,
    tint: 0,
    tintCol: RED,
  },
  /* Taking something in. The only state where grain travels inward. */
  ingest: {
    level: 0.6,
    form: 1,
    speed: 1.15,
    pulse: 0.7,
    ingest: 1,
    tint: 0,
    tintCol: RED,
  },
  /* Working on it: the thinking figure, bright, fast and breathing. */
  working: {
    level: 0.85,
    form: 1,
    speed: 1.9,
    pulse: 1,
    ingest: 0,
    tint: 0,
    tintCol: RED,
  },
  /* Stopped and waiting for a contact: a contradiction, an unreachable source, a
     licence it lacks. The thinking figure at rest, in amber: it stopped
     mid-thought, and nothing moves until someone looks. */
  warning: {
    level: 0.45,
    form: 1,
    speed: 0,
    pulse: 0.6,
    ingest: 0,
    tint: 1,
    tintCol: AMBER,
  },
  /* Broke. The thinking figure at rest, in red. */
  error: {
    level: 0.52,
    form: 1,
    speed: 0,
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
  calm: 0.02,
  level: 0.05,
  form: 0.035,
  speed: 0.04,
  pulse: 0.04,
  ingest: 0.045,
  tint: 0.05,
  tintCol: 0.06,
} as const;

/**
 * The least time between two drawn frames, in milliseconds.
 *
 * Thirty a second. The fastest motion is a ribbon drifting over seconds, so the
 * frames between these look the same. The phase is integrated from real time,
 * so drawing half as often slows nothing down.
 */
export const FRAME_MS = 1000 / 30;

/**
 * The draw budget below `CALM_SPEED`: twenty a second. At idle's drift a ribbon
 * moves well under a pixel between two such frames.
 */
export const CALM_FRAME_MS = 1000 / 20;
export const CALM_SPEED = 0.8;

/**
 * Idle keeps its full drift this long after the last sign of a reader, such as
 * a pointer on the orb's row. Then it slows to `CRAWL` and draws at
 * `CRAWL_FRAME_MS`, since an unwatched orb still costs every frame it draws.
 */
export const QUIET_AFTER_MS = 45_000;
export const CRAWL = 0.15;
export const CRAWL_SPEED = 0.2;
export const CRAWL_FRAME_MS = 1000 / 10;

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
