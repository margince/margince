/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { StoryProviders } from "../../testing/story-utils";
import type { ReportingEvaluation } from "./reporting.model";
import { reportingStoryEvaluation } from "./reporting.story-fixtures";
import { TargetComparison } from "./reporting.targetcomparison";

afterEach(cleanup);

type Comparison = NonNullable<
  ReportingEvaluation["metrics"][number]["target_comparison"]
>;

function evaluationWith(comparison: Comparison): ReportingEvaluation {
  return {
    ...reportingStoryEvaluation,
    metrics: [
      {
        id: "forecast_landing",
        version: "1",
        unit: "EUR",
        value: 76_000_000,
        coverage: { status: "ok", withheld: false },
        evidence: { metric: "forecast_landing", context_id: "state" },
        target_comparison: comparison,
      },
    ],
  };
}

function show(comparison: Comparison) {
  render(
    <StoryProviders>
      <TargetComparison evaluation={evaluationWith(comparison)} />
    </StoryProviders>,
  );
}

// The review's own example: 800k target, 420k won, a 760k landing and 1.36m
// open. The server gave every figure; the screen only formats them.
const sample: Comparison = {
  status: "compared",
  target_metric: "bookings_won",
  basis: "manager_call",
  target: 80_000_000,
  landing: 76_000_000,
  won: 42_000_000,
  gap: 4_000_000,
  remaining: 38_000_000,
  open_pipeline: 136_000_000,
  coverage: 136 / 38,
  qualified: { status: "not_enforced" },
};

it("shows the gap, what is left and the coverage the server compared", () => {
  show(sample);
  expect(screen.getByText("Gap to target")).toBeVisible();
  expect(screen.getByText("€40k")).toBeVisible();
  expect(screen.getByText("Short of the €800k target")).toBeVisible();
  expect(screen.getByText("3.6×")).toBeVisible();
  expect(screen.getByText("Not switched on")).toBeVisible();
});

it("labels qualified coverage apart from coverage over all open deals", () => {
  show({
    ...sample,
    qualified: { status: "compared", open_pipeline: 57_000_000, coverage: 1.5 },
  });
  expect(screen.getByText("Coverage, all open deals")).toBeVisible();
  expect(screen.getByText("Coverage, qualified deals")).toBeVisible();
  expect(screen.getByText("1.5×")).toBeVisible();
});

it("reads a target already won as reached, never as an endless coverage", () => {
  show({
    ...sample,
    status: "target_reached",
    remaining: 0,
    coverage: undefined,
    gap: -15_000_000,
  });
  expect(screen.getAllByText("Target reached").length).toBeGreaterThan(0);
  expect(screen.queryByText(/×$/)).toBeNull();
  expect(screen.getByText(/^Ahead of the /)).toBeVisible();
});

it.each([
  ["no_target", undefined, "No sales target for this quarter"],
  ["fx_missing", undefined, "Some deal amounts have no exchange rate"],
  ["partial_visibility", undefined, "You cannot see every deal in this scope"],
  [
    "incompatible",
    "period",
    "Pick this fiscal quarter to compare with a target",
  ],
  ["incompatible", "scope", "Pick one team to compare with a target"],
  ["incompatible", "pipeline", "No target is set for this pipeline"],
  [
    "incompatible",
    "manager_call_pipeline",
    "Pick All pipelines to compare the manager’s call",
  ],
  ["incompatible", "currency", "The target is in another currency"],
] as const)(
  "explains a %s (%s) comparison and draws no figure",
  (status, incompatibility, title) => {
    show({ status, incompatibility, target_metric: "bookings_won" });
    expect(screen.getByText(title)).toBeVisible();
    expect(screen.queryByText("Gap to target")).toBeNull();
  },
);
