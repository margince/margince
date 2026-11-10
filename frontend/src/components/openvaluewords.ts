// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatMoney } from "../format/format";
import { MONEY_ABSENT } from "../format/moneyword";
import type { Locale, useT } from "../i18n";
import type { OpenValue } from "../lib/openvalue";

/** An open value as a figure says it: the sum, "Mixed currencies", or a dash. */
export function openValueWords(
  value: OpenValue,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  if (value.kind === "sum")
    return formatMoney(value.minor, value.currency, locale);
  return value.kind === "mixed" ? t("contact.deals.noValue") : MONEY_ABSENT;
}
