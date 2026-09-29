// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A money clause's operand, typed in its currency's major units and sent as
// the minor units the filter engine compares.

import { MoneyInput } from "../design-system/moneyinput";
import { TokenInput } from "../design-system/tokeninput";
import {
  minorUnitDigits,
  toMajorUnits,
  toMinorUnits,
} from "../format/minorunits";
import { useT } from "../i18n";
import type { FilterOp, LeafValue } from "./segmentpredicate";

/** A money clause's operand: one amount, or a list of them for `in`. */
export function MoneyControl({
  op,
  value,
  onChange,
  label,
  currency,
}: Readonly<{
  op: FilterOp;
  label: string | undefined;
  value: LeafValue;
  onChange: (next: LeafValue) => void;
  currency: string;
}>) {
  const t = useT();
  if (op === "in") {
    return (
      <TokenInput
        values={
          Array.isArray(value) ? value.map((v) => majorText(v, currency)) : []
        }
        onChange={(next) => onChange(minorListOrText(next, currency))}
        aria-label={`${label ?? t("filters.values")} (${currency})`}
        placeholder={t("filters.addValue")}
      />
    );
  }
  return (
    <MoneyValue
      value={value}
      onChange={onChange}
      label={label ?? t("filters.value")}
      currency={currency}
    />
  );
}

/**
 * A money operand typed in the currency's major units and sent as the minor
 * units the engine compares — "10000" in EUR is 1,000,000 cents, in JPY 10,000
 * yen. The unit sits beside the box so the reader knows which they are typing.
 */
function MoneyValue({
  value,
  onChange,
  label,
  currency,
}: Readonly<{
  label: string;
  value: LeafValue;
  onChange: (next: LeafValue) => void;
  currency: string;
}>) {
  const typed = typeof value === "number";
  return (
    <span className="filter-money-value">
      <MoneyInput
        valueMinor={typed ? value : 0}
        currency={currency}
        onChangeMinor={(minor) => onChange(Number.isNaN(minor) ? "" : minor)}
        onClear={() => onChange("")}
        blankWhenZero={!typed}
        aria-label={`${label} (${currency})`}
        inputMode="decimal"
      />
      <span className="filter-money-unit" aria-hidden="true">
        {currency}
      </span>
    </span>
  );
}

/** A listed minor-unit amount as the major-unit text a reader typed. */
function majorText(minor: string | number, currency: string): string {
  if (typeof minor !== "number") {
    return String(minor);
  }
  return toMajorUnits(minor, currency).toFixed(minorUnitDigits(currency));
}

/**
 * Typed major-unit amounts as minor units, or the typed text while any of them
 * is not an amount this currency can hold — the engine then refuses the clause
 * by name rather than this screen guessing a value nobody typed.
 */
function minorListOrText(
  typed: readonly string[],
  currency: string,
): LeafValue {
  const minor = typed.map((v) => toMinorUnits(Number(v), currency));
  const exact = typed.every(
    (v, i) => v.trim() !== "" && !Number.isNaN(minor[i]),
  );
  return exact ? minor : typed;
}
