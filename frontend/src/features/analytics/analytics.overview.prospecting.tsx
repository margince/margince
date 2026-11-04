// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { UseQueryResult } from "@tanstack/react-query";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import { Disclosure } from "../../design-system/disclosure";
import { ErrorLine } from "../../design-system/errorline";
import { FilterBar } from "../../design-system/filterbar";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SurfaceState } from "../../design-system/surfacestate";
import { useLocale, useT } from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { useCan, useCanWrite } from "../../lib/capability";
import { problemCodeOf } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { AssessmentPanel } from "./analytics.assessment";
import { scopeKey } from "./analytics.context";
import { useSdrSummary, WaitingOnOthers } from "./analytics.outcomes.sdr";
import { DoNext } from "./analytics.overview.donext";
import { MonthlyTargets } from "./analytics.overview.monthly";
import { useOverviewNavigation } from "./analytics.overview.navigation";
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
import { OverviewSaveActions } from "./analytics.overview.readings";
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
import { viewRequest, workflowMetrics } from "./reporting.views";

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
  const { metrics: offered, blocks: sdrBlocks } = viewRequest(
    "sdr",
    catalog,
    filters.period,
  );
  const metrics = workflowMetrics(
    offered,
    workflowTemplate(workflowOf(framework, summary)),
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
  const workflow = workflowOf(framework, summary);
  useOverviewNavigation("prospecting", selection, {
    start: filters.start,
    end: filters.end,
  });
  const query = useReportingEvaluation(
    selection,
    !!self?.id && validDates && metrics.length > 0 && workflow !== "pending",
  );
  const { evidence, evidenceFrom, openEvidence, closeEvidence } =
    useSelectionEvidence(JSON.stringify(selection));
  const weeks = useProspectingWeeks(
    query.data,
    weekOutcome(
      workflowOf(framework, summary),
      catalog.metrics.some((metric) => metric.id === "leads_qualified"),
    ),
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
  const known = workflow === "handoff" || workflow === "qualification";
  const outcome = weekOutcome(
    workflow,
    catalog.metrics.some((metric) => metric.id === "leads_qualified"),
  );
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
        <DoNext />
      </div>
      <ProspectingWork
        self={self.id}
        workflow={workflow}
        summary={summary}
        week={query.data}
        weeks={weeks}
        onEvidence={openFrom}
        outlook={
          query.data &&
          known &&
          !query.isError && (
            <AssessmentPanel
              view="prospecting"
              work={summary.data}
              workPending={summary.isLoading}
              selection={query.data.selection}
              evaluation={query.data}
              onEvidence={openFrom}
            />
          )
        }
      />
      {evidence && query.data && (
        <ReportingEvidenceDrawer
          key={JSON.stringify(evidence)}
          evaluation={evidenceFrom ?? query.data}
          reference={evidence}
          onClose={closeEvidence}
        />
      )}
    </>
  );
}

// Meetings and waiting work precede interpretation; outcome history follows it.
function ProspectingWork({
  self,
  workflow,
  summary,
  week,
  weeks,
  onEvidence,
  outlook,
}: Readonly<{
  outlook: ReactNode;
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
      {outlook}
      {workflow === "qualification" && <QualificationNote />}
      {week && (
        <div className="analytics-pair">
          <Disclosure summary={t("analytics.prospect.historyTitle")}>
            <OutcomeHistory weeks={weeks} onEvidence={onEvidence} />
          </Disclosure>
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

export function QualificationNote() {
  const t = useT();
  const canEdit = useCanWrite("reporting_framework", "update");
  return (
    <Disclosure summary={t("reporting.definitions")}>
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
    </Disclosure>
  );
}

function workflowTemplate(workflow: WorkflowState): string | undefined {
  if (workflow === "handoff") return "sdr";
  if (workflow === "qualification") return "sales";
  return undefined;
}

function weekOutcome(
  workflow: WorkflowState,
  events: boolean,
): ProspectingOutcome | undefined {
  if (workflow === "handoff") return "accepted_opportunities";
  if (workflow === "qualification")
    return events ? "leads_qualified" : "leads_promoted";
  return undefined;
}

{
  /* A week's columns answer their own evaluation, so the drawer reads that one. */
}
function weeksOwn(
  reference: ReportingEvidenceRef,
  weeks: ReportingEvaluation | undefined,
): ReportingEvaluation | undefined {
  return reference.group_key?.startsWith("week:") ? weeks : undefined;
}
