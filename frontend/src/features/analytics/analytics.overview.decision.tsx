// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { AppLink } from "../../design-system/applink";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { SegmentBar } from "../../design-system/readings";
import { StatStrip } from "../../design-system/statstrip";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { ModelRangeCard, rangeOf } from "./analytics.forecast.landing";
import { INTERVENTIONS_ID } from "./analytics.overview.interventions";
import { metricOf } from "./analytics.overview.readings";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingCompactAmount,
} from "./reporting.model";
import {
  type ChecksReading,
  type LinkedSentence,
  salesReviewSummary,
} from "./reporting.written";

type Basis = NonNullable<
  ReportingEvaluation["metrics"][number]["forecast_basis"]
>;

/**
 * What the quarter's figures mean, in sentences that each open what they rest
 * on. Below them, what the expected finish is made of and the modelled finish.
 */
export function DecisionSummary({
  evaluation,
  checks,
  forecastHref,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  checks: ChecksReading;
  forecastHref: string;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const sentences = salesReviewSummary(
    evaluation,
    { t, plural, locale },
    checks,
  );
  return (
    <Panel title={t("analytics.overview.summary")} flatHead>
      <PanelBody>
        <ul className="analytics-decision">
          {sentences.map((sentence) => (
            <li key={sentence.text}>
              <Sentence
                sentence={sentence}
                forecastHref={forecastHref}
                onEvidence={onEvidence}
              />
            </li>
          ))}
        </ul>
        <Composition evaluation={evaluation} />
        <ModelledFinish evaluation={evaluation} />
      </PanelBody>
    </Panel>
  );
}

function Sentence({
  sentence,
  forecastHref,
  onEvidence,
}: Readonly<{
  sentence: LinkedSentence;
  forecastHref: string;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const { text, link } = sentence;
  if (!link) return text;
  if (link.kind === "forecast")
    return <AppLink href={forecastHref}>{text}</AppLink>;
  return (
    <Button
      variant="link"
      className="analytics-decision-link"
      onClick={() =>
        link.kind === "evidence"
          ? onEvidence(link.reference)
          : document.getElementById(INTERVENTIONS_ID)?.focus()
      }
    >
      {text}
    </Button>
  );
}

// The expected finish as its two parts, drawn only when both are money of one
// basis: won plus the forward half of the same measure. A manager's call is
// one authored total with no parts. A caveat means the plain sum does not
// hold. Neither draws a bar. The target and the modelled range sit on the
// same scale, so a finish below the range reads as a narrower count, not a
// contradiction. What the forecast left out is named.
function Composition({
  evaluation,
}: Readonly<{ evaluation: ReportingEvaluation }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const landing = metricOf(evaluation, "forecast_landing");
  const basis = landing?.forecast_basis;
  if (
    landing?.coverage.status !== "ok" ||
    landing.value == null ||
    !basis ||
    basis.landing.measure === "manager_call" ||
    basis.landing.caveat
  )
    return null;
  const { currency } = evaluation.context;
  const money = (value: number) =>
    reportingCompactAmount(value, currency, currency, locale);
  const { won_minor: won, remaining_minor: remaining } = basis.landing;
  const range = rangeOf(landing.range_low_minor, landing.range_high_minor);
  const target = landing.target_comparison?.target;
  // The card under the bar states the range in full, with its basis. Equal
  // ends leave the model nothing to spread, so they draw no bracket.
  const band =
    range && range.highMinor > range.lowMinor
      ? {
          label: t("analytics.decision.range"),
          low: range.lowMinor,
          high: range.highMinor,
        }
      : undefined;
  // With no parts and no range there is nothing on the scale but empty track;
  // the readings already say zero.
  if (won + remaining <= 0 && !band) return null;
  const { unpriced, fx } = leftOut(basis);
  const left = [
    unpriced > 0 &&
      plural("analytics.decision.unpriced", unpriced, {
        count: formatNumber(unpriced, locale),
      }),
    fx > 0 &&
      plural("analytics.decision.fx", fx, {
        count: formatNumber(fx, locale),
      }),
  ].filter((part): part is string => !!part);
  return (
    <div className="analytics-composition">
      <SegmentBar
        label={t("analytics.decision.composition")}
        parts={[
          {
            key: "won",
            label: t("analytics.decision.won"),
            value: won,
            amount: money(won),
          },
          {
            key: "remaining",
            label: t(`analytics.decision.part.${basis.landing.measure}`),
            value: remaining,
            amount: money(remaining),
          },
        ]}
        marker={
          target
            ? {
                key: "target",
                label: t("reporting.target"),
                value: target,
                amount: money(target),
              }
            : undefined
        }
        band={band}
      />
      {band && <p className="t-caption">{t("analytics.decision.rangeNote")}</p>}
      {/* The server's count; an edition captured without it states none. */}
      {basis.included_count != null && (
        <p className="t-caption">
          {t("analytics.decision.included", {
            included: formatNumber(basis.included_count, locale),
            eligible: formatNumber(basis.eligible_count, locale),
          })}
        </p>
      )}
      {left.length > 0 && (
        <p className="t-caption">
          {t("analytics.decision.excluded", { parts: left.join(", ") })}
        </p>
      )}
    </div>
  );
}

function leftOut(basis: Basis) {
  return {
    unpriced: Math.max(0, basis.eligible_count - basis.priced_count),
    fx: basis.fx_missing_count,
  };
}

// The model of finishes, its own labelled reading rather than the committed
// figure's uncertainty.
function ModelledFinish({
  evaluation,
}: Readonly<{ evaluation: ReportingEvaluation }>) {
  const t = useT();
  const { locale } = useLocale();
  const landing = metricOf(evaluation, "forecast_landing");
  if (landing?.value == null) return null;
  const { currency } = evaluation.context;
  const basis = landing.forecast_basis;
  return (
    <StatStrip label={t("forecast.model.quarter")} density="compact">
      <ModelRangeCard
        label={t("forecast.model.quarter")}
        range={rangeOf(landing.range_low_minor, landing.range_high_minor)}
        wonMinor={basis?.landing.won_minor}
        unpriced={basis ? leftOut(basis).unpriced : 0}
        fxMissing={basis?.fx_missing_count ?? 0}
        money={(value) =>
          reportingCompactAmount(value, currency, currency, locale)
        }
      />
    </StatStrip>
  );
}
