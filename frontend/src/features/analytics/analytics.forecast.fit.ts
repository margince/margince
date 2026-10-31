// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the Forecast tab can answer of the question an address carries. The
// forecast reads the current week, month or quarter over every pipeline, so
// a door from a figure about one pipeline or another period would otherwise
// show a wider population under the same name.

import type { ForecastPeriod } from "./forecast.queries";

export type ForecastFit =
  | Readonly<{ kind: "fits"; period: ForecastPeriod }>
  // The address names a period the forecast does not read.
  | Readonly<{ kind: "period" }>
  // The address names one pipeline of several, and the forecast has no
  // pipeline to narrow to.
  | Readonly<{ kind: "pipeline" }>
  // The pipelines are still being read, so whether one narrows is unknown.
  | Readonly<{ kind: "pending" }>;

const FORECAST_PERIOD_OF: ReadonlyMap<string, ForecastPeriod> = new Map([
  ["this_quarter", "quarter"],
  ["this_month", "month"],
  ["this_week", "week"],
]);

/** The address's spelling of a forecast window, shared with the Overview. */
export function periodDial(period: ForecastPeriod): string {
  return `this_${period}`;
}

/**
 * `pipelines` is undefined until read and null when the read failed. A named
 * pipeline that is the only one narrows nothing; any other, or one that
 * cannot be checked, does.
 */
export function forecastFit(
  period: string | null | undefined,
  pipeline: string | null | undefined,
  pipelines: readonly Readonly<{ id: string }>[] | null | undefined,
): ForecastFit {
  const window = period ? FORECAST_PERIOD_OF.get(period) : "quarter";
  if (!window) return { kind: "period" };
  if (pipeline && pipeline !== "all") {
    if (pipelines === undefined) return { kind: "pending" };
    const onlyOne = pipelines?.length === 1 && pipelines[0]?.id === pipeline;
    if (!onlyOne) return { kind: "pipeline" };
  }
  return { kind: "fits", period: window };
}
