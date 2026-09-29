// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { INTL_LOCALE } from "./format";

// English markers as the product spells them. ICU's en-GB draws "m" and "bn"
// (en-US "B"), so the compact part is mapped by meaning; other locales keep
// their own abbreviations ("Mio.", "Mrd.").
const ENGLISH_COMPACT_MARKER: Record<string, string> = {
  k: "K",
  m: "M",
  b: "B",
  bn: "B",
};

/**
 * A token count, abbreviated once it passes ten thousand: "22.5M of 24M".
 *
 * Tokens are metered in the millions, so the exact figure is noise beside the
 * scale. The threshold is `formatMoneyCompact`'s, so a money and a token
 * reading on one page step at the same place.
 */
export function formatTokens(value: number, locale: Locale): string {
  const compact = Math.abs(value) >= 10_000;
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    notation: compact ? "compact" : "standard",
    maximumFractionDigits: compact ? 1 : 0,
  })
    .formatToParts(value)
    .map((part) =>
      locale === "en" && part.type === "compact"
        ? (ENGLISH_COMPACT_MARKER[part.value.toLowerCase()] ?? part.value)
        : part.value,
    )
    .join("");
}
