import { type UseQueryResult, useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { navigate, useRoute } from "../app/router";
import {
  Button,
  Card,
  DataTable,
  EmptyState,
  SectionHeader,
  Skeleton,
  StatCard,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { RecordTabs } from "../design-system/recordtabs";
import { StatStrip } from "../design-system/statstrip";
import { SurfaceState } from "../design-system/surfacestate";
import {
  formatDateTime,
  formatMoneyOrAbsent,
  formatNumber,
  MONEY_ABSENT,
} from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  type AnalyticsSelection,
  useAnalyticsContext,
  useAnalyticsSelection,
} from "./analytics.context";
import { ForecastView } from "./analytics.forecast";
import { sourceName } from "./analytics.forecast.review";
import { AnalyticsScopePicker } from "./analytics.scope";
import { ShareViewButton } from "./analytics.share";
import {
  OverlayUnavailable,
  problemMessageOf,
  QueryGate,
  throwProblem,
  useSorMode,
} from "./common";
import { dealsFilteredBy } from "./dealsaddress";
import { EntityRef } from "./entityref";
import "./analytics.css";

// Analytics (B-EP09.12c, D-11): a picker over three reports — deals-by-stage
// (unweighted next to weighted), forecast (category readings, each showing
// unweighted and weighted, plus the server-derived "slipped" bucket), and
// open deals per company. "Explain this number" opens the executed plan +
// the exact rows the headline reconciles to. Both weighted figures come
// straight off the report's own weighted_amount_minor measure (AC-F1: round
// PER DEAL, then sum) — neither screen re-derives it from the raw total.
//
// All three report bodies render into ONE surface: a titled Card whose trailing
// .card-actions row carries the explain toggle. The segment picked changes what
// the card holds, never what kind of thing the page is.

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
};

type ReportKey =
  | "pipeline-current"
  | "forecast"
  | "open-deals-per-company"
  | "win-loss"
  | "stage-age";

// A SECTION is what the address names and what the tabs choose between; a
// REPORT is one result inside it. They were the same thing while every section
// held exactly one report, and keeping them the same would have meant the
// address changing the day a section grew a second result — breaking every
// link anyone had saved to it.
type Section =
  | "forecast"
  | "pipeline"
  | "performance"
  | "outcomes"
  | "coverage";

// Which results each section holds, in the order they are drawn. The tab strip
// and the bodies both read this, so a section cannot come to list a report it
// does not draw.
const SECTION_REPORTS = {
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
} as const satisfies Record<Section, readonly ReportKey[]>;

const SECTIONS = Object.keys(SECTION_REPORTS) as readonly Section[];

/** isSection narrows a URL segment, which is any string a reader can type. */
function isSection(value: string | undefined): value is Section {
  return SECTIONS.some((section) => section === value);
}

// The old address named a report. Those links are in bookmarks and in sent
// mail, so each one still answers, with the section that now holds it.
// Keyed by string rather than by ReportKey, because a RETIRED name is still an
// address somebody saved. `deals-by-stage` names no report this screen draws
// any more — the stage view reads pipeline-current — and the link in a
// bookmark or a sent mail must still land on the section that answers it.
const SECTION_OF_REPORT: Readonly<Record<string, Section>> = {
  forecast: "pipeline",
  "pipeline-current": "pipeline",
  "deals-by-stage": "pipeline",
  "open-deals-per-company": "pipeline",
  "win-loss": "performance",
  "stage-age": "performance",
};

export function sectionFromAddress(segment: string | undefined): Section {
  if (isSection(segment)) {
    return segment;
  }
  if (segment && segment in SECTION_OF_REPORT) {
    return SECTION_OF_REPORT[segment as ReportKey];
  }
  return "forecast";
}

type ReportRow = components["schemas"]["ReportResult"]["rows"][number];
type Derivation = components["schemas"]["ReportDerivation"];
type Stage = components["schemas"]["Stage"];

// A figure that names a set, drawn as the way into it. The count stays the
// text — a reader is looking for the number, not for a verb — and the link is
// what the number now is.
function CountLink({
  count,
  href,
  title,
}: Readonly<{ count: number; href: string; title: string }>) {
  const { locale } = useLocale();
  return (
    <a className="link-button" href={href} title={title}>
      {formatNumber(count, locale)}
    </a>
  );
}

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
// summing. One stage is one row, denominated in the installation's base
// currency — which is the whole point of that report and why it exists beside
// deals-by-stage rather than replacing it.
const REPORT_GROUP_BY: Record<ReportKey, string[]> = {
  "pipeline-current": ["stage_id"],
  forecast: ["forecast_category"],
  "open-deals-per-company": ["organization_id", FIELD_CURRENCY],
  "win-loss": ["status"],
  "stage-age": ["stage_id"],
};

