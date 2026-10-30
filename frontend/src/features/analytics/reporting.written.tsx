// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  formatDateAbbrev,
  formatDecimal,
  formatNumber,
} from "../../format/format";
import { useLocale, usePlural, useT, type Words } from "../../i18n";
import {
  type ReportingEvaluation,
  reportingCompactAmount,
  reportingPeriodLabel,
} from "./reporting.model";

type Metric = ReportingEvaluation["metrics"][number];

// The position in a few sentences, written from a fixed template over the
// figures this evaluation already carries. It adds no total of its own, and a
// figure that was not measured is left out rather than guessed.
export function WrittenSummary({
  evaluation,
}: Readonly<{ evaluation: ReportingEvaluation }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const sentences = writtenSummary(evaluation, { t, plural, locale });
  if (sentences.length === 0) {
    return null;
  }
  return <p className="reporting-written">{sentences.join(" ")}</p>;
}

export function writtenSummary(
  evaluation: ReportingEvaluation,
  words: Words,
): string[] {
  const metric = (id: Metric["id"]) => {
    const found = evaluation.metrics.find((candidate) => candidate.id === id);
    // A figure part of which the reader may not see would be stated as the
    // whole, so it is left out.
    return found?.coverage.withheld ? undefined : found;
  };
  const won = metric("bookings_won");
  return [
    wonSentence(evaluation, won, words),
    targetSentence(evaluation, won, words),
    landingSentence(evaluation, metric("forecast_landing"), words),
    openSentence(evaluation, metric("open_pipeline"), words),
  ].filter((sentence): sentence is string => sentence !== null);
}

function amount(
  evaluation: ReportingEvaluation,
  metric: Metric,
  value: number,
  words: Words,
): string {
  return reportingCompactAmount(
    value,
    metric.unit,
    evaluation.context.currency,
    words.locale,
  );
}

// The day a state figure was read. A saved edition reads it on its capture
// day, so the sentence says which day rather than calling it current.
function stateDay(evaluation: ReportingEvaluation, words: Words): string {
  const { context } = evaluation;
  return formatDateAbbrev(context.state_at, words.locale, context.timezone);
}

function wonSentence(
  evaluation: ReportingEvaluation,
  won: Metric | undefined,
  words: Words,
): string | null {
  if (!won) {
    return null;
  }
  const { context } = evaluation;
  const period = reportingPeriodLabel(
    context.interval,
    context.timezone,
    words.locale,
  );
  if (won.coverage.status === "no_data") {
    // An empty scope is told by the notice; "no sales won" is for a scope
    // that holds deals.
    if (context.scope_holds_deals === false) {
      return null;
    }
    return words.t("reporting.writtenNoneWon", { period });
  }
  if (won.coverage.status !== "ok" || won.value == null) {
    return null;
  }
  return words.t("reporting.writtenWon", {
    amount: amount(evaluation, won, won.value, words),
    period,
  });
}

// The target has its own period and its own actual. Both can differ from the
// period the page shows, so the target is a sentence of its own.
function targetSentence(
  evaluation: ReportingEvaluation,
  won: Metric | undefined,
  words: Words,
): string | null {
  const { context } = evaluation;
  const target = won?.target;
  const actual = won?.target_actual;
  if (
    won?.coverage.status !== "ok" ||
    target == null ||
    target <= 0 ||
    actual == null ||
    !context.target_interval
  ) {
    return null;
  }
  return words.t("reporting.writtenTarget", {
    targetPeriod: reportingPeriodLabel(
      context.target_interval,
      context.timezone,
      words.locale,
    ),
    target: amount(evaluation, won, target, words),
    actual: amount(evaluation, won, actual, words),
    percent: formatNumber(Math.round((actual / target) * 100), words.locale),
  });
}

function landingSentence(
  evaluation: ReportingEvaluation,
  landing: Metric | undefined,
  words: Words,
): string | null {
  if (landing?.coverage.status !== "ok" || landing.value == null) {
    return null;
  }
  return words.t("reporting.writtenLanding", {
    date: stateDay(evaluation, words),
    amount: amount(evaluation, landing, landing.value, words),
  });
}

function openSentence(
  evaluation: ReportingEvaluation,
  open: Metric | undefined,
  words: Words,
): string | null {
  const count = open?.coverage.eligible_count;
  if (open?.coverage.status !== "ok" || open.value == null || count == null) {
    return null;
  }
  return words.plural("reporting.writtenOpen", count, {
    date: stateDay(evaluation, words),
    amount: amount(evaluation, open, open.value, words),
    count: formatNumber(count, words.locale),
  });
}

type Comparison = NonNullable<Metric["target_comparison"]>;

// The headline figures each opening view states. A figure among them that the
// server could not calculate turns into a sentence that says so, and nothing
// is concluded from it.
const SALES_FIGURES: readonly Metric["id"][] = [
  "bookings_won",
  "forecast_landing",
  "open_pipeline",
];

/**
 * Sales review in two or three sentences. They say where the quarter is heading
 * against its target, what is won, and how far the open deals cover the rest.
 * The comparison is the server's; no gap is worked out here.
 */
export function salesReviewSummary(
  evaluation: ReportingEvaluation,
  words: Words,
): string[] {
  const metric = (id: Metric["id"]) => readable(evaluation, id);
  const landing = metric("forecast_landing");
  const comparison = landing?.target_comparison;
  return [
    failedSentence(evaluation, SALES_FIGURES, words),
    positionSentence(evaluation, landing, comparison, words),
    wonSentence(evaluation, metric("bookings_won"), words),
    comparison?.status === "compared" && landing?.coverage.status === "ok"
      ? coverageSentence(evaluation, comparison, words)
      : openSentence(evaluation, metric("open_pipeline"), words),
  ].filter((sentence): sentence is string => sentence !== null);
}

