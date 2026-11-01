// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, StatCard } from "../../design-system/atoms";
import { StatStrip } from "../../design-system/statstrip";
import {
  formatDateAbbrev,
  formatDecimal,
  formatNumber,
} from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { landingBasisLines } from "./analytics.forecast.landing";
import {
  MetricReading,
  metricOf,
  readingValue,
} from "./analytics.overview.readings";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingCompactAmount,
} from "./reporting.model";

type Comparison = NonNullable<
  ReportingEvaluation["metrics"][number]["target_comparison"]
>;

// Three readings beside the lead. They are the expected finish with its basis,
// coverage of what is still to win with both operands, and the win rate.
// Every figure is the server's; the page only formats.
export function SalesReadings({
  evaluation,
  onEvidence,
  forecastHref,
}: Readonly<{
  evaluation: ReportingEvaluation;
  onEvidence: (reference: ReportingEvidenceRef) => void;
  forecastHref: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const { context } = evaluation;
  const money = (value: number | undefined) =>
    reportingCompactAmount(value, context.currency, context.currency, locale);
  return (
    <StatStrip label={t("analytics.overview.readings")} density="compact">
      <ExpectedFinish
        evaluation={evaluation}
        money={money}
        forecastHref={forecastHref}
      />
      <CoverageReading
        comparison={metricOf(evaluation, "forecast_landing")?.target_comparison}
        money={money}
      />
      {metricOf(evaluation, "closed_win_rate") ? (
        <MetricReading
          evaluation={evaluation}
          id="closed_win_rate"
          detail={t("analytics.overview.winRateBasis")}
          onEvidence={onEvidence}
        />
      ) : (
        <StatCard
          narrow="row"
          label={t("analytics.overview.winRate")}
          value={t("analytics.readingUnavailable")}
        />
      )}
    </StatStrip>
  );
}

// The landing and the two lines that say what it counts. A figure with no
// basis says it is unknown rather than borrowing one.
function ExpectedFinish({
  evaluation,
  money,
  forecastHref,
}: Readonly<{
  evaluation: ReportingEvaluation;
  money: (value: number | undefined) => string;
  forecastHref: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const landing = metricOf(evaluation, "forecast_landing");
  if (!landing)
    return (
      <StatCard
        narrow="row"
        label={t("analytics.overview.expectedFinish")}
        value={t("analytics.overview.quarterOnly")}
      />
    );
  const basis = landing.forecast_basis;
  const lines =
    landing.value == null
      ? undefined
      : landingBasisLines(basis?.landing, basis?.call_minor, money, t);
  return (
    <StatCard
      narrow="row"
      label={t("analytics.overview.expectedFinish")}
      value={readingValue(landing, evaluation, t, locale)}
      tone={lines?.warning ? "warning" : undefined}
      detail={
        lines && (
          <>
            <span>{lines.basis}</span>
            {lines.beside && <span>{lines.beside}</span>}
          </>
        )
      }
      href={landing.value != null ? forecastHref : undefined}
    />
  );
}

// Coverage of what is still to win (target minus won) by the open deals
// eligible to close in the quarter, with both operands. A state that compares
// nothing says why; the hero carries that state's explanation and its fix.
// Its numerator's evidence is one authored total, not the deals behind it, so
// the reading offers no records door until the server lists those deals.
function CoverageReading({
  comparison,
  money,
}: Readonly<{
  comparison: Comparison | undefined;
  money: (value: number | undefined) => string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  if (comparison?.status === "target_reached")
    return (
      <StatCard
        narrow="row"
        label={t("analytics.overview.coverage")}
        value={t("reporting.compare.reached")}
        detail={t("reporting.compare.remainingBasis", {
          won: money(comparison.won),
          target: money(comparison.target),
        })}
      />
    );
  if (comparison?.status !== "compared" || comparison.coverage == null)
    return (
      <NotCompared
        detail={t(
          comparison
            ? "analytics.overview.coverageNotCompared"
            : "analytics.overview.quarterOnly",
        )}
      />
    );
  return (
    <StatCard
      narrow="row"
      label={t("analytics.overview.coverage")}
      value={t("reporting.compare.ratio", {
        ratio: formatDecimal(comparison.coverage, locale, 1),
      })}
      detail={t("reporting.compare.coverageBasis", {
        open: money(comparison.open_pipeline),
        remaining: money(comparison.remaining),
      })}
    />
  );
}

function NotCompared({ detail }: Readonly<{ detail: string }>) {
  const t = useT();
  return (
    <StatCard
      narrow="row"
      label={t("analytics.overview.coverage")}
      value={t("analytics.overview.notCompared")}
      detail={detail}
    />
  );
}

/**
 * All open pipeline as the secondary reading over the stock charts. It is
 * today's figure, dated, with what the reader may not see and what its source
 * lacks. It is never the coverage numerator, which counts only deals eligible
 * to close in the quarter.
 */
export function OpenPipelineLine({
  evaluation,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { context } = evaluation;
  const open = metricOf(evaluation, "open_pipeline");
  if (!open) return null;
  const at = formatDateAbbrev(context.state_at, locale, context.timezone);
  const count = open.coverage.eligible_count;
  const reason = open.coverage.status !== "ok" && open.coverage.reason;
  return (
    <p className="analytics-open-line">
      <span className="analytics-attention-title">
        {t(
          context.close_interval
            ? "analytics.overview.openClosing"
            : "analytics.overview.allOpen",
        )}
        : {readingValue(open, evaluation, t, locale)}
      </span>{" "}
      <span className="t-caption">
        {[
          count == null
            ? t("reporting.openPipelineAsOf", { at })
            : plural("reporting.openPipelineBasis", count, {
                count: formatNumber(count, locale),
                at,
              }),
          open.coverage.withheld && t("analytics.overview.partlyHidden"),
          reason,
        ]
          .filter(Boolean)
          .join(" · ")}
      </span>{" "}
      <Button variant="link" onClick={() => onEvidence(open.evidence)}>
        {t("reporting.viewRecords")}
      </Button>
    </p>
  );
}
