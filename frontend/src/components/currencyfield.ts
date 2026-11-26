// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  currencyOptions,
  type LeadingCurrencies,
  searchCurrencies,
} from "../format/currencychoices";
import { typedMinorUnits } from "../format/minorunits";
import type { Locale, Translator } from "../i18n";
import type { CreateField } from "./create.fields";

/**
 * A form's currency: searched by code or by name in the reader's language,
 * the leading currencies first, and opening on the first of them. Required,
 * because an amount without its currency is half a money value.
 */
export function currencyField(
  leading: LeadingCurrencies,
  locale: Locale,
  current?: string | null,
): CreateField {
  const options = currencyOptions(leading, current, locale);
  return {
    key: "currency",
    label: "create.currency",
    type: "select",
    required: true,
    options,
    searchTargets: searchCurrencies(options),
  };
}

/**
 * A form's amount, typed in major units and read the reader's way. Its check
 * is the one every money box uses, so "12,5" saves 12,50 and "1.500" is
 * refused with the form the reader wants, before anything is sent.
 */
export function amountField(
  field: Pick<CreateField, "key" | "label" | "wireField" | "required">,
  opts: Readonly<{
    // The currency the amount is in, from the form's current values.
    currency: (values: Record<string, string>) => string;
    locale: Locale;
    t: Translator;
    nonNegative?: boolean;
  }>,
): CreateField {
  return {
    ...field,
    type: "decimal",
    validate: (value, values = {}) => {
      if (value.trim() === "") {
        return undefined;
      }
      const typed = typedMinorUnits(value, opts.currency(values), opts.locale);
      if ("refused" in typed) {
        return opts.t(typed.refused.key, typed.refused.params);
      }
      return opts.nonNegative && typed.minor < 0
        ? opts.t("amount.refused.negative")
        : undefined;
    },
  };
}
