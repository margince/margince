// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatNumber } from "../../format/format";
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
  const metric = (id: Metric["id"]) =>
    evaluation.metrics.find((candidate) => candidate.id === id);
  return [
    wonSentence(evaluation, metric("bookings_won"), words),
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
    return words.t("reporting.writtenNoneWon", { period });
  }
  if (won.coverage.status !== "ok" || won.value == null) {
    return null;
  }
  const target = won.target;
  const actual = won.target_actual;
  if (target != null && target > 0 && actual != null) {
    return words.t("reporting.writtenWonTarget", {
      amount: amount(evaluation, won, won.value, words),
      period,
      target: amount(evaluation, won, target, words),
      percent: formatNumber(Math.round((actual / target) * 100), words.locale),
    });
  }
  return words.t("reporting.writtenWon", {
    amount: amount(evaluation, won, won.value, words),
    period,
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
    amount: amount(evaluation, open, open.value, words),
    count: formatNumber(count, words.locale),
  });
}
