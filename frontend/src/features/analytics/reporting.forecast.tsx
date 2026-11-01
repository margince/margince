import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { Field } from "../../design-system/atoms";
import { QueryGate } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import type { AnalyticsScope } from "./analytics.context";
import { ForecastShareActions } from "./analytics.share";
import { ReportingCharts } from "./reporting.charts";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import { useSelectionEvidence } from "./reporting.evidenceselection";
import {
  type ReportingEvaluation,
  type ReportingSelection,
  reportingQuery,
} from "./reporting.model";

export function ReportingForecastGraphs({
  scope,
}: Readonly<{ scope: AnalyticsScope }>) {
  const t = useT();
  const showCharts = useCan("report_definition", "read");
  const [snapshot, setSnapshot] = useState("latest");
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
    enabled: showCharts,
    queryFn: async () => {
      return unwrap(
        await api.GET("/analytics/evaluate", {
          params: { query: reportingQuery(selection) },
        }),
      );
    },
  });
  const { evidence, openEvidence, closeEvidence } = useSelectionEvidence(
    JSON.stringify(selection),
  );
  const evaluated = showCharts ? query.data : undefined;
  return (
    <div className="reporting-results">
      {/* The snapshot and the verbs that share it, as one row: the choice only
          means something to the share beside it. */}
      <div className="reporting-toolbar reporting-share-bar">
        {evaluated?.charts.some((chart) => chart.snapshot_id) && (
          <SnapshotPicker
            evaluation={evaluated}
            value={snapshot}
            onChange={setSnapshot}
          />
        )}
        <ForecastShareActions
          target="forecast"
          scope={scope}
          snapshotId={
            evaluated?.charts.find(
              (chart) => chart.kind === "pipeline_movement",
            )?.[snapshot === "opening" ? "opening_snapshot_id" : "snapshot_id"]
          }
        />
      </div>
      {showCharts && (
        <QueryGate query={query} pendingLabel={t("reporting.forecast_support")}>
          {(evaluation) => (
            <>
              <ReportingCharts
                evaluation={evaluation}
                onEvidence={openEvidence}
              />
              {evidence && (
                <ReportingEvidenceDrawer
                  key={JSON.stringify(evidence)}
                  evaluation={evaluation}
                  reference={evidence}
                  onClose={closeEvidence}
                />
              )}
            </>
          )}
        </QueryGate>
      )}
    </div>
  );
}

function SnapshotPicker({
  evaluation,
  value,
  onChange,
}: Readonly<{
  evaluation: ReportingEvaluation;
  value: string;
  onChange: (value: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const { timezone } = evaluation.context;
  return (
    <Field label={t("reporting.shareSnapshot")}>
      {(field) => (
        <Select
          {...field}
          value={value}
          onChange={onChange}
          options={evaluation.charts
            .filter((chart) => chart.kind === "pipeline_movement")
            .flatMap((chart) => [
              {
                value: "latest",
                label: chart.state_at
                  ? formatDateTime(chart.state_at, locale, timezone)
                  : t("reporting.live"),
              },
              ...(chart.opening_snapshot_id && chart.interval
                ? [
                    {
                      value: "opening",
                      label: formatDateTime(
                        chart.interval.start_at,
                        locale,
                        timezone,
                      ),
                    },
                  ]
                : []),
            ])}
        />
      )}
    </Field>
  );
}
