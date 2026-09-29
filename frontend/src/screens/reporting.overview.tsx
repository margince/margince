import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Button, Field, SegmentedControl } from "../design-system/atoms";
import { DateInput, type ISODate, isISODate } from "../design-system/dateinput";
import { Heading } from "../design-system/heading";
import { Select } from "../design-system/select";
import { startOfDayInZone } from "../format/timezone";
import { useT } from "../i18n";
import { openAnalyticsSection } from "./analytics.address";
import type { AnalyticsScope } from "./analytics.context";
import { QueryGate, throwProblem } from "./common";
import { ReportingCharts } from "./reporting.charts";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import {
  type ReportingEvidenceRef,
  type ReportingSelection,
  reportingQuery,
} from "./reporting.model";
import { SaveReportingDialog } from "./reporting.save";

type Catalog = components["schemas"]["ReportingCatalog"];
const PERIODS: readonly ReportingSelection["period"][] = [
  "this_month",
  "last_month",
  "last_week",
  "this_quarter",
  "custom",
];

export function ReportingOverview({
  scope,
}: Readonly<{ scope: AnalyticsScope }>) {
  const t = useT();
  const catalog = useQuery({
    queryKey: ["reporting-catalog"],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/metrics");
      if (error) throwProblem(error);
      return data;
    },
  });
  const setup = useQuery({
    queryKey: ["reporting-overview-setup"],
    queryFn: async () => {
      const [framework, pipelines] = await Promise.all([
        api.GET("/analytics/framework"),
        api.GET("/pipelines", { params: { query: {} } }),
      ]);
      if (framework.error) throwProblem(framework.error);
      if (pipelines.error) throwProblem(pipelines.error);
      return { framework: framework.data, pipelines: pipelines.data.data };
    },
  });
  return (
    <QueryGate query={catalog} pendingLabel={t("reporting.performance")}>
      {(catalog) => (
        <QueryGate query={setup} pendingLabel={t("reporting.performance")}>
          {(setup) => (
            <OverviewBody
              scope={scope}
              catalog={catalog}
              template={setup.framework.definition.template}
              pipelines={setup.pipelines}
            />
          )}
        </QueryGate>
      )}
    </QueryGate>
  );
}

function OverviewBody({
  scope,
  catalog,
  template: defaultTemplate,
  pipelines,
}: Readonly<{
  scope: AnalyticsScope;
  catalog: Catalog;
  template: "sales" | "sdr";
  pipelines: readonly components["schemas"]["Pipeline"][];
}>) {
  const t = useT();
  const zone = useRecordZone();
  const [template, setTemplate] = useState(defaultTemplate);
  const [start, setStart] = useState<ISODate | "">("");
  const [end, setEnd] = useState<ISODate | "">("");
  const canSave = useCanWrite("report_definition", "create");
  const [period, setPeriod] =
    useState<ReportingSelection["period"]>("this_month");
  const [pipelineId, setPipelineId] = useState(
    pipelines.find((pipeline) => pipeline.is_default)?.id ??
      pipelines[0]?.id ??
      "",
  );
  const [targetBasis, setTargetBasis] =
    useState<ReportingSelection["target_basis"]>("month");
  const [closeWindow, setCloseWindow] =
    useState<ReportingSelection["close_window"]>("fiscal_quarter");
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
  const validPeriod = period !== "custom" || !!(start && end && end >= start);
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
  return (
    <>
      <div className="reporting-header">
        <div>
          <Heading as="h2" size="large">
            {t("reporting.performance")}
          </Heading>
          <p className="t-caption">{t("reporting.purpose")}</p>
        </div>
        <div className="reporting-header-actions">
          <Button
            variant="ghost"
            onClick={() => openAnalyticsSection("questions")}
          >
            {t("reporting.advanced")}
          </Button>
          {canSave && (
            <Button onClick={() => setSaving(true)} disabled={!query.isSuccess}>
              {t("reporting.save")}
            </Button>
          )}
        </div>
      </div>
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
      <div className="reporting-toolbar">
        <Field label={t("reporting.period")}>
          {(field) => (
            <Select
              {...field}
              value={period}
              options={PERIODS.map((value) => ({
                value,
                label: t(`reporting.${value}`),
              }))}
              onChange={(value) => {
                const next = PERIODS.find((period) => period === value);
                if (next) {
                  setPeriod(next);
                  setEvidence(null);
                }
              }}
            />
          )}
        </Field>
        {period === "custom" && (
          <>
            <Field label={t("reporting.fromDate")}>
              {(field) => (
                <DateInput
                  {...field}
                  value={start}
                  onChange={(event) => {
                    const value = event.target.value;
                    setStart(isISODate(value) ? value : "");
                    setEvidence(null);
                  }}
                />
              )}
            </Field>
            <Field label={t("reporting.throughDate")}>
              {(field) => (
                <DateInput
                  {...field}
                  value={end}
                  onChange={(event) => {
                    const value = event.target.value;
                    setEnd(isISODate(value) ? value : "");
                    setEvidence(null);
                  }}
                />
              )}
            </Field>
          </>
        )}
        {template === "sales" && (
          <Field label={t("reporting.pipeline")}>
            {(field) => (
              <Select
                {...field}
                value={pipelineId}
                options={[
                  { value: "", label: t("reporting.allPipelines") },
                  ...pipelines.map((pipeline) => ({
                    value: pipeline.id,
                    label: pipeline.name,
                  })),
                ]}
                onChange={(value) => {
                  setPipelineId(value);
                  setEvidence(null);
                }}
              />
            )}
          </Field>
        )}
        <Field label={t("reporting.targetBasis")}>
          {(field) => (
            <Select
              {...field}
              value={targetBasis}
              options={[
                { value: "month", label: t("reporting.month") },
                {
                  value: "fiscal_quarter",
                  label: t("reporting.fiscal_quarter"),
                },
              ]}
              onChange={(value) => {
                if (value === "month" || value === "fiscal_quarter") {
                  setTargetBasis(value);
                  setEvidence(null);
                }
              }}
            />
          )}
        </Field>
        {template === "sales" && (
          <Field label={t("reporting.closeWindow")}>
            {(field) => (
              <Select
                {...field}
                value={closeWindow}
                options={[
                  {
                    value: "fiscal_quarter",
                    label: t("reporting.fiscal_quarter"),
                  },
                  { value: "all_open", label: t("reporting.all_open") },
                ]}
                onChange={(value) => {
                  if (value === "fiscal_quarter" || value === "all_open") {
                    setCloseWindow(value);
                    setEvidence(null);
                  }
                }}
              />
            )}
          </Field>
        )}
      </div>
      {!validPeriod && <p role="status">{t("reporting.chooseDates")}</p>}
      {validPeriod && (
        <QueryGate query={query} pendingLabel={t("reporting.performance")}>
          {(evaluation) => (
            <>
              <ReportingCharts
                evaluation={evaluation}
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
