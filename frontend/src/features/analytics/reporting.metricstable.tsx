// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, Disclosure } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { Panel } from "../../design-system/panel";
import { Popover } from "../../design-system/popover";
import { useLocale, useT } from "../../i18n";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingAmount,
} from "./reporting.model";
import { useMetricNames } from "./reporting.queries";

// Every metric of an evaluation as one table, folded under the charts: actual,
// target where any has one, and coverage. On a pane, like the charts above it.
export function MetricsTable({
  evaluation,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const metricName = useMetricNames();
  const { locale } = useLocale();
  const amount = (value: number | null | undefined, unit: string) =>
    reportingAmount(value, unit, evaluation.context.currency, locale);
  return (
    <Panel>
      <Disclosure summary={t("reporting.moreMetrics")}>
        <DataTable
          label={t("reporting.metrics")}
          rows={evaluation.metrics}
          rowKey={(metric) => metric.id}
          columns={[
            {
              key: "metric",
              header: t("reporting.metrics"),
              render: (metric) => (
                <Button
                  variant="link"
                  onClick={() => onEvidence(metric.evidence)}
                >
                  {metricName(metric.id)}
                </Button>
              ),
            },
            {
              key: "actual",
              header: t("reporting.actual"),
              render: (metric) => amount(metric.value, metric.unit),
            },
            ...(evaluation.metrics.some((metric) => metric.target != null)
              ? [
                  {
                    key: "target",
                    header: t("reporting.target"),
                    render: (metric: ReportingEvaluation["metrics"][number]) =>
                      metric.target == null
                        ? "—"
                        : amount(metric.target, metric.unit),
                  },
                ]
              : []),
            {
              key: "status",
              header: t("reporting.evidence"),
              render: (metric) => (
                <Popover
                  onHover
                  label={t(`reporting.status.${metric.coverage.status}`)}
                >
                  <p>
                    {metric.coverage.reason ??
                      t(`reporting.status.${metric.coverage.status}`)}
                  </p>
                </Popover>
              ),
            },
          ]}
        />
      </Disclosure>
    </Panel>
  );
}
