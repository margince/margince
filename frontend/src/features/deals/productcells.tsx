import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { formatMoney } from "../../format/format";
import { toMajorUnits, toMinorUnits } from "../../format/minorunits";
import { billingModelLabel } from "../../format/productbilling";
import { type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { unitChoiceOf } from "./productunit";

type Product = components["schemas"]["Product"];

type ComponentKind = NonNullable<Product["component_kind"]>;

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

type BillingMonths = NonNullable<Product["billing_interval_months"]>;

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

export function editableProduct(p: Product) {
  const unit = unitChoiceOf(p.unit);
  return {
    ...p,
    unit: unit.choice,
    unit_other: unit.other,
    unit_price: String(toMajorUnits(p.unit_price_minor, p.currency)),
    billing_model: p.billing_model ?? "",
    component_kind: p.component_kind ?? "",
    billing_interval_months:
      p.billing_interval_months == null
        ? ""
        : String(p.billing_interval_months),
  };
}

// text narrows one value off the edit form's Record<string, unknown> without
// asserting. A field the reader left alone is absent, not an empty string.
// An assertion would hand `undefined` to a scale that silently defaults.
export function text(value: unknown): string {
  return typeof value === "string" ? value : "";
}

// Major-unit price string -> integer minor units (P11: no float money on the
// wire), at the scale the PRODUCT's own currency carries. A hard-coded hundred
// priced a yen product at a hundredth and a dinar product at ten times.
export function toMinor(major: string | undefined, currency: string): number {
  return toMinorUnits(Number(major ?? "0"), currency);
}
