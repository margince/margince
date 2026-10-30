// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { ActionRow } from "../../design-system/actionrow";
import { Button, StatCard } from "../../design-system/atoms";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import {
  openAnalyticsSection,
  type Section,
} from "../../lib/analytics.address";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingCompactAmount,
  reportingMoneyUnit,
} from "./reporting.model";
import { useMetricNames } from "./reporting.queries";

type Metric = ReportingEvaluation["metrics"][number];

export function metricOf(
  evaluation: ReportingEvaluation,
  id: Metric["id"],
): Metric | undefined {
  return evaluation.metrics.find((metric) => metric.id === id);
}

// A count or a sum over no records is a measured zero. A median over none is
// no figure at all, and says so.
function measuredZero(metric: Metric, currency: string): boolean {
  return (
    metric.coverage.status === "no_data" &&
    (metric.unit === "count" || reportingMoneyUnit(metric.unit, currency))
  );
}

/** A metric's figure as a reading shows it, or the word for why there is none. */
export function readingValue(
  metric: Metric | undefined,
  evaluation: ReportingEvaluation,
  t: Translator,
  locale: Locale,
): string {
  if (!metric) return t("analytics.readingUnavailable");
  const { currency } = evaluation.context;
  const value =
    metric.value ?? (measuredZero(metric, currency) ? 0 : undefined);
  if (value == null) return t(`reporting.status.${metric.coverage.status}`);
  return reportingCompactAmount(value, metric.unit, currency, locale);
}

/**
 * One headline metric as a reading. The card opens the records behind it; a
 * figure with no records, or one that failed, has no door.
 */
export function MetricReading({
  evaluation,
  id,
  label,
  detail,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  id: Metric["id"];
  label?: string;
  detail?: ReactNode;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const metricName = useMetricNames();
  const metric = metricOf(evaluation, id);
  const status = metric?.coverage.status;
  const opens =
    metric &&
    status !== "no_data" &&
    status !== "failed" &&
    metric.value != null;
  return (
    <StatCard
      narrow="row"
      label={label ?? metricName(id)}
      value={readingValue(metric, evaluation, t, locale)}
      detail={
        <>
          {detail}
          {metric?.coverage.withheld && (
            <span>{t("analytics.overview.partlyHidden")}</span>
          )}
          {/* What a partial figure leaves out is said beside it, not behind
              a hover. */}
          {metric?.coverage.status !== "ok" && metric?.coverage.reason && (
            <span>{metric.coverage.reason}</span>
          )}
        </>
      }
      onOpen={opens ? () => onEvidence(metric.evidence) : undefined}
    />
  );
}

/** The deeper pages a view leads to, after its own readings and lists. */
export function OverviewLinks({
  sections,
}: Readonly<{
  sections: readonly Readonly<{ section: Section; label: string }>[];
}>) {
  return (
    <ActionRow>
      {sections.map(({ section, label }) => (
        <Button
          key={section}
          variant="ghost"
          onClick={() => openAnalyticsSection(section)}
        >
          {label}
        </Button>
      ))}
    </ActionRow>
  );
}
