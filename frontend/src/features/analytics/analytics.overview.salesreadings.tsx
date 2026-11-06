// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../../design-system/atoms";
import { formatAnalyticsNumber, formatDateAbbrev } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { metricOf, readingValue } from "./analytics.overview.readings";
import type {
  ReportingEvaluation,
  ReportingEvidenceRef,
} from "./reporting.model";

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
                count: formatAnalyticsNumber(count, locale),
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

/**
 * All open pipeline in the head of the stage chart, which draws that stock:
 * the figure and its deal count.
 */
export function OpenPipelineMeta({
  evaluation,
}: Readonly<{ evaluation: ReportingEvaluation }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const open = metricOf(evaluation, "open_pipeline");
  if (!open) return null;
  const count = open.coverage.eligible_count;
  const label = t(
    evaluation.context.close_interval
      ? "analytics.overview.openClosing"
      : "analytics.overview.allOpen",
  );
  return (
    <span className="reporting-meta t-num">
      {[
        `${label}: ${readingValue(open, evaluation, t, locale)}`,
        count != null &&
          plural("analytics.outlook.dealCount", count, {
            count: formatAnalyticsNumber(count, locale),
          }),
      ]
        .filter(Boolean)
        .join(" · ")}
    </span>
  );
}

/** What all open pipeline leaves out, said above the stage chart's bars. */
export function OpenPipelineNote({
  evaluation,
}: Readonly<{ evaluation: ReportingEvaluation }>) {
  const t = useT();
  const open = metricOf(evaluation, "open_pipeline");
  if (!open) return null;
  const caveats = [
    open.coverage.withheld && t("analytics.overview.partlyHidden"),
    open.coverage.status !== "ok" && open.coverage.reason,
  ].filter(Boolean);
  if (caveats.length === 0) return null;
  return <p className="t-caption">{caveats.join(" · ")}</p>;
}
