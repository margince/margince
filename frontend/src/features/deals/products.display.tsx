// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// How a product reads in the catalogue: its component kind, billing cadence,
// price and status.

import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { formatMoney } from "../../format/format";
import { billingModelLabel } from "../../format/productbilling";
import { type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type Product = components["schemas"]["Product"];

export type ComponentKind = NonNullable<Product["component_kind"]>;

export const COMPONENT_KINDS: readonly ComponentKind[] = [
  "software",
  "implementation",
  "other",
];

export const COMPONENT_LABEL: Readonly<Record<ComponentKind, MessageKey>> = {
  software: "product.componentSoftware",
  implementation: "product.componentImplementation",
  other: "product.componentOther",
};

export function componentKindOf(value: unknown): ComponentKind | undefined {
  return COMPONENT_KINDS.find((kind) => kind === value);
}

export type BillingMonths = NonNullable<Product["billing_interval_months"]>;

export const BILLING_MONTHS: readonly BillingMonths[] = [1, 3, 6, 12];

export const CADENCE: Readonly<Record<BillingMonths, MessageKey>> = {
  1: "product.billingMonthly",
  3: "product.billingQuarterly",
  6: "product.billingHalfYearly",
  12: "product.billingYearly",
};

const PRICE_PER: Readonly<Record<BillingMonths, MessageKey>> = {
  1: "product.pricePerMonth",
  3: "product.pricePerQuarter",
  6: "product.pricePerHalfYear",
  12: "product.pricePerYear",
};

export function billingLabel(p: Product, t: Translator): string {
  if (p.billing_model !== "recurring")
    return billingModelLabel(p.billing_model, t);
  return p.billing_interval_months
    ? t(CADENCE[p.billing_interval_months])
    : t("product.billingRecurring");
}

// The period under the amount, not after it: a 110px price column clips both.
export function ProductPrice({ product }: Readonly<{ product: Product }>) {
  const t = useT();
  const { locale } = useLocale();
  const amount = formatMoney(
    product.unit_price_minor,
    product.currency,
    locale,
  );
  const months = product.billing_interval_months;
  if (product.billing_model !== "recurring" || !months) {
    return amount;
  }
  return (
    <CellStack>
      <span>{amount}</span>
      <span className="t-caption">{t(PRICE_PER[months])}</span>
    </CellStack>
  );
}

// Only a status that differs from the usual one has a name.
export function productStatus(product: Product): MessageKey | null {
  if (product.archived_at) {
    return "product.archived";
  }
  return product.active ? null : "product.inactive";
}

export function ProductStatus({ product }: Readonly<{ product: Product }>) {
  const t = useT();
  const status = productStatus(product);
  return status === null ? null : <Badge>{t(status)}</Badge>;
}
