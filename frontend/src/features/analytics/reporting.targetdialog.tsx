import "./reporting.css";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch } from "../../api/version";
import { Button, Checkbox, Field, TextInput } from "../../design-system/atoms";
import { isISODate } from "../../design-system/dateinput";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../../design-system/drawerbands";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { QueryGate } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { formatDateAbbrev, monthName } from "../../format/format";
import { toMajorUnits, toMinorUnits } from "../../format/minorunits";
import { dayInZone, UTC_ZONE } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { fiscalQuarter, opensFiscalQuarter } from "../../lib/fiscalquarter";
import { usePipelines } from "../../lib/pipelines.queries";
import { throwProblem } from "../../lib/problem";
import { useInstallationSettings } from "../../lib/uploadlimit";
import { AnalyticsScopePicker } from "./analytics.scope";
import { metricLabel, type ReportingMetricID } from "./reporting.model";
import { useReportingCatalog } from "./reporting.queries";
import { TargetHistory } from "./reporting.targethistory";

type Target = components["schemas"]["ReportingTarget"];
type TargetInput = components["schemas"]["ReportingTargetInput"];
export function ReportingTargetDialog({
  target,
  context,
  onClose,
}: Readonly<{
  target?: Target;
  context: components["schemas"]["AnalyticsContext"];
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const actionLabel = t(target ? "reporting.revise" : "reporting.newTarget");
  const title = useId();
  const formId = useId();
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
  const installation = useInstallationSettings();
  const fiscalStart = installation.data?.fiscal_year_start_month;
  const [periodStart, setPeriodStart] = useState(
    initialTargetPeriod(target, context),
  );
  const [pipeline, setPipeline] = useState(
    target?.definition.pipeline_id ?? "",
  );
  const [retired, setRetired] = useState(isRetired(target));
  const [reason, setReason] = useState("");
  const [amount, setAmount] = useState(
    targetAmount(target, context.base_currency),
  );
  const catalog = useReportingCatalog();
  const definition = catalog.data?.metrics.find(
    (candidate) => candidate.id === metric,
  );
  const value = targetInputValue(
    amount,
    definition?.unit,
    context.base_currency,
  );
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
      await client.invalidateQueries({ queryKey: ["reporting-live"] });
      await client.invalidateQueries({ queryKey: ["reporting-target"] });
      onClose();
    },
  });
  return (
    <Modal open onClose={onClose} labelledBy={title} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={title} className="t-h2 modal-title">
          {actionLabel}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        <form
          id={formId}
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            write.mutate({
              previous: target,
              input: {
                metric,
                scope,
                pipeline_id: heldPipeline(target, definition, pipeline),
                period_kind: periodKind,
                period_start: periodStart,
                value,
                reason,
                retired,
              },
            });
          }}
        >
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
          <TargetPipelineField
            definition={definition}
            disabled={!!target}
            value={pipeline}
            onChange={setPipeline}
          />
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
                  if (value === "month" || value === "fiscal_quarter") {
                    setPeriodKind(value);
                    if (
                      value === "fiscal_quarter" &&
                      fiscalStart != null &&
                      periodStart
                    ) {
                      setPeriodStart(
                        fiscalQuarter(periodStart, fiscalStart).from,
                      );
                    }
                  }
                }}
              />
            )}
          </Field>
          <div className="reporting-period-controls">
            <Field label={t("reporting.periodYear")} required>
              {(field) => (
                <TextInput
                  {...field}
                  type="number"
                  min="1900"
                  max="9999"
                  disabled={!!target}
                  value={periodStart.split("-")[0]}
                  onChange={(event) =>
                    setPeriodStart(
                      `${event.target.value}-${periodStart.split("-")[1] || "01"}-01`,
                    )
                  }
                />
              )}
            </Field>
            <Field label={t("reporting.periodMonth")} required>
              {(field) => (
                <Select
                  {...field}
                  disabled={
                    !!target ||
                    (periodKind === "fiscal_quarter" && fiscalStart == null)
                  }
                  value={periodStart.split("-")[1] ?? "01"}
                  options={Array.from({ length: 12 }, (_, index) => index + 1)
                    .filter(
                      (month) =>
                        periodKind === "month" ||
                        (fiscalStart != null &&
                          opensFiscalQuarter(month, fiscalStart)),
                    )
                    .map((month) => ({
                      value: String(month).padStart(2, "0"),
                      label: monthName(month, locale),
                    }))}
                  onChange={(month) =>
                    setPeriodStart(`${periodStart.split("-")[0]}-${month}-01`)
                  }
                />
              )}
            </Field>
          </div>
          {isISODate(periodStart) && (
            <p className="t-caption">
              {formatDateAbbrev(periodStart, locale, context.timezone)} –{" "}
              {formatDateAbbrev(
                targetPeriodEnd(periodStart, periodKind),
                locale,
                context.timezone,
              )}
            </p>
          )}
          <ErrorLine error={installation.error} />
          <Field
            label={t("reporting.value")}
            hint={
              definition?.unit === "money"
                ? context.base_currency
                : t("reporting.wholeCount")
            }
            required
          >
            {(field) => (
              <TextInput
                {...field}
                type="number"
                min="0"
                step={definition?.unit === "money" ? "any" : "1"}
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
          {target && (
            <>
              <Checkbox
                label={t("reporting.retired")}
                checked={retired}
                onChange={(event) => setRetired(event.target.checked)}
              />
              <p className="t-caption">{t("reporting.retiredHelp")}</p>
              <TargetHistory target={target} />
            </>
          )}
          <ErrorLine error={write.error} />
        </form>
      </DrawerBody>
      <DrawerFoot className="actions">
        <Button variant="ghost" onClick={onClose}>
          {t("reporting.cancel")}
        </Button>
        <Button
          type="submit"
          form={formId}
          disabled={
            write.isPending ||
            !definition ||
            !amount ||
            !validTargetNumber(value) ||
            !targetPeriodReady(reason, periodStart) ||
            (periodKind === "fiscal_quarter" &&
              (fiscalStart == null ||
                !opensFiscalQuarter(
                  Number(periodStart.split("-")[1] ?? "01"),
                  fiscalStart,
                ))) ||
            scope.kind === "managed_teams"
          }
        >
          {actionLabel}
        </Button>
      </DrawerFoot>
    </Modal>
  );
}

