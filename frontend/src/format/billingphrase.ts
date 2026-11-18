// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale, PluralTranslator, Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { formatNumber } from "./format";

const CADENCE: Readonly<Record<number, MessageKey>> = {
  1: "product.billingMonthly",
  3: "product.billingQuarterly",
  6: "product.billingHalfYearly",
  12: "product.billingYearly",
};

export type LineBillingTerms = Readonly<{
  billing_model?: string | null;
  billing_interval_months?: number | null;
  interval_count?: number | null;
}>;

// How an offer line bills the buyer, in words; null for a line that names no
// billing model.
export function lineBillingPhrase(
  line: LineBillingTerms,
  t: Translator,
  plural: PluralTranslator,
  locale: Locale,
): string | null {
  if (line.billing_model === "one_time") return t("product.billingOneTime");
  if (line.billing_model !== "recurring") return null;
  const months = line.billing_interval_months;
  const cadence = t(
    (months != null && CADENCE[months]) || "product.billingRecurring",
  );
  if (line.interval_count == null) return cadence;
  return plural("legalReview.offerLineRecurring", line.interval_count, {
    cadence,
    count: formatNumber(line.interval_count, locale),
  });
}
