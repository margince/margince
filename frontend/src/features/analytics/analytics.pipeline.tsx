import type { ReactNode } from "react";
import { StatCard } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { Popover } from "../../design-system/popover";
import { StatStrip } from "../../design-system/statstrip";
import {
  formatAnalyticsMoney,
  formatAnalyticsMoneyOrAbsent,
  formatAnalyticsNumber,
} from "../../format/format";
import { type Locale, usePlural, useT } from "../../i18n";
import { WIN_PROBABILITY_DIAL } from "../../lib/dealranges";
import { dealsFilteredBy, type scopeDealDials } from "../../lib/dealsaddress";
import {
  addCoverage,
  coverageOf,
  hasGap,
  type MoneyCoverage,
} from "../../lib/moneycoverage";
import {
  bandDialValue,
  bandOfKey,
  WIN_PROBABILITY_BANDS,
} from "../../lib/winprobabilitybands";
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
import {
  CoverageFootnote,
  CoverageNote,
  PartialMark,
  withPartial,
} from "./analytics.moneygap";

// One row of the deals-by-stage table: a stage and a currency. A stage holding
// deals in two currencies has two totals and no third one that means anything.
// The money measures are nullable for the same reason the currency is: a SUM
// over deals nobody priced is absent, not zero.
type StageAgg = {
  stageId: string;
  stageName: string;
  stagePosition: number;
  count: number;
  rawMinor: number | null;
  weightedMinor: number | null;
  // The deals the two sums above leave out, by reason. Null when the server
  // left the counts out of the row: that is "we do not know", not "none".
  coverage: MoneyCoverage | null;
  derivationUrl: string | null;
};

// What a band with deals and no money says: which deals the money cannot
// cover, or that none carries an amount.
function noMoneyDetail(
  coverage: MoneyCoverage | null | undefined,
  t: ReturnType<typeof useT>,
): ReactNode {
  return hasGap(coverage) ? (
    <CoverageNote coverage={coverage} />
  ) : (
    t("analytics.forecastNoAmount")
  );
}

// One probability band as one slot of the strip. The raw total is the reading
// and the probability-weighted total is the basis it was drawn from, which is
// what StatCard's label/value/detail carry. Exported for the Storybook
// task so it renders without a live fetch (mirrors how FxLine in deals.tsx
// typed its `locale`). `weightedMinor` is optional so the slot still renders
// (raw only) for a caller with no weighted figure to hand.
export function ForecastTile({
  label,
  amountMinor,
  weightedMinor,
  dealCount,
  coverage,
  currency,
  locale,
  explainUrl,
  dealsUrl,
}: Readonly<{
  label: string;
  // Both halves are nullable and neither absence has a substitute. A band
  // the report returned no row for has no total in any currency. It is not a
  // zero, because nothing was measured for it to be zero of. A figure with no
  // currency cannot be rendered as money at all.
  amountMinor: number | null;
  weightedMinor?: number | null;
  // How many deals the band holds, which is not how many the money above
  // covers. A deal in a currency the rate sheet cannot price is counted here
  // but adds nothing to the total. The sum skips it rather than guess a rate.
  // A tile printing only money would say a band is worth less than it is,
  // with nothing on screen to suggest otherwise.
  // Optional: a caller with no count to hand shows none rather than a zero.
  dealCount?: number | null;
  // The deals of `dealCount` the money above leaves out, and why.
  coverage?: MoneyCoverage | null;
  currency: string | null;
  locale: Locale;
  // The band row's own handle; a tile with none draws no trigger.
  explainUrl?: string | null;
  // Where the band's deals are. A tile with none draws no door: a band this
  // build has no range for names a set the deals list cannot select.
  dealsUrl?: string;
}>) {
  const t = useT();
  const plural = usePlural();
  // No handle, no slot: StatCard draws a source box for any node, even empty.
  const source = explainUrl ? (
    <CellExplain url={explainUrl} figure={label} />
  ) : undefined;
  // "1 deals" is a sentence no call site should be able to spell.
  const deals = (n: number) =>
    plural("analytics.forecastDeals", n, {
      count: formatAnalyticsNumber(n, locale),
    });
  if (amountMinor == null || !currency) {
    // A word, never a glyph. With no money, the deals the band holds are
    // the reading; with no count either, nothing was measured at all.
    return (
      <StatCard
        narrow="row"
        label={label}
        source={source}
        href={dealsUrl}
        value={
          dealCount == null ? t("analytics.forecastNoFigure") : deals(dealCount)
        }
        detail={dealCount == null ? undefined : noMoneyDetail(coverage, t)}
      />
    );
  }
  // The line under the figure: what it was weighted to, how many deals it
  // covers, and the deals it leaves out. Fragments, never "Label: value"
  // pairs: a caption is read as one sentence about the figure.
  const parts: ReactNode[] = [];
  if (weightedMinor != null) {
    const weighted = t("analytics.forecastWeighted", {
      amount: formatAnalyticsMoney(weightedMinor, currency, locale),
    });
    parts.push(withPartial(weighted, coverage, t));
  }
  if (dealCount != null) {
    parts.push(deals(dealCount));
  }
  if (hasGap(coverage)) {
    parts.push(<CoverageNote key="coverage" coverage={coverage} />);
  }
  return (
    <StatCard
      narrow="row"
      label={label}
      source={source}
      href={dealsUrl}
      value={withPartial(
        formatAnalyticsMoney(amountMinor, currency, locale),
        coverage,
        t,
      )}
      detail={
        parts.length > 0 ? (
          parts.flatMap((part, index) =>
              index === 0 ? [part] : [" · ", part],
            )
        ) : undefined
      }
    />
  );
}

