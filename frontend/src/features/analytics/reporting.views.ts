import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";
import type { Section } from "../../lib/analytics.address";
import type {
  ReportingBlockKind,
  ReportingMetricID,
  ReportingSelection,
} from "./reporting.model";

type Catalog = components["schemas"]["ReportingCatalog"];
type PresetID = components["schemas"]["ReportingPreset"]["id"];

// The views of Performance, in the order the tabs offer them.
export const REPORTING_VIEWS = [
  "sales",
  "sdr",
  "leads",
  "partners",
  "conversion",
] as const;
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
  leads_awaiting_response: "leads",
  partner_pipeline: "partners",
  partner_won: "partners",
  partner_commission: "partners",
  partner_activity: "partners",
  stage_conversion: "conversion",
  lead_cohort_outcomes: "leads",
};

// What each view is called, the tab that opens it, what it asks, and whether
// its figures take a pipeline and a close window. Only deal views do: the lead
// readers and the partner commission and activity readers refuse a pipeline.
// Stage conversion takes a pipeline but follows entries, not close dates.
// A view asks either the server's preset of that name or the charts listed.
export const VIEW_SHAPE: Readonly<
  Record<
    ReportingView,
    Readonly<
      {
        label: MessageKey;
        section: Section;
        dealFilters: boolean;
        closeWindow: boolean;
      } & ({ preset: PresetID } | { blocks: readonly ReportingBlockKind[] })
    >
  >
> = {
  sales: {
    label: "reporting.sales",
    section: "performance",
    preset: "sales",
    dealFilters: true,
    closeWindow: true,
  },
  sdr: {
    label: "reporting.sdr",
    section: "sdr",
    preset: "sdr",
    dealFilters: false,
    closeWindow: false,
  },
  leads: {
    label: "reporting.leads",
    section: "leads",
    blocks: ["breakdown", "target_progress", "lead_cohort"],
    dealFilters: false,
    closeWindow: false,
  },
  partners: {
    label: "reporting.partners",
    section: "partners",
    blocks: ["breakdown", "target_progress"],
    dealFilters: false,
    closeWindow: false,
  },
  conversion: {
    label: "reporting.conversion",
    section: "conversion",
    blocks: ["stage_cohort"],
    dealFilters: true,
    closeWindow: false,
  },
};

/** The view a section draws, or undefined for a section that is no view. */
export function viewOfSection(section: Section): ReportingView | undefined {
  return REPORTING_VIEWS.find((view) => VIEW_SHAPE[view].section === section);
}

// The views a seat is offered on this catalog. A preset view is offered when
// the catalog publishes its preset, because that preset is all it asks. Any
// other view is offered when the catalog holds one of its metrics.
export function readableViews(
  catalog: Pick<Catalog, "metrics" | "presets">,
): ReportingView[] {
  return REPORTING_VIEWS.filter((view) => {
    const shape = VIEW_SHAPE[view];
    if ("preset" in shape) {
      return catalog.presets.some((preset) => preset.id === shape.preset);
    }
    return catalog.metrics.some((metric) => METRIC_VIEW[metric.id] === view);
  });
}

export function metricView(metric: ReportingMetricID): ReportingView {
  return METRIC_VIEW[metric];
}

// The expected finish is always this fiscal quarter's, so a view asks it only
// on a page that reads this quarter too.
const QUARTER_ONLY: ReportingMetricID = "forecast_landing";

/** The metrics and charts the server's preset of this id asks this reader. */
export function presetRequest(
  id: PresetID,
  catalog: Pick<Catalog, "presets">,
): Readonly<{ metrics: ReportingMetricID[]; blocks: ReportingBlockKind[] }> {
  const preset = catalog.presets.find((entry) => entry.id === id);
  return {
    metrics: [...(preset?.metrics ?? [])],
    blocks: [...(preset?.blocks ?? [])],
  };
}

/**
 * The metrics and charts a view asks on this catalog. A preset view asks the
 * preset the server published for this reader, plus the expected finish on a
 * page that reads this quarter. Any other view asks every metric it reads and
 * the charts they draw.
 */
export function viewRequest(
  view: ReportingView,
  catalog: Catalog,
  period: ReportingSelection["period"],
): Readonly<{ metrics: ReportingMetricID[]; blocks: ReportingBlockKind[] }> {
  const shape = VIEW_SHAPE[view];
  const quarterly =
    period === "this_quarter" &&
    METRIC_VIEW[QUARTER_ONLY] === view &&
    catalog.metrics.some((metric) => metric.id === QUARTER_ONLY);
  if ("preset" in shape) {
    const asked = presetRequest(shape.preset, catalog);
    return quarterly
      ? { ...asked, metrics: [...asked.metrics, QUARTER_ONLY] }
      : asked;
  }
  const metrics = catalog.metrics
    .filter(
      (metric) =>
        METRIC_VIEW[metric.id] === view &&
        (metric.id !== QUARTER_ONLY || quarterly),
    )
    .map((metric) => metric.id);
  return {
    metrics,
    blocks: shape.blocks.filter((block) =>
      catalog.metrics.some(
        (metric) =>
          metrics.includes(metric.id) && metric.blocks.includes(block),
      ),
    ),
  };
}
