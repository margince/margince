// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import { ActionRow } from "../../design-system/actionrow";
import { AppLink } from "../../design-system/applink";
import { Button, StatCard } from "../../design-system/atoms";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import {
  analyticsSectionHref,
  type Section,
} from "../../lib/analytics.address";
import { useCanWrite } from "../../lib/capability";
import { navigate } from "../../lib/router";
import type { UrlParams } from "../../lib/urlstate";
import { scopeDials } from "./analytics.overview.context";
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
 * One headline metric as a reading. The card opens the shared evidence for it.
 * That is the records behind a figure, or the empty population behind a
 * measured zero, or why a failed or hidden figure has none.
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
  // A figure hidden whole says so; one with a value hides only a part.
  const hidden = metric?.coverage.withheld && metric.value == null;
  return (
    <StatCard
      narrow="row"
      label={label ?? metricName(id)}
      value={
        hidden
          ? t("analytics.prospect.withheld")
          : readingValue(metric, evaluation, t, locale)
      }
      detail={
        <>
          {detail}
          {metric?.coverage.withheld && !hidden && (
            <span>{t("analytics.overview.partlyHidden")}</span>
          )}
          {/* What a partial figure leaves out is said beside it, not behind
              a hover. */}
          {metric?.coverage.status !== "ok" && metric?.coverage.reason && (
            <span>{metric.coverage.reason}</span>
          )}
        </>
      }
      opens="records"
      onOpen={metric ? () => onEvidence(metric.evidence) : undefined}
    />
  );
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
 * Save view and Report templates, the same pair on every view. Saving keeps
 * the view's selection, charts included, as a report the editor can revise.
 * The templates open for the view's own `scope`.
 */
export function OverviewSaveActions({
  selection,
  scope,
}: Readonly<{ selection: ReportingSelection | undefined; scope: string }>) {
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
      <AppLink
        className="btn btn-ghost"
        href={analyticsSectionHref("reports", scopeDials(scope))}
      >
        {t("analytics.overview.templates")}
      </AppLink>
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
