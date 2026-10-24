import type { MessageKey } from "../../i18n/en";
import type { ReportingBlockKind, ReportingMetricID } from "./reporting.model";

// The views of Performance, in the order the switch offers them.
export const REPORTING_VIEWS = ["sales", "sdr", "leads", "partners"] as const;
export type ReportingView = (typeof REPORTING_VIEWS)[number];

// Which view reads each metric. A Record over every metric id, so a metric the
// contract adds fails the type check until it is given a view.
const METRIC_VIEW: Readonly<Record<ReportingMetricID, ReportingView>> = {
  bookings_won: "sales",
  closed_win_rate: "sales",
  open_pipeline: "sales",
  stage_age: "sales",
  qualified_pipeline_created: "sales",
  forecast_landing: "sales",
  meetings_held: "sdr",
  accepted_opportunities: "sdr",
  leads_created: "leads",
  lead_status_moves: "leads",
  leads_promoted: "leads",
  lead_first_response: "leads",
  partner_pipeline: "partners",
  partner_won: "partners",
  partner_commission: "partners",
  partner_activity: "partners",
};

// What each view is called, which charts it draws, and whether its figures
// take a pipeline and a close window. Only deal views do: the lead readers and
// the partner commission and activity readers refuse a pipeline.
export const VIEW_SHAPE: Readonly<
  Record<
    ReportingView,
    Readonly<{
      label: MessageKey;
      blocks: readonly ReportingBlockKind[];
      dealFilters: boolean;
    }>
  >
> = {
  sales: {
    label: "reporting.sales",
    blocks: [
      "bookings_trend",
      "stage_distribution",
      "owner_attainment",
      "stage_age",
    ],
    dealFilters: true,
  },
  sdr: {
    label: "reporting.sdr",
    blocks: ["sdr_outcomes", "target_progress"],
    dealFilters: false,
  },
  leads: {
    label: "reporting.leads",
    blocks: ["breakdown", "target_progress"],
    dealFilters: false,
  },
  partners: {
    label: "reporting.partners",
    blocks: ["breakdown", "target_progress"],
    dealFilters: false,
  },
};

export function metricView(metric: ReportingMetricID): ReportingView {
  return METRIC_VIEW[metric];
}

export function isReportingView(
  value: string | null | undefined,
): value is ReportingView {
  return REPORTING_VIEWS.some((view) => view === value);
}
