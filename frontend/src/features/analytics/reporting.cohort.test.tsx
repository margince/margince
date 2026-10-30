/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { StoryProviders } from "../../testing/story-utils";
import { CohortTable } from "./reporting.cohort";
import type { ReportingChart } from "./reporting.model";

afterEach(cleanup);

const count = (key: string, members: number, median: number | null = null) => ({
  key,
  label: key,
  count: members,
  median_days: median,
  duration_basis:
    key === "unresolved" ? ("current_age" as const) : ("completed" as const),
  evidence: {
    metric: "stage_conversion" as const,
    context_id: "interval",
    group_key: `demo:${key}`,
  },
});

function stageCohort(partial: number): ReportingChart {
  return {
    kind: "stage_cohort",
    metric: "stage_conversion",
    context_id: "interval",
    unit: "percent",
    coverage: { status: "ok", withheld: false },
    points: [
      {
        key: "demo",
        label: "Demo",
        value: null,
        observations: 4 + partial,
        status: "insufficient_sample",
        evidence: {
          metric: "stage_conversion",
          context_id: "interval",
          group_key: "stage:demo",
        },
        outcomes: [
          count("advanced", 2, 3),
          count("lost", 1, 9),
          count("unresolved", 1, 14),
          count("history_partial", partial),
        ],
        entries: [
          { key: "first", label: "first", count: 3 + partial },
          { key: "repeat", label: "repeat", count: 1 },
          { key: "reopened", label: "reopened", count: 0 },
        ],
      },
    ],
  };
}

it("opens the members behind an outcome's count and says the still open stay out of the rate", async () => {
  const user = userEvent.setup({ delay: null });
  const evidence = vi.fn();
  render(
    <StoryProviders>
      <CohortTable
        chart={stageCohort(0)}
        label="Stage conversion"
        onEvidence={evidence}
      />
    </StoryProviders>,
  );

  await user.click(screen.getByRole("button", { name: "2" }));

  expect(evidence).toHaveBeenLastCalledWith({
    metric: "stage_conversion",
    context_id: "interval",
    group_key: "demo:advanced",
  });
  expect(screen.getByText(en["reporting.cohortStillOpenNote"])).toBeVisible();
  expect(screen.getByText(en["reporting.cohortNoRate"])).toBeVisible();
  expect(screen.queryByText(en["reporting.cohortPartialNote"])).toBeNull();
});

it("explains a direction that was not recorded only when a member has one", () => {
  render(
    <StoryProviders>
      <CohortTable
        chart={stageCohort(1)}
        label="Stage conversion"
        onEvidence={vi.fn()}
      />
    </StoryProviders>,
  );

  expect(screen.getByText(en["reporting.cohortPartialNote"])).toBeVisible();
});
