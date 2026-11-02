// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { UseQueryResult } from "@tanstack/react-query";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import { ErrorLine } from "../../design-system/errorline";
import { FilterBar } from "../../design-system/filterbar";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SurfaceState } from "../../design-system/surfacestate";
import { useLocale, usePlural, useT } from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { useCan, useCanWrite } from "../../lib/capability";
import { problemCodeOf } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { scopeKey } from "./analytics.context";
import { useSdrSummary, WaitingOnOthers } from "./analytics.outcomes.sdr";
import { outboundDials } from "./analytics.overview.context";
import { DoNext } from "./analytics.overview.donext";
import { MonthlyTargets } from "./analytics.overview.monthly";
import {
  ProspectingReadings,
  ProspectingResults,
} from "./analytics.overview.prospectinghero";
import {
  OutcomeHistory,
  type ProspectingOutcome,
  useProspectingWeeks,
} from "./analytics.overview.prospectingweeks";
import {
  HandoffCohort,
  UpcomingMeetings,
} from "./analytics.overview.prospectingwork";
import {
  OverviewLinks,
  OverviewSaveActions,
} from "./analytics.overview.readings";
import { periodCaption } from "./analytics.period";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import { useSelectionEvidence } from "./reporting.evidenceselection";
import { ReportingFilters } from "./reporting.filters";
import type {
  ReportingEvaluation,
  ReportingEvidenceRef,
  ReportingSelection,
} from "./reporting.model";
import {
  customInterval,
  useOverviewFilters,
  validDateRange,
} from "./reporting.overview";
import { useReportingEvaluation } from "./reporting.queries";
import { viewRequest } from "./reporting.views";
import { OpeningSummary, prospectingSummary } from "./reporting.written";

type Catalog = components["schemas"]["ReportingCatalog"];
type AnalyticsContext = components["schemas"]["AnalyticsContext"];
type Framework = components["schemas"]["ReportingFramework"];
type Workflow = "handoff" | "qualification";
// Which outcome counts is known from the framework, or else the summary. Until
// one answers it is pending; when neither can, it failed and nothing assumes it.
type WorkflowState = Workflow | "pending" | "failed";

function workflowOf(
  framework: UseQueryResult<Framework>,
  summary: ReturnType<typeof useSdrSummary>,
): WorkflowState {
  if (framework.data)
    return framework.data.definition.template === "sdr"
      ? "handoff"
      : "qualification";
  if (summary.data) return summary.data.workflow;
  return framework.isLoading || summary.isLoading ? "pending" : "failed";
}

/**
 * My prospecting: the reader's own outcomes this week, their monthly targets
 * on a strip of its own, and what waits on them and on others. One evaluation
 * carries the week and the month, because a target is read over its own
 * calendar month. No figure here takes a pipeline, so none is read.
 */
