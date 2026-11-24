// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { floorFigure } from "./figure";
import { formatNumber } from "./format";

// A count on a glyph's shoulder stops at two digits, so a third never covers
// the glyph it counts. The control's name still carries the whole figure.
const BADGE_MAX = 99;

export function formatBadgeCount(count: number, locale: Locale): string {
  const shown = Math.min(count, BADGE_MAX);
  return floorFigure(formatNumber(shown, locale), count > BADGE_MAX, shown);
}
