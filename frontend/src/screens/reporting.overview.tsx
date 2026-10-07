import { useQuery } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { currentParams, replaceParams, useUrlParams } from "../app/urlstate";
import { Button, SegmentedControl } from "../design-system/atoms";
import { type ISODate, isISODate } from "../design-system/dateinput";
import { ErrorLine } from "../design-system/errorline";
import { formatDateTime } from "../format/format";
import { startOfDayInZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { AnalyticsAttention } from "./analytics.attention";
import type { AnalyticsScope } from "./analytics.context";
import { problemCodeOf, QueryGate, throwProblem } from "./common";
import { ReportingCharts } from "./reporting.charts";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import { ReportingExportButton } from "./reporting.export";
import { REPORTING_PERIODS, ReportingFilters } from "./reporting.filters";
import {
  type ReportingEvidenceRef,
  type ReportingSelection,
  reportingQuery,
} from "./reporting.model";
import { useReportingPipelines } from "./reporting.queries";
import { SaveReportingDialog } from "./reporting.save";

type Catalog = components["schemas"]["ReportingCatalog"];

export function ReportingOverview({
  scope,
  scopeControl,
}: Readonly<{ scope: AnalyticsScope; scopeControl?: ReactNode }>) {
  const t = useT();
  const catalog = useQuery({
    queryKey: ["reporting-catalog"],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/metrics");
      if (error) throwProblem(error);
      return data;
    },
  });
  const pipelines = useReportingPipelines();
  const setup = useQuery({
    queryKey: ["reporting-overview-setup"],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/framework");
      if (error) throwProblem(error);
      return data;
    },
  });
  return (
    <QueryGate query={catalog} pendingLabel={t("reporting.performance")}>
      {(catalog) => (
        <QueryGate query={setup} pendingLabel={t("reporting.performance")}>
          {(setup) => (
            <QueryGate query={pipelines} pendingLabel={t("reporting.pipeline")}>
              {(pipelines) => (
                <OverviewBody
                  scope={scope}
                  scopeControl={scopeControl}
                  catalog={catalog}
                  template={setup.definition.template}
                  pipelines={pipelines.data}
                />
              )}
            </QueryGate>
          )}
        </QueryGate>
      )}
    </QueryGate>
  );
}

function useOverviewFilters(
  defaultTemplate: "sales" | "sdr",
  pipelines: readonly components["schemas"]["Pipeline"][],
) {
  const [params] = useUrlParams();
  const setParam = (key: string, value: string) => {
    const next = new Map(currentParams());
    next.set(key, value);
    replaceParams(next);
  };
  const template =
    params.get("view") === "sdr"
      ? "sdr"
      : params.get("view") === "sales"
        ? "sales"
        : defaultTemplate;
  const setTemplate = (value: string) => setParam("view", value);
  const from = params.get("from") ?? "";
  const through = params.get("through") ?? "";
  const start: ISODate | "" = isISODate(from) ? from : "";
  const end: ISODate | "" = isISODate(through) ? through : "";
  const setStart = (value: string) => setParam("from", value);
  const setEnd = (value: string) => setParam("through", value);
  const period =
    REPORTING_PERIODS.find((value) => value === params.get("period")) ??
    "this_month";
  const setPeriod = (value: string) => setParam("period", value);
  const defaultPipeline =
    pipelines.find((pipeline) => pipeline.is_default)?.id ??
    pipelines[0]?.id ??
    "";
  const pipelineId =
    params.get("pipeline") === "all"
      ? ""
      : (pipelines.find((pipeline) => pipeline.id === params.get("pipeline"))
          ?.id ?? defaultPipeline);
  const setPipelineId = (value: string) => setParam("pipeline", value || "all");
  const targetBasis: ReportingSelection["target_basis"] =
    period === "this_quarter" ? "fiscal_quarter" : "month";
  const setTargetBasis = (value: string) => setParam("target", value);
  const closeWindow: ReportingSelection["close_window"] =
    params.get("close") === "fiscal_quarter" ? "fiscal_quarter" : "all_open";
  const setCloseWindow = (value: string) => setParam("close", value);
  return {
    template,
    setTemplate,
    start,
    end,
    setStart,
    setEnd,
    period,
    setPeriod,
    pipelineId,
    setPipelineId,
    targetBasis,
    setTargetBasis,
    closeWindow,
    setCloseWindow,
  };
}

