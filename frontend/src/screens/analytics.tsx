import { useQuery } from "@tanstack/react-query";
import { Info } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan } from "../app/capability";
import { useRoute } from "../app/router";
import { EmptyState, StatCard } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { IconAction } from "../design-system/iconaction";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { RecordTabs } from "../design-system/recordtabs";
import { StatStrip } from "../design-system/statstrip";
import {
  formatDateTime,
  formatMoneyCompact,
  formatMoneyOrAbsent,
  formatNumber,
  MONEY_ABSENT,
} from "../format/format";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  openAnalyticsSection,
  SECTIONS,
  type Section,
  sectionFromAddress,
} from "./analytics.address";
import {
  BarFigure,
  CountLink,
  columnScale,
  type ReportRow,
  rowCount,
  rowCurrency,
  rowMoney,
  type Stage,
} from "./analytics.cells";
import {
  type AnalyticsSelection,
  useAnalyticsContext,
  useAnalyticsSelection,
} from "./analytics.context";
import {
  ProjectCommitmentsTable,
  ProjectsByPhaseTable,
  ProjectsGoneQuietTable,
} from "./analytics.delivery";
import {
  CellExplain,
  CompanyCellExplain,
  ExplainFrame,
  ExplainPanel,
  rowDerivationUrl,
} from "./analytics.explain";
import { ForecastView, SharedForecastView } from "./analytics.forecast";
import { sourceName } from "./analytics.forecast.review";
import { MyOutcomesView } from "./analytics.outcomes";
import { StageAgeTable, WinLossTable } from "./analytics.performance";
import { QuestionsView } from "./analytics.questions";
import { FORECAST_CATEGORIES } from "./analytics.questions.values";
import { ENTITY_LABEL_KEY } from "./analytics.questions.vocab";
import { AnalyticsScopePicker } from "./analytics.scope";
import { ForecastShareActions } from "./analytics.share";
import { QueryGate, throwProblem, useMe } from "./common";
import { dealsFilteredBy } from "./dealsaddress";
import { ReportingDefinitions } from "./reporting.definitions";
import { ReportingForecastGraphs } from "./reporting.forecast";
import { ReportingLibrary } from "./reporting.library";
import { ReportingOverview } from "./reporting.overview";
import { ReportingTargets } from "./reporting.targets";
import "./analytics.css";

// Analytics: a picker over three reports — deals-by-stage (unweighted beside
// weighted), forecast (category readings plus the server-derived "slipped"
// bucket), and open deals per company. "Explain this number" opens the executed
// plan and the exact rows the headline reconciles to. Both weighted figures come
// straight off the report's own weighted_amount_minor measure, rounded PER DEAL
// and then summed, so neither screen re-derives one from the raw total. All
// three bodies render into ONE panel whose action band carries the explain
// toggle: the segment changes what the panel holds, not what the page is.

// One row of the deals-by-stage table: a stage AND a currency, because a stage
// holding deals in two currencies has two totals and no third one that means
// anything. The money measures are nullable for the same reason the currency is
// — a SUM over deals nobody priced is absent, not zero.
type StageAgg = {
  stageId: string;
  stageName: string;
  stagePosition: number;
  count: number;
  rawMinor: number | null;
  weightedMinor: number | null;
  // How many of `count` the two sums above cover — a deal in a currency the
  // rate sheet cannot price is counted but priced nowhere in the money.
  // Null, not zero, when the server left the aggregate out of the row: that
  // is "we do not know", and folding it into 0 would print "0 of 2 priced"
  // for a stage nobody actually finished pricing.
  pricedDeals: number | null;
  derivationUrl: string | null;
};

type ReportKey =
  | "pipeline-current"
  | "forecast"
  | "open-deals-per-company"
  | "win-loss"
  | "stage-age"
  | "projects-by-phase"
  | "project-commitments"
  | "projects-gone-quiet";

