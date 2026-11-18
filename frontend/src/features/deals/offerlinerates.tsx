// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field } from "../../design-system/atoms";
import { typedNumberFault } from "../../format/decimalinput";
import { useLocale, useT } from "../../i18n";
import "./offers.css";

/**
 * The discount and tax controls on the add-line form.
 *
 * Extracted from offers.tsx, which sits at its length cap, and they belong
 * together anyway: both are percentages against the line's own price, and both
 * are empty-means-the-stored-default rather than empty-means-zero.
 */
export type LineRates = Readonly<{ discount_pct: string; tax_rate: string }>;

export function NewLineRates<T extends LineRates>({
  value,
  onChange,
}: Readonly<{
  value: T;
  onChange: (update: (prev: T) => T) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      <Field
        label={t("offer.discountPct")}
        error={typedNumberFault(value.discount_pct, locale, t)}
      >
        {(control) => (
          <input
            {...control}
            data-testid="new-line-discount"
            inputMode="decimal"
            className="input offers-line-rate"
            value={value.discount_pct}
            onChange={(event) =>
              onChange((prev) => ({
                ...prev,
                discount_pct: event.target.value,
              }))
            }
          />
        )}
      </Field>
      <Field
        label={t("offer.taxRate")}
        error={typedNumberFault(value.tax_rate, locale, t)}
      >
        {(control) => (
          <input
            {...control}
            data-testid="new-line-tax"
            inputMode="decimal"
            className="input offers-line-rate"
            value={value.tax_rate}
            onChange={(event) =>
              onChange((prev) => ({
                ...prev,
                tax_rate: event.target.value,
              }))
            }
          />
        )}
      </Field>
    </>
  );
}
