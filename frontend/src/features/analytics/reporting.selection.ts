import type { components } from "../../api/schema";
import { type ISODate, isoOrBlank } from "../../format/datepattern";
import { startOfDayInZone } from "../../format/timezone";
import { defaultPipelineOf } from "../../lib/pipelines.queries";
import {
  currentParams,
  replaceDial,
  replaceParams,
  type UrlParams,
  useUrlParams,
} from "../../lib/urlstate";
import type { Dimensions } from "./reporting.dimensions";
import type { ReportingSelection } from "./reporting.model";

// Every period the contract names, in the order a picker lists them.
// reporting.selection.test.ts holds the list to the contract's enum.
export const REPORTING_PERIODS = [
  "this_week",
  "last_week",
  "this_month",
  "last_month",
  "this_quarter",
  "last_quarter",
  "last_year",
  "custom",
] as const satisfies readonly ReportingSelection["period"][];

/** The periods words can name: all but a custom window, which they cannot bound. */
export const PRESET_PERIODS = REPORTING_PERIODS.filter(
  (period): period is Exclude<typeof period, "custom"> => period !== "custom",
);

/** Whether `period` is a fiscal quarter, so its target is one too. */
export function readsFiscalQuarter(period: ReportingSelection["period"]) {
  return period === "this_quarter" || period === "last_quarter";
}

/** Whether `period` moves with the installation's fiscal year start. */
export function followsFiscalYear(period: ReportingSelection["period"]) {
  return readsFiscalQuarter(period) || period === "last_year";
}

export type ValueBasis = NonNullable<ReportingSelection["value_basis"]>;

export const VALUE_BASES: readonly ValueBasis[] = [
  "all",
  "software",
  "implementation",
];

export function valueBasisOf(value: string): ValueBasis {
  return VALUE_BASES.find((basis) => basis === value) ?? "all";
}

export function readOverviewFilters(
  params: UrlParams,
  pipelines: readonly components["schemas"]["Pipeline"][],
  defaultPeriod: ReportingSelection["period"] = "this_month",
) {
  const start = isoOrBlank(params.get("from") ?? "");
  const end = isoOrBlank(params.get("through") ?? "");
  const period =
    REPORTING_PERIODS.find((value) => value === params.get("period")) ??
    defaultPeriod;
  const pipelineId =
    params.get("pipeline") === "all"
      ? ""
      : (pipelines.find((pipeline) => pipeline.id === params.get("pipeline"))
          ?.id ??
        defaultPipelineOf(pipelines)?.id ??
        "");
  const targetBasis: ReportingSelection["target_basis"] = readsFiscalQuarter(
    period,
  )
    ? "fiscal_quarter"
    : "month";
  const closeWindow: ReportingSelection["close_window"] =
    params.get("close") === "fiscal_quarter" ? "fiscal_quarter" : "all_open";
  const dimensions: Dimensions = {
    product_id: params.get("product") || undefined,
    initiative_id: params.get("initiative") || undefined,
    country: params.get("country") || undefined,
    industry: params.get("industry") || undefined,
    value_basis:
      params.get("basis") && params.get("basis") !== "all"
        ? valueBasisOf(params.get("basis") ?? "")
        : undefined,
  };
  return {
    start,
    end,
    period,
    pipelineId,
    targetBasis,
    closeWindow,
    dimensions,
  };
}

export function useOverviewFilters(
  pipelines: readonly components["schemas"]["Pipeline"][],
  defaultPeriod: ReportingSelection["period"] = "this_month",
) {
  const [params] = useUrlParams();
  const filters = readOverviewFilters(params, pipelines, defaultPeriod);
  const setDimensions = (next: Dimensions) => {
    const dials = new Map(currentParams());
    const pairs: [string, string | undefined][] = [
      ["product", next.product_id],
      ["initiative", next.initiative_id],
      ["country", next.country],
      ["industry", next.industry],
      ["basis", next.value_basis === "all" ? undefined : next.value_basis],
    ];
    for (const [dial, value] of pairs) {
      if (value) dials.set(dial, value);
      else dials.delete(dial);
    }
    replaceParams(dials);
  };
  return {
    ...filters,
    setStart: (value: string) => replaceDial("from", value),
    setEnd: (value: string) => replaceDial("through", value),
    setPeriod: (value: string) => replaceDial("period", value),
    setPipelineId: (value: string) => replaceDial("pipeline", value || "all"),
    setTargetBasis: (value: string) => replaceDial("target", value),
    setCloseWindow: (value: string) => replaceDial("close", value),
    setDimensions,
  };
}

export function customInterval(
  period: string,
  start: ISODate | "",
  end: ISODate | "",
  zone: string,
): ReportingSelection["interval"] {
  if (period !== "custom" || !start || !end) return undefined;
  return {
    start_at: startOfDayInZone(start, zone),
    end_at: startOfDayInZone(end, zone, 1),
  };
}

export function validDateRange(
  period: string,
  start: string,
  end: string,
): boolean {
  return period !== "custom" || !!(start && end);
}

// A pipeline the address names that is not among the reader's pipelines.
export function pipelineIsGone(
  named: string | undefined,
  pipelines: readonly components["schemas"]["Pipeline"][],
): boolean {
  return (
    !!named && named !== "all" && !pipelines.some(({ id }) => id === named)
  );
}