// Which results each section holds, in the order they are drawn. The tab strip
// and the bodies both read this, so a section cannot come to list a report it
// does not draw.
const SECTION_REPORTS = {
  reports: [],
  targets: [],
  definitions: [],
  // The forecast section draws its own view — readings, a call and a receipt,
  // none of which is a row set — so it lists no report card.
  forecast: [],
  // The forecast-category breakdown moves HERE rather than being deleted. It
  // is a real report and still the only place a reader sees how the pipeline
  // divides by category; what it is not is the forecast, which is now an
  // answer rather than a table.
  pipeline: ["pipeline-current", "forecast", "open-deals-per-company"],
  // Closed outcomes and stage velocity: what happened, and how long things
  // take. Both are the server's own report vocabulary — no rate or duration
  // is computed in this file.
  performance: ["win-loss", "stage-age"],
  // The rep's own week: a composed view like the forecast, not report cards.
  outcomes: [],
  // Source health: an ops view over the nightly check's own coverage rows.
  coverage: [],
  // What was sold becoming what is delivered: the three project reports.
  delivery: ["projects-by-phase", "project-commitments", "projects-gone-quiet"],
  questions: [],
} as const satisfies Record<Section, readonly ReportKey[]>;

// The report engine's own name for the currency dimension. Spelled once: it
// reaches the request, every row read and the column header, and a typo in any
// one of them is a cross-currency sum that looks right.
const FIELD_CURRENCY = "currency";

// The group key a row with NO forecast category arrives under. The wire allows
// the field to be null — nobody has said which way the deal is going — and the
// five named categories match none of it.
const UNCATEGORISED = "";

// A plan that sums NATIVE money groups by currency as well as by its own
// dimension: amount_minor is a minor-unit integer in the deal's own currency, so
// a total spanning currencies is a number with no unit.
//
// pipeline-current does not, because the server converted each deal before
// summing: one stage is one row in the installation's base currency, which is
// why that report exists beside deals-by-stage rather than replacing it.
const REPORT_GROUP_BY: Record<ReportKey, string[]> = {
  "pipeline-current": ["stage_id"],
  forecast: ["forecast_category"],
  "open-deals-per-company": ["company_id", FIELD_CURRENCY],
  "win-loss": ["status"],
  "stage-age": ["stage_id"],
  // The specs' own defaults: an empty plan takes each report's declared
  // grouping and aggregates, which for these three is the whole point.
  "projects-by-phase": [],
  "project-commitments": [],
  "projects-gone-quiet": [],
};

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

// The line under a report's title, for the reports whose copy says something
// the card's own title does not. A report absent from here gets no caption: an
// explanation beside a report it does not describe is worse than none.
const reportSub: Partial<Record<ReportKey, MessageKey>> = {
  "pipeline-current": "analytics.sub",
};

type ReportAggregate = NonNullable<
  components["schemas"]["RunReportRequest"]["aggregates"]
>[number];

// Which aggregates each report's own vocabulary serves (report.go's
// per-spec measures) — `weighted_amount_minor` only exists where a stage
// join computes it (deals-by-stage, forecast); requesting it against
// open-deals-per-company's narrower vocabulary would 422.
const REPORT_AGGREGATES: Record<ReportKey, ReportAggregate[]> = {
  "pipeline-current": [
    { fn: "sum", field: "amount_base_minor", as: "raw_minor" },
    { fn: "sum", field: "weighted_base_minor", as: "weighted_minor" },
    { fn: "count", as: "deal_count" },
    // How many of deal_count the sums above actually cover — a deal in a
    // currency the rate sheet cannot price is counted but contributes nothing
    // to either total, so a row printing only the money reads as complete when
    // it is short (margince#4201).
    { fn: "count", field: "amount_base_minor", as: "priced_deals" },
  ],
  forecast: [
    { fn: "sum", field: "amount_base_minor", as: "raw_minor" },
    { fn: "sum", field: "weighted_base_minor", as: "weighted_minor" },
    { fn: "count", as: "deal_count" },
    { fn: "count", field: "amount_base_minor", as: "priced_deals" },
  ],
  "open-deals-per-company": [
    { fn: "sum", field: "amount_minor", as: "raw_minor" },
    { fn: "count", as: "deal_count" },
  ],
  "win-loss": [
    { fn: "count", as: "deal_count" },
    { fn: "sum", field: "amount_base_minor", as: "raw_minor" },
    { fn: "median", field: "days_to_close", as: "median_days" },
    { fn: "p75", field: "days_to_close", as: "p75_days" },
  ],
  "stage-age": [
    { fn: "count", as: "deal_count" },
    { fn: "median", field: "days_in_stage", as: "median_days" },
    { fn: "p75", field: "days_in_stage", as: "p75_days" },
  ],
  "projects-by-phase": [],
  "project-commitments": [],
  "projects-gone-quiet": [],
};

