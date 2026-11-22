// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatNumber } from "../format/format";
import { translatePlural, useLocale } from "../i18n";
import type { MoneyCoverage } from "../lib/moneycoverage";
import { routeHash } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";

// Where the exchange rates are kept.
export const RATE_SHEET = routeHash(settingsHref("company", "money"));

// The two reasons a deal is left out of a converted total, worded apart. A
// missing rate is the one a reader can fix, so it links to the rate sheet.
export function useCoverageWords(): (coverage: MoneyCoverage) => {
  rate: string | null;
  value: string | null;
} {
  const { locale } = useLocale();
  return (coverage) => ({
    rate:
      coverage.noRate > 0
        ? translatePlural(locale, "coverage.noRate", coverage.noRate, {
            count: formatNumber(coverage.noRate, locale),
            currencies: coverage.rateCurrencies.length
              ? ` (${coverage.rateCurrencies.join(", ")})`
              : "",
          })
        : null,
    value:
      coverage.noValue > 0
        ? translatePlural(locale, "coverage.noValue", coverage.noValue, {
            count: formatNumber(coverage.noValue, locale),
          })
        : null,
  });
}
