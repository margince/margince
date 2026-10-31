// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { Button, StatCard } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { StatStrip } from "../../design-system/statstrip";
import { formatDateAbbrev, formatNumber } from "../../format/format";
import { type Translator, useLocale, usePlural, useT } from "../../i18n";
import {
  OutcomeWeeks,
  outcomeWeeks,
  type ProspectingOutcome,
  type useProspectingWeeks,
} from "./analytics.overview.prospectingweeks";
import { metricOf, readingValue } from "./analytics.overview.readings";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingPeriodLabel,
} from "./reporting.model";
import { useMetricNames } from "./reporting.queries";

type Metric = ReportingEvaluation["metrics"][number];
type SdrSummary = components["schemas"]["SdrSummary"];

/** The element the upcoming reading moves focus to: the meetings it counts. */
export const UPCOMING_LIST_ID = "analytics-upcoming-meetings";

// What a reading's figure is decides the words beside it. It is a measured
// zero, a figure hidden from this reader, a failed source, or a value.
export type ReadingState = "zero" | "withheld" | "failed" | "value" | "other";

export function readingState(metric: Metric | undefined): ReadingState {
  if (!metric || metric.coverage.status === "failed") return "failed";
  if (metric.value == null && metric.coverage.withheld) return "withheld";
  if (
    metric.value === 0 ||
    (metric.value == null &&
      metric.coverage.status === "no_data" &&
      metric.unit === "count")
  )
    return "zero";
  return metric.value == null ? "other" : "value";
}

// The figure as the page shows it. Hidden and failed figures each have their
// own word, so neither reads as the other or as a zero.
function figureText(
  metric: Metric | undefined,
  evaluation: ReportingEvaluation,
  t: Translator,
  locale: Parameters<typeof readingValue>[3],
): string {
  const state = readingState(metric);
  if (state === "withheld") return t("analytics.prospect.withheld");
  if (state === "failed") return t("analytics.prospect.failed");
  if (metric?.coverage.status === "insufficient_sample")
    return t("analytics.prospect.tooFew");
  return readingValue(metric, evaluation, t, locale);
}

/**
 * The reader's own results for the period: the outcome their workflow counts,
 * in display type, beside the weeks that led to it. Which outcome counts comes
 * from the configured workflow; while it is unknown the page names none.
 */
