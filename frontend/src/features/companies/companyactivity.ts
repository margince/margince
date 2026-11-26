// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { ReadingTone } from "../../design-system/readingtone";
import { activityDay } from "../../format/activitywhen";
import { formatNumber } from "../../format/format";
import { formatDayMonthNearYear } from "../../format/format.dates";
import type { Locale, Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { labelFor, valueFor } from "../../lib/labelfor";
import type { CompanyVerdict } from "./companystanding";

type Company360 = components["schemas"]["Company360"];
type DormancyRung = components["schemas"]["DormancyRung"];

// The rungs, warmest first, in the order a filter offers them.
export const DORMANCY_RUNGS: readonly DormancyRung[] = [
  "active",
  "cooling",
  "dormant",
  "stale",
];

// The word for each rung of the server's dormancy ladder.
export const DORMANCY_LABELS: Record<DormancyRung, MessageKey> = {
  active: "co.verdict.active",
  cooling: "co.verdict.cooling",
  dormant: "co.verdict.dormant",
  stale: "co.verdict.stale",
};

const DORMANCY_TONES: Record<DormancyRung, ReadingTone> = {
  active: "success",
  cooling: "warning",
  dormant: "warning",
  stale: "neutral",
};

/**
 * The account's call when nobody has rated it: the rung of the dormancy ladder
 * the server read from the account's last activity. The sentence names the
 * silent days and the ladder's own days, so a reader can check the call.
 * Undefined when the page carries no reading.
 */
export function companyActivityVerdict(
  view: Company360,
  {
    t,
    locale,
    zone,
  }: Readonly<{ t: Translator; locale: Locale; zone: string }>,
): CompanyVerdict | undefined {
  const reading = view.state_strip?.dormancy;
  if (!reading) {
    return undefined;
  }
  const asOf = new Date(view.as_of);
  const day = (at: string) => formatDayMonthNearYear(at, locale, zone, asOf);
  const word = labelFor(DORMANCY_LABELS, reading.rung);
  const tone = valueFor(DORMANCY_TONES, reading.rung, "neutral");
  if (reading.no_activity || !reading.last_activity_at) {
    return {
      word,
      sentence: t("co.verdict.noActivity", {
        date: day(view.company.created_at),
      }),
      tone,
    };
  }
  const date = activityDay(
    reading.last_activity_at,
    reading.last_activity_precision ?? "instant",
    locale,
    zone,
    (at, locale, zone) => formatDayMonthNearYear(at, locale, zone, asOf),
  );
  if (reading.rung === "active") {
    return { word, sentence: t("co.verdict.lastExchange", { date }), tone };
  }
  const [cooling = 0, dormant = 0, stale = 0] = reading.ladder_days;
  return {
    word,
    sentence: t("co.verdict.silentFor", {
      days: formatNumber(reading.silent_days, locale),
      date,
      cooling: formatNumber(cooling, locale),
      dormant: formatNumber(dormant, locale),
      stale: formatNumber(stale, locale),
    }),
    tone,
  };
}
