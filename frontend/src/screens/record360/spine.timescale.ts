// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { calendarDaysBetween } from "../../format/format";

// What the time scale reads off a spine stop: its kind and its instant.
type TimedStop = Readonly<{ tone: string; at?: string }>;

// The narrowest a stop may be drawn. A date stays on one line, and narrower
// stops run their dates together. At the floor the thread scrolls instead.
const STOP_FLOOR = "6.5rem";

// How much of the axis a stop takes: the days until the next dated stop.
// So the thread runs on one time scale, and the rule right of a dot is the
// time that stop covers. A conversation a day before the next is narrow, and
// a fortnight's silence is wide, because the width is the waiting. Today's
// marker takes only its label's width, because it is a line and not a span.
export function stopColumn(
  stop: TimedStop,
  index: number,
  stops: readonly TimedStop[],
): string {
  if (stop.tone === "now") {
    // Its label's width. `auto` would read the stop's minimum, which every
    // stop declares as zero, so a scrolling track would collapse today.
    return "max-content";
  }
  return `minmax(${STOP_FLOOR}, ${daysCovered(stop, stops.slice(index + 1))}fr)`;
}

// The calendar days from a stop to the next one that carries a date, at
// least one. A stop with no date, or none after it, takes a day's share and
// stands on its floor.
function daysCovered(stop: TimedStop, after: readonly TimedStop[]): number {
  const next = after.find((one) => one.at !== undefined);
  if (!stop.at || !next?.at) {
    return 1;
  }
  const days = calendarDaysBetween(new Date(stop.at), new Date(next.at));
  return Math.max(days, 1);
}
