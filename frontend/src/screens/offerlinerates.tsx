// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field } from "../design-system/atoms";
import { useT } from "../i18n";

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
  return (
    <>
      <Field label={t("offer.discountPct")}>
        {(control) => (
          <input
            {...control}
            data-testid="new-line-discount"
            type="number"
            step="0.01"
            className="input"
            style={{ width: 90 }}
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
      <Field label={t("offer.taxRate")}>
        {(control) => (
          <input
            {...control}
            data-testid="new-line-tax"
            type="number"
            step="0.01"
            className="input"
            style={{ width: 90 }}
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
