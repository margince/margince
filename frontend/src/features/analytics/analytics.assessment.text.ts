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
  let text = sentence.text;
  for (const id of sentence.cells) {
    const cell = assessment.cells?.find((cell) => cell.id === id);
    const match = cell?.value.match(/^(-?\d+(?:\.\d+)?) (\w+)$/);
    if (!match) continue;
    const raw = match[1];
    const unit = match[2];
    if (raw === undefined || unit === undefined) continue;
    const value = Number(raw);
    const display = cell?.display ?? assessmentQuantity(value, unit, locale);
    if (display === undefined) continue;
    for (const literal of [`${raw} ${unit}`, `${value} ${unit}`]) {
      const escaped = literal.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      text = text.replace(
        new RegExp(`(^|[^\\d.,])${escaped}(?![\\w])`, "g"),
        (_match, prefix: string) => prefix + display,
      );
    }
  }
  return text;
}

function assessmentQuantity(value: number, unit: string, locale: Locale) {
  if (/^[A-Z]{3}$/.test(unit))
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