// One forecast category as one slot of the strip: the raw total is the reading
// and the probability-weighted total is the basis it was drawn from, which is
// exactly what StatCard's label/value/detail carry. Exported for the Storybook
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
  // the report returned no row for has no total in any currency — not a zero,
  // because nothing was measured for it to be zero of — and a figure with no
  // currency cannot be rendered as money at all.
  amountMinor: number | null;
  weightedMinor?: number | null;
  // How many deals the category holds, which is NOT how many the money above
  // covers. A deal in a currency the rate sheet cannot price is counted here
  // and contributes nothing to the total — the sum skips it rather than
  // guessing a rate — so a tile printing only money would say a category is
  // worth less than it is, with nothing on screen to suggest otherwise.
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
    // A word, never a glyph: with no money the deals the category HOLDS are
    // the reading, and with no count either nothing was measured at all.
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
  // covers — FRAGMENTS, never "Label: value" pairs, because a caption is read
  // as one sentence about the figure and a colon makes it a small table.
  const parts: string[] = [];
  if (weightedMinor != null) {
    parts.push(
      t("analytics.forecastWeighted", {
        amount: formatMoneyCompact(weightedMinor, currency, locale),
      }),
    );
  }
  if (dealCount != null) {
    // The gap is stated only where there IS one: "8 of 8 priced" sends a reader
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
// match none of it — so a slot set built from the enum alone drops those deals
// off the screen entirely: the money is not moved to another slot, it leaves. On
// the demo dataset that was 22 of 27 open deals. The slot appears only where such
// deals exist, so an installation that categorises everything is not asked about
// a state it never reaches.
function uncategorisedSlot(
  band: readonly ReportRow[],
): { key: string; labelKey: MessageKey }[] {
  return band.some((row) => row.forecast_category == null)
    ? [{ key: UNCATEGORISED, labelKey: "deal.fcUncategorised" }]
    : [];
}

function ForecastStrip({
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
      {/* How to read the second figure in every slot, as the panel's one
          descriptive line: a notice box inside the panel was a pane inside a
          pane, and it outweighed the readings it was only explaining. */}
      <PanelIntro>{t("analytics.forecastBanner")}</PanelIntro>
      {/* ONE strip, because there is now one denomination. A plate of ruled
          slots claims its figures are ONE comparison, and a strip per currency
          — the only honest way to show native sums, since adding euros to dong
          is the unit-less total data-semantics §1 r4 forbids — had a manager
          comparing commit against best case inside each currency and never
          across the business, which is the question they actually have.

          The slots stay slots rather than becoming a bar list: every category
          carries TWO figures, the raw total and the probability-weighted one
          beneath it, and a ranked bar carries a single amount per row. */}
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
                // did not answer, and rowCount's 0 default would print
                // that as "0 priced" instead of leaving the detail out.
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
 * Sound because a report result is complete: `ReportResult` carries no cursor
 * and no truncation flag, so the rows in hand are all the rows there are. Were
 * it ever paged, a key whose second currency row fell off the page would look
 * single-currency here and get a door that lies.
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

function CompanyTable({
  rows,
  locale,
}: Readonly<{ rows: ReportRow[]; locale: Locale }>) {
  const t = useT();
  const addressable = singleCurrencyKeys(rows.map(companyKey));
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
              split={!addressable.has(companyKey(row))}
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
            typeof row.company_id === "string" &&
            addressable.has(row.company_id) ? (
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
      // A company with deals in two currencies is two rows now, so the
      // company id alone no longer identifies one.
      rowKey={(row) =>
        row.company_id != null
          ? `${String(row.company_id)}:${rowCurrency(row) ?? ""}`
          : String(rows.indexOf(row))
      }
    />
  );
}

// The grouped rows as table rows, in pipeline order and then by currency code.
// The report answers in its own row order, which puts a stage's two currency rows
// anywhere relative to each other — a table a reader scans down has to follow the
// board.
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
          // A stage the pipeline no longer carries sorts last rather than first:
          // its rows are still real deals, but they are not part of the ladder the
          // reader is reading down.
          stagePosition: stage?.position ?? Number.MAX_SAFE_INTEGER,
          count: rowCount(row, "deal_count"),
          rawMinor: rowMoney(row, "raw_minor"),
          // AC-F1: the server's own per-deal-rounded weighted sum
          // (weighted_amount_minor), never round(rawMinor × p / 100)
          // — that rounds the column sum once instead of every deal.
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

function StageTable({
  rows,
  stages,
  locale,
  baseCurrency,
}: Readonly<{
  rows: ReportRow[];
  stages: readonly Stage[];
  locale: Locale;
  // The currency every figure in this table is denominated in. One code for
  // the whole table rather than a column, because the server converted each
  // deal before summing — a column would repeat the same code down the page
  // and imply it could differ per row.
  baseCurrency: string | null;
}>) {
  const t = useT();
  const aggregates = buildStageAggregates(rows, stages);
  const scale = columnScale(aggregates.map((row) => row.rawMinor));
  return (
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
          // Every row addresses its deals: converted, one stage is one row and
          // one set again, where a stage split across two currency rows had no
          // single set to open. The link asks for OPEN deals, which is what
          // this report counts: a won deal keeps the stage it closed in, so
          // narrowing on the stage would hand back a shorter list than the
          // figure above it.
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
          // with the weighted worth solid inside it: the two money columns as
          // one shape, so how much of each stage the probabilities discount
          // is read at a glance rather than by dividing.
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
  );
}

// Every word a coverage row's state can carry, spelled per state so an
// unread source never borrows a read one's copy. A hand-kept mirror of the
// server's own vocabulary; an unknown state renders its raw word rather than
// a wrong sentence.
const COVERAGE_STATE_KEY: Readonly<Record<string, MessageKey>> = {
  checked: "analytics.covChecked",
  stale: "analytics.covStale",
  unavailable: "analytics.covUnavailable",
  permission_limited: "analytics.covPermissionLimited",
  not_connected: "analytics.covNotConnected",
};

// Which connectors the nightly check could read, and how far. Ops-facing: the
// server gates the read, and a seat without the grant never sees the tab.
function DataCoverageView({
  locale,
  timezone,
}: Readonly<{ locale: Locale; timezone: string }>) {
  const t = useT();
  const coverage = useDataCoverage();
  const allowed = useCan("data_coverage", "read");
  if (!allowed) {
    return <EmptyState>{t("common.permissionDenied")}</EmptyState>;
  }
  return (
    <QueryGate query={coverage} pendingLabel={t("analytics.sectionCoverage")}>
      {(run) =>
        run == null ? (
          <Panel title={t("analytics.sectionCoverage")}>
            <EmptyState>{t("analytics.coverageNeverRun")}</EmptyState>
          </Panel>
        ) : (
          <Panel title={t("analytics.sectionCoverage")}>
            <PanelBody>
              <PanelIntro>{t("analytics.coverageSub")}</PanelIntro>
              <DataTable
                label={t("analytics.sectionCoverage")}
                columns={[
                  {
                    key: "source",
                    header: t("analytics.covSource"),
                    render: (row: DataCoverageRow) => sourceName(row.source, t),
                  },
                  {
                    key: "state",
                    header: t("analytics.covState"),
                    render: (row: DataCoverageRow) =>
                      COVERAGE_STATE_KEY[row.state]
                        ? t(COVERAGE_STATE_KEY[row.state])
                        : row.state,
                  },
                  {
                    key: "through",
                    header: t("analytics.covThrough"),
                    render: (row: DataCoverageRow) =>
                      row.checked_through
                        ? formatDateTime(row.checked_through, locale, timezone)
                        : "—",
                  },
                ]}
                rows={run.sources}
                rowKey={(row) => row.source}
              />
              {/* Record-level input problems live where they are answered: the
                Forecast input review. One resolution surface, not two. */}
              <p className="t-sub">{t("analytics.coverageInputsElsewhere")}</p>
            </PanelBody>
          </Panel>
        )
      }
    </QueryGate>
  );
}

type DataCoverageRow = components["schemas"]["DataCoverage"]["sources"][number];

function useDataCoverage() {
  const allowed = useCan("data_coverage", "read");
  return useQuery({
    enabled: allowed,
    queryKey: ["analytics-coverage"],
    retry: false,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/analytics/coverage");
      if (response.status === 404) {
        // A fresh installation: no run has completed yet. Null, so the view
        // says that in words rather than drawing headers over blank space —
        // "nothing has looked yet" and "everything looked fine" are opposite
        // instructions about whether to trust the numbers elsewhere.
        return null;
      }
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// Which table a report's rows become — a switch in its own component so the
// card's render stays a frame plus a choice.
function ReportBody({
  report,
  run,
  stages,
  locale,
}: Readonly<{
  report: ReportKey;
  run: {
    rows: ReportRow[];
    base_currency?: string | null;
    timezone?: string | null;
  };
  stages: readonly Stage[];
  locale: Locale;
}>) {
  const base = run.base_currency ?? null;
  switch (report) {
    case "forecast":
      return (
        <ForecastStrip rows={run.rows} baseCurrency={base} locale={locale} />
      );
    case "open-deals-per-company":
      return <CompanyTable rows={run.rows} locale={locale} />;
    case "pipeline-current":
      return (
        <StageTable
          rows={run.rows}
          stages={stages}
          locale={locale}
          baseCurrency={base}
        />
      );
    case "win-loss":
      return (
        <WinLossTable rows={run.rows} locale={locale} baseCurrency={base} />
      );
    case "stage-age":
      return <StageAgeTable rows={run.rows} stages={stages} locale={locale} />;
    case "projects-by-phase":
      return (
        <ProjectsByPhaseTable
          rows={run.rows}
          locale={locale}
          baseCurrency={base}
        />
      );
    case "project-commitments":
      return <ProjectCommitmentsTable rows={run.rows} locale={locale} />;
    case "projects-gone-quiet":
      return (
        <ProjectsGoneQuietTable
          rows={run.rows}
          locale={locale}
          timezone={run.timezone ?? null}
        />
      );
    default:
      return null;
  }
}

// One report: its own query, its own explain disclosure, its own card. A
// section may hold several, and each has to be able to load, fail and be
// explained on its own — a single query per screen would have made a section
// with two results show one spinner for both and one error for either.
function ReportCard({
  report,
  stages,
  locale,
}: Readonly<{
  report: ReportKey;
  stages: readonly Stage[];
  locale: Locale;
}>) {
  const t = useT();
  const [explain, setExplain] = useState(false);
  const explainId = useId();

  const reportQuery = useQuery({
    queryKey: ["report", report],
    queryFn: async () => {
      const { data, error } = await api.POST("/reports/{report}", {
        params: { path: { report } },
        // An empty list and an absent one both take the report's own declared
        // defaults (report.go branches on len==0), so the delivery reports'
        // empty plans ask for exactly what their specs declare.
        body: {
          group_by: REPORT_GROUP_BY[report],
          aggregates: REPORT_AGGREGATES[report],
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  return (
    <QueryGate query={reportQuery} pendingLabel={t(ENTITY_LABEL_KEY[report])}>
      {(run) => (
        <ExplainFrame
          frame={{
            baseCurrency: run.base_currency ?? null,
            timezone: run.timezone ?? null,
          }}
        >
          <Panel
            title={t(ENTITY_LABEL_KEY[report])}
            // The verb that reveals the derivation under this panel, in the
            // head beside the title it explains: it announces its open state
            // and names what it controls, so a reader who cannot see the panel
            // appear is still told it did.
            titleAction={
              // The same glyph as a row's own explain, so the panel's verb and
              // a cell's read as one control at two sizes of subject — and a
              // glyph rather than words, because the band is one line shared
              // with the title and a phrase there cut the title to "O…".
              <IconAction
                label={t("explain.open")}
                icon={<Info aria-hidden />}
                disclosure={{ expanded: explain, controls: explainId }}
                onClick={() => setExplain((value) => !value)}
              />
            }
            // The frame every figure above was cut in: the instant and the zone
            // it is stated in, because a total with no zone is a number a
            // reader places by assumption. A fact about the WHOLE panel, so it
            // is the panel's footer rather than one more line of its body.
            //
            // It names no CURRENCY and must not: open-deals-per-company prints
            // native rows beside converted blocks, so one line can say nothing
            // true of them all — naming one read ₫367,620,000,000 as a euro
            // figure. Drawn only when the server sent both halves; half a frame
            // is worse than none.
            footer={
              run.as_of && run.timezone ? (
                <span className="t-caption">
                  {t("analytics.frame", {
                    asOf: formatDateTime(run.as_of, locale, run.timezone),
                    zone: run.timezone,
                  })}
                </span>
              ) : undefined
            }
          >
            <PanelBody>
              {reportSub[report] && (
                <PanelIntro>
                  {t(reportSub[report], { currency: run.base_currency ?? "" })}
                </PanelIntro>
              )}
              <ReportBody
                report={report}
                run={run}
                stages={stages}
                locale={locale}
              />
            </PanelBody>
          </Panel>
          {explain && (
            <ExplainPanel id={explainId} url={run.derivation_url ?? null} />
          )}
        </ExplainFrame>
      )}
    </QueryGate>
  );
}

export function AnalyticsScreen() {
  const t = useT();
  const { locale } = useLocale();
  // Which SECTION is open is an ADDRESS, so a reader can link to one and Back
  // steps between the sections they looked at rather than leaving the screen.
  // Read here rather than taken as a prop, so this screen stays drivable on its
  // own: a suite that renders it directly goes on pressing the tabs.
  const route = useRoute();
  const reportingEnabled =
    useMe().data?.settings_availability?.reporting === true;
  const requested = sectionFromAddress(
    route.screen === "analytics" ? route.id : undefined,
    reportingEnabled ? "performance" : "forecast",
  );
  const section =
    !reportingEnabled &&
    (requested === "reports" ||
      requested === "targets" ||
      requested === "definitions" ||
      (route.screen === "analytics" && !route.id))
      ? "forecast"
      : requested;
  // The server decides which population this reader measures and which ones
  // they may choose. Read once here and handed down, so every card on the page
  // is answering about the same set.
  const context = useAnalyticsContext();
  const { selection, selectScope } = useAnalyticsSelection(context.data);

  const pipelineQuery = useQuery({
    queryKey: ["pipelines"],
    queryFn: async () => {
      const { data, error } = await api.GET("/pipelines", {
        params: { query: {} },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data.find((pipeline) => pipeline.is_default) ?? data.data[0];
    },
  });

  // Sharing sits beside the tabs rather than inside a section, because the
  // thing being shared is the SECTION the reader is on — a button that moved
  // with the content would read as sharing one card.
  const canReadReports = useCan("report_definition", "read");
  const canReadTargets = useCan("sales_target", "read");
  const canReadFramework = useCan("reporting_framework", "read");
  const canReadCoverage = useCan("data_coverage", "read");
  const coverageProbe = useDataCoverage();
  const header = (
    <div className="analytics-header">
      <RecordTabs
        options={SECTIONS.filter((candidate) => {
          if (candidate === "reports")
            return reportingEnabled && canReadReports;
          if (candidate === "targets")
            return reportingEnabled && canReadTargets;
          if (candidate === "definitions")
            return reportingEnabled && canReadFramework;
          if (candidate === "outcomes") {
            return context.data?.default_scope.kind === "owner";
          }
          if (candidate === "coverage") {
            // The read starts only with the ops grant; the tab appears
            // after the server has answered.
            return canReadCoverage && coverageProbe.isSuccess;
          }
          return true;
        })}
        value={section}
        onChange={openAnalyticsSection}
        labels={{
          reports: t("reporting.reports"),
          targets: t("reporting.targets"),
          definitions: t("reporting.definitions"),
          forecast: t("analytics.sectionForecast"),
          pipeline: t("analytics.sectionPipeline"),
          performance: t("analytics.sectionPerformance"),
          outcomes: t("analytics.sectionOutcomes"),
          coverage: t("analytics.sectionCoverage"),
          delivery: t("analytics.sectionDelivery"),
          questions: t("analytics.sectionQuestions"),
        }}
        label={t("analytics.sections")}
      />
      <div className="analytics-header-end">
        {selection && context.data ? (
          <AnalyticsScopePicker
            scopes={context.data.allowed_scopes}
            selected={selection.scope}
            onSelect={selectScope}
          />
        ) : null}
        {section === "forecast" && selection && !reportingEnabled ? (
          <ForecastShareActions target="forecast" scope={selection.scope} />
        ) : null}
      </div>
    </div>
  );

  if (route.screen === "analytics" && route.id === "shared" && route.id2)
    return <SharedForecastView token={route.id2} />;

  return (
    <div className="wrap">
      {header}
      <div className="analytics-body">
        <SectionBody
          section={section}
          reportingEnabled={reportingEnabled}
          locale={locale}
          context={context.data}
          selection={selection}
          onSelectScope={selectScope}
          stages={pipelineQuery.data?.stages ?? []}
        />
      </div>
    </div>
  );
}

// One section's body, chosen in its own component so the screen's own render
// stays a header plus a choice rather than a ladder of ternaries.
function SectionBody({
  section,
  reportingEnabled,
  locale,
  context,
  selection,
  onSelectScope,
  stages,
}: Readonly<{
  section: Section;
  reportingEnabled: boolean;
  locale: Locale;
  context: components["schemas"]["AnalyticsContext"] | undefined;
  selection: AnalyticsSelection | null;
  onSelectScope: (scope: AnalyticsSelection["scope"]) => void;
  stages: readonly Stage[];
}>) {
  switch (section) {
    case "performance":
      return (
        <PerformanceSection
          enabled={reportingEnabled}
          selection={selection}
          stages={stages}
          locale={locale}
        />
      );
    case "reports":
      return <ReportingLibrary />;
    case "targets":
      return <ReportingTargets />;
    case "definitions":
      return <ReportingDefinitions />;
    case "questions":
      return selection && context ? (
        <QuestionsView
          context={context}
          selection={selection}
          onSelectScope={onSelectScope}
        />
      ) : null;
    case "coverage":
      // Nothing renders before the frame arrives, so no zone is guessed.
      return context ? (
        <DataCoverageView locale={locale} timezone={context.timezone} />
      ) : null;
    case "outcomes":
      return context ? (
        <MyOutcomesView defaultScope={context.default_scope} locale={locale} />
      ) : null;
    case "forecast":
      return selection && context ? (
        <>
          {reportingEnabled ? (
            <ReportingForecastGraphs
              key={JSON.stringify(selection.scope)}
              scope={selection.scope}
            />
          ) : null}
          <ForecastView
            selection={selection}
            canSubmit={context.capabilities.submit_manager_forecast}
          />
        </>
      ) : null;
    default:
      return (
        <>
          {SECTION_REPORTS[section].map((report) => (
            <ReportCard
              key={report}
              report={report}
              stages={stages}
              locale={locale}
            />
          ))}
        </>
      );
  }
}

function PerformanceSection({
  enabled,
  selection,
  stages,
  locale,
}: Readonly<{
  enabled: boolean;
  selection: AnalyticsSelection | null;
  stages: readonly Stage[];
  locale: Locale;
}>) {
  if (enabled)
    return selection ? (
      <ReportingOverview
        key={JSON.stringify(selection.scope)}
        scope={selection.scope}
      />
    ) : null;
  return (
    <>
      {SECTION_REPORTS.performance.map((report) => (
        <ReportCard
          key={report}
          report={report}
          stages={stages}
          locale={locale}
        />
      ))}
    </>
  );
}
