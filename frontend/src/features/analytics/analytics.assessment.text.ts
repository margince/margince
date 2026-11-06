// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  formatAnalyticsHours,
  formatAnalyticsMoney,
  formatAnalyticsNumber,
} from "../../format/format";
import { minorUnitDigits } from "../../format/minorunits";
import type { Locale } from "../../i18n";
import type { SalesAssessment } from "./analytics.assessment";

export function assessmentText(
  sentence: Pick<SalesAssessment["sentences"][number], "text" | "cells">,
  assessment: SalesAssessment,
  locale: Locale,
): string {
  return withCells(sentence, assessment, (value, unit, cell) =>
    shownQuantity(value, unit, cell, locale),
  );
}

// Money takes the page's own format, currency sign first, wherever the
// server's display spelled it with the ISO code.
function shownQuantity(
  value: number,
  unit: string,
  cell: Cell,
  locale: Locale,
) {
  if (isCurrency(unit)) return assessmentQuantity(value, unit, locale);
  return cell.display ?? assessmentQuantity(value, unit, locale);
}

function isCurrency(unit: string) {
  return /^[A-Z]{3}$/.test(unit);
}

/**
 * The sentence with every quantity its cells account for taken out. What is
 * left was typed by the writer itself, and only that is held to
 * `readableQuantities`: the page's own formatting carries a currency sign.
 */
export function assessmentResidue(
  sentence: Pick<SalesAssessment["sentences"][number], "text" | "cells">,
  assessment: SalesAssessment,
  locale: Locale,
): string {
  return withCells(sentence, assessment, (value, unit, cell) =>
    shownQuantity(value, unit, cell, locale) === undefined ? undefined : "",
  );
}

type Cell = SalesAssessment["cells"][number];

function withCells(
  sentence: Pick<SalesAssessment["sentences"][number], "text" | "cells">,
  assessment: SalesAssessment,
  display: (value: number, unit: string, cell: Cell) => string | undefined,
): string {
  let text = sentence.text;
  for (const id of sentence.cells) {
    const cell = assessment.cells?.find((cell) => cell.id === id);
    const match = cell?.value.match(/^(-?\d+(?:\.\d+)?) (\w+)$/);
    if (!cell || !match) continue;
    const raw = match[1];
    const unit = match[2];
    if (raw === undefined || unit === undefined) continue;
    const value = Number(raw);
    const shown = display(value, unit, cell);
    if (shown === undefined) continue;
    // The server writes its own display into the text, so that spelling is
    // matched beside the raw value.
    const literals = [`${raw} ${unit}`, `${value} ${unit}`, cell.display];
    for (const literal of literals) {
      if (!literal) continue;
      const escaped = literal.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      text = text.replace(
        new RegExp(`(^|[^\\d.,])${escaped}(?![\\w])`, "g"),
        (_match, prefix: string) => prefix + shown,
      );
    }
  }
  return text;
}

function assessmentQuantity(value: number, unit: string, locale: Locale) {
  if (isCurrency(unit))
    return formatAnalyticsMoney(
      value * 10 ** minorUnitDigits(unit),
      unit,
      locale,
    );
  if (unit === "hours") return formatAnalyticsHours(value, locale);
  if (unit === "count") return formatAnalyticsNumber(value, locale);
  return undefined;
}

export function readableQuantities(text: string): boolean {
  return !/[€$£¥₫]|\d[\d.,]*\s?[kmb]\b|\d[\d.,]*[.,]\d{1,2}\s+[A-Z]{3}\b/i.test(
    text,
  );
}
