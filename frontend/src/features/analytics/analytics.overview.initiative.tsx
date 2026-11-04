// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { ProgressBar } from "../../design-system/progressbar";
import { QueryGate } from "../../design-system/querystates";
import { formatAnalyticsPercent } from "../../format/format";
import { formatDateTime } from "../../format/format.dates";
import { useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { useRecordZone } from "../../lib/recordzone";
import { BoardChart } from "./analytics.overview.board";
import { metricOf } from "./analytics.overview.readings";
import { InitiativeDrawer } from "./reporting.initiativedrawer";
import { useSalesInitiatives } from "./reporting.initiatives";
import type {
  ReportingEvaluation,
  ReportingEvidenceRef,
  ReportingSelection,
} from "./reporting.model";
import { reportingAmount } from "./reporting.model";
import { useMetricNames, useReportingEvaluation } from "./reporting.queries";

/**
 * The initiative's own readings above the sales review. One is the objective
 * since the launch against its target. The other is the open pipeline by
 * where each deal came from. Both come from one evaluation of the
 * initiative's two metrics, from the launch to now, whatever period the page
 * reads.
 */
export function InitiativePanel({
  selection,
  onEvidence,
}: Readonly<{
  selection: ReportingSelection;
  onEvidence: (
    reference: ReportingEvidenceRef,
    from?: ReportingEvaluation,
  ) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const initiatives = useSalesInitiatives();
  const canEdit = useCanWrite("sales_initiative", "update");
  const [editing, setEditing] = useState(false);
  const [openedAt] = useState(() => new Date().toISOString());
  const initiative = initiatives.data?.find(
    (row) => row.id === selection.initiative_id,
  );
  const launch = initiative?.launch_at;
  const asked: ReportingSelection = {
    ...selection,
    period: "custom",
    interval: launch ? { start_at: launch, end_at: openedAt } : undefined,
    metrics: initiative
      ? [initiative.objective_metric, "initiative_pipeline"]
      : ["initiative_pipeline"],
    blocks: ["breakdown"],
  };
  const query = useReportingEvaluation(
    asked,
    initiative !== undefined && launch !== undefined,
  );
  if (!initiative) return null;
  return (
    <Panel
      title={initiative.name}
      titleAction={
        canEdit && initiative.status === "live" ? (
          <Button variant="ghost" onClick={() => setEditing(true)}>
            {t("reporting.includedRecords")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        <p className="t-caption">
          {launch
            ? t("reporting.initiativeSince", {
                launch: formatDateTime(launch, locale, zone),
                deadline: initiative.deadline,
              })
            : t("reporting.initiativeDraftNotice")}
        </p>
        {launch && (
          <QueryGate query={query} pendingLabel={initiative.name}>
            {(evaluation) => (
              <>
                <Objective
                  evaluation={evaluation}
                  metric={initiative.objective_metric}
                  target={initiative.target_value ?? null}
                  onEvidence={(reference) => onEvidence(reference, evaluation)}
                />
                <BoardChart
                  evaluation={evaluation}
                  kind="breakdown"
                  onEvidence={(reference) => onEvidence(reference, evaluation)}
                />
              </>
            )}
          </QueryGate>
        )}
      </PanelBody>
      {editing && (
        <InitiativeDrawer
          initiative={initiative}
          onClose={() => setEditing(false)}
        />
      )}
    </Panel>
  );
}

function Objective({
  evaluation,
  metric,
  target,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  metric: ReportingSelection["metrics"][number];
  target: number | null;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const metricName = useMetricNames();
  const reading = metricOf(evaluation, metric);
  if (!reading) return null;
  const value = reading.value ?? null;
  const narrowed =
    !!evaluation.selection.product_id ||
    !!evaluation.selection.country ||
    !!evaluation.selection.industry;
  const shown = reportingAmount(
    value,
    reading.unit,
    evaluation.context.currency,
    locale,
  );
  return (
    <div className="analytics-hero-figure">
      <p className="t-label">{metricName(metric)}</p>
      <p className="analytics-hero-value t-num">
        {target !== null && !narrowed
          ? t("reporting.objectiveOfTarget", {
              value: shown,
              target: reportingAmount(
                target,
                reading.unit,
                evaluation.context.currency,
                locale,
              ),
            })
          : shown}
      </p>
      {target !== null && !narrowed && value !== null && (
        <ProgressBar
          fraction={Math.min(1, value / target)}
          label={metricName(metric)}
          valueText={t("reporting.objectiveProgress", {
            share: formatAnalyticsPercent(Math.min(1, value / target), locale),
          })}
        />
      )}
      {narrowed && (
        <p className="t-caption">{t("reporting.noTargetForSlice")}</p>
      )}
      {target === null && (
        <p className="t-caption">{t("reporting.trackingOnlyVerdict")}</p>
      )}
      {reading.coverage.status !== "ok" && reading.coverage.reason && (
        <p className="t-caption">{reading.coverage.reason}</p>
      )}
      <Button variant="link" onClick={() => onEvidence(reading.evidence)}>
        {t("reporting.viewRecords")}
      </Button>
    </div>
  );
}
