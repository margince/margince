// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { INTL_LOCALE } from "./intllocale";
import type { TimeFormat } from "./preferences";

/** A wall-clock time as the contract and a `time` field spell it: `HH:MM`. */
export type ClockTime = `${number}:${number}`;

// The end of a range that runs until midnight, which no `HH:MM` of the day spells.
const MIDNIGHT_END = "24:00";

/**
 * Narrows a shape and checks the hour and minute exist. `untilMidnight` also
 * admits `24:00`, the end of a range that runs to the end of the day.
 */
export function isClockTime(
  raw: string,
  untilMidnight = false,
): raw is ClockTime {
  if (untilMidnight && raw === MIDNIGHT_END) {
    return true;
  }
  const match = /^(\d{2}):(\d{2})$/.exec(raw);
  return match !== null && Number(match[1]) < 24 && Number(match[2]) < 60;
}

/** A stored or wire value as the time field's `value`: the time, or `""`. */
export function clockOrBlank(
  raw: string | null | undefined,
  untilMidnight = false,
): ClockTime | "" {
  const minutes = raw?.slice(0, 5) ?? "";
  return isClockTime(minutes, untilMidnight) ? minutes : "";
}

/** How a reader writes a time: on which clock, and the words for its halves. */
export type TimePattern = Readonly<{
  twelveHours: boolean;
  separator: string;
  // The language's own words for the two halves of a 12-hour day.
  morning: string;
  evening: string;
}>;

/** The installation's time format, or the UI language's own when it names none. */
export function timePattern(locale: Locale, format: TimeFormat): TimePattern {
  const twelveHours =
    format === "12h" ||
    (format === "locale" && localeCycle(locale).startsWith("h1"));
  const parts = clockFormat(locale, true).formatToParts(
    new Date(2000, 0, 1, 9),
  );
  const evening = clockFormat(locale, true)
    .formatToParts(new Date(2000, 0, 1, 21))
    .find((part) => part.type === "dayPeriod")?.value;
  return {
    twelveHours,
    separator: parts.find((part) => part.type === "literal")?.value ?? ":",
    morning: parts.find((part) => part.type === "dayPeriod")?.value ?? "AM",
    evening: evening ?? "PM",
  };
}

function localeCycle(locale: Locale): string {
  return (
    new Intl.DateTimeFormat(INTL_LOCALE[locale], {
      hour: "numeric",
    }).resolvedOptions().hourCycle ?? "h23"
  );
}

function clockFormat(locale: Locale, twelveHours: boolean) {
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: twelveHours ? "h12" : "h23",
  });
}

export function formatClockTime(
  time: ClockTime,
  locale: Locale,
  pattern: TimePattern,
): string {
  if (time === MIDNIGHT_END && !pattern.twelveHours) {
    return `24${pattern.separator}00`;
  }
  const [hour, minute] = time.split(":").map(Number);
  return clockFormat(locale, pattern.twelveHours).format(
    new Date(2000, 0, 1, hour, minute),
  );
}

/** The pattern spelled in the reader's letters, such as `hh:mm am`. */
export function timePlaceholder(
  pattern: TimePattern,
  letters: Readonly<{ hour: string; minute: string }>,
): string {
  const clock = `${letters.hour}${pattern.separator}${letters.minute}`;
  return pattern.twelveHours ? `${clock} ${pattern.morning}` : clock;
}

const fold = (text: string) => text.toLowerCase().replace(/[.\s]/g, "");

/**
 * A typed time as `HH:MM`, or null. Takes `9`, `930`, `9:30`, `9.30` and `09h30`,
 * and on either clock a trailing half of the day in English or in the reader's
 * language, so a time pasted from a 12-hour page keeps its meaning.
 */
export function parseTypedTime(
  text: string,
  pattern: TimePattern,
  untilMidnight = false,
): ClockTime | null {
  const folded = fold(text);
  const halves = [
    ["am", false],
    ["pm", true],
    [fold(pattern.morning), false],
    [fold(pattern.evening), true],
  ] as const;
  const half = halves.find(([word]) => word !== "" && folded.endsWith(word));
  const digits = half ? folded.slice(0, -half[0].length) : folded;
  const match = /^(\d{1,2})(?:[:h]?(\d{2}))?$/.exec(digits);
  if (!match) {
    return null;
  }
  let hour = Number(match[1]);
  const minute = Number(match[2] ?? "0");
  if (half) {
    if (hour < 1 || hour > 12) {
      return null;
    }
    hour = (hour % 12) + (half[1] ? 12 : 0);
  }
  const time = `${String(hour).padStart(2, "0")}:${String(minute).padStart(2, "0")}`;
  return isClockTime(time, untilMidnight) ? time : null;
}
