import type { Translator } from "../i18n";

export function billingModelLabel(
  model: string | null | undefined,
  t: Translator,
): string {
  if (model === "one_time") return t("product.billingOneTime");
  if (model === "recurring") return t("product.billingRecurring");
  return t("product.billingUnclassified");
}
