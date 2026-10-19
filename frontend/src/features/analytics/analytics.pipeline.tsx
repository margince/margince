import { StatCard } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { Popover } from "../../design-system/popover";
import { StatStrip } from "../../design-system/statstrip";
import {
  formatMoneyCompact,
  formatMoneyOrAbsent,
  formatNumber,
} from "../../format/format";
import { type Locale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { dealsFilteredBy } from "../deals/dealsaddress";
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
import { FORECAST_CATEGORIES } from "./analytics.questions.values";

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
  // How many of `count` the two sums above cover. A deal in a currency the
  // rate sheet cannot price is counted but priced nowhere in the money.
  // Null, not zero, when the server left the aggregate out of the row: that
  // is "we do not know". Folding it into 0 would print "0 of 2 priced" for a
  // stage nobody finished pricing.
  pricedDeals: number | null;
  derivationUrl: string | null;
};

// The group key a row with no forecast category arrives under. The wire allows
// the field to be null: nobody has said which way the deal is going. The five
// named categories match none of it.
const UNCATEGORISED = "";

// The footnote for a money figure a currency the rate sheet cannot price left
// short. Null when every counted deal was priced. The TABLE's spelling; a stat
// card states the same gap through `analytics.forecastPriced`, as a fragment.
function pricedFootnote(
  pricedDeals: number,
  total: number,
  locale: Locale,
  t: ReturnType<typeof useT>,
): string | null {
  if (pricedDeals >= total) {
    return null;
  }
  return t("analytics.priced", {
    priced: formatNumber(pricedDeals, locale),
    total: formatNumber(total, locale),
  });
}

// One forecast category as one slot of the strip. The raw total is the reading
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
  pricedDeals,
  currency,
  locale,
  explainUrl,
}: Readonly<{
  label: string;
  // Both halves are nullable and neither absence has a substitute. A category
  // the report returned no row for has no total in any currency. It is not a
  // zero, because nothing was measured for it to be zero of. A figure with no
  // currency cannot be rendered as money at all.
  amountMinor: number | null;
  weightedMinor?: number | null;
  // How many deals the category holds, which is not how many the money above
  // covers. A deal in a currency the rate sheet cannot price is counted here
  // but adds nothing to the total. The sum skips it rather than guess a rate.
  // A tile printing only money would say a category is worth less than it is,
  // with nothing on screen to suggest otherwise.
  // Optional: a caller with no count to hand shows none rather than a zero.
  dealCount?: number | null;
  // How many of `dealCount` the money above actually covers (margince#4201).
  // Optional and paired with dealCount: a caller with no count has nothing
  // this could qualify either.
  pricedDeals?: number | null;
  currency: string | null;
  locale: Locale;
  // The category row's own handle; a tile with none draws no trigger.
  explainUrl?: string | null;
}>) {
  const t = useT();
  const plural = usePlural();
  // No handle, no slot: StatCard draws a source box for any node, even empty.
  const source = explainUrl ? (
    <CellExplain url={explainUrl} figure={label} />
  ) : undefined;
  // "1 deals" is a sentence no call site should be able to spell.
  const deals = (n: number) =>
    plural("analytics.forecastDeals", n, { count: formatNumber(n, locale) });
  if (amountMinor == null || !currency) {
    // A word, never a glyph. With no money, the deals the category holds are
    // the reading; with no count either, nothing was measured at all.
    return (
      <StatCard
        narrow="row"
        label={label}
        source={source}
        value={
          dealCount == null ? t("analytics.forecastNoFigure") : deals(dealCount)
        }
        detail={dealCount == null ? undefined : t("analytics.forecastNoAmount")}
      />
    );
  }
  // The line under the figure: what it was weighted to, and how many deals it
  // covers. Fragments, never "Label: value" pairs: a caption is read as one
  // sentence about the figure, and a colon makes it a small table.
  const parts: string[] = [];
  if (weightedMinor != null) {
    parts.push(
      t("analytics.forecastWeighted", {
        amount: formatMoneyCompact(weightedMinor, currency, locale),
      }),
    );
  }
  if (dealCount != null) {
    // The gap is stated only where there is one: "8 of 8 priced" sends a reader
    // looking for a shortfall the category does not have.
    parts.push(
      pricedDeals != null && pricedDeals < dealCount
        ? t("analytics.forecastPriced", {
            priced: formatNumber(pricedDeals, locale),
            count: formatNumber(dealCount, locale),
          })
        : deals(dealCount),
    );
  }
  return (
    <StatCard
      narrow="row"
      label={label}
      source={source}
      value={formatMoneyCompact(amountMinor, currency, locale)}
      detail={parts.join(" · ") || undefined}
    />
  );
}

