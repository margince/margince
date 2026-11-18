// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Field } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import type { OfferLineItem, UpdateOfferLineItemRequest } from "./offers";
import "./offers.css";

/**
 * The three controls that say whether an offer line's price repeats.
 *
 * In its own file because offers.tsx sits at its length cap, and because the
 * add-line form and each line's row need the same three together.
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

type Translate = ReturnType<typeof useT>;

function billingModelOptions(t: Translate) {
  return [
    { value: "", label: t("product.billingUnclassified") },
    { value: "one_time", label: t("product.billingOneTime") },
    { value: "recurring", label: t("product.billingRecurring") },
  ];
}

function billingIntervalOptions(t: Translate) {
  return [
    { value: "", label: t("product.billingNoInterval") },
    { value: "1", label: t("product.billingMonthly") },
    { value: "3", label: t("product.billingQuarterly") },
    { value: "6", label: t("product.billingHalfYearly") },
    { value: "12", label: t("product.billingYearly") },
  ];
}

// Leaving "recurring" takes the cadence and the term with it: a one-off price
// has neither, and the server refuses a line that still carries them.
function withBillingModel(value: LineBilling, picked: string): LineBilling {
  return picked === "recurring"
    ? { ...value, billingModel: "recurring" }
    : { billingModel: picked, billingIntervalMonths: "", intervalCount: "" };
}

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
            onChange={(picked) => onChange(withBillingModel(value, picked))}
            options={billingModelOptions(t)}
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
              options={billingIntervalOptions(t)}
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
              className="input offers-line-periods"
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

type LineBillingPatch = Required<
  Pick<
    UpdateOfferLineItemRequest,
    "billing_model" | "billing_interval_months" | "interval_count"
  >
>;

/**
 * The three controls as the wire takes them, for an offer-line update, or
 * null while a recurring line has no period yet, which the server refuses.
 *
 * Unlike a create, every field is named: the server judges the line as the
 * patch leaves it, so a field left out keeps a cadence or term the line no
 * longer means. A typed term goes as typed, so a bad one comes back refused
 * with the reason instead of being cleared.
 */
export function lineBillingPatch(value: LineBilling): LineBillingPatch | null {
  if (value.billingModel !== "recurring") {
    return {
      billing_model: value.billingModel === "one_time" ? "one_time" : null,
      billing_interval_months: null,
      interval_count: null,
    };
  }
  const months = lineBillingBody(value).billing_interval_months;
  if (months === undefined) {
    return null;
  }
  const count = value.intervalCount.trim();
  return {
    billing_model: "recurring",
    billing_interval_months: months,
    interval_count: count === "" ? null : Number(count),
  };
}

function lineBillingOf(line: OfferLineItem): LineBilling {
  return {
    billingModel: line.billing_model ?? "",
    billingIntervalMonths: String(line.billing_interval_months ?? ""),
    intervalCount: String(line.interval_count ?? ""),
  };
}

// The three controls in a line's row: a pick saves at once, the term on blur.
export function BillingCell({
  line,
  label,
  onSave,
}: Readonly<{
  line: OfferLineItem;
  label: (column: string) => string;
  onSave: (patch: UpdateOfferLineItemRequest) => void;
}>) {
  const t = useT();
  const [value, setValue] = useState(() => lineBillingOf(line));
  const commit = (next: LineBilling) => {
    setValue(next);
    const patch = lineBillingPatch(next);
    if (patch) {
      onSave(patch);
    }
  };
  const recurring = value.billingModel === "recurring";
  return (
    <div className="offers-line-billing">
      <Select
        aria-label={label(t("product.billingModel"))}
        testId={`line-billing-${line.id}`}
        value={value.billingModel}
        onChange={(picked) => commit(withBillingModel(value, picked))}
        options={billingModelOptions(t)}
      />
      {recurring && (
        <Select
          aria-label={label(t("product.billingInterval"))}
          testId={`line-billing-interval-${line.id}`}
          value={value.billingIntervalMonths}
          onChange={(picked) =>
            commit({ ...value, billingIntervalMonths: picked })
          }
          options={billingIntervalOptions(t)}
        />
      )}
      {recurring && (
        <input
          aria-label={label(t("offer.committedPeriods"))}
          data-testid={`line-interval-count-${line.id}`}
          type="number"
          min="1"
          step="1"
          className="input offers-line-periods"
          value={value.intervalCount}
          onChange={(event) =>
            setValue({ ...value, intervalCount: event.target.value })
          }
          onBlur={() => {
            if (value.intervalCount !== String(line.interval_count ?? "")) {
              commit(value);
            }
          }}
        />
      )}
    </div>
  );
}
