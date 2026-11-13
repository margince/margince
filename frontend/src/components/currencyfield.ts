// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  currencyOptions,
  type LeadingCurrencies,
  searchCurrencies,
} from "../format/currencychoices";
import type { Locale } from "../i18n";
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