// The band of every row: the bands this build knows, in tile order, then any
// value the server added since, under its own name. A slot set built from the
// known bands alone would drop that value's money off the screen.
function bandSlots(rows: readonly ReportRow[]): string[] {
  const known = WIN_PROBABILITY_BANDS.map((band) => band.key);
  const added = rows
    .map((row) => String(row.win_probability_band ?? ""))
    .filter((key) => !known.includes(key));
  return [...known, ...new Set(added)];
}

function bandDealsUrl(key: string, scope: DealDials): string | undefined {
  const band = bandOfKey(key);
  return band && scope
    ? dealsFilteredBy(WIN_PROBABILITY_DIAL, bandDialValue(band), scope)
    : undefined;
}

// The deals-list dials the report's scope names. Null where the list cannot
// say it, because a link would then open more deals than the count held.
type DealDials = ReturnType<typeof scopeDealDials>;

export function ForecastStrip({
  rows,
  baseCurrency,
  locale,
  dealDials,
}: Readonly<{
  rows: ReportRow[];
  baseCurrency: string | null;
  locale: Locale;
  dealDials: DealDials;
}>) {
  const t = useT();
  return (
    <>
      {/* How to read every slot's second figure, as an aside: a notice box
          here was a pane inside a pane that outweighed the readings. */}
      <Popover
        onHover
        className="evmark-trigger t-caption analytics-strip-aside"
        label={t("analytics.weighted")}
      >
        <p>{t("analytics.forecastBanner")}</p>
      </Popover>
      {/* One strip, because there is one denomination. A strip per currency
          had a manager comparing bands inside each currency and never across
          the business. Slots rather than a bar list, because each band
          carries two figures and a ranked bar carries one. */}
      <StatStrip>
        {bandSlots(rows).map((key) => {
          const row = rows.find(
            (candidate) => String(candidate.win_probability_band ?? "") === key,
          );
          const band = bandOfKey(key);
          return (
            <ForecastTile
              key={key}
              dealsUrl={bandDealsUrl(key, dealDials)}
              label={band ? t(band.label) : key}
              amountMinor={row ? rowMoney(row, "raw_minor") : null}
              weightedMinor={row ? rowMoney(row, "weighted_minor") : null}
              dealCount={row ? rowCount(row, "deal_count") : null}
              coverage={row ? coverageOf(row) : null}
              explainUrl={row ? rowDerivationUrl(row) : null}
              currency={baseCurrency}
              locale={locale}
            />
          );
        })}
      </StatStrip>
    </>
  );
}

// The grouped rows as table rows, in pipeline order and then by currency code.
// The report answers in its own row order, which puts a stage's two currency rows
// anywhere relative to each other. A table a reader scans down has to follow
// the board.
export function buildStageAggregates(
  rows: readonly ReportRow[],
  stages: readonly Stage[],
): StageAgg[] {
  const byId = new Map(stages.map((stage) => [stage.id, stage]));
  return (
    rows
      .map((row) => {
        const stageId = String(row.stage_id ?? "");
        const stage = byId.get(stageId);
        return {
          stageId,
          stageName: stage?.name ?? stageId,
          // A stage the pipeline no longer carries sorts last rather than first.
          // Its rows are still real deals, but they are not part of the ladder
          // the reader is reading down.
          stagePosition: stage?.position ?? Number.MAX_SAFE_INTEGER,
          count: rowCount(row, "deal_count"),
          rawMinor: rowMoney(row, "raw_minor"),
          // AC-F1: the server's own per-deal-rounded weighted sum
          // (weighted_amount_minor), never round(rawMinor × p / 100).
          // That rounds the column sum once instead of every deal.
          weightedMinor: rowMoney(row, "weighted_minor"),
          coverage: coverageOf(row),
          derivationUrl: rowDerivationUrl(row),
        };
      })
      // Stage position alone orders the ladder now. The old tiebreak on currency
      // existed because one stage could be several rows; converted, it is one.
      .sort((left, right) => left.stagePosition - right.stagePosition)
  );
}

