// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import { ActionRow } from "../../design-system/actionrow";
import { AppLink } from "../../design-system/applink";
import { Button, StatCard } from "../../design-system/atoms";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import {
  analyticsSectionHref,
  openAnalyticsSection,
  type Section,
} from "../../lib/analytics.address";
import { useCanWrite } from "../../lib/capability";
import { navigate } from "../../lib/router";
import type { UrlParams } from "../../lib/urlstate";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  type ReportingSelection,
  reportingCompactAmount,
  reportingMoneyUnit,
} from "./reporting.model";
import { useMetricNames } from "./reporting.queries";
import { SaveReportingDialog } from "./reporting.save";

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

/**
 * The evidence a reader opened, for as long as the question it was read from
 * stands. A changed selection closes it, so a drawer never lists one
 * question's records under another's figures.
 */
export function useSelectionEvidence(question: string) {
  const [held, setHeld] = useState<Readonly<{
    question: string;
    reference: ReportingEvidenceRef;
  }> | null>(null);
  const evidence = held?.question === question ? held.reference : null;
  const openEvidence = (reference: ReportingEvidenceRef) =>
    setHeld({ question, reference });
  const closeEvidence = () => setHeld(null);
  return { evidence, openEvidence, closeEvidence };
}

/**
 * The deeper pages a view leads to, after its own readings and lists. Each
 * is a link carrying the view's question, so the page it opens answers it.
 * A modified click opens it in a new tab.
 */
export function OverviewLinks({
  sections,
  dials,
}: Readonly<{
  sections: readonly Readonly<{ section: Section; label: string }>[];
  dials: UrlParams;
}>) {
  return (
    <ActionRow>
      {sections.map(({ section, label }) => (
        <AppLink
          key={section}
          className="btn btn-ghost"
          href={analyticsSectionHref(section, dials)}
        >
          {label}
        </AppLink>
      ))}
    </ActionRow>
  );
}

/**
 * Save view and Report templates, the same pair on both views. Saving keeps
 * the view's selection, charts included, as a report the editor can revise.
 */
export function OverviewSaveActions({
  selection,
}: Readonly<{ selection: ReportingSelection | undefined }>) {
  const t = useT();
  const canSave = useCanWrite("report_definition", "create");
  const [saving, setSaving] = useState(false);
  return (
    <>
      {canSave && (
        <Button onClick={() => setSaving(true)} disabled={!selection}>
          {t("analytics.overview.saveView")}
        </Button>
      )}
      <Button variant="ghost" onClick={() => openAnalyticsSection("reports")}>
        {t("analytics.overview.templates")}
      </Button>
      {saving && selection && (
        <SaveReportingDialog
          selection={selection}
          note={t("analytics.overview.saveViewNote")}
          onClose={() => setSaving(false)}
          onSaved={(report) => {
            setSaving(false);
            navigate({ screen: "analytics", id: "reports", id2: report.id });
          }}
        />
      )}
    </>
  );
}
