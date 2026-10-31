// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { UseQueryResult } from "@tanstack/react-query";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import { StatCard } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { FilterBar } from "../../design-system/filterbar";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { StatStrip } from "../../design-system/statstrip";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { useCan, useCanWrite } from "../../lib/capability";
import { problemCodeOf } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { scopeKey } from "./analytics.context";
import {
  NeedsYourAction,
  useSdrSummary,
  WaitingOnOthers,
} from "./analytics.outcomes.sdr";
import { outboundDials } from "./analytics.overview.context";
import { MonthlyTargets } from "./analytics.overview.monthly";
import {
  MetricReading,
  metricOf,
  OverviewLinks,
  OverviewSaveActions,
} from "./analytics.overview.readings";
import { periodCaption } from "./analytics.period";
import {
  ReportingEvidenceDrawer,
  useSelectionEvidence,
} from "./reporting.evidence";
import { ReportingFilters } from "./reporting.filters";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  type ReportingSelection,
  reportingPeriodLabel,
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
  const plural = usePlural();
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
  const { evidence, openEvidence, closeEvidence } = useSelectionEvidence(
    JSON.stringify(selection),
  );
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
  return (
    <>
      <FilterBar
        label={t("analytics.filters")}
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
      {problemCodeOf(query.error) === "reporting_interval_invalid" ? (
        <ErrorLine error={query.error} />
      ) : (
        <QueryGate
          query={query}
          pendingLabel={t("analytics.overview.prospecting")}
        >
          {(evaluation) => (
            <>
              <ProspectingReadings
                evaluation={evaluation}
                workflow={workflow}
                upcoming={summary.data?.upcoming_meetings}
                onEvidence={openEvidence}
              />
              <OpeningSummary
                sentences={prospectingSummary(
                  evaluation,
                  known ? workflow : undefined,
                  summary.data?.handoffs?.waiting,
                  { t, plural, locale },
                )}
              />
              <MonthlyTargets
                evaluation={evaluation}
                ownScope={scopeKey(self)}
              />
              {evidence && (
                <ReportingEvidenceDrawer
                  key={JSON.stringify(evidence)}
                  evaluation={evaluation}
                  reference={evidence}
                  onClose={closeEvidence}
                />
              )}
            </>
          )}
        </QueryGate>
      )}
      <NeedsYourAction locale={locale} />
      {workflow === "handoff" && <Waiting self={self.id} summary={summary} />}
      {workflow === "qualification" && <QualificationNote />}
      {workflow === "failed" && (
        <WorkflowFailed
          onRetry={() => {
            void framework.refetch();
            void summary.refetch();
          }}
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

// The four readings. Without SDR handoffs the qualified leads are the outcome,
// and the meetings booked ahead take the freed place.
function ProspectingReadings({
  evaluation,
  workflow,
  upcoming,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  workflow: WorkflowState;
  upcoming: number | undefined;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { context } = evaluation;
  const period = reportingPeriodLabel(
    context.interval,
    context.timezone,
    locale,
  );
  const qualified = metricOf(evaluation, "leads_promoted");
  const waiting = metricOf(evaluation, "leads_awaiting_response");
  const qualifiedReading = (
    <MetricReading
      evaluation={evaluation}
      id="leads_promoted"
      detail={
        qualified?.denominator
          ? t("reporting.outOf", {
              total: formatNumber(qualified.denominator, locale),
            })
          : period
      }
      onEvidence={onEvidence}
    />
  );
  return (
    <StatStrip label={t("analytics.overview.readings")}>
      <MetricReading
        evaluation={evaluation}
        id="meetings_held"
        detail={period}
        onEvidence={onEvidence}
      />
      {workflow === "handoff" && (
        <>
          <MetricReading
            evaluation={evaluation}
            id="accepted_opportunities"
            detail={period}
            onEvidence={onEvidence}
          />
          {qualifiedReading}
        </>
      )}
      {workflow === "qualification" && (
        <>
          {qualifiedReading}
          <StatCard
            narrow="row"
            label={t("analytics.sdrUpcomingMeetings")}
            value={
              upcoming == null
                ? t("analytics.readingLoading")
                : formatNumber(upcoming, locale)
            }
            detail={t("analytics.sdrUpcomingMeetingsDetail")}
          />
        </>
      )}
      {(workflow === "pending" || workflow === "failed") && (
        <>
          {qualifiedReading}
          {/* Accepted handoffs or meetings ahead: which one counts here is
              not known yet, so neither is shown in its place. */}
          <StatCard
            narrow="row"
            label={t("analytics.overview.mainOutcome")}
            value={t(
              workflow === "pending"
                ? "analytics.readingLoading"
                : "analytics.readingUnavailable",
            )}
            detail={
              workflow === "failed"
                ? t("analytics.overview.workflowFailed")
                : undefined
            }
          />
        </>
      )}
      <MetricReading
        evaluation={evaluation}
        id="lead_first_response"
        detail={
          waiting?.coverage.status === "ok" && waiting.value != null
            ? plural("analytics.overview.stillWaiting", waiting.value, {
                count: formatNumber(waiting.value, locale),
              })
            : undefined
        }
        onEvidence={onEvidence}
      />
    </StatStrip>
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

// Neither the framework nor the summary said which workflow applies, so the
// page shows no handoff panel and no qualification note in its place.
function WorkflowFailed({ onRetry }: Readonly<{ onRetry: () => void }>) {
  const t = useT();
  return (
    <Panel title={t("analytics.overview.workflowTitle")}>
      <PanelBody>
        <SurfaceState
          state="failed"
          emptyLabel={t("common.empty")}
          loadingLabel={t("analytics.overview.workflowTitle")}
          detail={{ onRetry }}
        >
          {null}
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}
