// What a converted sum leaves out. The report adds the deals that convert.
// The same row, from the same statement, says how many deals it left out.
// It says why too: no amount, or a currency with no rate. Every screen
// showing such a sum reads that statement here.

import { stable } from "../format/collate";

type Row = Record<string, unknown>;

export type MoneyCoverage = Readonly<{
  partial: boolean;
  noValue: number;
  noRate: number;
  rateCurrencies: readonly string[];
}>;

function count(value: unknown): number | null {
  if (value == null || value === "") return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

// Null when the row states no coverage: that is "not answered", never "nothing
// is missing". A client older than the server meets that on a row it did not
// ask the server to qualify.
export function coverageOf(row: Row): MoneyCoverage | null {
  return coverageOfWire(row.coverage);
}

// The same, for a coverage object met outside a report row.
export function coverageOfWire(stated: unknown): MoneyCoverage | null {
  if (stated == null || typeof stated !== "object") return null;
  const fields = stated as Row;
  const noValue = count(fields.no_value_deals);
  const noRate = count(fields.no_rate_deals);
  if (noValue == null || noRate == null) return null;
  const listed = fields.missing_currencies;
  return {
    partial: noValue + noRate > 0,
    noValue,
    noRate,
    rateCurrencies: Array.isArray(listed) ? listed.map(String) : [],
  };
}

// What a typed answer's row says about one measure column. Null unless the
// row names that column among the sums its coverage qualifies.
export function coverageOfMeasure(
  row: Row,
  column: string,
): MoneyCoverage | null {
  const stated = row._coverage;
  if (stated == null || typeof stated !== "object") return null;
  const named = (stated as Row).aggregates;
  if (!Array.isArray(named) || !named.includes(column)) return null;
  return coverageOfWire(stated);
}

export function hasGap(
  coverage: MoneyCoverage | null | undefined,
): coverage is MoneyCoverage {
  return coverage?.partial === true;
}

// The coverage of several groups taken together. Null as soon as one group
// did not answer, so a sum is never stated over counts that are partly unknown.
export function addCoverage(
  parts: readonly (MoneyCoverage | null)[],
): MoneyCoverage | null {
  const known = parts.filter((part): part is MoneyCoverage => part != null);
  if (known.length !== parts.length) return null;
  const noValue = known.reduce((sum, part) => sum + part.noValue, 0);
  const noRate = known.reduce((sum, part) => sum + part.noRate, 0);
  return {
    partial: noValue + noRate > 0,
    noValue,
    noRate,
    rateCurrencies: [
      ...new Set(known.flatMap((part) => part.rateCurrencies)),
    ].sort(stable),
  };
}
