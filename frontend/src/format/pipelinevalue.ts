// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale, PluralTranslator } from "../i18n";
import { floorFigure } from "./figure";
import { formatNumber } from "./format";

export function pipelineValueDetail(
  reading: Readonly<{
    currency: string | null | undefined;
    basis: string;
    unpriced: number | undefined;
    incomplete: boolean;
  }>,
  locale: Locale,
  plural: PluralTranslator,
): string {
  const unpriced = reading.unpriced ?? 0;
  return [
    reading.currency,
    reading.basis,
    unpriced > 0
      ? plural("brief.readings.unpricedCount", unpriced, {
          count: floorFigure(
            formatNumber(unpriced, locale),
            reading.incomplete,
            unpriced,
          ),
        })
      : undefined,
  ]
    .filter(Boolean)
    .join(" · ");
}
