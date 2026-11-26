// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { PartialMark } from "../../components/partialmark";
import {
  formatAnalyticsMoneyOrAbsent,
  formatAnalyticsNumber,
  MONEY_ABSENT,
} from "../../format/format";
import type { Locale, useT } from "../../i18n";
import { coverageOfMeasure, hasGap } from "../../lib/moneycoverage";
import { beyondExactRange } from "./analytics.cells";
import { CoverageFootnote } from "./analytics.moneygap";
import { type MoneyFields, measureReading } from "./analytics.questions.money";
import {
  type AnalyticsQuery,
  type AnswerRow,
  measureOfColumn,
} from "./analytics.questions.vocab";

type Translate = ReturnType<typeof useT>;

// A sum that adds only the deals that convert carries its mark and the gap in words.
// So does a blank sum, because no deal converts.
export function measureCell(
  query: AnalyticsQuery,
  column: string,
  row: AnswerRow,
  baseCurrency: string | null,
  money: MoneyFields,
  locale: Locale,
  t: Translate,
): ReactNode {
  if (beyondExactRange(row, column)) {
    return t("format.tooLargeExact");
  }
  const coverage = coverageOfMeasure(row, column);
  const figure = measureFigure(
    query,
    column,
    row,
    baseCurrency,
    money,
    locale,
    t,
  );
  if (!hasGap(coverage)) {
    return figure;
  }
  return (
    <span className="analytics-money-cell">
      {figure}
      <PartialMark coverage={coverage} />
      <CoverageFootnote coverage={coverage} />
    </span>
  );
}

function measureFigure(
  query: AnalyticsQuery,
  column: string,
  row: AnswerRow,
  baseCurrency: string | null,
  money: MoneyFields,
  locale: Locale,
  t: Translate,
): ReactNode {
  const measure = measureOfColumn(query, column);
  const value = row[column];
  if (!measure || typeof value !== "number") {
    return value == null ? MONEY_ABSENT : String(value);
  }
  const reading = measureReading(measure, row, query, baseCurrency, money);
  if (reading.kind === "mixed") {
    return t("analytics.q.mixedCurrencies");
  }
  if (reading.kind === "noBase") {
    return t("analytics.noBaseCurrency");
  }
  return (
    <span className="t-num">
      {reading.kind === "money"
        ? formatAnalyticsMoneyOrAbsent(value, reading.currency, locale)
        : formatAnalyticsNumber(value, locale)}
    </span>
  );
}
