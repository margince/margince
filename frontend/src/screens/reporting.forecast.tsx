import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { AnalyticsScope } from "./analytics.context";
import { ForecastShareActions } from "./analytics.share";
import { QueryGate, throwProblem } from "./common";
import { ReportingCharts } from "./reporting.charts";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import {
  type ReportingEvidenceRef,
  type ReportingSelection,
  reportingQuery,
} from "./reporting.model";

export function ReportingForecastGraphs({
  scope,
}: Readonly<{ scope: AnalyticsScope }>) {
  const t = useT();
  const { locale } = useLocale();
  const [snapshot, setSnapshot] = useState("latest");
  const [evidence, setEvidence] = useState<ReportingEvidenceRef | null>(null);
  const selection: ReportingSelection = {
    scope: { ...scope, id: scope.id ?? undefined },
    period: "this_quarter",
    target_basis: "fiscal_quarter",
    close_window: "fiscal_quarter",
    metrics: ["open_pipeline"],
    blocks: ["pipeline_movement"],
  };
  const query = useQuery({
    queryKey: ["reporting-forecast", selection],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/evaluate", {
        params: { query: reportingQuery(selection) },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  return (
    <QueryGate query={query} pendingLabel={t("reporting.forecast_support")}>
      {(evaluation) => (
        <>
          {evaluation.charts.some((chart) => chart.snapshot_id) && (
            <div className="reporting-toolbar">
              <Field label={t("reporting.shareSnapshot")}>
                {(field) => (
                  <Select
                    {...field}
                    value={snapshot}
                    onChange={setSnapshot}
                    options={evaluation.charts
                      .filter((chart) => chart.kind === "pipeline_movement")
                      .flatMap((chart) => [
                        {
                          value: "latest",
                          label: chart.state_at
                            ? formatDateTime(
                                chart.state_at,
                                locale,
                                evaluation.context.timezone,
                              )
                            : t("reporting.live"),
                        },
                        ...(chart.opening_snapshot_id && chart.interval
                          ? [
                              {
                                value: "opening",
                                label: formatDateTime(
                                  chart.interval.start_at,
                                  locale,
                                  evaluation.context.timezone,
                                ),
                              },
                            ]
                          : []),
                      ])}
                  />
                )}
              </Field>
              <ForecastShareActions
                target="forecast"
                scope={scope}
                snapshotId={
                  evaluation.charts.find(
                    (chart) => chart.kind === "pipeline_movement",
                  )?.[
                    snapshot === "opening"
                      ? "opening_snapshot_id"
                      : "snapshot_id"
                  ]
                }
              />
            </div>
          )}
          <ReportingCharts evaluation={evaluation} onEvidence={setEvidence} />
          {evidence && (
            <ReportingEvidenceDrawer
              key={JSON.stringify(evidence)}
              evaluation={evaluation}
              reference={evidence}
              onClose={() => setEvidence(null)}
            />
          )}
        </>
      )}
    </QueryGate>
  );
}
