// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A money clause's operand, typed in its currency's major units and sent as
// the minor units the filter engine compares.

import { useState } from "react";
import { ErrorLine } from "../../design-system/errorline";
import { MoneyInput } from "../../design-system/moneyinput";
import { TokenInput, type TokenParse } from "../../design-system/tokeninput";
import { typedAmountText, typedMinorUnits } from "../../format/minorunits";
import { type Locale, useLocale, useT } from "../../i18n";
import type { FilterOp, LeafValue } from "../../lib/segmentpredicate";

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
  const { locale } = useLocale();
  // The first amount the box refused, named under it until none is left.
  const [refused, setRefused] = useState<string | undefined>();
  if (op === "in") {
    // A semicolon parts the amounts, not a comma: "12,5" is one amount.
    const parse: TokenParse = {
      keys: [";"],
      read: (raw) => {
        const parts = raw.split(";").filter((part) => part.trim() !== "");
        const unread = (part: string) =>
          "refused" in typedMinorUnits(part, currency, locale);
        return {
          values: parts.filter((part) => !unread(part)),
          refused: parts.filter(unread),
        };
      },
      onRefuse: (parts) => setRefused(parts[0]),
    };
    const fault = refused && typedMinorUnits(refused, currency, locale);
    return (
      <>
        <TokenInput
          values={
            Array.isArray(value)
              ? value.map((v) => majorText(v, currency, locale))
              : []
          }
          onChange={(next) => onChange(minorListOrText(next, currency, locale))}
          parse={parse}
          aria-label={`${label ?? t("filters.values")} (${currency})`}
          placeholder={t("filters.addValue")}
        />
        {fault && "refused" in fault && (
          <ErrorLine>{t(fault.refused.key, fault.refused.params)}</ErrorLine>
        )}
      </>
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
function majorText(
  minor: string | number,
  currency: string,
  locale: Locale,
): string {
  if (typeof minor !== "number") {
    return String(minor);
  }
  return typedAmountText(minor, currency, locale);
}

/**
 * Typed major-unit amounts as minor units, or the typed text while any of them
 * is not an amount this currency can hold — the engine then refuses the clause
 * by name rather than this screen guessing a value nobody typed.
 */
function minorListOrText(
  typed: readonly string[],
  currency: string,
  locale: Locale,
): LeafValue {
  const minor: number[] = [];
  for (const text of typed) {
    const read = typedMinorUnits(text, currency, locale);
    if (!("minor" in read)) {
      return typed;
    }
    minor.push(read.minor);
  }
  return minor;
}
