// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { middayInstant } from "../format/calendarday";
import { formatDate } from "../format/format";
import type { Locale, Translator } from "../i18n";
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
    ? t("home.receipt.date", {
        before: change.before
          ? formatDate(middayInstant(change.before, zone), locale, zone)
          : t("home.receipt.undated"),
        after: change.after
          ? formatDate(middayInstant(change.after, zone), locale, zone)
          : t("home.receipt.undated"),
      })
    : t("home.receipt.confidence");
  return [
    summary,
    change.date_changed && change.forecast_changed
      ? t("home.receipt.forecast")
      : undefined,
    change.basis,
  ]
    .filter(Boolean)
    .join(" · ");
}
