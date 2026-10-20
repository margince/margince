// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatNumber } from "../../format/format";
import type { Locale, PluralTranslator } from "../../i18n";

// A stage's value beside how many priced deals make it, so one large bet and
// many small deals read differently at the same total. The count is of priced
// deals because that is what the value is a sum of.
export function withDealCount(
  amount: string,
  deals: number | undefined,
  plural: PluralTranslator,
  locale: Locale,
): string {
  return deals == null
    ? amount
    : plural("reporting.stageAmountDeals", deals, {
        amount,
        count: formatNumber(deals, locale),
      });
}
