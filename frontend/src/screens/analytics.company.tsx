// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { DataTable } from "../design-system/datatable";
import {
  formatMoneyOrAbsent,
  formatNumber,
  MONEY_ABSENT,
} from "../format/format";
import { type Locale, useT } from "../i18n";
import {
  BarFigure,
  CountLink,
  FIELD_CURRENCY,
  type ReportRow,
  rowCount,
  rowCurrency,
  rowMoney,
} from "./analytics.cells";
import { CompanyCellExplain, rowDerivationUrl } from "./analytics.explain";
import { dealsFilteredBy } from "./dealsaddress";

/**
 * The group keys the report answered with exactly ONE currency row.
 *
 * Every money report groups by currency as well as by its own dimension, so a
 * company trading in two currencies is two rows with two counts. `/deals` reads
 * no `currency` dial — the parameter does not exist on the endpoint — so a link
 * beside such a row narrows to the company but not to the row and opens the
 * union of both: the figure promises one set and the door opens a larger one.
 *
 * So the door survives exactly where it is exact. A key with a single currency
 * row has no sibling to be confused with, and `company_id` alone addresses
 * precisely the deals that row counted; a key with two or more gets the plain
 * number, the answer a row whose company is "none" already gets.
 *
 * Exact only on a complete answer. On a capped one a key's second currency row
 * may lie past the cap and look single here, so a capped table opens no door.
 */
function singleCurrencyKeys(keys: readonly string[]): ReadonlySet<string> {
  const rowsPerKey = new Map<string, number>();
  for (const key of keys) {
    rowsPerKey.set(key, (rowsPerKey.get(key) ?? 0) + 1);
  }
  return new Set(
    [...rowsPerKey].filter(([, rows]) => rows === 1).map(([key]) => key),
  );
}

// The key a row groups under: its company, or "" for the deals that have none.
const companyKey = (row: ReportRow) =>
  typeof row.company_id === "string" ? row.company_id : "";

// The largest native sum in each currency, the scale that currency's bars
// share. A row with no currency has no unit to be drawn in and joins none.
function currencyScales(
  rows: readonly ReportRow[],
): ReadonlyMap<string, number> {
  const scales = new Map<string, number>();
  for (const row of rows) {
    const currency = rowCurrency(row);
    const rawMinor = rowMoney(row, "raw_minor");
    if (currency && rawMinor != null) {
      scales.set(currency, Math.max(scales.get(currency) ?? 0, rawMinor));
    }
  }
  return scales;
}

export function CompanyTable({
  rows,
  complete,
  locale,
}: Readonly<{ rows: ReportRow[]; complete: boolean; locale: Locale }>) {
  const t = useT();
  const single = singleCurrencyKeys(rows.map(companyKey));
  // One scale PER CURRENCY: these sums are native, so a euro bar measured
  // against a dong one would compare numbers with no common unit — the
  // unit-less comparison the table's own rows refuse to add up.
  const scaleOf = currencyScales(rows);
  return (
    <DataTable
      label={t("analytics.reportOpenByCompany")}
      columns={[
        {
          key: "company",
          header: t("analytics.company"),
          // The report answers with a company id and nothing else, so the
          // column read `01a0131c-3154-74cb-…` for every row — a company report
          // nobody could read. One record lookup per row, cached by id for a
          // minute and shared with every other reference on screen. The cost is
          // per row and this table is one report page long; the alternative is a
          // table of uuids, which is not a cheaper report but an unusable one.
          render: (row: ReportRow) => (
            <CompanyCellExplain
              url={rowDerivationUrl(row)}
              currency={rowCurrency(row)}
              companyId={companyKey(row) || null}
              split={!single.has(companyKey(row))}
            />
          ),
        },
        {
          key: FIELD_CURRENCY,
          header: t("analytics.currency"),
          render: (row: ReportRow) => rowCurrency(row) ?? MONEY_ABSENT,
        },
        {
          key: "count",
          header: t("analytics.openDeals"),
          align: "end",
          render: (row: ReportRow) =>
            complete &&
            typeof row.company_id === "string" &&
            single.has(row.company_id) ? (
              // `status`, because this report counts OPEN deals and the list
              // otherwise answers with every status the company ever had.
              <CountLink
                count={rowCount(row, "deal_count")}
                href={dealsFilteredBy("company_id", row.company_id, {
                  status: "open",
                })}
                title={t("analytics.openCompanyDeals")}
              />
            ) : (
              // The same figure as the link branch above, in the same notation:
              // whether a row can be opened is not a reason for its count to be
              // written differently.
              formatNumber(rowCount(row, "deal_count"), locale)
            ),
        },
        {
          key: "raw",
          header: t("analytics.unweighted"),
          align: "end",
          grow: true,
          render: (row: ReportRow) => {
            const currency = rowCurrency(row);
            const rawMinor = rowMoney(row, "raw_minor");
            return (
              <BarFigure
                value={currency ? rawMinor : null}
                scale={currency ? (scaleOf.get(currency) ?? 0) : 0}
                label={t("analytics.unweighted")}
              >
                {formatMoneyOrAbsent(rawMinor, currency, locale)}
              </BarFigure>
            );
          },
        },
      ]}
      rows={rows}
      // One company is a row per currency, so the key carries both.
      rowKey={(row) =>
        row.company_id != null
          ? `${String(row.company_id)}:${rowCurrency(row) ?? ""}`
          : String(rows.indexOf(row))
      }
    />
  );
}
