// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { nearestRank } from "./quantile";

/** One Element Timing entry of Home's day content: when, and how much area. */
export type DayPaint = { at: number; area: number };
/** One layout shift; `input` marks a shift right after user input. */
export type Shift = { at: number; value: number; input: boolean };
/**
 * What one document load painted of the day and shifted, as recorded. `roots`
 * counts its day roots when read; any count but one fails the load.
 */
export type Vitals = {
  dayPaints: DayPaint[];
  shifts: Shift[];
  roots: number;
  lcpMs: number | null;
  settled: boolean;
};

/** Home's fixed-profile browser budgets are "at most". */
export const VITALS_BUDGET = { dayPaintMs: 1_000, lcpMs: 1_000, cls: 0.1 };

/**
 * When the day content's largest block painted; the earliest of equal ones.
 * Null when nothing of it painted. Paints outside the day content never count,
 * however large, so the shell and the agent rail cannot move this number.
 */
export function dayPaint(paints: readonly DayPaint[]): DayPaint | null {
  let largest: DayPaint | null = null;
  for (const paint of paints) {
    if (largest === null || paint.area > largest.area) largest = paint;
  }
  return largest;
}

// A session window closes after a 1 s gap or at 5 s long, as Chrome's CLS does.
const SHIFT_GAP_MS = 1_000;
const SHIFT_WINDOW_MS = 5_000;

/** CLS as the largest session window of shifts no input caused. */
export function sessionWindowCls(shifts: readonly Shift[]): number {
  let worst = 0;
  let current = 0;
  let first = Number.NEGATIVE_INFINITY;
  let last = Number.NEGATIVE_INFINITY;
  for (const shift of shifts) {
    if (shift.input) continue;
    if (shift.at - last < SHIFT_GAP_MS && shift.at - first < SHIFT_WINDOW_MS) {
      current += shift.value;
    } else {
      current = shift.value;
      first = shift.at;
    }
    last = shift.at;
    worst = Math.max(worst, current);
  }
  return worst;
}

/** The quantile `q` of native LCP, day paint and CLS over a run of loads. */
export function vitalsQuantile(
  loads: readonly Vitals[],
  q: number,
): { dayPaintMs: number; lcpMs: number; cls: number } {
  const paints = loads.map(
    (load) => dayPaint(load.dayPaints)?.at ?? Number.POSITIVE_INFINITY,
  );
  const shifts = loads.map((load) => sessionWindowCls(load.shifts));
  const lcps = loads.map((load) =>
    validLcp(load.lcpMs) ? load.lcpMs : Number.POSITIVE_INFINITY,
  );
  return {
    dayPaintMs: nearestRank(paints, q),
    lcpMs: nearestRank(lcps, q),
    cls: nearestRank(shifts, q),
  };
}

/** The p95 of native LCP, day paint and CLS over a run of loads. */
export function vitalsP95(loads: readonly Vitals[]): {
  dayPaintMs: number;
  lcpMs: number;
  cls: number;
} {
  return vitalsQuantile(loads, 0.95);
}

/**
 * How the vitals check judges speed. "absolute" holds it to `VITALS_BUDGET`.
 * "relative" judges only whether every load was measured, and leaves speed to
 * the comparison with the merge base in scripts/home-budget-compare.sh.
 */
export type VitalsMode = "absolute" | "relative";

/**
 * Why a run of loads breaks the vitals budget; empty when it keeps it. A load
 * whose day content never painted fails by itself, whatever the p95.
 */
export function vitalsReasons(
  loads: readonly Vitals[],
  mode: VitalsMode = "absolute",
): string[] {
  if (loads.length === 0) return ["no load was measured"];
  const reasons: string[] = [];
  for (const [i, load] of loads.entries()) {
    reasons.push(...loadReasons(load, i + 1));
  }
  if (mode === "relative") return reasons;
  const p95 = vitalsP95(loads);
  if (
    Number.isFinite(p95.dayPaintMs) &&
    p95.dayPaintMs > VITALS_BUDGET.dayPaintMs
  )
    reasons.push(
      `day paint p95 ${Math.round(p95.dayPaintMs)} ms is over ${VITALS_BUDGET.dayPaintMs} ms`,
    );
  if (Number.isFinite(p95.lcpMs) && p95.lcpMs > VITALS_BUDGET.lcpMs)
    reasons.push(
      `LCP p95 ${Math.round(p95.lcpMs)} ms is over ${VITALS_BUDGET.lcpMs} ms`,
    );
  if (p95.cls > VITALS_BUDGET.cls)
    reasons.push(`CLS p95 ${p95.cls.toFixed(3)} is over ${VITALS_BUDGET.cls}`);
  return reasons;
}

function loadReasons(load: Vitals, number: number): string[] {
  const reasons: string[] = [];
  if (!load.settled) reasons.push(`load ${number}: Home did not settle`);
  if (!validLcp(load.lcpMs))
    reasons.push(`load ${number}: LCP was not measured`);
  if (load.roots !== 1)
    reasons.push(`load ${number}: Home has ${load.roots} day roots, not one`);
  if (dayPaint(load.dayPaints) === null)
    reasons.push(`load ${number}: Home's day content never painted`);
  return reasons;
}

function validLcp(value: number | null): value is number {
  return value !== null && Number.isFinite(value) && value > 0;
}
