import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { reportingSectionsCatalog } from "./reporting.story-fixtures";
import { REPORTING_VIEWS, VIEW_SHAPE, viewRequest } from "./reporting.views";

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

const CONTRACT = readFileSync(
  new URL("../../../../backend/api/crm.yaml", import.meta.url),
  "utf8",
).split("\n");

// The lines of one top-level schema, read by indent: a schema name sits at
// four spaces.
function schemaLines(name: string): string[] {
  const start = CONTRACT.indexOf(`    ${name}:`);
  if (start < 0) throw new Error(`crm.yaml has no schema ${name}`);
  const end = CONTRACT.findIndex(
    (line, index) => index > start && /^ {0,4}\S/.test(line),
  );
  return CONTRACT.slice(start + 1, end < 0 ? undefined : end);
}

function selectionMetricsCap(): number {
  const lines = schemaLines("ReportingSelection");
  const metrics = lines.indexOf("        metrics:");
  for (const line of lines.slice(metrics + 1)) {
    if (/^ {8}\S/.test(line)) break;
    const cap = /^ {10}maxItems: (\d+)$/.exec(line);
    if (cap) return Number(cap[1]);
  }
  throw new Error("ReportingSelection.metrics declares no maxItems");
}

// A seat that may read every metric the contract names, each drawing every
// chart. Preset views ask the server's presets, which reportingpresets_test.go
// holds to the same bound.
function everyMetricCatalog(): components["schemas"]["ReportingCatalog"] {
  const ids = schemaLines("ReportingMetricID").flatMap((line) => {
    const id = /^ {6}- (\w+)$/.exec(line)?.[1];
    return id ? [id] : [];
  });
  return {
    metrics: ids.map((id) => ({
      ...leadCohort,
      id: id as Definition["id"],
      blocks: Object.values(VIEW_SHAPE).flatMap((shape) =>
        "blocks" in shape ? [...shape.blocks] : [],
      ),
    })),
    presets: [],
  };
}

describe("a view's request", () => {
  it("fits in one selection on a catalog holding every metric", () => {
    const cap = selectionMetricsCap();
    const catalog = everyMetricCatalog();
    expect(catalog.metrics.length).toBeGreaterThan(cap);

    for (const view of REPORTING_VIEWS) {
      for (const period of ["this_week", "this_quarter"] as const) {
        const asked = viewRequest(view, catalog, period).metrics;
        expect(asked.length, `${view} on ${period}`).toBeLessThanOrEqual(cap);
      }
    }
    expect(viewRequest("leads", catalog, "this_week").metrics).toContain(
      "source_quality",
    );
  });
});
