// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** How far back the line looks: a month, a quarter, a year, or back to the
 *  earliest thing the record holds. */
export type TimelineZoom = "month" | "quarter" | "year" | "all";

export const TIMELINE_ZOOMS: readonly TimelineZoom[] = [
  "month",
  "quarter",
  "year",
  "all",
];

const ZOOM_MONTHS: Readonly<Record<Exclude<TimelineZoom, "all">, number>> = {
  month: 1,
  quarter: 3,
  year: 12,
};

/** The instant a zoom starts the axis at, read back from `today`; `all`
 *  starts at the earliest of `moments`. */
export function zoomStart(
  zoom: TimelineZoom,
  today: number,
  moments: readonly number[],
): number {
  if (zoom === "all") {
    return Math.min(today, ...moments);
  }
  const back = new Date(today);
  back.setUTCMonth(back.getUTCMonth() - ZOOM_MONTHS[zoom]);
  return back.getTime();
}

// The zooms a default may open at, narrowest first.
const WIDENING: readonly TimelineZoom[] = ["quarter", "year"];

/**
 * The zoom a line opens at: the narrowest from a quarter up that holds every
 * event the record names. The default never cuts a named event, and never
 * shows less than three months.
 */
export function defaultZoom(
  today: number,
  events: readonly number[],
): TimelineZoom {
  const earliest = Math.min(today, ...events);
  const fits = WIDENING.find(
    (zoom) => zoomStart(zoom, today, events) <= earliest,
  );
  return fits ?? "all";
}
