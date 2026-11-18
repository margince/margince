// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { amountFault } from "../format/minorunits";
import type { MessageKey } from "../i18n/en";
import type { CreateField } from "./create.fields";

type Translate = (key: MessageKey, params?: Record<string, string>) => string;

/** What to tell the reader about an amount `currency` cannot hold. */
export function amountRefusal(
  t: Translate,
  value: string,
  currency: string,
): string | undefined {
  switch (amountFault(value, currency)) {
    case "too_precise":
      return t("record.amountTooPrecise", { currency });
    case "too_large":
      return t("record.amountTooLarge");
    default:
      return undefined;
  }
}

/**
 * A deal's money amount, typed in major units of the form's `currency` (or
 * `currency` here when the form shows none). An amount that currency cannot
 * hold is refused under the field: converted, it is NaN, which a request sends
 * as null, and a null Expected ARR clears the stored one.
 */
export function moneyAmountField(
  t: Translate,
  field: Readonly<{
    key: string;
    label: MessageKey;
    wireField: string;
    currency: string;
  }>,
): CreateField {
  return {
    key: field.key,
    label: field.label,
    type: "number",
    step: "any",
    min: "0",
    wireField: field.wireField,
    validate: (value, values) => {
      const currency = values.currency?.trim() || field.currency;
      if (Number(value) < 0) return t("create.amountNegative");
      return amountRefusal(t, value, currency);
    },
  };
}
