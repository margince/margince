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
 * on; what the expected finish is made of; and the modelled finish beside it.
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
    <Panel title={t("analytics.overview.summary")}>
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
      </PanelBody>
      <ModelledFinish evaluation={evaluation} />
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
// basis: won plus the forward half of the same measure. A manager's call is a
// single authored total with no parts, and a caveat means the plain sum does
// not hold, so neither draws a bar. What the forecast left out is named.
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
            label: t(`forecast.basis.${basis.landing.measure}`),
            value: remaining,
            amount: money(remaining),
          },
        ]}
      />
      <p className="t-caption">
        {t("analytics.decision.included", {
          priced: formatNumber(basis.priced_count, locale),
          eligible: formatNumber(basis.eligible_count, locale),
        })}
      </p>
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