// A report row arrives as `{ [key: string]: unknown }`, so every read narrows.
// These keep the narrowing in one place, and keep the distinction the cells
// depend on: an absent measure is not a zero, and an absent currency is not EUR.
function rowCurrency(row: ReportRow): string | null {
  // An empty code is not a currency. Left as "" it renders a blank cell where a
  // code belongs, and it groups apart from null while meaning the same thing —
  // which would give the forecast two bands with the same key.
  return typeof row.currency === "string" && row.currency !== ""
    ? row.currency
    : null;
}

function rowMoney(row: ReportRow, key: string): number | null {
  const value = row[key];
  return value == null ? null : Number(value);
}

function rowCount(row: ReportRow, key: string): number {
  return Number(row[key] ?? 0);
}

// A report's own name, spelled once: the segment picker and the heading of the
// card that segment opens read the same key, so the tab and the surface behind
// it cannot drift into two names for one report.
const REPORT_LABEL_KEY = {
  "pipeline-current": "analytics.reportDeals",
  forecast: "analytics.reportForecast",
  "open-deals-per-company": "analytics.reportOpenByCompany",
  "win-loss": "analytics.reportWinLoss",
  "stage-age": "analytics.reportStageAge",
} as const satisfies Record<ReportKey, string>;

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
  ],
  forecast: [
    { fn: "sum", field: "amount_base_minor", as: "raw_minor" },
    { fn: "sum", field: "weighted_base_minor", as: "weighted_minor" },
    { fn: "count", as: "deal_count" },
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
};

// Parse a server-minted `derivation_url` into the typed derivation query.
// The generated client's derivation query is ONLY `{ by?, agg? }` (no
// predicate params, no index signature), so callers forward just those two;
// the extra predicate keys ride along on the return value for inspection
// only (spec constraint 6: never raw-fetch the URL itself).
export function parseDerivationQuery(
  url: string,
): { by: string[]; agg: string[] } & Record<string, unknown> {
  const qs = new URLSearchParams(url.split("?")[1] ?? "");
  const extra: Record<string, unknown> = {};
  for (const [k, v] of qs.entries()) {
    if (k !== "by" && k !== "agg") extra[k] = v;
  }
  return { ...extra, by: qs.getAll("by"), agg: qs.getAll("agg") };
}

// The derivation URL's path names the report key (prebuilt or saved-report
// id) the typed path param expects.
function derivationReportKey(url: string): string {
  return url.match(/reports\/([^/?]+)\/derivation/)?.[1] ?? "";
}

// forecast_category dimension values (report.go's forecastCategoryExpr):
// the four the deal itself can carry, plus the server-derived "slipped" —
// a claimed commit/best_case deal whose close date is past, missing, or
// still provisional (formulas §11). Omitting it here doesn't shrink the
// total; it moves the deal's amount into no tile at all.
const FORECAST_CATEGORIES = [
  { key: "commit", labelKey: "deal.fcCommit" },
  { key: "best_case", labelKey: "deal.fcBestCase" },
  { key: "pipeline", labelKey: "deal.fcPipeline" },
  { key: "omitted", labelKey: "deal.fcOmitted" },
  { key: "slipped", labelKey: "deal.fcSlipped" },
] as const;

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
  currency,
  locale,
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
  currency: string | null;
  locale: Locale;
}>) {
  const t = useT();
  return (
    <StatCard
      label={label}
      numeric
      value={formatMoneyOrAbsent(amountMinor, currency, locale)}
      detail={forecastTileDetail(
        { weightedMinor, dealCount, currency, locale },
        t,
      )}
    />
  );
}

// The second line of a tile: the weighted total, and how many deals the
// category holds. Both are optional and each stands without the other, so a
// caller with one figure to give is not made to invent the other.
function forecastTileDetail(
  {
    weightedMinor,
    dealCount,
    currency,
    locale,
  }: Readonly<{
    weightedMinor?: number | null;
    dealCount?: number | null;
    currency: string | null;
    locale: Locale;
  }>,
  t: ReturnType<typeof useT>,
): string | undefined {
  const parts: string[] = [];
  if (weightedMinor != null) {
    parts.push(
      `${t("analytics.weighted")}: ${formatMoneyOrAbsent(weightedMinor, currency, locale)}`,
    );
  }
  if (dealCount != null) {
    parts.push(`${t("analytics.count")}: ${formatNumber(dealCount, locale)}`);
  }
  return parts.length > 0 ? parts.join(" · ") : undefined;
}

