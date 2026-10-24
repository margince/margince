// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { formatNumber, INTL_LOCALE } from "../../format/format";
import type { Locale, PluralTranslator, Translator } from "../../i18n";
import { dayLabelKey, WEEK } from "./working-hours";

// The sentences the Meetings cards say about time: a span in words, and the
// hours a booking link offers.

type WorkingHours = components["schemas"]["WorkingHours"];

/** A span in the largest whole unit that says it without rounding: 120 is 2 hours. */
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

// A run of three or more days reads as a range, Monday to Friday. Anything
// else is named day by day in the reader's own list grammar.
function daysPhrase(
  t: Translator,
  locale: Locale,
  days: readonly number[],
): string {
  const week = WEEK.filter((day) => days.includes(day));
  const names = week.map((day) => t(dayLabelKey(day)));
  const run = week.every(
    (day, index) => index === 0 || day === (week[index - 1] ?? 0) + 1,
  );
  const first = names[0];
  const last = names.at(-1);
  if (week.length >= 3 && run && first && last)
    return t("scheduling.dayRange", { from: first, until: last });
  return new Intl.ListFormat(INTL_LOCALE[locale], {
    type: "conjunction",
  }).format(names);
}
