import { useEffect, useState } from "react";
import type { Locale, Translator } from "../i18n";
import { formatNumber } from "./format";

// AC-7 groundwork (feeds Task 10's live approvals-inbox countdown). useNow
// is the ONLY place a real clock touches this codebase's rendering — every
// consumer (formatCountdown included) stays pure and takes epoch ms as
// input, so tests never race a real setInterval (craft T11:
// vi.useFakeTimers() + vi.advanceTimersByTime() drive both sides).

// Re-renders the calling component every `intervalMs`, exposing the current
// epoch ms. The interval is cleared on unmount or when intervalMs changes.
export function useNow(intervalMs = 1000): number {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    // A non-positive interval disables the clock: the caller doesn't render
    // anything time-dependent (e.g. a read-only row), so there is no reason to
    // re-render every tick. `now` stays pinned at its mount value.
    if (intervalMs <= 0) {
      return;
    }
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);

  return now;
}

const SECONDS_PER_MINUTE = 60;
const MINUTES_PER_HOUR = 60;
const HOURS_PER_DAY = 24;

// Pure: given a millisecond span and the caller's `t` (e.g. useT()'s bound
// translator, called as t(key, params)), renders the two largest units the
// span reaches, or the localized "expired" sentinel once it has run out.
//
// Two units, and the span picks which two: a deadline three days out reads
// "3d 4h", not the 4316 minutes that carrying minutes as the top unit
// produces. The pair always answers "how long have I got" at the precision
// that span deserves — seconds matter in the last minutes of an approval
// window and are noise a day earlier.
//
// Every unit is a MAGNITUDE, so each reaches the sentence through the reader's
// own grouping. Only the day count can pass four digits, and it is the one a
// long-lived window actually shows.
export function formatCountdown(
  msRemaining: number,
  t: Translator,
  locale: Locale,
): string {
  if (msRemaining <= 0) {
    return t("countdown.expired");
  }
  const totalSeconds = Math.floor(msRemaining / 1000);
  const totalMinutes = Math.floor(totalSeconds / SECONDS_PER_MINUTE);
  const totalHours = Math.floor(totalMinutes / MINUTES_PER_HOUR);
  const days = Math.floor(totalHours / HOURS_PER_DAY);

  if (days >= 1) {
    return t("countdown.daysHours", {
      days: formatNumber(days, locale),
      hours: formatNumber(totalHours % HOURS_PER_DAY, locale),
    });
  }
  if (totalHours >= 1) {
    return t("countdown.hoursMinutes", {
      hours: formatNumber(totalHours, locale),
      minutes: formatNumber(totalMinutes % MINUTES_PER_HOUR, locale),
    });
  }
  return t("countdown.minutesSeconds", {
    minutes: formatNumber(totalMinutes, locale),
    seconds: formatNumber(totalSeconds % SECONDS_PER_MINUTE, locale),
  });
}

// Pure, and the other direction from formatCountdown: how long ago something
// happened, at the coarsest unit that still says something.
//
// One unit rather than two. A countdown is read to decide whether there is time
// left, so the second unit earns its width; "how long since" is read to decide
// whether a thing is still current, and "2 h 14 min ago" answers that no better
// than "2 h ago" while costing a line.
//
// A span at or below zero is "just now" rather than a negative age: clocks
// disagree, and a row stamped a few seconds into a reader's future is a skew
// between two machines rather than an event that has not happened.
export function formatElapsed(
  msSince: number,
  t: Translator,
  locale: Locale,
): string {
  const totalMinutes = Math.floor(msSince / 60_000);
  if (totalMinutes < 1) {
    return t("elapsed.justNow");
  }
  const totalHours = Math.floor(totalMinutes / MINUTES_PER_HOUR);
  if (totalHours < 1) {
    return t("elapsed.minutes", {
      minutes: formatNumber(totalMinutes, locale),
    });
  }
  const days = Math.floor(totalHours / HOURS_PER_DAY);
  if (days < 1) {
    return t("elapsed.hours", { hours: formatNumber(totalHours, locale) });
  }
  return t("elapsed.days", { days: formatNumber(days, locale) });
}
