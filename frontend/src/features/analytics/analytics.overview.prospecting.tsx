// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { UseQueryResult } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import type { components } from "../../api/schema";
import { Button, StatCard } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { FilterBar } from "../../design-system/filterbar";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { StatStrip } from "../../design-system/statstrip";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { openAnalyticsSection } from "../../lib/analytics.address";
import { useCan, useCanWrite } from "../../lib/capability";
import { problemCodeOf } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import {
  NeedsYourAction,
  useSdrSummary,
  WaitingOnOthers,
} from "./analytics.outcomes.sdr";
import {
  MetricReading,
  metricOf,
  OverviewLinks,
} from "./analytics.overview.readings";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import { ReportingFilters } from "./reporting.filters";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  type ReportingSelection,
  reportingCompactAmount,
  reportingPeriodLabel,
} from "./reporting.model";
import {
  customInterval,
  resultsThrough,
  useOverviewFilters,
  validDateRange,
} from "./reporting.overview";
import { useMetricNames, useReportingEvaluation } from "./reporting.queries";
import { viewRequest } from "./reporting.views";
import { OpeningSummary, prospectingSummary } from "./reporting.written";

type Catalog = components["schemas"]["ReportingCatalog"];
type AnalyticsContext = components["schemas"]["AnalyticsContext"];
type Framework = components["schemas"]["ReportingFramework"];
type Workflow = "handoff" | "qualification";

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
  const [evidence, setEvidence] = useState<ReportingEvidenceRef | null>(null);
  const readsLeads = useCan("lead", "read");
  const summary = useSdrSummary(self?.id ?? "", !!self?.id && readsLeads);
  const { metrics } = viewRequest("sdr", catalog, filters.period);
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
  const workflow: Workflow = framework.data
    ? framework.data.definition.template === "sdr"
      ? "handoff"
      : "qualification"
    : (summary.data?.workflow ?? "handoff");
  return (
    <>
      <FilterBar
        label={t("analytics.filters")}
        caption={
          validDates
            ? resultsThrough(query.data, locale, t)
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
              setEvidence(null);
            },
          }}
          onChange={(next) => {
            if (next.period !== filters.period) filters.setPeriod(next.period);
            setEvidence(null);
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
                onEvidence={setEvidence}
              />
              <OpeningSummary
                sentences={prospectingSummary(
                  evaluation,
                  workflow,
                  summary.data?.handoffs?.waiting,
                  { t, plural, locale },
                )}
              />
              <MonthlyTargets evaluation={evaluation} />
              {evidence && (
                <ReportingEvidenceDrawer
                  key={JSON.stringify(evidence)}
                  evaluation={evaluation}
                  reference={evidence}
                  onClose={() => setEvidence(null)}
                />
              )}
            </>
          )}
        </QueryGate>
      )}
      <NeedsYourAction locale={locale} />
      {workflow === "handoff" ? (
        <Waiting self={self.id} summary={summary} />
      ) : (
        <QualificationNote />
      )}
      <OverviewLinks
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
  workflow: Workflow;
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
      {workflow === "handoff" ? (
        <>
          <MetricReading
            evaluation={evaluation}
            id="accepted_opportunities"
            detail={period}
            onEvidence={onEvidence}
          />
          {qualifiedReading}
        </>
      ) : (
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

// This month's targets, named by their own month so a week's figure is never
// read against a month's target.
function MonthlyTargets({
  evaluation,
}: Readonly<{ evaluation: ReportingEvaluation }>) {
  const t = useT();
  const { locale } = useLocale();
  const metricName = useMetricNames();
  const readsTargets = useCan("sales_target", "read");
  const canSet = useCanWrite("sales_target", "create");
  const { context } = evaluation;
  if (!readsTargets || !context.target_interval) return null;
  const month = reportingPeriodLabel(
    context.target_interval,
    context.timezone,
    locale,
  );
  const held = evaluation.metrics.filter(
    (metric) =>
      metric.target != null && metric.target > 0 && !metric.coverage.withheld,
  );
  const amount = (value: number, unit: string) =>
    reportingCompactAmount(value, unit, context.currency, locale);
  return (
    <Panel title={t("analytics.overview.monthlyTitle", { month })}>
      <PanelBody>
        {held.length === 0 ? (
          <Callout
            tone="info"
            kind="standing"
            title={t("analytics.overview.noMonthlyTarget")}
            actions={
              canSet ? (
                <Button onClick={() => openAnalyticsSection("targets")}>
                  {t("analytics.overview.setTarget")}
                </Button>
              ) : undefined
            }
          >
            <p>
              {t(
                canSet
                  ? "analytics.overview.setMonthlyTargetBody"
                  : "analytics.overview.targetWho",
              )}
            </p>
          </Callout>
        ) : (
          <StatStrip label={t("analytics.overview.monthlyTitle", { month })}>
            {held.map((metric) => {
              const target = metric.target ?? 0;
              const actual = metric.target_actual ?? 0;
              return (
                <StatCard
                  key={metric.id}
                  narrow="row"
                  label={metricName(metric.id)}
                  value={t("analytics.overview.ofTarget", {
                    actual: amount(actual, metric.unit),
                    target: amount(target, metric.unit),
                  })}
                  detail={
                    actual >= target
                      ? t("reporting.compare.reached")
                      : t("analytics.overview.moreToTarget", {
                          amount: amount(target - actual, metric.unit),
                        })
                  }
                  meter={{ filled: actual, total: target }}
                />
              );
            })}
          </StatStrip>
        )}
      </PanelBody>
    </Panel>
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
function QualificationNote() {
  const t = useT();
  const canEdit = useCanWrite("reporting_framework", "update");
  return (
    <p className="t-caption">
      {t("analytics.overview.noHandoffs")}{" "}
      {canEdit ? (
        <Button
          variant="link"
          onClick={() => openAnalyticsSection("definitions")}
        >
          {t("analytics.overview.changeWorkflow")}
        </Button>
      ) : (
        t("analytics.overview.workflowWho")
      )}
    </p>
  );
}
