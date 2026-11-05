// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";

/** A weekday as `Date.getDay()` numbers it: 0 is Sunday. */
export type Weekday = 0 | 1 | 2 | 3 | 4 | 5 | 6;

// Not Intl.Locale weekInfo: Firefox has none, so the grid would change with the browser.
const WEEK_START: Record<Locale, Weekday> = { de: 1, en: 1, vi: 1 };

/** The weekday a calendar's week opens on, for the UI language. */
export function weekStart(locale: Locale): Weekday {
  return WEEK_START[locale];
}
