// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which language a reader gets, kept apart from the catalogs so a page that
// must stay small (the offline page) can ask without carrying every string.

import { readStored, STORAGE_KEYS } from "../app/storage";

export type Locale = "en" | "de" | "vi";

// Display order for the switcher. `satisfies` proves each entry is a real
// locale; i18n.test.ts proves the list is exhaustive.
export const LOCALES = ["en", "de", "vi"] as const satisfies readonly Locale[];

export const DEFAULT_LOCALE: Locale = "en";

/**
 * Whether a string names a language this product speaks.
 *
 * Exported for the public pages: an email's `?lang=` is text a link
 * carried, so it is validated here rather than trusted, and the same
 * predicate that admits a stored pick admits that one.
 */
export function isLocale(value: string): value is Locale {
  return LOCALES.some((locale) => locale === value);
}

// detectLocale reads the visitor's own language preference and maps it to a
// locale we ship, falling back to the A100 default when none of the shipped
// locales is asked for. It never throws off-browser (SSR, tests): an absent
// navigator yields the default.
export function detectLocale(
  languages: readonly string[] = globalThis.navigator?.languages ??
    (globalThis.navigator?.language ? [globalThis.navigator.language] : []),
): Locale {
  for (const tag of languages) {
    const base = tag.toLowerCase().split("-")[0];
    if (isLocale(base)) {
      return base;
    }
  }
  return DEFAULT_LOCALE;
}

/**
 * The locale a reader has chosen, if they have chosen one.
 *
 * Only an explicit pick is stored, never the detected default. Persisting what
 * the browser asked for would freeze it: a reader who later changes their
 * browser's language would keep getting the old one from a value they never set.
 *
 * The stored string is validated rather than trusted. It outlives the release
 * that wrote it, so a locale we have since stopped shipping — or a hand-edited
 * value — must fall back to detection instead of reaching the catalogs as a key
 * they have no entry for.
 */
export function storedLocale(): Locale | null {
  const stored = readStored(STORAGE_KEYS.locale);
  return stored !== null && isLocale(stored) ? stored : null;
}

/** The reader's stored pick, else the browser's language: what a page speaks
 *  before the signed-in account says otherwise. */
export function preferredLocale(): Locale {
  return storedLocale() ?? detectLocale();
}
