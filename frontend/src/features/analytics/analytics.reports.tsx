import {
  type CardKey,
  reportOf,
  useReportRun,
} from "./analytics.reports.queries";

export {
  type CardKey,
  SECTION_REPORTS,
  useReportRun,
} from "./analytics.reports.queries";

import { Info } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { IconAction } from "../../design-system/iconaction";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { rowsState, SurfaceState } from "../../design-system/surfacestate";
import { formatDateTime } from "../../format/format";
import { INTL_LOCALE } from "../../format/intllocale";
import { zoneNameAndOffset } from "../../format/timezone";
import { type Locale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { ReportRow, Stage } from "./analytics.cells";
import { CloseMonthTable } from "./analytics.closemonth";
import { CompanyTable } from "./analytics.company";
import type { AnalyticsScope } from "./analytics.context";
import {
  ProjectCommitmentsTable,
  ProjectsByPhaseTable,
  ProjectsGoneQuietTable,
} from "./analytics.delivery";
import { ExplainFrame, ExplainPanel } from "./analytics.explain";
import { ForecastStrip, StageTable } from "./analytics.pipeline";
import { ENTITY_LABEL_KEY } from "./analytics.questions.vocab";

const CARD_TITLE: Partial<Record<CardKey, MessageKey>> = {
  "pipeline-by-close-month": "analytics.reportPipelineByCloseMonth",
};

// The line under a report's title, for the reports whose copy says something
// the card's own title does not. A report absent from here gets no caption: an
// explanation beside a report it does not describe is worse than none.
const reportSub: Partial<Record<CardKey, MessageKey>> = {
  "pipeline-current": "analytics.sub",
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
  report: CardKey;
  run: {
    rows: ReportRow[];
    base_currency?: string | null;
    timezone?: string | null;
    excluded_by_permission?: number | null;
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
        <CompanyTable
          rows={run.rows}
          complete={complete}
          withheld={run.excluded_by_permission ?? 0}
          locale={locale}
        />
      );
    case "pipeline-by-close-month":
      return (
        <CloseMonthTable rows={run.rows} locale={locale} baseCurrency={base} />
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
  report: card,
  stages,
  locale,
  scope,
  section = false,
}: Readonly<{
  report: CardKey;
  stages: readonly Stage[];
  locale: Locale;
  // The picker's record scope. Absent, the report measures its own default:
  // the reader's lens for deals, every readable project for delivery.
  scope?: AnalyticsScope;
  // A section of a larger sheet or rail: its title sits flat over the report.
  section?: boolean;
}>) {
  const t = useT();
  const [explain, setExplain] = useState(false);
  const explainId = useId();
  const report = reportOf(card);
  const title = t(CARD_TITLE[card] ?? ENTITY_LABEL_KEY[report]);

  const reportQuery = useReportRun(card, scope);

  return (
    <QueryGate query={reportQuery} pendingLabel={title}>
      {(run) => (
        <ExplainFrame
          frame={{
            baseCurrency: run.base_currency ?? null,
            timezone: run.timezone ?? null,
          }}
        >
          <Panel
            title={title}
            flatHead={section}
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
                    zone: zoneNameAndOffset(
                      INTL_LOCALE[locale],
                      new Date(run.as_of),
                      run.timezone,
                    ),
                  })}
                </span>
              ) : undefined
            }
          >
            <PanelBody>
              {reportSub[card] && (
                <PanelIntro>
                  {t(reportSub[card], { currency: run.base_currency ?? "" })}
                </PanelIntro>
              )}
              <ReportRemainder run={run} label={title}>
                {(complete) => (
                  <ReportBody
                    report={card}
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
