// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { middayInstant } from "../../format/calendarday";
import { formatDate, formatNumber } from "../../format/format";
import type { Locale, Translator } from "../../i18n";
import { pluralKey } from "../../i18n";
import type { Receipt } from "./worklist.queries";

export function receiptSummary(
  receipt: Receipt,
  t: Translator,
  locale: Locale,
  zone: string,
): string {
  const change = receipt.close_date_change;
  if (!change) return receipt.summary;
  const summary = change.date_changed
    ? t(
        receipt.review?.reversed || receipt.undo?.reversed
          ? "home.receipt.dateReverted"
          : receipt.review?.accepted
            ? "home.receipt.date"
            : "home.receipt.dateUnconfirmed",
        {
          before: change.before
            ? formatDate(middayInstant(change.before, zone), locale, zone)
            : t("home.receipt.undated"),
          after: change.after
            ? formatDate(middayInstant(change.after, zone), locale, zone)
            : t("home.receipt.undated"),
        },
      )
    : t("home.receipt.confidence");
  return [
    summary,
    change.date_changed && change.forecast_changed
      ? t("home.receipt.forecast")
      : undefined,
  ]
    .filter(Boolean)
    .join(" · ");
}

export function receiptBasis(
  receipt: Receipt,
  t: Translator,
  locale: Locale,
): string | null {
  const change = receipt.close_date_change;
  if (
    !change?.date_changed ||
    receipt.review?.accepted ||
    receipt.review?.reversed ||
    receipt.undo?.reversed
  )
    return null;
  const basis = change.estimate_basis;
  if (!basis) return t("home.receipt.basisUnknown");
  return t(pluralKey(locale, "home.receipt.basis", basis.won_deals), {
    count: formatNumber(basis.won_deals, locale),
    days: formatNumber(basis.median_days_per_stage, locale),
    stages: formatNumber(basis.remaining_stages, locale),
  });
}
