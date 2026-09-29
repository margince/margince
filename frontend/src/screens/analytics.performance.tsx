// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { EmptyState } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { formatMoneyOrAbsent, formatNumber } from "../format/format";
import { type Locale, useT } from "../i18n";
import {
  BarFigure,
  CountLink,
  columnScale,
  type ReportRow,
  rowCount,
  rowMoney,
  type Stage,
} from "./analytics.cells";
import { CellExplain, rowDerivationUrl } from "./analytics.explain";
import { dealsFilteredBy } from "./dealsaddress";

// The performance section's two tables: closed outcomes, and how long open
// deals have sat in their stage. Every duration is the server's own.

// A duration cell: the server's median or p75, or the withheld state the
// engine answers below its sample floor. A dash would read as zero-ish; the
// words say why there is no number.
function DaysCell({
  value,
  locale,
}: Readonly<{ value: unknown; locale: Locale }>) {
  const t = useT();
  if (value == null) {
    return <span>{t("analytics.tooFewForMedian")}</span>;
  }
  return (
    <>{t("analytics.days", { days: formatNumber(Number(value), locale) })}</>
  );
}

// A duration the engine answered, as a number to draw, or null where it
// withheld one below its sample floor — which draws no bar, the words in the
// cell saying why.
function daysOrNull(value: unknown): number | null {
  return value == null ? null : Number(value);
}

type OutcomeRow = {
  status: string;
  count: number;
  baseMinor: number | null;
  medianDays: unknown;
  p75Days: unknown;
  derivationUrl: string | null;
};

// Won and lost, side by side: counts, converted value, and how long the
// closed deals took. No rate is computed here — a win rate is the server's to
// answer the day it owns a denominator, and a browser-made quotient would be
// a second answer to what the cohort is.
export function WinLossTable({
  rows,
  locale,
  baseCurrency,
}: Readonly<{
  rows: ReportRow[];
  locale: Locale;
  baseCurrency: string | null;
}>) {
  const t = useT();
  const outcomes: OutcomeRow[] = rows
    .filter((row) => typeof row.status === "string")
    .map((row) => ({
      status: String(row.status),
      count: rowCount(row, "deal_count"),
      baseMinor: rowMoney(row, "raw_minor"),
      medianDays: row.median_days,
      p75Days: row.p75_days,
      derivationUrl: rowDerivationUrl(row),
    }));
  if (outcomes.length === 0) {
    // Nothing closed yet is a real answer, not a broken table: the population
    // is closed deals, and a young installation has none.
    return <EmptyState>{t("analytics.noClosedDeals")}</EmptyState>;
  }
  const scale = columnScale(outcomes.map((row) => daysOrNull(row.p75Days)));
  return (
    <DataTable
      label={t("analytics.reportWinLoss")}
      columns={[
        {
          key: "outcome",
          header: t("analytics.outcome"),
          render: (row: OutcomeRow) => {
            const outcome =
              row.status === "won" ? t("analytics.won") : t("analytics.lost");
            return (
              <CellExplain url={row.derivationUrl} figure={outcome}>
                {outcome}
              </CellExplain>
            );
          },
        },
        {
          key: "count",
          header: t("analytics.closedDeals"),
          align: "end",
          render: (row: OutcomeRow) => (
            <CountLink
              count={row.count}
              href={dealsFilteredBy("status", row.status)}
              title={t("analytics.openOutcomeDeals", {
                outcome:
                  row.status === "won"
                    ? t("analytics.won")
                    : t("analytics.lost"),
              })}
            />
          ),
        },
        {
          key: "value",
          header: t("analytics.baseValue", { currency: baseCurrency ?? "" }),
          align: "end",
          render: (row: OutcomeRow) =>
            formatMoneyOrAbsent(row.baseMinor, baseCurrency, locale),
        },
        {
          key: "median",
          header: t("analytics.medianDaysToClose"),
          align: "end",
          render: (row: OutcomeRow) => (
            <DaysCell value={row.medianDays} locale={locale} />
          ),
        },
        {
          key: "p75",
          header: t("analytics.p75DaysToClose"),
          align: "end",
          grow: true,
          // The slower quarter's bound as the bar, the median solid inside
          // it: how long a typical deal takes and how long the tail runs, in
          // one shape.
          render: (row: OutcomeRow) => (
            <BarFigure
              value={daysOrNull(row.p75Days)}
              part={daysOrNull(row.medianDays)}
              scale={scale}
              label={t("analytics.p75DaysToClose")}
            >
              <DaysCell value={row.p75Days} locale={locale} />
            </BarFigure>
          ),
        },
      ]}
      rows={outcomes}
      rowKey={(row) => row.status}
    />
  );
}

type StageAgeRow = {
  stageId: string;
  stageName: string;
  stagePosition: number;
  count: number;
  medianDays: unknown;
  p75Days: unknown;
  derivationUrl: string | null;
};

// How long open deals have sat in each stage, from the stage history's own
// entry instants. The median and p75 arrive computed; below the sample floor
// they arrive withheld, and the count beside the blank still says how many.
export function StageAgeTable({
  rows,
  stages,
  locale,
}: Readonly<{
  rows: ReportRow[];
  stages: readonly Stage[];
  locale: Locale;
}>) {
  const t = useT();
  const byId = new Map(stages.map((stage) => [stage.id, stage]));
  const aged: StageAgeRow[] = rows
    .filter((row) => typeof row.stage_id === "string")
    .map((row) => {
      const stage = byId.get(String(row.stage_id));
      return {
        stageId: String(row.stage_id),
        stageName: stage?.name ?? t("analytics.unknownStage"),
        stagePosition: stage?.position ?? Number.MAX_SAFE_INTEGER,
        count: rowCount(row, "deal_count"),
        medianDays: row.median_days,
        p75Days: row.p75_days,
        derivationUrl: rowDerivationUrl(row),
      };
    })
    .sort((a, b) => a.stagePosition - b.stagePosition);
  const scale = columnScale(aged.map((row) => daysOrNull(row.p75Days)));
  return (
    <DataTable
      label={t("analytics.reportStageAge")}
      columns={[
        {
          key: "stage",
          header: t("deals.stage"),
          render: (row: StageAgeRow) => (
            <CellExplain url={row.derivationUrl} figure={row.stageName}>
              {row.stageName}
            </CellExplain>
          ),
        },
        {
          key: "count",
          header: t("analytics.count"),
          align: "end",
          render: (row: StageAgeRow) => (
            <CountLink
              count={row.count}
              href={dealsFilteredBy("stage_id", row.stageId, {
                status: "open",
              })}
              title={t("analytics.openStageDeals", { stage: row.stageName })}
            />
          ),
        },
        {
          key: "median",
          header: t("analytics.medianDaysInStage"),
          align: "end",
          render: (row: StageAgeRow) => (
            <DaysCell value={row.medianDays} locale={locale} />
          ),
        },
        {
          key: "p75",
          header: t("analytics.p75DaysInStage"),
          align: "end",
          grow: true,
          render: (row: StageAgeRow) => (
            <BarFigure
              value={daysOrNull(row.p75Days)}
              part={daysOrNull(row.medianDays)}
              scale={scale}
              label={t("analytics.p75DaysInStage")}
            >
              <DaysCell value={row.p75Days} locale={locale} />
            </BarFigure>
          ),
        },
      ]}
      rows={aged}
      rowKey={(row) => row.stageId}
    />
  );
}
