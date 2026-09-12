// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";

/**
 * The three controls that say whether an offer line's price repeats.
 *
 * In its own file because offers.tsx sits at its length cap, and because the
 * add-line form and a later line editor need the same three together.
 *
 * The committed-periods control is the one that matters operationally: sending
 * an offer is refused while a recurring line has no settled term, so a form
 * without this control classifies a line into a state it cannot then send —
 * and, before the line PATCH carried these fields, could not correct either.
 */
export type LineBilling = Readonly<{
  billingModel: string;
  billingIntervalMonths: string;
  intervalCount: string;
}>;

export const EMPTY_LINE_BILLING: LineBilling = {
  billingModel: "",
  billingIntervalMonths: "",
  intervalCount: "",
};

export function OfferLineBillingFields({
  value,
  onChange,
}: Readonly<{
  value: LineBilling;
  onChange: (next: LineBilling) => void;
}>) {
  const t = useT();
  const recurring = value.billingModel === "recurring";
  return (
    <>
      <Field label={t("product.billingModel")}>
        {(control) => (
          <Select
            {...control}
            value={value.billingModel}
            onChange={(picked) =>
              // Leaving "recurring" takes the cadence and the term with it: a
              // one-off price has neither, and the server refuses a line that
              // still carries them.
              onChange(
                picked === "recurring"
                  ? { ...value, billingModel: "recurring" }
                  : {
                      billingModel: picked,
                      billingIntervalMonths: "",
                      intervalCount: "",
                    },
              )
            }
            options={[
              { value: "", label: t("product.billingUnclassified") },
              { value: "one_time", label: t("product.billingOneTime") },
              { value: "recurring", label: t("product.billingRecurring") },
            ]}
          />
        )}
      </Field>
      {/* Both only where they mean something. A cadence beside a one-off price
          is a question with no answer, and showing it invites somebody to fill
          it in and be refused. */}
      {recurring && (
        <Field label={t("product.billingInterval")}>
          {(control) => (
            <Select
              {...control}
              value={value.billingIntervalMonths}
              onChange={(picked) =>
                onChange({ ...value, billingIntervalMonths: picked })
              }
              options={[
                { value: "", label: t("product.billingNoInterval") },
                { value: "1", label: t("product.billingMonthly") },
                { value: "3", label: t("product.billingQuarterly") },
                { value: "6", label: t("product.billingHalfYearly") },
                { value: "12", label: t("product.billingYearly") },
              ]}
            />
          )}
        </Field>
      )}
      {recurring && (
        <Field label={t("offer.committedPeriods")}>
          {(control) => (
            <input
              {...control}
              data-testid="new-line-interval-count"
              type="number"
              min="1"
              step="1"
              className="input"
              style={{ width: 110 }}
              value={value.intervalCount}
              onChange={(event) =>
                onChange({ ...value, intervalCount: event.target.value })
              }
            />
          )}
        </Field>
      )}
    </>
  );
}

/**
 * The three controls as the wire takes them, for an offer-line create.
 *
 * Mirrors `billingOf` for products, plus the committed term. An empty
 * selection stays absent rather than becoming null, because a create that
 * names no classification is a line that says nothing about whether its price
 * repeats — and where the line names a product, that is what lets the product's
 * own classification travel onto it.
 */
export function lineBillingBody(value: LineBilling): {
  billing_model?: "one_time" | "recurring";
  billing_interval_months?: 1 | 3 | 6 | 12;
  interval_count?: number;
} {
  if (value.billingModel === "one_time") {
    return { billing_model: "one_time" };
  }
  if (value.billingModel !== "recurring") {
    return {};
  }
  const months = Number(value.billingIntervalMonths);
  const count = Number(value.intervalCount);
  return {
    billing_model: "recurring",
    billing_interval_months:
      months === 1 || months === 3 || months === 6 || months === 12
        ? months
        : undefined,
    interval_count: Number.isInteger(count) && count > 0 ? count : undefined,
  };
}
