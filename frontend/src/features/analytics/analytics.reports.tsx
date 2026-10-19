import { useQuery } from "@tanstack/react-query";
import { Info } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { IconAction } from "../../design-system/iconaction";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { rowsState, SurfaceState } from "../../design-system/surfacestate";
import { formatDateTime } from "../../format/format";
import { type Locale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { unwrap } from "../../lib/problem";
import type { Section } from "./analytics.address";
import { FIELD_CURRENCY, type ReportRow, type Stage } from "./analytics.cells";
import { CompanyTable } from "./analytics.company";
import {
  ProjectCommitmentsTable,
  ProjectsByPhaseTable,
  ProjectsGoneQuietTable,
} from "./analytics.delivery";
import { ExplainFrame, ExplainPanel } from "./analytics.explain";
import { ForecastStrip, StageTable } from "./analytics.pipeline";
import { ENTITY_LABEL_KEY } from "./analytics.questions.vocab";

type ReportKey =
  | "pipeline-current"
  | "forecast"
  | "open-deals-per-company"
  | "projects-by-phase"
  | "project-commitments"
  | "projects-gone-quiet";

// Which results each section holds, in the order they are drawn. The tab strip
// and the bodies both read this, so a section cannot come to list a report it
// does not draw.
export const SECTION_REPORTS = {
  reports: [],
  targets: [],
  definitions: [],
  // The forecast section draws its own view (readings, a call and a receipt,
  // none of them a row set), so it lists no report card.
  forecast: [],
  // The forecast-category breakdown lives here. It is a real report and the
  // only place a reader sees how the pipeline divides by category. It is not
  // the forecast, which is an answer rather than a table.
  pipeline: ["pipeline-current", "forecast", "open-deals-per-company"],
  performance: [],
  // The rep's own week: a composed view like the forecast, not report cards.
  outcomes: [],
  // Source health: an ops view over the nightly check's own coverage rows.
  coverage: [],
  // What was sold becoming what is delivered: the three project reports.
  delivery: ["project-commitments", "projects-gone-quiet", "projects-by-phase"],
  questions: [],
} as const satisfies Record<Section, readonly ReportKey[]>;

// A plan that sums native money groups by currency as well as by its own
// dimension. amount_minor is a minor-unit integer in the deal's own currency,
// so a total spanning currencies is a number with no unit.
//
// pipeline-current does not, because the server converted each deal before
// summing. One stage is one row in the installation's base currency; that is
// why the report exists beside deals-by-stage rather than replacing it.
const REPORT_GROUP_BY: Record<ReportKey, string[]> = {
  "pipeline-current": ["stage_id"],
  forecast: ["forecast_category"],
  "open-deals-per-company": ["company_id", FIELD_CURRENCY],
  // The specs' own defaults: an empty plan takes each report's declared
  // grouping and aggregates, which is what these three need.
  "projects-by-phase": [],
  "project-commitments": [],
  "projects-gone-quiet": [],
};

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
// per-spec measures). `weighted_amount_minor` only exists where a stage join
// computes it (deals-by-stage, forecast). Requesting it against
// open-deals-per-company's narrower vocabulary would 422.
const REPORT_AGGREGATES: Record<ReportKey, ReportAggregate[]> = {
  "pipeline-current": [
    { fn: "sum", field: "amount_base_minor", as: "raw_minor" },
    { fn: "sum", field: "weighted_base_minor", as: "weighted_minor" },
    { fn: "count", as: "deal_count" },
    // How many of deal_count the sums above cover. A deal in a currency the
    // rate sheet cannot price is counted but adds nothing to either total.
    // A row printing only the money reads as complete when it is short
    // (margince#4201).
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
  "projects-by-phase": [],
  "project-commitments": [],
  "projects-gone-quiet": [],
};

// Which table a report's rows become: a switch in its own component, so the
// card's render stays a frame plus a choice.
function ReportBody({
  report,
  run,
  complete,
  stages,
  locale,
}: Readonly<{
  report: ReportKey;
  run: {
    rows: ReportRow[];
    base_currency?: string | null;
    timezone?: string | null;
  };
  complete: boolean;
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
      return (
        <CompanyTable rows={run.rows} complete={complete} locale={locale} />
      );
    case "pipeline-current":
      return (
        <StageTable
          rows={run.rows}
          stages={stages}
          locale={locale}
          baseCurrency={base}
        />
      );
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

// A capped report keeps its table and states the rest beneath it. An uncapped
// one, empty included, draws as it is: each table words its own empty answer.
function ReportRemainder({
  run,
  label,
  children,
}: Readonly<{
  run: Readonly<{ rows: readonly ReportRow[]; total_rows?: number }>;
  label: string;
  children: (complete: boolean) => ReactNode;
}>) {
  const t = useT();
  const { state, remaining } = rowsState(run.rows.length, run.total_rows);
  const capped = state === "partial";
  return (
    <SurfaceState
      state={capped ? "partial" : "ready"}
      emptyLabel={t("common.empty")}
      loadingLabel={label}
      detail={{ remaining }}
    >
      {children(!capped)}
    </SurfaceState>
  );
}

// One report: its own query, its own explain disclosure, its own card. A
// section may hold several, and each has to load, fail and be explained on its
// own. A single query per screen would show one spinner for both of a
// section's two results, and one error for either.
export function ReportCard({
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
      return unwrap(
        await api.POST("/reports/{report}", {
          params: { path: { report } },
          // An empty list and an absent one both take the report's declared defaults
          // (report.go branches on len==0). So the delivery reports' empty plans ask
          // for what their specs declare.
          body: {
            group_by: REPORT_GROUP_BY[report],
            aggregates: REPORT_AGGREGATES[report],
          },
        }),
      );
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
            // head beside the title it explains. It announces its open state
            // and names what it controls, so a reader who cannot see the panel
            // appear is still told it did.
            titleAction={
              // The same glyph as a row's own explain, so the panel's verb and
              // a cell's read as one control at two sizes of subject. A glyph,
              // not words: the band is one line shared with the title, and a
              // phrase there cuts the title to "O…".
              <IconAction
                label={t("explain.open")}
                icon={<Info aria-hidden />}
                disclosure={{ expanded: explain, controls: explainId }}
                onClick={() => setExplain((value) => !value)}
              />
            }
            // The frame every figure above was cut in: the instant and its zone.
            // A total with no zone is a number a reader places by assumption.
            // A fact about the whole panel, so it is the panel's footer.
            //
            // It names no currency and must not. open-deals-per-company prints
            // native rows beside converted blocks, so one line says nothing
            // true of them all; naming one read ₫367,620,000,000 as a euro
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
              <ReportRemainder run={run} label={t(ENTITY_LABEL_KEY[report])}>
                {(complete) => (
                  <ReportBody
                    report={report}
                    run={run}
                    complete={complete}
                    stages={stages}
                    locale={locale}
                  />
                )}
              </ReportRemainder>
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
