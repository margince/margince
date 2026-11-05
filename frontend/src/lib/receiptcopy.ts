// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  summaryFieldsLine,
  summarySentence,
} from "../design-system/approvalsummary";
import { middayInstant } from "../format/calendarday";
import { formatDate, formatNumber } from "../format/format";
import type { Locale, Translator } from "../i18n";
import { pluralKey } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { summaryWords } from "./approvalpreviewfields";
import type { Receipt } from "./handled";

// A close date the nightly verdict read from the customer's words. It is no
// estimate, so it carries neither the estimate's wording nor its basis.
const VERDICT_DATE = "deal_close_date";

export function receiptSummary(
  receipt: Receipt,
  t: Translator,
  locale: Locale,
  zone: string,
): string {
  const change = receipt.close_date_change;
  if (!change) {
    const fields = summaryFieldsLine(receipt.summary_parts, summaryWords(t));
    return [summarySentence(receipt), fields].filter(Boolean).join(" — ");
  }
  const summary = change.date_changed
    ? t(dateSummaryKey(receipt), {
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
  ]
    .filter(Boolean)
    .join(" · ");
}

function dateSummaryKey(receipt: Receipt): MessageKey {
  const reversed = receipt.review?.reversed || receipt.undo?.reversed;
  if (receipt.kind === VERDICT_DATE) {
    return reversed
      ? "home.receipt.verdictDateReverted"
      : "home.receipt.verdictDate";
  }
  if (reversed) return "home.receipt.dateReverted";
  return receipt.review?.accepted
    ? "home.receipt.date"
    : "home.receipt.dateUnconfirmed";
}

export function receiptBasis(
  receipt: Receipt,
  t: Translator,
  locale: Locale,
): string | null {
  const change = receipt.close_date_change;
  if (
    !change?.date_changed ||
    receipt.kind === VERDICT_DATE ||
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
