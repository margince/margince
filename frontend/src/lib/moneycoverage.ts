// Why a deal's money is missing from a converted total: no amount was
// entered, or the amount's currency has no exchange rate. The server sums the
// deals that convert and counts these two apart. This module reads those
// counts for every screen that shows such a total.

import type { components } from "../api/schema";
import { stable } from "../format/collate";

type ReportAggregate = NonNullable<
  components["schemas"]["RunReportRequest"]["aggregates"]
>[number];

type Row = Record<string, unknown>;

export const COVERAGE_AGGREGATES: ReportAggregate[] = [
  { fn: "count", field: "no_value_deal", as: "no_value_deals" },
  { fn: "count", field: "no_rate_deal", as: "no_rate_deals" },
];

const RATE_CURRENCIES = "no_rate_currencies";
const CURRENCY = "currency";

export type MoneyCoverage = Readonly<{
  noValue: number;
  noRate: number;
  rateCurrencies: readonly string[];
}>;

function count(value: unknown): number | null {
  if (value == null || value === "") return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

// Null when the row carries no counts: that is "not answered", never "nothing
// is missing".
export function coverageOf(row: Row): MoneyCoverage | null {
  const noValue = count(row.no_value_deals);
  const noRate = count(row.no_rate_deals);
  if (noValue == null || noRate == null) return null;
  const listed = row[RATE_CURRENCIES];
  return {
    noValue,
    noRate,
    rateCurrencies:
      typeof listed === "string" && listed ? listed.split(",") : [],
  };
}

export function hasGap(
  coverage: MoneyCoverage | null | undefined,
): coverage is MoneyCoverage {
  return coverage != null && coverage.noValue + coverage.noRate > 0;
}

// The coverage of several groups taken together. Null as soon as one group
// did not answer, so a sum is never stated over counts that are partly unknown.
export function addCoverage(
  parts: readonly (MoneyCoverage | null)[],
): MoneyCoverage | null {
  const known = parts.filter((part): part is MoneyCoverage => part != null);
  if (known.length !== parts.length) return null;
  return {
    noValue: known.reduce((sum, part) => sum + part.noValue, 0),
    noRate: known.reduce((sum, part) => sum + part.noRate, 0),
    rateCurrencies: [
      ...new Set(known.flatMap((part) => part.rateCurrencies)),
    ].sort(stable),
  };
}

function groupKey(row: Row, groupBy: readonly string[]): string {
  return JSON.stringify(groupBy.map((field) => String(row[field] ?? "")));
}

// Names the currencies behind each group's no-rate count. It asks again,
// split by currency, only when some group has such a deal.
// `ask` runs the same report and filters, grouped as it is told, with the
// no-rate count as its one aggregate.
export async function withRateCurrencies<R extends Row>(
  rows: readonly R[],
  groupBy: readonly string[],
  ask: (groupBy: string[]) => Promise<readonly Row[]>,
): Promise<R[]> {
  if (!rows.some((row) => (count(row.no_rate_deals) ?? 0) > 0)) {
    return [...rows];
  }
  const split = await ask([...groupBy, CURRENCY]);
  const byGroup = new Map<string, string[]>();
  for (const row of split) {
    if ((count(row.no_rate_deals) ?? 0) === 0) continue;
    const currency = String(row[CURRENCY] ?? "");
    if (!currency) continue;
    const key = groupKey(row, groupBy);
    byGroup.set(key, [...(byGroup.get(key) ?? []), currency]);
  }
  return rows.map((row) => ({
    ...row,
    [RATE_CURRENCIES]: (byGroup.get(groupKey(row, groupBy)) ?? [])
      .sort(stable)
      .join(","),
  }));
}

export const NO_RATE_ONLY: ReportAggregate[] = COVERAGE_AGGREGATES.filter(
  (aggregate) => aggregate.as === "no_rate_deals",
);
