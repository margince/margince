// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import { Field, type FieldControl } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import {
  BILLING_MONTHS,
  CADENCE,
  type LineBillingTerms,
} from "../../format/billingphrase";
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
    ...BILLING_MONTHS.map((months) => ({
      value: String(months),
      label: t(CADENCE[months]),
    })),
  ];
}

// Leaving "recurring" takes the cadence and the term with it: a one-off price
// has neither, and the server refuses a line that still carries them.
function withBillingModel(value: LineBilling, picked: string): LineBilling {
  return picked === "recurring"
    ? { ...value, billingModel: "recurring" }
    : { billingModel: picked, billingIntervalMonths: "", intervalCount: "" };
}

type ControlName = FieldControl | Readonly<{ "aria-label": string }>;

// On a form each control sits under its Field. In a table row the header
// names the column, so `label` names each control's column and line instead.
function Labelled({
  column,
  label,
  children,
}: Readonly<{
  column: string;
  label?: (column: string) => string;
  children: (control: ControlName) => ReactNode;
}>) {
  return label ? (
    children({ "aria-label": label(column) })
  ) : (
    <Field label={column}>{children}</Field>
  );
}

export function OfferLineBillingFields({
  value,
  onChange,
  label,
  onTermTyped,
  onTermBlur,
}: Readonly<{
  value: LineBilling;
  onChange: (next: LineBilling) => void;
  label?: (column: string) => string;
  // Where the term is saved on blur: typing reaches onTermTyped, not onChange.
  onTermTyped?: (intervalCount: string) => void;
  onTermBlur?: () => void;
}>) {
  const t = useT();
  const recurring = value.billingModel === "recurring";
  const typed =
    onTermTyped ??
    ((intervalCount: string) => onChange({ ...value, intervalCount }));
  return (
    <>
      <Labelled column={t("product.billingModel")} label={label}>
        {(control) => (
          <Select
            {...control}
            value={value.billingModel}
            onChange={(picked) => onChange(withBillingModel(value, picked))}
            options={billingModelOptions(t)}
          />
        )}
      </Labelled>
      {/* Both only where they mean something. A cadence beside a one-off price
          is a question with no answer, and showing it invites somebody to fill
          it in and be refused. */}
      {recurring && (
        <Labelled column={t("product.billingInterval")} label={label}>
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
        </Labelled>
      )}
      {recurring && (
        <Labelled column={t("offer.committedPeriods")} label={label}>
          {(control) => (
            <input
              {...control}
              type="number"
              min="1"
              step="1"
              className="input offers-line-periods"
              value={value.intervalCount}
              onChange={(event) => typed(event.target.value)}
              onBlur={onTermBlur}
            />
          )}
        </Labelled>
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
 * The three controls as the wire takes them, for an offer-line update. It is
 * null while a recurring line has no period yet, which the server refuses.
 *
 * Unlike a create, every field is named. The server judges the line as the
 * patch leaves it, and a field left out keeps a cadence or term the line no
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

// A line's saved terms as the controls hold them. No interval and no term
// both read as empty, so 0 is never offered back as a choice.
export function lineBillingOf(line: LineBillingTerms): LineBilling {
  return {
    billingModel: line.billing_model ?? "",
    billingIntervalMonths: line.billing_interval_months
      ? String(line.billing_interval_months)
      : "",
    intervalCount: line.interval_count ? String(line.interval_count) : "",
  };
}

function sameBilling(a: LineBilling, b: LineBilling): boolean {
  return (
    a.billingModel === b.billingModel &&
    a.billingIntervalMonths === b.billingIntervalMonths &&
    a.intervalCount === b.intervalCount
  );
}

type BillingField = keyof LineBilling;

const BILLING_FIELDS: readonly BillingField[] = [
  "billingModel",
  "billingIntervalMonths",
  "intervalCount",
];

// The fields the reader changed whose value no saved answer has shown yet.
function editedSince(
  edited: ReadonlySet<BillingField>,
  before: LineBilling,
  after: LineBilling,
): ReadonlySet<BillingField> {
  const next = new Set(edited);
  for (const field of BILLING_FIELDS) {
    if (before[field] !== after[field]) next.add(field);
  }
  return next;
}

// A newer answer moves each control the reader has not edited. An edited one
// stays, and its mark clears once an answer shows the value it holds.
function reconciled(
  value: LineBilling,
  edited: ReadonlySet<BillingField>,
  saved: LineBilling,
): [LineBilling, ReadonlySet<BillingField>] {
  const next = { ...value };
  const still = new Set<BillingField>();
  for (const field of BILLING_FIELDS) {
    if (!edited.has(field)) next[field] = saved[field];
    else if (saved[field] !== value[field]) still.add(field);
  }
  return [next, still];
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
  const saved = lineBillingOf(line);
  const [value, setValue] = useState(saved);
  // Typed back to the old value, a field still counts as edited.
  const [edited, setEdited] = useState<ReadonlySet<BillingField>>(new Set());
  const [base, setBase] = useState(saved);
  if (!sameBilling(base, saved)) {
    const [next, still] = reconciled(value, edited, saved);
    setBase(saved);
    setValue(next);
    setEdited(still);
  }
  const change = (next: LineBilling) => {
    setEdited(editedSince(edited, value, next));
    setValue(next);
  };
  const commit = (next: LineBilling) => {
    const patch = lineBillingPatch(next);
    // A line just made recurring waits for its period; a saved period is put
    // back, since the server keeps it until another one is picked.
    if (!patch && line.billing_interval_months) {
      setValue(saved);
      setEdited(new Set());
      return;
    }
    change(next);
    if (patch) {
      onSave(patch);
    }
  };
  return (
    <div className="offers-line-billing">
      <OfferLineBillingFields
        value={value}
        label={label}
        onChange={commit}
        onTermTyped={(intervalCount) => change({ ...value, intervalCount })}
        onTermBlur={() => {
          if (value.intervalCount !== saved.intervalCount) {
            commit(value);
          } else if (edited.has("intervalCount")) {
            // Left at the saved term: nothing is waiting for an answer.
            const rest = new Set(edited);
            rest.delete("intervalCount");
            setEdited(rest);
          }
        }}
      />
    </div>
  );
}
