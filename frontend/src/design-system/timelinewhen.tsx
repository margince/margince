import { activityDay, activityTimeOfDay } from "../format/activitywhen";
import { type Locale, useLocale } from "../i18n";
import type { TimelineEntry } from "./timelineentry";

/**
 * TimelineWhen is the row's place on the axis: the day, and under it the time
 * of day when one is known. The day alone told a reader two calls happened on
 * the 26th, and not which came first. Nor did it say whether the reply landed
 * an hour after the ask or a working day later. That is the one thing a
 * chronology is opened to settle. Tabular digits keep the column straight
 * whatever each date's digits are.
 */
export function TimelineWhen({
  entry,
  zone,
  repeatsDay = false,
}: Readonly<{
  entry: TimelineMoment;
  zone: string;
  // The row above fell on the same day. The day then stays for a screen
  // reader and leaves the screen, so the gutter reads as one date per day.
  repeatsDay?: boolean;
}>) {
  const { locale } = useLocale();
  const time = entryTimeOfDay(entry, locale, zone);
  const day = entryDay(entry, locale, zone);
  return (
    <span className="tl-when t-num">
      {repeatsDay ? <span className="sr-only">{day}</span> : day}
      {time && <span className="tl-when-time">{time}</span>}
    </span>
  );
}

/**
 * dayRepeats says, for each moment in a newest-first list, whether the one
 * above it fell on the same day.
 */
export function dayRepeats(
  moments: readonly (TimelineMoment | undefined)[],
  locale: Locale,
  zone: string,
): boolean[] {
  const days = moments.map((moment) =>
    moment ? entryDay(moment, locale, zone) : undefined,
  );
  return days.map(
    (day, index) => index > 0 && day !== undefined && day === days[index - 1],
  );
}

type TimelineMoment = Pick<TimelineEntry, "atIso" | "atPrecision">;

/** The date an entry falls on. */
export function entryDay(
  entry: TimelineMoment,
  locale: Locale,
  zone: string,
): string {
  return activityDay(entry.atIso, entry.atPrecision, locale, zone);
}

/** An entry's clock reading, or undefined when only its day is known. */
export function entryTimeOfDay(
  entry: TimelineMoment,
  locale: Locale,
  zone: string,
): string | undefined {
  return (
    activityTimeOfDay(entry.atIso, entry.atPrecision, locale, zone) ?? undefined
  );
}