// The wire allows a deal to carry no forecast category, and the five named ones
// match none of it. A slot set built from the enum alone drops those deals off
// the screen: the money is not moved to another slot, it leaves. On the demo
// dataset that is 22 of 27 open deals. The slot appears only where such deals
// exist, so an installation that categorises everything is not asked about a
// state it never reaches.
function uncategorisedSlot(
  band: readonly ReportRow[],
): { key: string; labelKey: MessageKey }[] {
  return band.some((row) => row.forecast_category == null)
    ? [{ key: UNCATEGORISED, labelKey: "deal.fcUncategorised" }]
    : [];
}

export function ForecastStrip({
  rows,
  baseCurrency,
  locale,
}: Readonly<{
  rows: ReportRow[];
  baseCurrency: string | null;
  locale: Locale;
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
          had a manager comparing commit against best case inside each currency
          and never across the business. Slots rather than a bar list, because
          each category carries two figures and a ranked bar carries one. */}
      <StatStrip>
        {[...FORECAST_CATEGORIES, ...uncategorisedSlot(rows)].map(
          (category) => {
            const row = rows.find(
              (candidate) =>
                String(candidate.forecast_category ?? UNCATEGORISED) ===
                category.key,
            );
            return (
              <ForecastTile
                key={category.key}
                label={t(category.labelKey)}
                amountMinor={row ? rowMoney(row, "raw_minor") : null}
                weightedMinor={row ? rowMoney(row, "weighted_minor") : null}
                dealCount={row ? rowCount(row, "deal_count") : null}
                // Not rowCount: an absent aggregate here means the server
                // did not answer. rowCount's 0 default would print that as
                // "0 priced" instead of leaving the detail out.
                pricedDeals={row ? rowMoney(row, "priced_deals") : null}
                explainUrl={row ? rowDerivationUrl(row) : null}
                currency={baseCurrency}
                locale={locale}
              />
            );
          },
        )}
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
          // Not rowCount: an absent aggregate here means the server did not
          // answer the question, and rowCount's 0 default would print that
          // as an answer.
          pricedDeals: rowMoney(row, "priced_deals"),
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
}: Readonly<{
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
                href={dealsFilteredBy("stage_id", row.stageId, {
                  status: "open",
                })}
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
              const footnote =
                row.pricedDeals == null
                  ? null
                  : pricedFootnote(row.pricedDeals, row.count, locale, t);
              return (
                <BarFigure
                  value={row.rawMinor}
                  part={row.weightedMinor}
                  scale={scale}
                  label={row.stageName}
                >
                  <span className="analytics-money-cell">
                    {formatMoneyOrAbsent(row.rawMinor, baseCurrency, locale)}
                    {footnote && <span className="t-caption">{footnote}</span>}
                  </span>
                </BarFigure>
              );
            },
          },
          {
            key: "weighted",
            header: t("analytics.weighted"),
            align: "end",
            render: (row: StageAgg) =>
              formatMoneyOrAbsent(row.weightedMinor, baseCurrency, locale),
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
  // Stated only when every stage answered it. A stage that did not would
  // count its deals into the total and none into the priced: a shortfall the
  // pipeline does not have.
  const pricedDeals = aggregates.every((row) => row.pricedDeals != null)
    ? sumKnown(aggregates.map((row) => row.pricedDeals))
    : null;
  // A stage the rate sheet could not price drops out of the sums. With the
  // priced count in hand, the footnote says so. Without it the shortfall has
  // no words, so the totals are absent rather than short with no note.
  const unstated =
    pricedDeals == null &&
    aggregates.some((row) => row.count > 0 && row.rawMinor == null);
  const rawMinor = unstated
    ? null
    : sumKnown(aggregates.map((row) => row.rawMinor));
  const weightedMinor = unstated
    ? null
    : sumKnown(aggregates.map((row) => row.weightedMinor));
  const footnote =
    pricedDeals == null
      ? undefined
      : (pricedFootnote(pricedDeals, count, locale, t) ?? undefined);
  return (
    <StatStrip>
      <StatCard
        narrow="row"
        label={t("analytics.openDeals")}
        value={formatNumber(count, locale)}
      />
      <StatCard
        narrow="row"
        label={t("analytics.unweighted")}
        value={formatMoneyOrAbsent(rawMinor, baseCurrency, locale)}
        detail={footnote}
      />
      <StatCard
        narrow="row"
        label={t("analytics.weighted")}
        value={formatMoneyOrAbsent(weightedMinor, baseCurrency, locale)}
        detail={footnote}
      />
    </StatStrip>
  );
}
