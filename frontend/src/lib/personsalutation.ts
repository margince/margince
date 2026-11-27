// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { useT } from "../i18n";

type Choice = NonNullable<components["schemas"]["Salutation"]>;
export const SALUTATION_LABELS = {
  mr: "salutation.mr",
  ms: "salutation.ms",
  none: "salutation.none",
} as const;

export function salutationChoice(
  values: Record<string, unknown>,
): Choice | null | undefined {
  if (!Object.hasOwn(values, "salutation")) return undefined;
  const value = values.salutation;
  return value === "mr" || value === "ms" || value === "none" ? value : null;
}

export function salutationOptions(t: ReturnType<typeof useT>) {
  return [
    { value: "", label: t("salutation.unset") },
    ...Object.entries(SALUTATION_LABELS).map(([value, key]) => ({
      value,
      label: t(key),
    })),
  ];
}

export function salutationField(t: ReturnType<typeof useT>) {
  return {
    key: "salutation",
    label: "create.salutation" as const,
    type: "select" as const,
    options: salutationOptions(t),
    hint: t("salutation.hint"),
  };
}

export function salutationLabel(
  value: string | null | undefined,
  t: ReturnType<typeof useT>,
) {
  return value === "mr" || value === "ms" || value === "none"
    ? t(SALUTATION_LABELS[value])
    : undefined;
}

export function salutationFilter(t: ReturnType<typeof useT>) {
  return {
    key: "salutation",
    label: "create.salutation" as const,
    allLabel: "salutation.any" as const,
    options: salutationOptions(t).map((option) => ({
      ...option,
      value: option.value || "unset",
    })),
  };
}
