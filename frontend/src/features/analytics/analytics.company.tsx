// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { DataTable } from "../../design-system/datatable";
import {
  formatMoneyOrAbsent,
  formatNumber,
  MONEY_ABSENT,
} from "../../format/format";
import { type Locale, useT } from "../../i18n";
import { dealsFilteredBy } from "../../screens/dealsaddress";
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

// `/deals` has no currency filter, so only a company with one currency row has
// a link that opens the deals its row counted.
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
  // One scale per currency: the sums are native, and a euro bar against a
  // dong one compares numbers with no common unit.
  const scaleOf = currencyScales(rows);
  return (
    <DataTable
      label={t("analytics.reportOpenByCompany")}
      columns={[
        {
          key: "company",
          header: t("analytics.company"),
          // The report answers with a company id only, so each row looks up
          // its name. The lookup is cached by id and shared across the screen.
          render: (row: ReportRow) => (
            <CompanyCellExplain
              url={rowDerivationUrl(row)}
              currency={rowCurrency(row)}
              companyId={companyKey(row) || null}
              split={!complete || !single.has(companyKey(row))}
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
            // A capped answer may hide a company's second row, so it links none.
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
              // The same notation as the link branch, whether or not it links.
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
