// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { CreateField } from "../../components/create";
import { amountField } from "../../components/currencyfield";
import {
  currencyOptions,
  type LeadingCurrencies,
} from "../../format/currencychoices";
import { typedNumberFault } from "../../format/decimalinput";
import { billingModelLabel } from "../../format/productbilling";
import type { Locale, Translator } from "../../i18n";
import { labelFor } from "../../lib/labelfor";
import {
  BILLING_MONTHS,
  CADENCE,
  COMPONENT_KINDS,
  COMPONENT_LABEL,
} from "./products.display";
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
    amountField(
      { key: "unit_price", label: "product.unitPrice", required: true },
      {
        currency: (values) => values.currency || leadingCurrencies[0],
        locale,
        t,
      },
    ),
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
      validate: (value) => typedNumberFault(value, locale, t),
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
