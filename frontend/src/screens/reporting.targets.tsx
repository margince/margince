import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { useRecordZone } from "../app/recordzone";
import {
  Button,
  Field,
  SegmentedControl,
  TextInput,
} from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { isISODate } from "../design-system/dateinput";
import { Panel, PanelBody } from "../design-system/panel";
import { useLocale, useT } from "../i18n";
import {
  useAnalyticsContext,
  useAnalyticsSelection,
} from "./analytics.context";
import { QueryGate, throwProblem } from "./common";
import {
  metricLabel,
  reportingAmount,
  reportingPeriodLabel,
} from "./reporting.model";
import { useReportingPages } from "./reporting.pagination";
import { useReportingPipelines } from "./reporting.queries";
import { ReportingTargetDialog } from "./reporting.targetdialog";

type Target = components["schemas"]["ReportingTarget"];
export function ReportingTargets() {
  const t = useT();
  const { locale } = useLocale();
  const context = useAnalyticsContext();
  const zone = useRecordZone();
  const { selection } = useAnalyticsSelection(context.data);
  const pipelines = useReportingPipelines();
  const canCreate = useCanWrite("sales_target", "create");
  const canUpdate = useCanWrite("sales_target", "update");
  const { cursor, next: setCursor, back, canBack } = useReportingPages();
  const [status, setStatus] = useState("active");
  const [periodFilter, setPeriodFilter] = useState("");
  const [editing, setEditing] = useState<Target | "new" | null>(null);
  const query = useQuery({
    queryKey: ["reporting-targets", cursor, status, periodFilter],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/targets", {
        params: {
          query: {
            cursor,
            limit: 50,
            retired: status === "all" ? undefined : status === "retired",
            period_start: isISODate(`${periodFilter}-01`)
              ? `${periodFilter}-01`
              : undefined,
          },
        },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  return (
    <Panel
      title={t("reporting.targets")}
      titleAction={
        canCreate ? (
          <Button onClick={() => setEditing("new")}>
            {t("reporting.newTarget")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        <div className="reporting-toolbar">
          <SegmentedControl
            label={t("reporting.targetStatus")}
            value={status}
            options={["active", "retired", "all"]}
            labels={{
              active: t("reporting.activeTargets"),
              retired: t("reporting.retired"),
              all: t("reporting.allTargets"),
            }}
            onChange={(value) => {
              setStatus(value);
              setCursor(undefined);
            }}
          />
          <Field label={t("reporting.periodMonth")}>
            {(field) => (
              <TextInput
                {...field}
                type="month"
                value={periodFilter}
                onChange={(event) => {
                  setPeriodFilter(event.target.value);
                  setCursor(undefined);
                }}
              />
            )}
          </Field>
        </div>
        <QueryGate
          query={query}
          pendingLabel={t("reporting.targets")}
          empty={(result) => !result.data.length}
        >
          {(result) => (
            <>
              <DataTable
                label={t("reporting.targets")}
                rows={result.data}
                rowKey={(target) => target.id}
                columns={[
                  {
                    key: "scope",
                    header: t("reporting.targetFor"),
                    render: (target) => (
                      <>
                        <strong>
                          {target.definition.scope.label ??
                            target.definition.scope.kind}
                        </strong>
                        <div>{metricLabel(target.definition.metric, t)}</div>
                        <div className="t-caption">
                          {reportingPeriodLabel(target.interval, zone, locale)}
                        </div>
                        <div className="t-caption">
                          {target.definition.pipeline_id
                            ? (pipelines.data?.data.find(
                                (pipeline) =>
                                  pipeline.id === target.definition.pipeline_id,
                              )?.name ??
                              t(
                                pipelines.isPending
                                  ? "common.loading"
                                  : "reporting.restricted",
                              ))
                            : t("reporting.allPipelines")}
                          {target.definition.retired === true
                            ? ` · ${t("reporting.retired")}`
                            : ""}
                        </div>
                      </>
                    ),
                  },
                  {
                    key: "amount",
                    header: t("reporting.target"),
                    render: (target) => (
                      <>
                        <strong>
                          {reportingAmount(
                            target.definition.value,
                            target.unit === "count" ? "count" : "money",
                            target.unit,
                            locale,
                          )}
                        </strong>
                        {target.allocation_difference != null &&
                          target.allocation_difference !== 0 && (
                            <div className="t-caption">
                              {t(
                                target.allocation_difference < 0
                                  ? "reporting.overallocated"
                                  : "reporting.unallocated",
                              )}
                              :{" "}
                              {reportingAmount(
                                Math.abs(target.allocation_difference),
                                target.unit === "count" ? "count" : "money",
                                target.unit,
                                locale,
                              )}
                            </div>
                          )}
                      </>
                    ),
                  },
                  {
                    key: "edit",
                    header: t("reporting.revise"),
                    render: (target) =>
                      canUpdate ? (
                        <Button
                          variant="link"
                          onClick={() => setEditing(target)}
                        >
                          {t("reporting.revise")}
                        </Button>
                      ) : null,
                  },
                ]}
              />
              {canBack && (
                <Button variant="ghost" onClick={back}>
                  {t("reporting.back")}
                </Button>
              )}
              {result.next_cursor && (
                <Button
                  variant="ghost"
                  onClick={() => setCursor(result.next_cursor)}
                >
                  {t("reporting.next")}
                </Button>
              )}
            </>
          )}
        </QueryGate>
        {editing && context.data && (
          <ReportingTargetDialog
            target={editing === "new" ? undefined : editing}
            context={{
              ...context.data,
              default_scope: selection?.scope ?? context.data.default_scope,
            }}
            onClose={() => setEditing(null)}
          />
        )}
      </PanelBody>
    </Panel>
  );
}
