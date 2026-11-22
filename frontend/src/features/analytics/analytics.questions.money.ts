// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which of a question's measures are money, and in which currency each figure
// is stated. Read only from the schema's money marks, never from a field name.

import type { components } from "../../api/schema";
import type {
  AnalyticsEntity,
  AnalyticsQuery,
  AnswerRow,
} from "./analytics.questions.vocab";

type AnalyticsMeasure = components["schemas"]["AnalyticsMeasure"];

/**
 * Each measure of a population that is an amount, and the currency it is in,
 * as the schema serves it. A field not named here is a plain number, whatever
 * its name.
 */
export type MoneyFields = AnalyticsEntity["money"];

export const NO_MONEY: MoneyFields = {};

export function moneyFieldsOf(
  entities: readonly AnalyticsEntity[],
  entity: string,
): MoneyFields {
  return entities.find((e) => e.name === entity)?.money ?? NO_MONEY;
}

export function isMoneyField(field: string, money: MoneyFields): boolean {
  return Object.hasOwn(money, field);
}

export function isNativeMoneyField(field: string, money: MoneyFields): boolean {
  return money[field] === "row_currency";
}

// A filter as the draft and the wire both spell it.
type FilterLike = Readonly<{ field: string; op: string; value?: unknown }>;

// The one currency a question's filters pin every record to, if any.
function pinnedCurrency(filters: readonly FilterLike[]): string | null {
  const pinned = filters.find((f) => f.field === "currency" && f.op === "eq");
  return typeof pinned?.value === "string" && pinned.value.trim() !== ""
    ? pinned.value.trim()
    : null;
}

/**
 * The currency an amount compared by a filter is in: the base currency for a
 * converted measure, the pinned currency for one kept in each deal's own.
 * Null when neither is known, undefined for a field that is not money. The
 * reader types major units and the engine compares minor ones, so no money
 * filter is sent without this.
 */
export function filterAmountCurrency(
  field: string,
  filters: readonly FilterLike[],
  baseCurrency: string | null,
  money: MoneyFields,
): string | null | undefined {
  if (!isMoneyField(field, money)) {
    return undefined;
  }
  return isNativeMoneyField(field, money)
    ? pinnedCurrency(filters)
    : baseCurrency || null;
}

/**
 * How one measure cell reads: a plain number, an amount in one currency, or
 * an amount over several currencies that no single figure can state.
 */
export type MeasureReading =
  | Readonly<{ kind: "number" }>
  | Readonly<{ kind: "money"; currency: string }>
  | Readonly<{ kind: "mixed" }>
  | Readonly<{ kind: "noBase" }>;

export function measureReading(
  measure: AnalyticsMeasure,
  row: AnswerRow,
  query: AnalyticsQuery,
  baseCurrency: string | null,
  money: MoneyFields,
): MeasureReading {
  const field = measure.field ?? "";
  if (measure.fn === "count_distinct" || !isMoneyField(field, money)) {
    return { kind: "number" };
  }
  const currency = moneyCurrency(field, row, query, baseCurrency, money);
  if (currency) {
    return { kind: "money", currency };
  }
  // A converted amount with no base currency to state it in is not a mix of
  // currencies, and grouping by currency would not give it one.
  return isNativeMoneyField(field, money)
    ? { kind: "mixed" }
    : { kind: "noBase" };
}

/**
 * The currency a money field's value on this row is in, null when no single
 * one applies, and undefined for a field that is not money at all. A native
 * amount is in the row's own currency, or the one a filter pinned; a
 * converted one is in the base currency.
 */
export function moneyCurrency(
  field: string,
  row: AnswerRow,
  query: AnalyticsQuery,
  baseCurrency: string | null,
  money: MoneyFields,
): string | null | undefined {
  if (!isMoneyField(field, money)) {
    return undefined;
  }
  if (!isNativeMoneyField(field, money)) {
    return baseCurrency || null;
  }
  return rowText(row, "currency") ?? pinnedCurrency(query.filters ?? []);
}

function rowText(row: AnswerRow, column: string): string | null {
  const value = row[column];
  return typeof value === "string" && value !== "" ? value : null;
}
