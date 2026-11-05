import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { reportingSectionsCatalog } from "./reporting.story-fixtures";
import { VIEW_SHAPE, viewRequest } from "./reporting.views";

type Definition = components["schemas"]["ReportingMetricDefinition"];

const leadCohort: Definition = {
  id: "lead_cohort_outcomes",
  label: "What became of new leads",
  version: "1",
  unit: "percent",
  temporal_basis: "event_period",
  supports_target: false,
  definition: "How lead_cohort_outcomes is counted.",
  blocks: ["lead_cohort", "metric_reading"],
};

describe("the cohort readings", () => {
  it("open stage conversion on a view that takes a pipeline and no close window", () => {
    const asked = viewRequest(
      "conversion",
      reportingSectionsCatalog,
      "this_month",
    );

    expect(asked).toEqual({
      metrics: ["stage_conversion"],
      blocks: ["stage_cohort"],
    });
    expect(VIEW_SHAPE.conversion.dealFilters).toBe(true);
    expect(VIEW_SHAPE.conversion.closeWindow).toBe(false);
  });

  it("draw the lead cohort on the Leads view when the catalog holds it", () => {
    const catalog = {
      ...reportingSectionsCatalog,
      metrics: [...reportingSectionsCatalog.metrics, leadCohort],
    };

    const asked = viewRequest("leads", catalog, "this_month");

    expect(asked.metrics).toContain("lead_cohort_outcomes");
    expect(asked.blocks).toContain("lead_cohort");
    expect(
      viewRequest("leads", reportingSectionsCatalog, "this_month").blocks,
    ).not.toContain("lead_cohort");
  });
});