// Five money figures read across as one comparison, so they are ONE plate of
// ruled slots rather than five free-standing cards. The banner above them is
// what the surface says about itself — how to read the second figure in every
// slot — which is a Callout, not a paragraph tinted by hand.
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
      <Callout tone="info">{t("analytics.forecastBanner")}</Callout>
      {/* ONE strip, because there is now one denomination.
          
          This drew one strip PER CURRENCY until the server learned to convert.
          A slot carries a single figure and adding euros to dong is the
          unit-less total data-semantics §1 r4 forbids, so banding was the only
          honest way to show native sums — and it defeated the thing a strip is
          for. A plate of ruled slots claims its figures are ONE comparison;
          split across bands, a manager compared commit against best case
          inside each currency and never across the business, which is the
          question they actually have.

          The slots stay slots rather than becoming a bar list: every category
          carries TWO figures, the raw total and the probability-weighted one
          beneath it, and a ranked bar carries a single amount per row. */}
      <div style={{ marginTop: "var(--space-4)" }}>
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
                  currency={baseCurrency}
                  locale={locale}
                />
              );
            },
          )}
        </StatStrip>
      </div>
    </>
  );
}

/**
 * The group keys the report answered with exactly ONE currency row.
 *
 * Every money report groups by currency as well as by its own dimension, so a
 * company trading in two currencies is two rows with two counts. `/deals` reads
 * no `currency` dial — the parameter does not exist on the endpoint — so a link
 * beside such a row can narrow to the company but not to the row, and it opens
 * the union of both. The figure promises one set and the door opens a larger
 * one, which is the defect a figure-as-door has to avoid to be worth drawing.
 *
 * So the door survives exactly where it is exact. A key with a single currency
 * row has no sibling row to be confused with, and `organization_id` alone
 * addresses precisely the deals that row counted. A key with two or more gets
 * the plain number — the same answer this table already gives a row whose
 * company is "none".
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

function CompanyTable({
  rows,
  locale,
}: Readonly<{ rows: ReportRow[]; locale: Locale }>) {
  const t = useT();
  const addressable = singleCurrencyKeys(
    rows
      .map((row) => row.organization_id)
      .filter((id): id is string => typeof id === "string"),
  );
  return (
    <DataTable
      label={t("analytics.reportOpenByCompany")}
      columns={[
        {
          key: "company",
          header: t("analytics.company"),
          // The report answers with an organization id and nothing else, so the
          // column read `01a0131c-3154-74cb-…` for every row — a company report
          // nobody could read. One record lookup per row, cached by id for a
          // minute and shared with every other reference on screen. The cost is
          // per row and this table is one report page long; the alternative is a
          // table of uuids, which is not a cheaper report but an unusable one.
          render: (row: ReportRow) =>
            typeof row.organization_id === "string" ? (
              <EntityRef kind="organization" id={row.organization_id} />
            ) : (
              // Deals with no company at all, grouped into one row. An empty
              // cell read as a rendering fault; this says what the row is, and
              // it is a fact about the data rather than a permission — so it
              // says "none", which no other state is allowed to claim.
              <span className="t-caption">{t("analytics.noCompany")}</span>
            ),
        },
        {
          key: FIELD_CURRENCY,
          header: t("analytics.currency"),
          render: (row: ReportRow) => (
            <span className="t-mono">{rowCurrency(row) ?? MONEY_ABSENT}</span>
          ),
        },
        {
          key: "count",
          header: t("analytics.openDeals"),
          render: (row: ReportRow) =>
            typeof row.organization_id === "string" &&
            addressable.has(row.organization_id) ? (
              // `status`, because this report counts OPEN deals and the list
              // otherwise answers with every status the company ever had.
              <CountLink
                count={rowCount(row, "deal_count")}
                href={dealsFilteredBy("organization_id", row.organization_id, {
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
          render: (row: ReportRow) => (
            <span className="t-mono">
              {formatMoneyOrAbsent(
                rowMoney(row, "raw_minor"),
                rowCurrency(row),
                locale,
              )}
            </span>
          ),
        },
      ]}
      rows={rows}
      // A company with deals in two currencies is two rows now, so the
      // organization id alone no longer identifies one.
      rowKey={(row) =>
        row.organization_id != null
          ? `${String(row.organization_id)}:${rowCurrency(row) ?? ""}`
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
  return (
    <DataTable
      label={t("analytics.reportDeals")}
      columns={[
        {
          key: "stage",
          header: t("deals.stage"),
          render: (row: StageAgg) => row.stageName,
        },
        {
          key: "count",
          header: t("analytics.count"),
          // Every row addresses its deals now. The old table linked only the
          // stages trading in ONE currency, because a stage split across two
          // rows had no single set to open; converted, one stage is one row
          // and one set again.
          //
          // The link asks for OPEN deals, which is what this report counts —
          // deals-by-stage could not, because a won deal keeps the stage it
          // closed in and narrowing there would have handed back a shorter
          // list than the figure above it.
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
          render: (row: StageAgg) => (
            <span className="t-mono">
              {formatMoneyOrAbsent(row.rawMinor, baseCurrency, locale)}
            </span>
          ),
        },
        {
          key: "weighted",
          header: t("analytics.weighted"),
          render: (row: StageAgg) => (
            <span className="t-mono">
              {formatMoneyOrAbsent(row.weightedMinor, baseCurrency, locale)}
            </span>
          ),
        },
      ]}
      rows={aggregates}
      rowKey={(row) => row.stageId}
    />
  );
}

// The vocabulary's own words for the columns a drill-through can carry.
// A column outside it keeps its wire name, which is honest: the reader sees
// what the plan selected rather than a guess at what it meant.
const DERIVATION_HEADERS: Readonly<Record<string, MessageKey>> = {
  label: "explain.col.record",
  amount_base_minor: "analytics.unweighted",
  weighted_base_minor: "analytics.weighted",
  amount_minor: "analytics.unweighted",
  currency: "analytics.currency",
  stage_id: "explain.col.stage",
  owner_id: "explain.col.owner",
  pipeline_id: "explain.col.pipeline",
  organization_id: "analytics.company",
};

// A column the vocabulary knows gets its word; anything else keeps the wire
// name the plan selected it under.
function derivationHeader(col: string, t: (key: MessageKey) => string): string {
  const key = DERIVATION_HEADERS[col];
  return key ? t(key) : col;
}

// The server names the row and the reader reads the name, so the raw id
// becomes noise beside it — but only once EVERY row has a name.
//
// Labelling is per row: the seam withholds a name for a record this reader
// may not read, and the label column appears as soon as one row was named.
// Dropping the id on that alone would leave the withheld rows showing a blank
// where their only identifier used to be, so the rows a reader can least
// account for become the ones they cannot identify at all.
export function derivationColumns(derivation: Derivation): string[] {
  const rows = derivation.rows ?? [];
  const everyRowNamed =
    derivation.columns.includes("label") &&
    rows.length > 0 &&
    rows.every((row) => typeof row.label === "string" && row.label !== "");
  return derivation.columns.filter((col) => !everyRowNamed || col !== "id");
}

// Which money a row's minor-unit figure is written in.
//
// The two are not the same column. A `_base_minor` measure was converted by
// the server, so it is in the installation's base currency. A plain `_minor`
// measure is the deal's OWN amount, and the forecast's rows carry the
// currency it was written in beside it — reading the base currency there
// would put a euro sign on a dollar deal, which is the exact misreading this
// renderer exists to prevent.
export function derivationCellCurrency(
  col: string,
  row: Record<string, unknown>,
  baseCurrency: string | null,
): string | null {
  if (col.endsWith("_base_minor")) {
    return baseCurrency;
  }
  const own = row.currency;
  return typeof own === "string" && own !== "" ? own : null;
}

// Money on these rows is stored in minor units, and a minor-unit integer
// printed raw is the single most misread thing on this screen: 500000 next
// to €5,000.00 are the same number wearing different clothes.
function renderDerivationCell(
  col: string,
  row: Record<string, unknown>,
  baseCurrency: string | null,
  locale: Locale,
): string {
  const value = row[col];
  if (value == null) {
    return "";
  }
  if (col.endsWith("_minor") && typeof value === "number") {
    return formatMoneyOrAbsent(
      value,
      derivationCellCurrency(col, row, baseCurrency),
      locale,
    );
  }
  return String(value);
}

// The source rows the explained figure reconciles to. A section INSIDE the
// explain card's own section, so its heading steps down with the outline
// rather than reading as a peer of the card's title.
function DerivationRows({
  derivation,
  baseCurrency,
}: Readonly<{ derivation: Derivation; baseCurrency: string | null }>) {
  const t = useT();
  const { locale } = useLocale();
  const columns = derivationColumns(derivation);
  return (
    <>
      <SectionHeader title={t("explain.sources")} level={3} />
      {derivation.rows.length === 0 ? (
        <SurfaceState
          state="empty"
          emptyLabel={t("common.empty")}
          loadingLabel={t("explain.sources")}
        >
          {null}
        </SurfaceState>
      ) : (
        <DataTable
          label={t("explain.sources")}
          columns={columns.map((col) => ({
            key: col,
            header: derivationHeader(col, t),
            render: (row: Record<string, unknown>) =>
              renderDerivationCell(col, row, baseCurrency, locale),
          }))}
          rows={derivation.rows}
          rowKey={(row) => derivation.rows.indexOf(row).toString()}
        />
      )}
    </>
  );
}

// "Explain this number": the titled Card that shows where a figure came from,
// so a number on this screen is never presented without its derivation.
function ExplainCard({
  id,
  url,
  query,
  baseCurrency,
}: Readonly<{
  // The toggle above points `aria-controls` here, so the card has to carry the
  // id the toggle was given rather than mint one of its own.
  id: string;
  url: string | null;
  query: UseQueryResult<Derivation>;
  // The currency the report converted into, so the source rows behind a
  // converted total are written in the same money as the total.
  baseCurrency: string | null;
}>) {
  const t = useT();
  return (
    <Card
      id={id}
      ariaLabel={t("explain.title")}
      title={t("explain.title")}
      sub={query.data?.definition ?? t("analytics.planNote")}
    >
      {url == null && <p className="t-caption">{t("common.empty")}</p>}
      {url != null && query.isPending && (
        <div
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "var(--space-2)",
          }}
        >
          <Skeleton width="60%" />
          <Skeleton width="90%" />
        </div>
      )}
      {query.isError && (
        <>
          <p className="t-caption">{problemMessageOf(query.error, t)}</p>
          <div className="card-actions">
            <Button small onClick={() => query.refetch()}>
              {t("common.retry")}
            </Button>
          </div>
        </>
      )}
      {/* A link minted before the handle carried an instant — an old one, or one
          a reader saved. The figures below were recomputed at a NEW moment, so
          a rate sheet effective in between makes them disagree with the number
          they explain. Said plainly: this is opened by someone checking a
          figure they already doubt, and a detail that quietly reconciles to
          something else reads as proof rather than as a discrepancy. */}
      {query.data?.as_of_pinned === false && (
        <p className="surfacestate-stale">{t("explain.mayHaveMoved")}</p>
      )}
      {query.data && (
        <DerivationRows derivation={query.data} baseCurrency={baseCurrency} />
      )}
    </Card>
  );
}