export function MyProspecting({
  catalog,
  context,
  workflow: framework,
  switcher,
}: Readonly<{
  catalog: Catalog;
  context: AnalyticsContext;
  workflow: UseQueryResult<Framework>;
  switcher: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const self = context.allowed_scopes.find((scope) => scope.kind === "owner");
  const filters = useOverviewFilters([], "this_week");
  const readsLeads = useCan("lead", "read");
  const summary = useSdrSummary(self?.id ?? "", !!self?.id && readsLeads);
  const { metrics, blocks: sdrBlocks } = viewRequest(
    "sdr",
    catalog,
    filters.period,
  );
  const selection: ReportingSelection = {
    scope: { kind: "owner", id: self?.id, label: self?.label },
    period: filters.period,
    interval: customInterval(filters.period, filters.start, filters.end, zone),
    // The strip compares each target with its own month, whatever the period.
    target_basis: "month",
    close_window: "all_open",
    metrics,
    blocks: [],
  };
  const validDates = validDateRange(filters.period, filters.start, filters.end);
  const query = useReportingEvaluation(
    selection,
    !!self?.id && validDates && metrics.length > 0,
  );
  const { evidence, evidenceFrom, openEvidence, closeEvidence } =
    useSelectionEvidence(JSON.stringify(selection));
  const weeks = useProspectingWeeks(
    query.data,
    weekOutcome(workflowOf(framework, summary)),
  );
  // Each opener keeps the read it was pressed in: a week's column its weeks,
  // every other figure the week's evaluation.
  const openFrom = (reference: ReportingEvidenceRef) =>
    openEvidence(reference, weeksOwn(reference, weeks.data) ?? query.data);
  if (!self?.id) {
    return (
      <SurfaceState
        state="withheld"
        emptyLabel={t("common.empty")}
        loadingLabel={t("analytics.overview.prospecting")}
        detail={{ withheldReason: t("analytics.outcomesOwnLensOnly") }}
      >
        {null}
      </SurfaceState>
    );
  }
  const workflow = workflowOf(framework, summary);
  const known = workflow === "handoff" || workflow === "qualification";
  const outcome = weekOutcome(workflow);
  const retryWorkflow = () => {
    void framework.refetch();
    void summary.refetch();
  };
  return (
    <>
      <FilterBar
        label={t("analytics.filters")}
        quiet
        actions={
          <OverviewSaveActions
            selection={
              query.data
                ? { ...query.data.selection, blocks: sdrBlocks }
                : undefined
            }
            scope={scopeKey(self)}
          />
        }
        caption={
          validDates
            ? periodCaption(query.data, locale, t)
            : t("reporting.chooseDates")
        }
      >
        {switcher}
        <ReportingFilters
          selection={selection}
          showScope={false}
          showPipeline={false}
          showTargets={false}
          showCloseWindow={false}
          dates={{
            from: filters.start,
            through: filters.end,
            onChange: (from, through) => {
              filters.setStart(from);
              filters.setEnd(through);
            },
          }}
          onChange={(next) => {
            if (next.period !== filters.period) filters.setPeriod(next.period);
          }}
        />
      </FilterBar>
      {problemCodeOf(query.error) === "reporting_interval_invalid" && (
        <ErrorLine error={query.error} />
      )}
      {/* The results and the first moves share the first screen; on a narrow
          one the moves come straight after the results, before any history. */}
      <div className="analytics-lead">
        <div className="analytics-column">
          <QueryGate
            query={query}
            pendingLabel={t("analytics.overview.prospecting")}
          >
            {(evaluation) => (
              <>
                <ProspectingResults
                  evaluation={evaluation}
                  outcome={outcome}
                  workflowState={known ? "known" : workflow}
                  weeks={weeks}
                  onEvidence={openFrom}
                  onRetryWorkflow={retryWorkflow}
                  aside={
                    <MonthlyTargets
                      evaluation={evaluation}
                      ownScope={scopeKey(self)}
                    />
                  }
                />
                <ProspectingReadings
                  evaluation={evaluation}
                  summary={summary}
                  onEvidence={openFrom}
                />
              </>
            )}
          </QueryGate>
        </div>
        <ProspectingMoves
          evaluation={query.isError ? undefined : query.data}
          workflow={known ? workflow : undefined}
          waiting={summary.data?.handoffs?.waiting}
        />
      </div>
      <ProspectingWork
        self={self.id}
        workflow={workflow}
        summary={summary}
        week={query.data}
        weeks={weeks}
        onEvidence={openFrom}
      />
      {evidence && query.data && (
        <ReportingEvidenceDrawer
          key={JSON.stringify(evidence)}
          evaluation={evidenceFrom ?? query.data}
          reference={evidence}
          onClose={closeEvidence}
        />
      )}
      <OverviewLinks
        dials={outboundDials("prospecting", {
          // The reader's own work, never a colleague's or a team's.
          scope: scopeKey(self),
          period: filters.period,
          from: filters.period === "custom" ? filters.start : undefined,
          through: filters.period === "custom" ? filters.end : undefined,
        })}
        sections={[
          { section: "sdr", label: t("reporting.sdr") },
          { section: "leads", label: t("reporting.leads") },
        ]}
      />
    </>
  );
}

// The short explanation and the first moves of the day, beside the results.
function ProspectingMoves({
  evaluation,
  workflow,
  waiting,
}: Readonly<{
  evaluation: ReportingEvaluation | undefined;
  workflow: Workflow | undefined;
  waiting: number | undefined;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <div className="analytics-column">
      {evaluation && (
        <OpeningSummary
          title={t("analytics.overview.summary")}
          sentences={prospectingSummary(evaluation, workflow, waiting, {
            t,
            plural,
            locale,
          })}
        />
      )}
      <DoNext />
    </div>
  );
}

// Below the first screen: who the reader waits on, the meetings ahead, and how
// the recent weeks and the submitted handoffs turned out.
function ProspectingWork({
  self,
  workflow,
  summary,
  week,
  weeks,
  onEvidence,
}: Readonly<{
  self: string;
  workflow: WorkflowState;
  summary: ReturnType<typeof useSdrSummary>;
  week: ReportingEvaluation | undefined;
  weeks: ReturnType<typeof useProspectingWeeks>;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  return (
    <>
      <div className="analytics-pair">
        {workflow === "handoff" && <Waiting self={self} summary={summary} />}
        <UpcomingMeetings
          self={self}
          summary={summary}
          title={t(
            workflow === "qualification"
              ? "analytics.prospect.prepTitle"
              : "analytics.prospect.upcomingTitle",
          )}
        />
      </div>
      {workflow === "qualification" && <QualificationNote />}
      {week && (
        <div className="analytics-pair">
          <OutcomeHistory weeks={weeks} onEvidence={onEvidence} />
          {workflow === "handoff" && (
            <HandoffCohort self={self} evaluation={week} />
          )}
        </div>
      )}
    </>
  );
}

// Handoffs waiting on someone else. Which workflow applies came from the
// framework, so a failed summary says so rather than reading as none waiting.
function Waiting({
  self,
  summary,
}: Readonly<{
  self: string;
  summary: ReturnType<typeof useSdrSummary>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  if (summary.isError) {
    return (
      <Panel title={t("analytics.sdrWaiting")}>
        <PanelBody>
          <SurfaceState
            state="failed"
            emptyLabel={t("common.empty")}
            loadingLabel={t("analytics.sdrWaiting")}
            detail={{ onRetry: () => void summary.refetch() }}
          >
            {null}
          </SurfaceState>
        </PanelBody>
      </Panel>
    );
  }
  const handoffs = summary.data?.handoffs;
  if (!summary.data || !handoffs) return null;
  return (
    <WaitingOnOthers
      self={self}
      locale={locale}
      summary={summary.data}
      totals={handoffs}
    />
  );
}

// Without SDR handoffs the page says why it counts qualified leads, and shows
// the one who can change that where the setting is.
export function QualificationNote() {
  const t = useT();
  const canEdit = useCanWrite("reporting_framework", "update");
  return (
    <p className="t-caption">
      {t("analytics.overview.noHandoffs")}{" "}
      {canEdit ? (
        <AppLink
          className="btn btn-link"
          href={analyticsSectionHref("definitions", new Map())}
        >
          {t("analytics.overview.changeWorkflow")}
        </AppLink>
      ) : (
        t("analytics.overview.workflowWho")
      )}
    </p>
  );
}

function weekOutcome(workflow: WorkflowState): ProspectingOutcome | undefined {
  if (workflow === "handoff") return "accepted_opportunities";
  if (workflow === "qualification") return "leads_promoted";
  return undefined;
}

// A week's columns answer their own evaluation, so the drawer reads that one.
function weeksOwn(
  reference: ReportingEvidenceRef,
  weeks: ReportingEvaluation | undefined,
): ReportingEvaluation | undefined {
  return reference.group_key?.startsWith("week:") ? weeks : undefined;
}
