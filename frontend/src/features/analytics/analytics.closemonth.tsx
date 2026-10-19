// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { DataTable } from "../../design-system/datatable";
import {
  formatMoneyOrAbsent,
  formatNumber,
  monthAndYear,
} from "../../format/format";
import { type Locale, useT } from "../../i18n";
import {
  BarFigure,
  columnScale,
  type ReportRow,
  rowCount,
  rowMoney,
} from "./analytics.cells";
import { CellExplain, rowDerivationUrl } from "./analytics.explain";

const closeMonth = (row: ReportRow) =>
  typeof row.close_month === "string" ? row.close_month : null;

// Months in calendar order, which the YYYY-MM key sorts as text, and the deals
// with no close date last: they are not in any month.
function byMonth(rows: readonly ReportRow[]): ReportRow[] {
  return [...rows].sort((a, b) => {
    const left = closeMonth(a);
    const right = closeMonth(b);
    if (left === right) return 0;
    if (left === null) return 1;
    if (right === null) return -1;
    return left < right ? -1 : 1;
  });
}

// The month a `YYYY-MM` key names. Built as a local date, because a close date
// has no zone and must not move to a neighbouring month.
function monthLabel(key: string, locale: Locale): string {
  const [year, month] = key.split("-").map(Number);
  return year && month
    ? monthAndYear(new Date(year, month - 1, 1), locale)
    : key;
}

// Open pipeline by the month each deal is expected to close, in the base
// currency every row was converted to.
export function CloseMonthTable({
  rows,
  locale,
  baseCurrency,
}: Readonly<{
  rows: ReportRow[];
  locale: Locale;
  baseCurrency: string | null;
}>) {
  const t = useT();
  const ordered = byMonth(rows);
  const scale = columnScale(ordered.map((row) => rowMoney(row, "raw_minor")));
  return (
    <DataTable
      label={t("analytics.reportPipelineByCloseMonth")}
      columns={[
        {
          key: "month",
          header: t("analytics.closeMonth"),
          render: (row: ReportRow) => {
            const key = closeMonth(row);
            const label =
              key === null
                ? t("analytics.noCloseDate")
                : monthLabel(key, locale);
            return (
              <CellExplain url={rowDerivationUrl(row)} figure={label}>
                {label}
              </CellExplain>
            );
          },
        },
        {
          key: "count",
          header: t("analytics.count"),
          align: "end",
          render: (row: ReportRow) =>
            formatNumber(rowCount(row, "deal_count"), locale),
        },
        {
          key: "raw",
          header: t("analytics.unweighted"),
          align: "end",
          grow: true,
          render: (row: ReportRow) => (
            <BarFigure
              value={rowMoney(row, "raw_minor")}
              part={rowMoney(row, "weighted_minor")}
              scale={scale}
              label={t("analytics.unweighted")}
            >
              {formatMoneyOrAbsent(
                rowMoney(row, "raw_minor"),
                baseCurrency,
                locale,
              )}
            </BarFigure>
          ),
        },
        {
          key: "weighted",
          header: t("analytics.weighted"),
          align: "end",
          render: (row: ReportRow) =>
            formatMoneyOrAbsent(
              rowMoney(row, "weighted_minor"),
              baseCurrency,
              locale,
            ),
        },
      ]}
      rows={ordered}
      rowKey={(row) => closeMonth(row) ?? "none"}
    />
  );
}