// A duration cell: the server's median or p75, or the withheld state the
// engine answers below its sample floor. A dash would read as zero-ish; the
// words say why there is no number.
function DaysCell({
  value,
  locale,
}: Readonly<{ value: unknown; locale: Locale }>) {
  const t = useT();
  if (value == null) {
    return <span className="t-caption">{t("analytics.tooFewForMedian")}</span>;
  }
  return (
    <>{t("analytics.days", { days: formatNumber(Number(value), locale) })}</>
  );
}

type OutcomeRow = {
  status: string;
  count: number;
  baseMinor: number | null;
  medianDays: unknown;
  p75Days: unknown;
};

// Won and lost, side by side: counts, converted value, and how long the
// closed deals took. No rate is computed here — a win rate is the server's to
// answer the day it owns a denominator, and a browser-made quotient would be
// a second answer to what the cohort is.
function WinLossTable({
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
    }));
  if (outcomes.length === 0) {
    // Nothing closed yet is a real answer, not a broken table: the population
    // is closed deals, and a young installation has none.
    return <EmptyState>{t("analytics.noClosedDeals")}</EmptyState>;
  }
  return (
    <DataTable
      label={t("analytics.reportWinLoss")}
      columns={[
        {
          key: "outcome",
          header: t("analytics.outcome"),
          render: (row: OutcomeRow) =>
            row.status === "won" ? t("analytics.won") : t("analytics.lost"),
        },
        {
          key: "count",
          header: t("analytics.count"),
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
          render: (row: OutcomeRow) =>
            formatMoneyOrAbsent(row.baseMinor, baseCurrency, locale),
        },
        {
          key: "median",
          header: t("analytics.medianDaysToClose"),
          render: (row: OutcomeRow) => (
            <DaysCell value={row.medianDays} locale={locale} />
          ),
        },
        {
          key: "p75",
          header: t("analytics.p75DaysToClose"),
          render: (row: OutcomeRow) => (
            <DaysCell value={row.p75Days} locale={locale} />
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
};

// How long open deals have sat in each stage, from the stage history's own
// entry instants. The median and p75 arrive computed; below the sample floor
// they arrive withheld, and the count beside the blank still says how many.
function StageAgeTable({
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
      };
    })
    .sort((a, b) => a.stagePosition - b.stagePosition);
  return (
    <DataTable
      label={t("analytics.reportStageAge")}
      columns={[
        {
          key: "stage",
          header: t("deals.stage"),
          render: (row: StageAgeRow) => row.stageName,
        },
        {
          key: "count",
          header: t("analytics.count"),
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
          render: (row: StageAgeRow) => (
            <DaysCell value={row.medianDays} locale={locale} />
          ),
        },
        {
          key: "p75",
          header: t("analytics.p75DaysInStage"),
          render: (row: StageAgeRow) => (
            <DaysCell value={row.p75Days} locale={locale} />
          ),
        },
      ]}
      rows={aged}
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
  return (
    <QueryGate query={coverage} pendingLabel={t("analytics.sectionCoverage")}>
      {(run) =>
        run == null ? (
          <Card title={t("analytics.sectionCoverage")}>
            <EmptyState>{t("analytics.coverageNeverRun")}</EmptyState>
          </Card>
        ) : (
          <Card title={t("analytics.sectionCoverage")}>
            <p className="sub">{t("analytics.coverageSub")}</p>
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
            <p className="sub">{t("analytics.coverageInputsElsewhere")}</p>
          </Card>
        )
      }
    </QueryGate>
  );
}

type DataCoverageRow = components["schemas"]["DataCoverage"]["sources"][number];

function useDataCoverage() {
  return useQuery({
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

type AnalyticsScopeWire = components["schemas"]["AnalyticsScope"];

// The current standing a meeting can hold, in the order a week reads: what is
// ahead, what happened, what did not, what was called off. A hand-kept mirror
// of the server's CHECK vocabulary — a status the server grows is absent here
// until this list learns it, rather than mislabeled.
const MEETING_STATUSES = [
  { key: "booked", labelKey: "analytics.meetingsBooked" },
  { key: "held", labelKey: "analytics.meetingsHeld" },
  { key: "no_show", labelKey: "analytics.meetingsNoShow" },
  { key: "canceled", labelKey: "analytics.meetingsCanceled" },
] as const;

// The seat's own outcomes: open pipeline and meetings, nothing computed here.
//
// Drawn only under an OWNER default lens. The report engine's population
// default is the caller's own row scope, so for a wider lens the same
// requests would measure a team while the heading said "my" — and there is
// no per-report scope override on the wire to force self. The tab is hidden
// for those lenses; a hand-typed address gets the explanation instead.
function MyOutcomesView({
  defaultScope,
  locale,
}: Readonly<{ defaultScope: AnalyticsScopeWire; locale: Locale }>) {
  const t = useT();
  const self = defaultScope.kind === "owner" ? (defaultScope.id ?? null) : null;

  const pipelineQuery = useQuery({
    queryKey: ["report", "pipeline-current", "outcomes", self],
    enabled: self != null,
    queryFn: async () => {
      const { data, error } = await api.POST("/reports/{report}", {
        params: { path: { report: "pipeline-current" } },
        body: {
          // The seat pinned EXPLICITLY, not left to the server's default
          // population: the default is also the caller's own today, so the
          // two agree, but this card's heading says "my" and a heading must
          // not be true by a coincidence this file cannot see.
          filters: { owner_id: self },
          aggregates: [
            { fn: "count", as: "deal_count" },
            { fn: "sum", field: "amount_base_minor", as: "raw_minor" },
          ],
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  const meetingsQuery = useQuery({
    queryKey: ["report", "activities-by-kind", "outcomes", self],
    enabled: self != null,
    queryFn: async () => {
      const { data, error } = await api.POST("/reports/{report}", {
        params: { path: { report: "activities-by-kind" } },
        body: {
          filters: { kind: "meeting", host_user_id: self },
          group_by: ["meeting_status"],
          aggregates: [{ fn: "count", as: "meetings" }],
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  if (self == null) {
    // A hand-typed address under a manager lens: the numbers this view could
    // fetch would measure the default population, not the person.
    return <Callout tone="info">{t("analytics.outcomesOwnLensOnly")}</Callout>;
  }

  const pipelineRow = pipelineQuery.data?.rows[0];
  const meetingRows = meetingsQuery.data?.rows ?? [];
  const meetingsByStatus = new Map(
    meetingRows
      .filter((row) => typeof row.meeting_status === "string")
      .map((row) => [String(row.meeting_status), rowCount(row, "meetings")]),
  );

  return (
    <>
      <Card title={t("analytics.myPipeline")}>
        <StatStrip>
          <StatCard
            label={t("analytics.count")}
            value={
              pipelineRow
                ? formatNumber(rowCount(pipelineRow, "deal_count"), locale)
                : "…"
            }
          />
          <StatCard
            label={t("analytics.baseValue", {
              currency: pipelineQuery.data?.base_currency ?? "",
            })}
            value={
              pipelineRow
                ? formatMoneyOrAbsent(
                    rowMoney(pipelineRow, "raw_minor"),
                    pipelineQuery.data?.base_currency ?? null,
                    locale,
                  )
                : "…"
            }
          />
        </StatStrip>
      </Card>
      <Card title={t("analytics.myMeetings")}>
        {/* Current standing, stated as such: a held meeting was once booked
            and the record no longer says so, so these are today's facts and
            not a funnel. */}
        <p className="sub">{t("analytics.meetingsAsTheyStand")}</p>
        <StatStrip>
          {MEETING_STATUSES.map((status) => (
            <StatCard
              key={status.key}
              label={t(status.labelKey)}
              value={formatNumber(
                meetingsByStatus.get(status.key) ?? 0,
                locale,
              )}
            />
          ))}
        </StatStrip>
      </Card>
    </>
  );
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

  // Hooks can't run inside the QueryGate render-prop callback (the run
  // result lives there), so the derivation handle is lifted to the top
  // level from the already-top-level run query.
  const derivationUrl = reportQuery.data?.derivation_url ?? null;
  const derivationQuery = useQuery({
    queryKey: ["derivation", derivationUrl],
    enabled: explain && derivationUrl != null,
    queryFn: async () => {
      // parsed carries by/agg PLUS every equality predicate from the handle
      // (group-key values + plan filters). The endpoint treats each extra key
      // as a predicate, so forward the whole object — dropping the predicates
      // would explain the wrong slice (or 422 on a bound grouping dimension).
      const parsed = parseDerivationQuery(derivationUrl ?? "");
      const { data, error } = await api.GET("/reports/{report}/derivation", {
        params: {
          path: { report: derivationReportKey(derivationUrl ?? "") },
          query: parsed,
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  return (
    <QueryGate query={reportQuery} pendingLabel={t(REPORT_LABEL_KEY[report])}>
      {(run) => (
        <>
          <Card title={t(REPORT_LABEL_KEY[report])}>
            {reportSub[report] && (
              <p className="sub">
                {t(reportSub[report], { currency: run.base_currency ?? "" })}
              </p>
            )}
            {report === "forecast" && (
              <ForecastStrip
                rows={run.rows}
                baseCurrency={run.base_currency ?? null}
                locale={locale}
              />
            )}
            {report === "open-deals-per-company" && (
              <CompanyTable rows={run.rows} locale={locale} />
            )}
            {report === "pipeline-current" && (
              <StageTable
                rows={run.rows}
                stages={stages}
                locale={locale}
                baseCurrency={run.base_currency ?? null}
              />
            )}
            {report === "win-loss" && (
              <WinLossTable
                rows={run.rows}
                locale={locale}
                baseCurrency={run.base_currency ?? null}
              />
            )}
            {report === "stage-age" && (
              <StageAgeTable rows={run.rows} stages={stages} locale={locale} />
            )}
            {/* The frame every figure above was cut in: the instant, and the
                zone that instant is stated in. A total with no zone beside it
                is a number a reader places by assumption, and the assumption
                is usually their own.

                It does NOT state a currency, and that is still the point,
                though for a narrower reason than it once was. The stage table
                and the forecast strip are both converted now and both name
                their base currency themselves; open-deals-per-company is not,
                and prints a currency COLUMN because its rows are native. So
                the figures above are no longer all in several currencies at
                once — but they are not all in one either, and a single line
                over the card cannot say something true of every block beneath
                it.

                The frame used to end in `run.base_currency`, which read as the
                denomination of numbers that were never converted into it: a
                reader taking it at its word read ₫367,620,000,000 as a euro
                figure. That is the failure this omission exists to prevent,
                and it stays available for exactly as long as one block on the
                tab is unconverted.

                Drawn only when the server sent both halves: a caption naming
                one of the two would be worse than none, and a server
                mid-upgrade is exactly where a partial one arrives. */}
            {run.as_of && run.timezone && (
              <p className="sub analytics-frame">
                {t("analytics.frame", {
                  asOf: formatDateTime(run.as_of, locale, run.timezone),
                  zone: run.timezone,
                })}
              </p>
            )}
            <div className="card-actions">
              {/* A toggle, and it says so: the button reveals and hides the
                  card below, so it announces the open state and names what
                  it controls. */}
              <Button
                small
                aria-expanded={explain}
                aria-controls={explainId}
                onClick={() => setExplain((value) => !value)}
              >
                {t("explain.open")}
              </Button>
            </div>
          </Card>
          {explain && (
            <ExplainCard
              id={explainId}
              url={derivationUrl}
              query={derivationQuery}
              baseCurrency={run.base_currency ?? null}
            />
          )}
        </>
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
  const section = sectionFromAddress(
    route.screen === "analytics" ? route.id : undefined,
  );
  const setSection = (next: Section) =>
    navigate({ screen: "analytics", id: next });
  // Deal reports aggregate over the pipeline/stage structure the overlay mirror
  // does not hold (the report endpoints answer 422 unsupported_by_sor in
  // overlay), so the sections show the honest unavailable state.
  const overlay = useSorMode() === "overlay";
  // The server decides which population this reader measures and which ones
  // they may choose. Read once here and handed down, so every card on the page
  // is answering about the same set.
  const context = useAnalyticsContext();
  const { selection, selectScope } = useAnalyticsSelection(context.data);

  const pipelineQuery = useQuery({
    queryKey: ["pipelines"],
    enabled: !overlay,
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
  const coverageProbe = useDataCoverage();
  const header = (
    <div className="analytics-header">
      <RecordTabs
        options={SECTIONS.filter((candidate) => {
          if (candidate === "outcomes") {
            return context.data?.default_scope.kind === "owner";
          }
          if (candidate === "coverage") {
            // The server gates this read on the ops grant. The tab appears
            // when the probe ANSWERS — hidden while pending, so a seat the
            // server refuses never sees it flicker in and out.
            return coverageProbe.isSuccess;
          }
          return true;
        })}
        value={section}
        onChange={setSection}
        labels={{
          forecast: t("analytics.sectionForecast"),
          pipeline: t("analytics.sectionPipeline"),
          performance: t("analytics.sectionPerformance"),
          outcomes: t("analytics.sectionOutcomes"),
          coverage: t("analytics.sectionCoverage"),
        }}
        label={t("analytics.sections")}
      />
      {selection && context.data ? (
        <AnalyticsScopePicker
          scopes={context.data.allowed_scopes}
          selected={selection.scope}
          onSelect={selectScope}
        />
      ) : null}
      {section === "forecast" && selection ? (
        <ShareViewButton target="forecast" scope={selection.scope} />
      ) : null}
    </div>
  );

  if (overlay) {
    return (
      <div className="wrap">
        {header}
        <OverlayUnavailable />
      </div>
    );
  }

  return (
    <div className="wrap">
      {header}
      <SectionBody
        section={section}
        locale={locale}
        context={context.data}
        selection={selection}
        stages={pipelineQuery.data?.stages ?? []}
      />
    </div>
  );
}

// One section's body, chosen in its own component so the screen's own render
// stays a header plus a choice rather than a ladder of ternaries.
function SectionBody({
  section,
  locale,
  context,
  selection,
  stages,
}: Readonly<{
  section: Section;
  locale: Locale;
  context: components["schemas"]["AnalyticsContext"] | undefined;
  selection: AnalyticsSelection | null;
  stages: readonly Stage[];
}>) {
  switch (section) {
    case "coverage":
      return (
        <DataCoverageView
          locale={locale}
          timezone={context?.timezone ?? null}
        />
      );
    case "outcomes":
      return context ? (
        <MyOutcomesView defaultScope={context.default_scope} locale={locale} />
      ) : null;
    case "forecast":
      return selection && context ? (
        <ForecastView
          selection={selection}
          canSubmit={context.capabilities.submit_manager_forecast}
        />
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