export function ProspectingResults({
  evaluation,
  outcome,
  workflowState,
  weeks,
  onEvidence,
  onRetryWorkflow,
  aside,
}: Readonly<{
  evaluation: ReportingEvaluation;
  outcome: ProspectingOutcome | undefined;
  // Under the figure: the month's targets, dated as their own month.
  aside?: ReactNode;
  workflowState: "known" | "pending" | "failed";
  weeks: ReturnType<typeof useProspectingWeeks>;
  onEvidence: (reference: ReportingEvidenceRef) => void;
  onRetryWorkflow: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const { context } = evaluation;
  const period = reportingPeriodLabel(
    context.interval,
    context.timezone,
    locale,
  );
  const metric = outcome ? metricOf(evaluation, outcome) : undefined;
  const chart = outcome ? outcomeWeeks(weeks.data, outcome) : undefined;
  const opens =
    metric?.value != null && readingState(metric) !== "failed" && !!outcome;
  return (
    <Panel
      title={t("analytics.prospect.results")}
      flatHead
      titleAction={
        opens &&
        metric && (
          <Button variant="link" onClick={() => onEvidence(metric.evidence)}>
            {t("reporting.viewRecords")}
          </Button>
        )
      }
    >
      <PanelBody>
        <div className="analytics-hero-body">
          <div className="analytics-hero-figure">
            {outcome ? (
              <OutcomeFigure
                outcome={outcome}
                evaluation={evaluation}
                period={period}
              />
            ) : (
              <WorkflowUnknown
                state={workflowState}
                onRetry={onRetryWorkflow}
              />
            )}
            {aside}
          </div>
          {chart && (
            <div className="analytics-hero-chart">
              <OutcomeWeeks
                chart={chart}
                evaluation={weeks.data ?? evaluation}
                onEvidence={onEvidence}
              />
            </div>
          )}
          {outcome && !chart && weeks.isError && (
            <p className="t-caption">{t("analytics.prospect.weeksFailed")}</p>
          )}
        </div>
      </PanelBody>
    </Panel>
  );
}

// The outcome's figure and what it counts. With no lead arrived there is no
// cohort to count, as the week's column says too, so no figure stands for one.
function OutcomeFigure({
  outcome,
  evaluation,
  period,
}: Readonly<{
  outcome: ProspectingOutcome;
  evaluation: ReportingEvaluation;
  period: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const metricName = useMetricNames();
  const metric = metricOf(evaluation, outcome);
  const noCohort = outcome === "leads_promoted" && metric?.denominator === 0;
  let basis = t("analytics.prospect.basisAccepted", { period });
  if (noCohort) basis = t("analytics.prospect.noArrivals", { period });
  else if (outcome === "leads_promoted" && metric?.denominator != null)
    basis = t("analytics.prospect.basisQualified", {
      arrived: formatNumber(metric.denominator, locale),
      period,
    });
  return (
    <>
      <p className="t-label">{metricName(outcome)}</p>
      <p className="analytics-hero-value t-num">
        {noCohort ? "—" : figureText(metric, evaluation, t, locale)}
      </p>
      <p className="t-caption">{basis}</p>
      {metric?.coverage.withheld && metric.value != null && (
        <p className="t-caption">{t("analytics.overview.partlyHidden")}</p>
      )}
    </>
  );
}

// Which outcome counts is not known yet, or could not be read. The page says
// so and offers the read again instead of guessing a workflow.
function WorkflowUnknown({
  state,
  onRetry,
}: Readonly<{ state: "known" | "pending" | "failed"; onRetry: () => void }>) {
  const t = useT();
  if (state === "pending")
    return <p className="t-caption">{t("analytics.readingLoading")}</p>;
  return (
    <>
      <p className="analytics-hero-value">{t("analytics.prospect.failed")}</p>
      <p className="t-caption">{t("analytics.overview.workflowFailed")}</p>
      <Button onClick={onRetry}>{t("common.retry")}</Button>
    </>
  );
}

/**
 * The readings that support the outcome: meetings held, and how fast leads are
 * answered beside how many still wait. The meetings booked ahead come last,
 * and are never counted as held.
 */
export function ProspectingReadings({
  evaluation,
  summary,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  summary: Readonly<{ data?: SdrSummary; isError: boolean }>;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const metricName = useMetricNames();
  const held = metricOf(evaluation, "meetings_held");
  const response = metricOf(evaluation, "lead_first_response");
  const heldState = readingState(held);
  const week = evaluation.context.period_kind === "this_week";
  return (
    <StatStrip label={t("analytics.overview.readings")}>
      <StatCard
        narrow="row"
        label={metricName("meetings_held")}
        value={figureText(held, evaluation, t, locale)}
        detail={
          heldState === "zero"
            ? t(
                week
                  ? "analytics.prospect.noMeetingsWeek"
                  : "analytics.prospect.noMeetingsPeriod",
              )
            : heldState === "failed"
              ? t("analytics.prospect.failedDetail")
              : undefined
        }
        // A zero opens its empty population, so the reader sees where held
        // meetings are counted from.
        onOpen={
          held && (heldState === "value" || heldState === "zero")
            ? () => onEvidence(held.evidence)
            : undefined
        }
      />
      <StatCard
        narrow="row"
        label={metricName("lead_first_response")}
        value={figureText(response, evaluation, t, locale)}
        detail={
          <>
            <span>{unansweredText(evaluation, t, plural, locale)}</span>
            {response?.coverage.status === "insufficient_sample" && (
              <span>{t("analytics.prospect.tooFewResponses")}</span>
            )}
          </>
        }
        onOpen={
          response?.value != null
            ? () => onEvidence(response.evidence)
            : undefined
        }
      />
      <UpcomingReading summary={summary} />
    </StatStrip>
  );
}

// The unanswered leads always stand beside the response time: a count, or the
// reason there is none. A median stays shown when only this count failed.
function unansweredText(
  evaluation: ReportingEvaluation,
  t: Translator,
  plural: ReturnType<typeof usePlural>,
  locale: Parameters<typeof formatNumber>[1],
): string {
  const waiting = metricOf(evaluation, "leads_awaiting_response");
  const state = readingState(waiting);
  if (state === "withheld") return t("analytics.prospect.unansweredWithheld");
  if (state === "failed" || state === "other" || !waiting)
    return t("analytics.prospect.unansweredFailed");
  const count = waiting.value ?? 0;
  return plural("analytics.overview.stillWaiting", count, {
    count: formatNumber(count, locale),
  });
}

// Meetings booked ahead, in the window the server declares for both the count
// and the list behind it.
function UpcomingReading({
  summary,
}: Readonly<{ summary: Readonly<{ data?: SdrSummary; isError: boolean }> }>) {
  const t = useT();
  const { locale } = useLocale();
  const label = t("analytics.prospect.upcoming");
  const data = summary.data;
  if (summary.isError)
    return (
      <StatCard
        narrow="row"
        label={label}
        value={t("analytics.prospect.failed")}
        detail={t("analytics.prospect.failedDetail")}
      />
    );
  if (!data)
    return (
      <StatCard
        narrow="row"
        label={label}
        value={t("analytics.readingLoading")}
      />
    );
  const window = data.upcoming_window;
  if (data.upcoming_meetings == null || !window)
    return (
      <StatCard
        narrow="row"
        label={label}
        value={t("analytics.prospect.withheld")}
      />
    );
  return (
    <StatCard
      narrow="row"
      label={label}
      value={formatNumber(data.upcoming_meetings, locale)}
      detail={t("analytics.prospect.upcomingThrough", {
        date: formatDateAbbrev(
          new Date(Date.parse(window.end_at) - 1).toISOString(),
          locale,
          window.timezone,
        ),
      })}
      onOpen={() => document.getElementById(UPCOMING_LIST_ID)?.focus()}
    />
  );
}