type MetricDefinition = components["schemas"]["ReportingMetricDefinition"];

// A figure without the pipeline filter counts every pipeline, and the server
// refuses a target held to one.
function takesPipeline(definition: MetricDefinition | undefined): boolean {
  return !!definition?.allowed_filters?.includes("pipeline");
}

function TargetPipelineField({
  definition,
  disabled,
  value,
  onChange,
}: Readonly<{
  definition: MetricDefinition | undefined;
  disabled: boolean;
  value: string;
  onChange: (value: string) => void;
}>) {
  const t = useT();
  const pipelines = usePipelines();
  if (!takesPipeline(definition)) return null;
  return (
    <Field label={t("reporting.pipeline")}>
      {(field) => (
        <Select
          {...field}
          disabled={disabled}
          value={value}
          options={[
            { value: "", label: t("reporting.allPipelines") },
            ...(pipelines.data ?? []).map((pipeline) => ({
              value: pipeline.id,
              label: pipeline.name,
            })),
          ]}
          onChange={onChange}
        />
      )}
    </Field>
  );
}

// A revision keeps its target's pipeline, which the server holds immutable,
// whether or not the catalog has loaded.
function heldPipeline(
  target: Target | undefined,
  definition: MetricDefinition | undefined,
  pipeline: string,
): string | undefined {
  if (target) return target.definition.pipeline_id;
  return takesPipeline(definition) && pipeline ? pipeline : undefined;
}

function targetAmount(target: Target | undefined, currency: string): string {
  if (!target) return "";
  return String(
    target.unit === "count"
      ? target.definition.value
      : toMajorUnits(target.definition.value, currency),
  );
}

function validTargetNumber(value: number): boolean {
  return Number.isSafeInteger(value) && value >= 0;
}

function targetPeriodReady(reason: string, start: string): boolean {
  return !!reason.trim() && isISODate(start) && start.endsWith("-01");
}

function isRetired(target: Target | undefined): boolean {
  return target?.definition.retired === true;
}

function initialTargetPeriod(
  target: Target | undefined,
  context: components["schemas"]["AnalyticsContext"],
): string {
  return (
    target?.definition.period_start ??
    `${dayInZone(Date.parse(context.as_of), context.timezone).slice(0, 7)}-01`
  );
}

function targetPeriodEnd(
  start: string,
  kind: TargetInput["period_kind"],
): string {
  const year = Number(start.slice(0, 4));
  const month = Number(start.slice(5, 7));
  return dayInZone(
    Date.UTC(year, month - 1 + (kind === "month" ? 1 : 3), 0),
    UTC_ZONE,
  );
}

function targetInputValue(
  amount: string,
  unit: string | undefined,
  currency: string,
): number {
  return unit === "money"
    ? toMinorUnits(Number(amount), currency)
    : Number(amount);
}
