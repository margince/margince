// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { Locale, PluralTranslator, Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { formatNumber } from "./format";

export type BillingMonths = NonNullable<
  components["schemas"]["Product"]["billing_interval_months"]
>;

export const BILLING_MONTHS: readonly BillingMonths[] = [1, 3, 6, 12];

// The cadence words for a product, a deal line and an offer line alike.
export const CADENCE: Readonly<Record<BillingMonths, MessageKey>> = {
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
  const months = BILLING_MONTHS.find(
    (admitted) => admitted === line.billing_interval_months,
  );
  const cadence = t(
    months === undefined ? "product.billingRecurring" : CADENCE[months],
  );
  if (line.interval_count == null) return cadence;
  return plural("legalReview.offerLineRecurring", line.interval_count, {
    cadence,
    count: formatNumber(line.interval_count, locale),
  });
}
