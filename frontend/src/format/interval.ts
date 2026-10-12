// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n/locale";
import { INTL_LOCALE } from "./format";

const INTERVAL_UNITS = [
  ["day", 86_400],
  ["hour", 3_600],
  ["minute", 60],
  ["second", 1],
] as const;

// A configured interval read back in words, every whole unit kept: 5,400
// seconds is "1 hour, 30 minutes", where formatDuration would floor it to "1 hr".
export function formatInterval(seconds: number, locale: Locale): string {
  const unitWords = (unit: string, count: number) =>
    new Intl.NumberFormat(INTL_LOCALE[locale], {
      style: "unit",
      unit,
      unitDisplay: "long",
    }).format(count);
  let rest = Math.max(0, Math.round(seconds));
  const parts: string[] = [];
  for (const [unit, size] of INTERVAL_UNITS) {
    const count = Math.floor(rest / size);
    rest -= count * size;
    if (count > 0) parts.push(unitWords(unit, count));
  }
  if (parts.length === 0) return unitWords("second", 0);
  return new Intl.ListFormat(INTL_LOCALE[locale], {
    style: "long",
    type: "unit",
  }).format(parts);
}

// How long something has waited, in its largest whole unit: a queue that jammed
// 61 minutes ago reads "1 hour", where the remainder would only be noise.
export function formatAge(seconds: number, locale: Locale): string {
  const whole = Math.max(0, Math.floor(seconds));
  const size =
    INTERVAL_UNITS.find(([, unit]) => whole >= unit)?.[1] ??
    INTERVAL_UNITS[INTERVAL_UNITS.length - 1][1];
  return formatInterval(Math.floor(whole / size) * size, locale);
}
