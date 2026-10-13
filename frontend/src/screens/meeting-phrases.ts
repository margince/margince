// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { formatNumber } from "../format/format";
import type { Locale, PluralTranslator, Translator } from "../i18n";
import { dayLabelKey, WEEK } from "./working-hours";

// The sentences the Meetings cards say about time: a span in words, and the
// hours a booking link offers.

type WorkingHours = components["schemas"]["WorkingHours"];

/** A span in the largest whole unit that says it exactly: 120 is 2 hours. */
export function amountLabel(
  t: Translator,
  plural: PluralTranslator,
  locale: Locale,
  amount: number,
  unit: "minutes" | "days",
): string {
  const say = (base: Parameters<PluralTranslator>[0], count: number) =>
    plural(base, count, { count: formatNumber(count, locale) });
  if (unit === "days") return say("scheduling.unit.days", amount);
  if (amount === 0) return t("scheduling.noneChosen");
  if (amount % 10080 === 0) return say("scheduling.unit.weeks", amount / 10080);
  if (amount % 1440 === 0) return say("scheduling.unit.days", amount / 1440);
  if (amount % 60 === 0) return say("scheduling.unit.hours", amount / 60);
  return say("scheduling.unit.minutes", amount);
}

/** The hours a booking link offers, as one sentence. */
export function hoursSentence(
  t: Translator,
  plural: PluralTranslator,
  locale: Locale,
  hours: WorkingHours,
  notice: number,
): string {
  const params = {
    days: daysPhrase(t, locale, hours.days),
    from: hours.start_time,
    until: hours.end_time,
    zone:
      hours.timezone.split("/").pop()?.replaceAll("_", " ") ?? hours.timezone,
  };
  return notice === 0
    ? t("scheduling.usesHoursAnyNotice", params)
    : t("scheduling.usesHours", {
        ...params,
        notice: amountLabel(t, plural, locale, notice, "minutes"),
      });
}

// A run of three or more days reads as a range, Monday to Friday; anything
// else is named day by day in the reader's own list grammar.
function daysPhrase(
  t: Translator,
  locale: Locale,
  days: readonly number[],
): string {
  const week = WEEK.filter((day) => days.includes(day));
  const names = week.map((day) => t(dayLabelKey(day)));
  const run = week.every(
    (day, index) => index === 0 || day === week[index - 1] + 1,
  );
  if (week.length >= 3 && run)
    return t("scheduling.dayRange", {
      from: names[0],
      until: names[names.length - 1],
    });
  return new Intl.ListFormat(locale, { type: "conjunction" }).format(names);
}
