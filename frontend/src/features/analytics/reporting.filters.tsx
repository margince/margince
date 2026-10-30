import { useState } from "react";
import { Field } from "../../design-system/atoms";
import {
  DateInput,
  isISODate,
  isoOrBlank,
} from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { Select } from "../../design-system/select";
import { useNow } from "../../format/now";
import { dayInZone, startOfDayInZone } from "../../format/timezone";
import { useT } from "../../i18n";
import { usePipelines } from "../../lib/pipelines.queries";
import { useRecordZone } from "../../lib/recordzone";
import { useAnalyticsContext } from "./analytics.context";
import { AnalyticsScopePicker } from "./analytics.scope";
import type { ReportingSelection } from "./reporting.model";

export const REPORTING_PERIODS: readonly ReportingSelection["period"][] = [
  "this_week",
  "this_month",
  "last_month",
  "last_week",
  "this_quarter",
  "custom",
];

export function ReportingFilters({
  selection,
  onChange,
  showScope = true,
  showPipeline = true,
  showTargets = true,
  showCloseWindow = showPipeline,
  dates: controlledDates,
}: Readonly<{
  showScope?: boolean;
  showPipeline?: boolean;
  showTargets?: boolean;
  showCloseWindow?: boolean;
  dates?: Readonly<{
    from: string;
    through: string;
    onChange: (from: string, through: string) => void;
  }>;
  selection: ReportingSelection;
  onChange: (selection: ReportingSelection) => void;
}>) {
  const t = useT();
  const zone = useRecordZone();
  // The day the period's bounds are counted in, not the viewer's own.
  const today = isoOrBlank(dayInZone(useNow(0), zone)) || undefined;
  const context = useAnalyticsContext();
  // A view whose figures take no pipeline never waits on the pipelines.
  const pipelines = usePipelines(showPipeline);
  const [draftFrom, setDraftFrom] = useState(
    selection.interval
      ? dayInZone(Date.parse(selection.interval.start_at), zone)
      : "",
  );
  const [draftThrough, setDraftThrough] = useState(
    selection.interval
      ? dayInZone(Date.parse(selection.interval.end_at) - 1, zone)
      : "",
  );
  const dates = controlledDates;
  const from = dates?.from ?? draftFrom;
  const through = dates?.through ?? draftThrough;
  const setDates = (start: string, end: string) => {
    if (dates) {
      dates.onChange(start, end);
      return;
    }
    setDraftFrom(start);
    setDraftThrough(end);
    if (!isISODate(start) || !isISODate(end)) {
      onChange({ ...selection, interval: undefined });
      return;
    }
    onChange({
      ...selection,
      interval: {
        start_at: startOfDayInZone(start, zone),
        end_at: startOfDayInZone(end, zone, 1),
      },
    });
  };
  return (
    <div className="reporting-toolbar">
      {showScope && context.data && (
        <AnalyticsScopePicker
          scopes={context.data.allowed_scopes}
          selected={{ ...selection.scope, label: selection.scope.label ?? "" }}
          onSelect={(scope) =>
            onChange({
              ...selection,
              scope: { ...scope, id: scope.id ?? undefined },
            })
          }
        />
      )}
      <Field label={t("reporting.period")}>
        {(field) => (
          <Select
            {...field}
            value={selection.period}
            options={REPORTING_PERIODS.map((value) => ({
              value,
              label: t(`reporting.${value}`),
            }))}
            onChange={(value) => {
              const period = REPORTING_PERIODS.find(
                (candidate) => candidate === value,
              );
              if (period) onChange({ ...selection, period });
            }}
          />
        )}
      </Field>
      {selection.period === "custom" && (
        <>
          <Field label={t("reporting.fromDate")}>
            {(field) => (
              <DateInput
                {...field}
                value={isoOrBlank(from)}
                today={today}
                onChange={(event) => setDates(event.target.value, through)}
              />
            )}
          </Field>
          <Field label={t("reporting.throughDate")}>
            {(field) => (
              <DateInput
                {...field}
                value={isoOrBlank(through)}
                today={today}
                onChange={(event) => setDates(from, event.target.value)}
              />
            )}
          </Field>
        </>
      )}
      {showPipeline && (
        <Field label={t("reporting.pipeline")}>
          {(field) => (
            <Select
              {...field}
              value={selection.pipeline_id ?? ""}
              options={[
                { value: "", label: t("reporting.allPipelines") },
                ...(pipelines.data ?? []).map((pipeline) => ({
                  value: pipeline.id,
                  label: pipeline.name,
                })),
              ]}
              onChange={(value) =>
                onChange({ ...selection, pipeline_id: value || undefined })
              }
            />
          )}
        </Field>
      )}
      {showTargets && (
        <Field label={t("reporting.targetBasis")}>
          {(field) => (
            <Select
              {...field}
              value={selection.target_basis}
              options={[
                { value: "month", label: t("reporting.month") },
                {
                  value: "fiscal_quarter",
                  label: t("reporting.fiscal_quarter"),
                },
              ]}
              onChange={(value) => {
                if (value === "month" || value === "fiscal_quarter")
                  onChange({ ...selection, target_basis: value });
              }}
            />
          )}
        </Field>
      )}
      {showCloseWindow && (
        <Field label={t("reporting.closeWindow")}>
          {(field) => (
            <Select
              {...field}
              value={selection.close_window}
              options={[
                { value: "all_open", label: t("reporting.all_open") },
                {
                  value: "fiscal_quarter",
                  label: t("reporting.fiscal_quarter"),
                },
              ]}
              onChange={(value) => {
                if (value === "all_open" || value === "fiscal_quarter")
                  onChange({ ...selection, close_window: value });
              }}
            />
          )}
        </Field>
      )}
      <ErrorLine error={context.error ?? pipelines.error} />
    </div>
  );
}