export function StageTable({
  rows,
  stages,
  locale,
  baseCurrency,
  dealDials,
}: Readonly<{
  dealDials: DealDials;
  rows: ReportRow[];
  stages: readonly Stage[];
  locale: Locale;
  // The currency every figure in this table is denominated in. One code for
  // the whole table rather than a column: the server converted each deal
  // before summing. A column would repeat the same code down the page and
  // imply it could differ per row.
  baseCurrency: string | null;
}>) {
  const t = useT();
  const aggregates = buildStageAggregates(rows, stages);
  const scale = columnScale(aggregates.map((row) => row.rawMinor));
  return (
    <>
      <PipelineTotals
        aggregates={aggregates}
        baseCurrency={baseCurrency}
        locale={locale}
      />
      <DataTable
        label={t("analytics.reportDeals")}
        columns={[
          {
            key: "stage",
            header: t("deals.stage"),
            render: (row: StageAgg) => (
              <CellExplain url={row.derivationUrl} figure={row.stageName}>
                {row.stageName}
              </CellExplain>
            ),
          },
          {
            key: "count",
            header: t("analytics.count"),
            align: "end",
            // Every row addresses its deals. Converted, one stage is one row
            // and one set; a stage split across two currency rows has no single
            // set to open. The link asks for open deals, which this report
            // counts. A won deal keeps the stage it closed in, so narrowing on
            // the stage would hand back a shorter list than the figure above
            // it.
            render: (row: StageAgg) => (
              <CountLink
                count={row.count}
                href={
                  dealDials
                    ? dealsFilteredBy("stage_id", row.stageId, {
                        ...dealDials,
                        status: "open",
                      })
                    : undefined
                }
                title={t("analytics.openStageDeals", { stage: row.stageName })}
              />
            ),
          },
          {
            key: "raw",
            header: t("analytics.unweighted"),
            align: "end",
            grow: true,
            // The bar is the stage's open value against the largest stage's,
            // with the weighted worth solid inside it. The two money columns
            // form one shape, so how much of each stage the probabilities
            // discount is read at a glance rather than by dividing.
            render: (row: StageAgg) => {
              return (
                <BarFigure
                  value={row.rawMinor}
                  part={row.weightedMinor}
                  scale={scale}
                  label={row.stageName}
                >
                  <span className="analytics-money-cell">
                    {formatAnalyticsMoneyOrAbsent(
                      row.rawMinor,
                      baseCurrency,
                      locale,
                    )}
                    <PartialMark coverage={row.coverage} />
                    <CoverageFootnote coverage={row.coverage} />
                  </span>
                </BarFigure>
              );
            },
          },
          {
            key: "weighted",
            header: t("analytics.weighted"),
            align: "end",
            render: (row: StageAgg) => (
              <>
                {formatAnalyticsMoneyOrAbsent(
                  row.weightedMinor,
                  baseCurrency,
                  locale,
                )}
                <PartialMark coverage={row.coverage} />
              </>
            ),
          },
        ]}
        rows={aggregates}
        rowKey={(row) => row.stageId}
      />
    </>
  );
}

// A column summed down the stages, or null when no stage answered it: absent
// is not zero.
function sumKnown(values: readonly (number | null)[]): number | null {
  const priced = values.filter((value): value is number => value != null);
  return priced.length === 0
    ? null
    : priced.reduce((total, value) => total + value, 0);
}

// The whole pipeline above its stages. Every stage row is already in the base
// currency, so the column sums are sums of one unit.
function PipelineTotals({
  aggregates,
  baseCurrency,
  locale,
}: Readonly<{
  aggregates: readonly StageAgg[];
  baseCurrency: string | null;
  locale: Locale;
}>) {
  const t = useT();
  const count = aggregates.reduce((total, row) => total + row.count, 0);
  // Every stage's sum already leaves out the deals it cannot price, so the
  // header adds the sums and the gaps of the same stages. A stage with deals
  // that states no coverage (an older server) would leave a shortfall unsaid,
  // so then the totals are withheld.
  const coverage = addCoverage(aggregates.map((row) => row.coverage));
  const unstated = aggregates.some(
    (row) => row.count > 0 && row.coverage == null,
  );
  const rawMinor = unstated
    ? null
    : sumKnown(aggregates.map((row) => row.rawMinor));
  const weightedMinor = unstated
    ? null
    : sumKnown(aggregates.map((row) => row.weightedMinor));
  const footnote = <CoverageFootnote coverage={coverage} />;
  return (
    <StatStrip density="plate">
      <StatCard
        narrow="row"
        label={t("analytics.openDeals")}
        value={formatAnalyticsNumber(count, locale)}
      />
      <StatCard
        narrow="row"
        label={t("analytics.unweighted")}
        value={withPartial(
          formatAnalyticsMoneyOrAbsent(rawMinor, baseCurrency, locale),
          coverage,
          t,
        )}
        detail={footnote}
      />
      <StatCard
        narrow="row"
        label={t("analytics.weighted")}
        value={withPartial(
          formatAnalyticsMoneyOrAbsent(weightedMinor, baseCurrency, locale),
          coverage,
          t,
        )}
        detail={footnote}
      />
    </StatStrip>
  );
}
