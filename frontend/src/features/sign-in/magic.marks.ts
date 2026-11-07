// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The mark beside each receipt line, coloured by who acted, and the clock the
// line's time is written in.

import { formatDayMonth, formatTimeOfDay } from "../../format/format";
import type { Locale } from "../../i18n";
import type { MagicLine } from "../../lib/magic.queries";
import "./magic.marks.css";

// Agent work wears indigo because a model decided it; sync and rules are grey
// because nothing was inferred. Waiting is the staged ring, failure the one
// state colour.
export type MarkKind = "agent" | "sync" | "waiting" | "failed";

const HOUR_MS = 60 * 60 * 1000;
const DAY_HOURS = 24;

// A watching line is dated when the condition was OBSERVED, so it takes no
// mark of its own; its own section says since when.
export function markKind(line: MagicLine): MarkKind | null {
  switch (line.lane) {
    case "needs_you":
      return "waiting";
    case "could_not_complete":
      return "failed";
    case "watching":
      return null;
    case "done":
      return line.actor.type === "agent" ? "agent" : "sync";
  }
}

// Past a day "03:00" alone would not say which day, so a line's own time
// names the day instead.
export function spansDays(since: string, asOf: string): boolean {
  return Date.parse(asOf) - Date.parse(since) > DAY_HOURS * HOUR_MS;
}

export function axisLabel(
  iso: string,
  day: boolean,
  locale: Locale,
  zone: string,
): string {
  return day
    ? formatDayMonth(iso, locale, zone)
    : formatTimeOfDay(iso, locale, zone);
}
