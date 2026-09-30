import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import { useCanWrite } from "../app/capability";
import { Button, Field, TextInput } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { Select } from "../design-system/select";
import { formatNumber } from "../format/format";
import { toMajorUnits, toMinorUnits } from "../format/minorunits";
import { useLocale, useT } from "../i18n";
import { useAnalyticsContext } from "./analytics.context";
import { AnalyticsScopePicker } from "./analytics.scope";
import { QueryGate, throwProblem } from "./common";
import {
  metricLabel,
  type ReportingMetricID,
  reportingAmount,
} from "./reporting.model";

type Target = components["schemas"]["ReportingTarget"];
type TargetInput = components["schemas"]["ReportingTargetInput"];
export function ReportingTargets() {
  const t = useT();
  const { locale } = useLocale();
  const context = useAnalyticsContext();
  const canCreate = useCanWrite("sales_target", "create");
  const canUpdate = useCanWrite("sales_target", "update");
  const [cursor, setCursor] = useState<string>();
  const [editing, setEditing] = useState<Target | "new" | null>(null);
  const query = useQuery({
    queryKey: ["reporting-targets", cursor],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/targets", {
        params: { query: { cursor, limit: 50 } },
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
        <PanelIntro>{t("reporting.commitments")}</PanelIntro>
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
                    key: "metric",
                    header: t("reporting.metrics"),
                    render: (target) =>
                      metricLabel(target.definition.metric, t),
                  },
                  {
                    key: "scope",
                    header: t("reporting.audience"),
                    render: (target) =>
                      target.definition.scope.label ??
                      target.definition.scope.kind,
                  },
                  {
                    key: "period",
                    header: t("reporting.period"),
                    render: (target) =>
                      `${target.definition.period_start} · ${t(`reporting.${target.definition.period_kind}`)}`,
                  },
                  {
                    key: "amount",
                    header: t("reporting.target"),
                    render: (target) =>
                      reportingAmount(
                        target.definition.value,
                        target.unit === "count" ? "count" : "money",
                        target.unit === "count" ? "" : target.unit,
                        locale,
                      ),
                  },
                  {
                    key: "allocation",
                    header: t("reporting.allocationDifference"),
                    render: (target) =>
                      target.allocation_difference == null
                        ? "—"
                        : reportingAmount(
                            target.allocation_difference,
                            target.unit === "count" ? "count" : "money",
                            target.unit,
                            locale,
                          ),
                  },
                  {
                    key: "revision",
                    header: t("reporting.details"),
                    render: (target) =>
                      t("reporting.revision", {
                        revision: formatNumber(target.revision, locale),
                      }),
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
          <TargetDialog
            target={editing === "new" ? undefined : editing}
            context={context.data}
            onClose={() => setEditing(null)}
          />
        )}
      </PanelBody>
    </Panel>
  );
}

