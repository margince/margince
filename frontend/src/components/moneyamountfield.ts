// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";
import { amountRefusal, type Translate } from "./amountrefusal";
import type { CreateField } from "./create.fields";

/**
 * A deal's money amount, typed in major units of the form's `currency` (or
 * `currency` here when the form shows none). An amount that currency cannot
 * hold is refused under the field. Converted, it is NaN, which a request sends
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
