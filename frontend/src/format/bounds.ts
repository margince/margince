// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { formatNumber } from "./format";

type Bounds = Readonly<{ min: number; max: number }>;

// The range a setting box refuses past, grouped for the reader as the
// refusal sentence quotes it.
export function boundsText(
  { min, max }: Bounds,
  locale: Locale,
): Readonly<{ min: string; max: string }> {
  return { min: formatNumber(min, locale), max: formatNumber(max, locale) };
}
