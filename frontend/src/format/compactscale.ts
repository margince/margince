// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";

const SCALES = {
  thousand: 1_000,
  million: 1_000_000,
  billion: 1_000_000_000,
} as const;
// The scales Intl's compact notation does not abbreviate the way the locale
// reads them. German has no short form for thousands ("12.345,6 $"), and
// Vietnamese business usage is not CLDR's.
const HAND_SCALED_UNITS: Partial<
  Record<Locale, Partial<Record<keyof typeof SCALES, string>>>
> = {
  vi: { thousand: "K", million: " triệu", billion: " tỷ" },
  de: { thousand: "\u00a0Tsd." },
};

// The scale a compact figure of this size is read in, and the unit the locale
// writes after it by hand. No unit leaves the figure to Intl's compact form.
export function handScaledUnit(major: number, locale: Locale) {
  const magnitude =
    Math.abs(major) >= SCALES.billion
      ? "billion"
      : Math.abs(major) >= SCALES.million
        ? "million"
        : "thousand";
  return {
    scale: SCALES[magnitude],
    unit: HAND_SCALED_UNITS[locale]?.[magnitude],
  };
}
