// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A risk signal as one sentence with its numbers. The numbers come from the
// server's assessment; this file only words them.

import type { components } from "../../api/schema";
import type { PluralTranslator, Translator } from "../../i18n";

export type RiskSignal = components["schemas"]["ForecastRiskSignal"];

/** The sentence a badge shows for one signal. */
export function signalSentence(
  signal: RiskSignal,
  t: Translator,
  plural: PluralTranslator,
  stageName: (stageId: string) => string | undefined,
  number: (value: number) => string,
): string {
  const count = (value: number) => ({ count: number(value) });
  switch (signal.code) {
    case "stuck": {
      const stage = signal.stage_id ? stageName(signal.stage_id) : undefined;
      if (signal.days === undefined) return t("forecast.signal.stuck");
      return stage
        ? plural("forecast.signal.stuckIn", signal.days, {
            ...count(signal.days),
            stage,
          })
        : plural("forecast.signal.stuckDays", signal.days, count(signal.days));
    }
    case "close_pushed":
      if (signal.close_passed) return t("forecast.signal.closePassed");
      return signal.count === undefined
        ? t("forecast.signal.close_pushed")
        : plural(
            "forecast.signal.closeMoved",
            signal.count,
            count(signal.count),
          );
    case "quiet":
      return signal.days === undefined
        ? t("forecast.signal.quiet")
        : plural("forecast.signal.noReply", signal.days, count(signal.days));
    case "no_next_step":
      return closesSentence(signal.closes_in_days, t, plural, count);
  }
}

function closesSentence(
  days: number | undefined,
  t: Translator,
  plural: PluralTranslator,
  count: (value: number) => Record<string, string>,
): string {
  if (days === undefined) return t("forecast.signal.no_next_step");
  if (days === 0) return t("forecast.signal.closesToday");
  if (days === 1) return t("forecast.signal.closesTomorrow");
  return plural("forecast.signal.closesNoStep", days, count(days));
}