/**
 * My prospecting in a few sentences. They name the outcome that counts for this
 * workflow, the leads waiting for a reply, and the handoffs waiting on others.
 * A count the summary has not read yet is left out.
 */
export function prospectingSummary(
  evaluation: ReportingEvaluation,
  workflow: "handoff" | "qualification",
  handoffsWaiting: number | undefined,
  words: Words,
): string[] {
  const outcome =
    workflow === "handoff" ? "accepted_opportunities" : "leads_promoted";
  const period = reportingPeriodLabel(
    evaluation.context.interval,
    evaluation.context.timezone,
    words.locale,
  );
  return [
    failedSentence(
      evaluation,
      [outcome, "meetings_held", "leads_awaiting_response"],
      words,
    ),
    countSentence(evaluation, outcome, period, words),
    countSentence(evaluation, "meetings_held", period, words),
    waitingLeadsSentence(evaluation, words),
    handoffsWaiting != null && handoffsWaiting > 0
      ? words.plural("reporting.writtenHandoffsWaiting", handoffsWaiting, {
          count: formatNumber(handoffsWaiting, words.locale),
        })
      : null,
  ].filter((sentence): sentence is string => sentence !== null);
}

export function OpeningSummary({
  sentences,
}: Readonly<{ sentences: readonly string[] }>) {
  if (sentences.length === 0) return null;
  return <p className="reporting-written">{sentences.join(" ")}</p>;
}

// A figure the reader sees in full. Part of one would be stated as the whole.
function readable(
  evaluation: ReportingEvaluation,
  id: Metric["id"],
): Metric | undefined {
  const found = evaluation.metrics.find((candidate) => candidate.id === id);
  return found?.coverage.withheld ? undefined : found;
}

function failedSentence(
  evaluation: ReportingEvaluation,
  ids: readonly Metric["id"][],
  words: Words,
): string | null {
  const failed = evaluation.metrics.some(
    (metric) => ids.includes(metric.id) && metric.coverage.status === "failed",
  );
  return failed ? words.t("reporting.writtenSomeFailed") : null;
}

function positionSentence(
  evaluation: ReportingEvaluation,
  landing: Metric | undefined,
  comparison: Comparison | undefined,
  words: Words,
): string | null {
  const { context } = evaluation;
  const quarter = context.target_interval
    ? reportingPeriodLabel(
        context.target_interval,
        context.timezone,
        words.locale,
      )
    : undefined;
  if (!landing || !quarter) return null;
  const money = (value: number | undefined) =>
    value == null ? null : amount(evaluation, landing, value, words);
  if (comparison?.status === "target_reached") {
    return words.t("reporting.writtenTargetReached", {
      quarter,
      target: money(comparison.target) ?? "",
    });
  }
  if (comparison?.status === "no_target") {
    return words.t("reporting.writtenNoTarget", { quarter });
  }
  if (landing.coverage.status !== "ok" || landing.value == null) return null;
  const finish = amount(evaluation, landing, landing.value, words);
  const gap = comparison?.gap;
  const target = money(comparison?.target);
  if (comparison?.status !== "compared" || gap == null || target == null) {
    return words.t("reporting.writtenFinish", { quarter, amount: finish });
  }
  const key =
    gap > 0
      ? "reporting.writtenFinishShort"
      : gap < 0
        ? "reporting.writtenFinishAhead"
        : "reporting.writtenFinishOnTarget";
  return words.t(key, {
    quarter,
    amount: finish,
    gap: money(Math.abs(gap)) ?? "",
    target,
  });
}

function coverageSentence(
  evaluation: ReportingEvaluation,
  comparison: Comparison,
  words: Words,
): string | null {
  if (comparison.coverage == null || comparison.remaining == null) return null;
  return words.t("reporting.writtenCoverage", {
    ratio: formatDecimal(comparison.coverage, words.locale, 1),
    remaining: reportingCompactAmount(
      comparison.remaining,
      evaluation.context.currency,
      evaluation.context.currency,
      words.locale,
    ),
  });
}

const COUNT_SENTENCE = {
  accepted_opportunities: "reporting.writtenAccepted",
  leads_promoted: "reporting.writtenQualified",
  meetings_held: "reporting.writtenMeetings",
} as const;

// A count over the period. Zero is a measured answer and is said as one; a
// figure that was not measured is left out.
function countSentence(
  evaluation: ReportingEvaluation,
  id: "accepted_opportunities" | "leads_promoted" | "meetings_held",
  period: string,
  words: Words,
): string | null {
  const metric = readable(evaluation, id);
  if (!metric) return null;
  const counted =
    metric.coverage.status === "no_data"
      ? 0
      : metric.coverage.status === "ok"
        ? metric.value
        : null;
  if (counted == null) return null;
  if (counted === 0) return words.t(`${COUNT_SENTENCE[id]}None`, { period });
  return words.plural(COUNT_SENTENCE[id], counted, {
    count: formatNumber(counted, words.locale),
    period,
  });
}

function waitingLeadsSentence(
  evaluation: ReportingEvaluation,
  words: Words,
): string | null {
  const waiting = readable(evaluation, "leads_awaiting_response");
  if (waiting?.coverage.status !== "ok" || !waiting.value) return null;
  return words.plural("reporting.writtenLeadsWaiting", waiting.value, {
    count: formatNumber(waiting.value, words.locale),
  });
}
