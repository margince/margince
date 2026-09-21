// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";

/**
 * A money reading, or the WORD the caller gives for its absence.
 *
 * The same question `formatMoneyOrAbsent` asks — can this pair be said as money
 * at all — answered for a surface that must not draw the em dash. A stat card's
 * value is a READING and a glyph is not one: "None won yet" and "Not forecast"
 * are facts a reader acts on, where a dash reads as a slot that failed.
 *
 * It exists because three screens had each reached that answer by calling
 * `formatMoneyOrAbsent` and then STRING-COMPARING its sentinel, which makes the
 * em dash a protocol between files — change the glyph and every one of those
 * branches silently stops firing. The dash is `format.ts`'s own answer, not a
 * value to test for. The WORD belongs to the slot, because "no deal open" and
 * "none won yet" are different facts and a dash states neither.
 *
 * Its own module so `format.ts` can express the dash-returning spelling through
 * it without importing itself, and so the FORMATTER is named at every call
 * site: a slot on a strip is a hundred points wide and takes
 * `formatMoneyCompact`, while money somebody is OWED keeps every digit — that
 * form carries no fraction below ten thousand, so forty cents outstanding reads
 * "€0". Only the call site knows which of the two it holds, and a default here
 * would let it not say.
 */
export function formatMoneyOrWord(
  amountMinor: number | null | undefined,
  currency: string | null | undefined,
  locale: Locale,
  absent: string,
  format: (amountMinor: number, currency: string, locale: Locale) => string,
): string {
  if (amountMinor == null || !currency) {
    return absent;
  }
  return format(amountMinor, currency, locale);
}
