// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field } from "../../design-system/atoms";
import { typedNumberFault } from "../../format/decimalinput";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { percentFault } from "./products.fields";
import "./offers.css";

/**
 * The discount and tax controls on the add-line form.
 *
 * Extracted from offers.tsx, which sits at its length cap, and they belong
 * together anyway: both are percentages against the line's own price, and both
 * are empty-means-the-stored-default rather than empty-means-zero.
 */
export type LineRates = Readonly<{ discount_pct: string; tax_rate: string }>;

/** A typed discount from 0 to 100 percent, or the reason it is not one. */
export function discountFault(
  text: string,
  locale: Locale,
  t: Translator,
): string | undefined {
  return (
    typedNumberFault(text, locale, t) ??
    percentFault(text, locale, t("offer.discountPct"), {
      min: "0",
      max: "100",
    })
  );
}

/** A typed tax rate of 0 percent or more, or the reason it is not one. */
export function taxRateFault(
  text: string,
  locale: Locale,
  t: Translator,
): string | undefined {
  return (
    typedNumberFault(text, locale, t) ??
    percentFault(text, locale, t("offer.taxRate"), { min: "0" })
  );
}

export function NewLineRates<T extends LineRates>({
  value,
  onChange,
  refused = {},
  onEdited,
}: Readonly<{
  value: T;
  onChange: (update: (prev: T) => T) => void;
  // What the server refused on each rate, until that rate is edited.
  refused?: Readonly<Partial<Record<keyof LineRates, string>>>;
  onEdited?: (field: keyof LineRates) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      <Field
        label={t("offer.discountPct")}
        error={
          discountFault(value.discount_pct, locale, t) ?? refused.discount_pct
        }
      >
        {(control) => (
          <input
            {...control}
            data-testid="new-line-discount"
            inputMode="decimal"
            className="input offers-line-rate"
            value={value.discount_pct}
            onChange={(event) => {
              const discount_pct = event.target.value;
              onChange((prev) => ({ ...prev, discount_pct }));
              onEdited?.("discount_pct");
            }}
          />
        )}
      </Field>
      <Field
        label={t("offer.taxRate")}
        error={taxRateFault(value.tax_rate, locale, t) ?? refused.tax_rate}
      >
        {(control) => (
          <input
            {...control}
            data-testid="new-line-tax"
            inputMode="decimal"
            className="input offers-line-rate"
            value={value.tax_rate}
            onChange={(event) => {
              const tax_rate = event.target.value;
              onChange((prev) => ({ ...prev, tax_rate }));
              onEdited?.("tax_rate");
            }}
          />
        )}
      </Field>
    </>
  );
}
