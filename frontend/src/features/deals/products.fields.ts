// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { CreateField } from "../../components/create";
import { amountField } from "../../components/currencyfield";
import { BILLING_MONTHS, CADENCE } from "../../format/billingphrase";
import {
  currencyOptions,
  type LeadingCurrencies,
} from "../../format/currencychoices";
import { typedNumberFault, typedNumberOrNaN } from "../../format/decimalinput";
import { typedMinorUnits } from "../../format/minorunits";
import { billingModelLabel } from "../../format/productbilling";
import type { Locale, Translator } from "../../i18n";
import { labelFor } from "../../lib/labelfor";
import {
  fieldCheckSentence,
  moneyLimit,
  percentLimit,
} from "../../lib/problem";
import { COMPONENT_KINDS, COMPONENT_LABEL } from "./products.display";
import { OTHER_UNIT, unitOptions } from "./productunit";
import { TAX_CATEGORIES, TAX_CATEGORY_LABEL } from "./taxcategory";

// Built per render: option labels are shown as given, so they arrive translated.
export function productFields(
  t: Translator,
  leadingCurrencies: LeadingCurrencies,
  locale: Locale,
): CreateField[] {
  return [
    { key: "name", label: "product.name", required: true },
    { key: "sku", label: "product.sku" },
    { key: "description", label: "product.description" },
    {
      key: "unit",
      label: "product.unit",
      type: "select",
      required: true,
      options: unitOptions(t),
    },
    {
      key: "unit_other",
      label: "product.unitOtherLabel",
      required: true,
      showWhen: (values) => values.unit === OTHER_UNIT,
    },
    priceField(leadingCurrencies, locale, t),
    {
      key: "currency",
      label: "product.currency",
      type: "select",
      required: true,
      options: currencyOptions(leadingCurrencies),
    },
    {
      key: "default_tax_rate",
      label: "product.taxRate",
      type: "decimal",
      limit: percentLimit(locale),
      validate: (value) =>
        typedNumberFault(value, locale, t) ??
        percentFault(value, locale, t("product.taxRate"), { min: "0" }),
    },
    {
      // Which rate of the country an offer is issued from applies. Unset is
      // a real answer: nobody has classified it, and a line takes the rate
      // above.
      key: "tax_category",
      label: "product.taxCategory",
      type: "select",
      options: [
        { value: "", label: t("product.taxCategoryUnset") },
        ...TAX_CATEGORIES.map((category) => ({
          value: category,
          label: t(labelFor(TAX_CATEGORY_LABEL, category)),
        })),
      ],
    },
    {
      // The empty option is a real answer, and every product written before
      // this field existed carries it: nobody has said whether the price
      // repeats. It comes first, so a form left alone keeps saying that
      // instead of asserting a one-off price.
      key: "billing_model",
      label: "product.billingModel",
      type: "select",
      options: [
        { value: "", label: billingModelLabel(null, t) },
        { value: "one_time", label: billingModelLabel("one_time", t) },
        { value: "recurring", label: billingModelLabel("recurring", t) },
      ],
    },
    {
      // Only a recurring price has a period, and the server refuses one
      // without it, so the choice appears, and is owed, only then.
      key: "billing_interval_months",
      label: "product.billingInterval",
      type: "select",
      required: true,
      showWhen: (values) => values.billing_model === "recurring",
      options: BILLING_MONTHS.map((months) => ({
        value: String(months),
        label: t(CADENCE[months]),
      })),
    },
    {
      // What the product is, which the commercial mix on Analytics groups won
      // value by. Unset is a real answer: nobody has classified it.
      key: "component_kind",
      label: "product.componentKind",
      type: "select",
      options: [
        { value: "", label: t("product.componentUnset") },
        ...COMPONENT_KINDS.map((kind) => ({
          value: kind,
          label: t(labelFor(COMPONENT_LABEL, kind)),
        })),
      ],
    },
  ];
}

// The shared money box, with below zero in the server's words.
function priceField(
  leadingCurrencies: LeadingCurrencies,
  locale: Locale,
  t: Translator,
): CreateField {
  const currency = (values: Record<string, string>) =>
    values.currency || leadingCurrencies[0];
  const money = amountField(
    {
      key: "unit_price",
      label: "product.unitPrice",
      required: true,
      wireField: "unit_price_minor",
    },
    { currency, locale, t },
  );
  const label = t("product.unitPrice");
  return {
    ...money,
    // A refusal names no currency: the one the refused form was sent in.
    limit: (wire, sent) => moneyLimit(currency(sent), locale)(wire),
    validate: (value, values = {}) => {
      const typed = typedMinorUnits(value, currency(values), locale);
      if ("minor" in typed && typed.minor < 0) {
        return fieldCheckSentence(
          "out_of_range",
          locale,
          { label, limit: moneyLimit(currency(values), locale) },
          { min: "0" },
        );
      }
      return money.validate?.(value, values);
    },
  };
}

/**
 * A typed percent outside the server's bounds, in the words of its refusal.
 * `bounds` are percent, as the wire states them.
 */
export function percentFault(
  text: string,
  locale: Locale,
  label: string,
  bounds: Readonly<{ min?: string; max?: string }>,
): string | undefined {
  if (text.trim() === "") {
    return undefined;
  }
  const value = typedNumberOrNaN(text, locale);
  const { min, max } = bounds;
  const below = min !== undefined && value < Number(min);
  const above = max !== undefined && value > Number(max);
  return below || above
    ? fieldCheckSentence(
        "out_of_range",
        locale,
        { label, limit: percentLimit(locale) },
        bounds,
      )
    : undefined;
}
