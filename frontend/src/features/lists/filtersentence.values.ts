// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatMoney, formatNumber } from "../../format/format";
import type { Locale, Translator } from "../../i18n";
import type { RadiusOperand } from "../../lib/segmentpredicate";

/** A radius as "50 km of Stuttgart", or of its coordinates when no place was named. */
export function radiusText(
  operand: RadiusOperand,
  t: Translator,
  locale: Locale,
): string {
  return t("filters.sentence.radiusOf", {
    km: formatNumber(operand.radius_km, locale),
    place:
      operand.label ??
      `${formatNumber(operand.lat, locale)}, ${formatNumber(operand.lon, locale)}`,
  });
}

/**
 * A money operand or value (minor units of the field's currency) as the
 * reader reads money, or undefined for anything that is not an amount.
 */
export function moneyText(
  raw: unknown,
  currency: string,
  locale: Locale,
): string | undefined {
  const one = (v: unknown) =>
    typeof v === "number" || (typeof v === "string" && /^-?\d+$/.test(v))
      ? formatMoney(Number(v), currency, locale)
      : undefined;
  if (Array.isArray(raw)) {
    const each = raw.map(one);
    return each.every((v) => v !== undefined) ? each.join(", ") : undefined;
  }
  return one(raw);
}
