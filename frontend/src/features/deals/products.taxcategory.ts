// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";

type Product = components["schemas"]["Product"];

export type TaxCategory = NonNullable<Product["tax_category"]>;

export const TAX_CATEGORIES: readonly TaxCategory[] = [
  "standard",
  "reduced",
  "exempt",
];

export const TAX_CATEGORY_LABEL: Readonly<Record<TaxCategory, MessageKey>> = {
  standard: "taxRates.category.standard",
  reduced: "taxRates.category.reduced",
  exempt: "taxRates.category.exempt",
};

// taxCategoryOf narrows a form value to a category, undefined for unset.
export function taxCategoryOf(value: unknown): TaxCategory | undefined {
  return TAX_CATEGORIES.find((category) => category === value);
}
