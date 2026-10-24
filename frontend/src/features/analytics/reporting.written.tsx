// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatDateAbbrev, formatNumber } from "../../format/format";
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
    !won ||
    won.coverage.status !== "ok" ||
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
  if (!landing || landing.coverage.status !== "ok" || landing.value == null) {
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
  if (
    !open ||
    open.coverage.status !== "ok" ||
    open.value == null ||
    count == null
  ) {
    return null;
  }
  return words.plural("reporting.writtenOpen", count, {
    date: stateDay(evaluation, words),
    amount: amount(evaluation, open, open.value, words),
    count: formatNumber(count, words.locale),
  });
}