function TargetDialog({
  target,
  context,
  onClose,
}: Readonly<{
  target?: Target;
  context: components["schemas"]["AnalyticsContext"];
  onClose: () => void;
}>) {
  const t = useT();
  const title = useId();
  const client = useQueryClient();
  const [metric, setMetric] = useState<ReportingMetricID>(
    target?.definition.metric ?? "bookings_won",
  );
  const [scope, setScope] = useState(
    target?.definition.scope ?? {
      ...context.default_scope,
      id: context.default_scope.id ?? undefined,
    },
  );
  const [periodKind, setPeriodKind] = useState<TargetInput["period_kind"]>(
    target?.definition.period_kind ?? "month",
  );
  const [periodStart, setPeriodStart] = useState(
    target?.definition.period_start ?? "",
  );
  const [pipeline, setPipeline] = useState(
    target?.definition.pipeline_id ?? "",
  );
  const [reason, setReason] = useState("");
  const [amount, setAmount] = useState(
    target
      ? String(
          target.unit !== "count"
            ? toMajorUnits(target.definition.value, context.base_currency)
            : target.definition.value,
        )
      : "",
  );
  const catalog = useQuery({
    queryKey: ["reporting-catalog"],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/metrics");
      if (error) throwProblem(error);
      return data;
    },
  });
  const pipelines = useQuery({
    queryKey: ["reporting-target-pipelines"],
    queryFn: async () => {
      const { data, error } = await api.GET("/pipelines", {
        params: { query: {} },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  const definition = catalog.data?.metrics.find(
    (candidate) => candidate.id === metric,
  );
  const value =
    definition?.unit === "money"
      ? toMinorUnits(Number(amount), context.base_currency)
      : Number(amount);
  const write = useMutation({
    mutationFn: async ({
      previous,
      input,
    }: {
      previous?: Target;
      input: TargetInput;
    }) => {
      const result = previous
        ? await api.PATCH("/analytics/targets/{id}", {
            params: {
              path: { id: previous.id },
              ...ifMatch(previous.version),
            },
            body: input,
          })
        : await api.POST("/analytics/targets", { body: input });
      if (result.error) throwProblem(result.error);
      return result.data;
    },
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["reporting-targets"] });
      await client.invalidateQueries({ queryKey: ["reporting-evaluation"] });
      onClose();
    },
  });
  return (
    <Modal open onClose={onClose} labelledBy={title}>
      <form
        className="reporting-dialog"
        onSubmit={(event) => {
          event.preventDefault();
          write.mutate({
            previous: target,
            input: {
              metric,
              scope,
              pipeline_id: pipeline || undefined,
              period_kind: periodKind,
              period_start: periodStart,
              value,
              reason,
            },
          });
        }}
      >
        <Heading id={title} as="h2" size="medium">
          {t(target ? "reporting.revise" : "reporting.newTarget")}
        </Heading>
        <QueryGate query={catalog} pendingLabel={t("reporting.metrics")}>
          {(catalog) => (
            <Field label={t("reporting.metrics")}>
              {(field) => (
                <Select
                  {...field}
                  disabled={!!target}
                  value={metric}
                  options={catalog.metrics
                    .filter((metric) => metric.supports_target)
                    .map((metric) => ({
                      value: metric.id,
                      label: metricLabel(metric.id, t),
                    }))}
                  onChange={(value) => {
                    const selected = catalog.metrics.find(
                      (metric) => metric.id === value,
                    );
                    if (selected) setMetric(selected.id);
                  }}
                />
              )}
            </Field>
          )}
        </QueryGate>
        {target ? (
          <p>{scope.label}</p>
        ) : (
          <AnalyticsScopePicker
            scopes={context.allowed_scopes.filter(
              (scope) => scope.kind !== "managed_teams",
            )}
            selected={{ ...scope, label: scope.label ?? "" }}
            onSelect={(scope) =>
              setScope({ ...scope, id: scope.id ?? undefined })
            }
          />
        )}
        <Field label={t("reporting.pipeline")}>
          {(field) => (
            <Select
              {...field}
              disabled={!!target}
              value={pipeline}
              options={[
                { value: "", label: t("reporting.allPipelines") },
                ...(pipelines.data?.data ?? []).map((pipeline) => ({
                  value: pipeline.id,
                  label: pipeline.name,
                })),
              ]}
              onChange={setPipeline}
            />
          )}
        </Field>
        <Field label={t("reporting.targetBasis")}>
          {(field) => (
            <Select
              {...field}
              disabled={!!target}
              value={periodKind}
              options={[
                { value: "month", label: t("reporting.month") },
                {
                  value: "fiscal_quarter",
                  label: t("reporting.fiscal_quarter"),
                },
              ]}
              onChange={(value) => {
                if (value === "month" || value === "fiscal_quarter")
                  setPeriodKind(value);
              }}
            />
          )}
        </Field>
        <Field label={t("reporting.periodStart")} required>
          {(field) => (
            <TextInput
              {...field}
              type="date"
              value={periodStart}
              disabled={!!target}
              onChange={(event) => setPeriodStart(event.target.value)}
            />
          )}
        </Field>
        <Field
          label={t("reporting.value")}
          hint={t("reporting.moneyUnit", { currency: context.base_currency })}
          required
        >
          {(field) => (
            <TextInput
              {...field}
              type="number"
              min="0"
              step="any"
              value={amount}
              onChange={(event) => setAmount(event.target.value)}
            />
          )}
        </Field>
        <Field label={t("reporting.reason")} required>
          {(field) => (
            <TextInput
              {...field}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
          )}
        </Field>
        <ErrorLine error={write.error} />
        <div className="reporting-dialog-actions">
          <Button variant="ghost" onClick={onClose}>
            {t("reporting.cancel")}
          </Button>
          <Button
            type="submit"
            disabled={
              write.isPending ||
              !definition ||
              !amount ||
              !Number.isSafeInteger(value) ||
              value < 0 ||
              !reason.trim() ||
              !periodStart ||
              scope.kind === "managed_teams"
            }
          >
            {t("reporting.targets")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