function OverviewBody({
  scope,
  scopeControl,
  catalog,
  template: defaultTemplate,
  pipelines,
}: Readonly<{
  scope: AnalyticsScope;
  scopeControl?: ReactNode;
  catalog: Catalog;
  template: "sales" | "sdr";
  pipelines: readonly components["schemas"]["Pipeline"][];
}>) {
  const t = useT();
  const zone = useRecordZone();
  const { locale } = useLocale();
  const {
    template,
    setTemplate,
    start,
    end,
    setStart,
    setEnd,
    period,
    setPeriod,
    pipelineId,
    setPipelineId,
    targetBasis,
    setTargetBasis,
    closeWindow,
    setCloseWindow,
  } = useOverviewFilters(defaultTemplate, pipelines);
  const canSave = useCanWrite("report_definition", "create");
  const [saving, setSaving] = useState(false);
  const [evidence, setEvidence] = useState<ReportingEvidenceRef | null>(null);
  const metrics = catalog.metrics
    .filter((metric) =>
      template === "sdr"
        ? ["meetings_held", "accepted_opportunities"].includes(metric.id)
        : ![
            "meetings_held",
            "accepted_opportunities",
            "forecast_landing",
          ].includes(metric.id),
    )
    .map((metric) => metric.id);
  const desired: ReportingSelection["blocks"] =
    template === "sdr"
      ? ["sdr_outcomes", "target_progress"]
      : [
          "bookings_trend",
          "stage_distribution",
          "owner_attainment",
          "stage_age",
        ];
  const selection: ReportingSelection = {
    scope: { kind: scope.kind, id: scope.id ?? undefined, label: scope.label },
    pipeline_id: template === "sales" ? pipelineId || undefined : undefined,
    period,
    interval:
      period === "custom" && start && end
        ? {
            start_at: startOfDayInZone(start, zone),
            end_at: startOfDayInZone(end, zone, 1),
          }
        : undefined,
    target_basis: targetBasis,
    close_window: closeWindow,
    metrics,
    blocks: desired.filter((block) =>
      catalog.metrics.some(
        (metric) =>
          metrics.includes(metric.id) && metric.blocks.includes(block),
      ),
    ),
  };
  const validPeriod = validDateRange(period, start, end);
  const query = useQuery({
    enabled: validPeriod,
    queryKey: ["reporting-evaluation", selection],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/evaluate", {
        params: { query: reportingQuery(selection) },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  const contextKey = JSON.stringify(selection);
  const invalidSelection =
    problemCodeOf(query.error) === "reporting_interval_invalid";
  return (
    <>
      <div className="reporting-controlbar">
        {scopeControl}
        <SegmentedControl
          label={t("reporting.view")}
          options={["sales", "sdr"]}
          value={template}
          labels={{ sales: t("reporting.sales"), sdr: t("reporting.sdr") }}
          onChange={(next) => {
            setTemplate(next);
            setEvidence(null);
          }}
        />
        <ReportingFilters
          selection={selection}
          showScope={false}
          showCloseWindow={template === "sales"}
          showPipeline={template === "sales"}
          showTargets={false}
          dates={{
            from: start,
            through: end,
            onChange: (from, through) => {
              setStart(from);
              setEnd(through);
              setEvidence(null);
            },
          }}
          onChange={(next) => {
            if (next.period !== period) {
              setPeriod(next.period);
              setTargetBasis(
                next.period === "this_quarter" ? "fiscal_quarter" : "month",
              );
            }
            if (
              template === "sales" &&
              next.pipeline_id !== selection.pipeline_id
            )
              setPipelineId(next.pipeline_id ?? "");
            if (next.target_basis !== targetBasis)
              setTargetBasis(next.target_basis);
            if (next.close_window !== closeWindow)
              setCloseWindow(next.close_window);
            setEvidence(null);
          }}
        />
        <div className="reporting-header-actions">
          {query.data && <ReportingExportButton evaluation={query.data} />}
          {canSave && (
            <Button onClick={() => setSaving(true)} disabled={!query.isSuccess}>
              {t("reporting.save")}
            </Button>
          )}
        </div>
      </div>
      {!validPeriod && <p role="status">{t("reporting.chooseDates")}</p>}
      {invalidSelection && <ErrorLine error={query.error} />}
      {/* Its sources are its own, so a failed or unasked evaluation does not
          take the list down with it. */}
      {(!validPeriod || query.isError) && <AnalyticsAttention scope={scope} />}
      {validPeriod && !invalidSelection && (
        <QueryGate query={query} pendingLabel={t("reporting.performance")}>
          {(evaluation) => (
            <>
              {evaluation.context.interval.end_at ===
                evaluation.context.evaluated_at && (
                <p className="t-caption" role="status">
                  {t("reporting.resultsThrough", {
                    at: formatDateTime(
                      evaluation.context.interval.end_at,
                      locale,
                      evaluation.context.timezone,
                    ),
                  })}
                </p>
              )}
              <ReportingCharts
                evaluation={evaluation}
                afterSummary={<AnalyticsAttention scope={scope} />}
                onEvidence={setEvidence}
              />
              {evidence && (
                <ReportingEvidenceDrawer
                  key={`${contextKey}:${JSON.stringify(evidence)}`}
                  evaluation={evaluation}
                  reference={evidence}
                  onClose={() => setEvidence(null)}
                />
              )}
              {saving && (
                <SaveReportingDialog
                  selection={evaluation.selection}
                  onClose={() => setSaving(false)}
                  onSaved={(report) => {
                    setSaving(false);
                    navigate({
                      screen: "analytics",
                      id: "reports",
                      id2: report.id,
                    });
                  }}
                />
              )}
            </>
          )}
        </QueryGate>
      )}
    </>
  );
}

function validDateRange(period: string, start: string, end: string): boolean {
  return period !== "custom" || !!(start && end);
}
