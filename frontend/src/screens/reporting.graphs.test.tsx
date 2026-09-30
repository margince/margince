/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { pickOption } from "../design-system/select-testing";
import { ReportingCharts } from "./reporting.charts";
import { ReportingForecastGraphs } from "./reporting.forecast";
import { forecastEvaluation, sdrEvaluation } from "./reporting.scenarios";
import {
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";
import { installFetchStub, StoryProviders } from "./story-utils";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
  window.location.hash = "";
});

it("keeps meeting and accepted-handoff evidence distinct in the paired graph", async () => {
  const user = userEvent.setup({ delay: null });
  const evidence = vi.fn();
  installFetchStub(reportingStoryRoutes(sdrEvaluation));
  const { container } = render(
    <StoryProviders>
      <ReportingCharts evaluation={sdrEvaluation} onEvidence={evidence} />
    </StoryProviders>,
  );
  const bars = container.querySelectorAll<HTMLButtonElement>(
    ".report-chart-column",
  );
  expect(bars).toHaveLength(8);
  await user.click(bars[0]);
  expect(evidence).toHaveBeenLastCalledWith({
    metric: "meetings_held",
    context_id: "month",
    group_key: "week:0",
  });
  await user.click(bars[1]);
  expect(evidence).toHaveBeenLastCalledWith({
    metric: "accepted_opportunities",
    context_id: "month",
    group_key: "week:0",
  });
  expect(
    screen.getByRole("heading", { name: "Meetings and accepted handoffs" }),
  ).toBeVisible();
});

it("renders forecast support and reconciled movement with boundary evidence", async () => {
  const user = userEvent.setup({ delay: null });
  const evidence = vi.fn();
  installFetchStub(reportingStoryRoutes(forecastEvaluation));
  render(
    <StoryProviders>
      <ReportingCharts evaluation={forecastEvaluation} onEvidence={evidence} />
    </StoryProviders>,
  );
  await user.click(screen.getByRole("button", { name: /Opening.*760/ }));
  expect(evidence).toHaveBeenLastCalledWith({
    metric: "open_pipeline",
    context_id: "movement_opening",
  });
  await user.click(screen.getByRole("button", { name: /Closing.*880/ }));
  expect(evidence).toHaveBeenLastCalledWith({
    metric: "open_pipeline",
    context_id: "movement_closing",
  });
  await user.click(screen.getByRole("button", { name: /New.*220/ }));
  expect(evidence).toHaveBeenLastCalledWith({
    metric: "open_pipeline",
    context_id: "movement_delta",
    group_key: "new",
  });
  expect(screen.getByText(/Manager call/)).toBeVisible();
});

it("changes forecast pipeline context and removes whole-scope snapshot sharing", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub(reportingStoryRoutes(forecastEvaluation));
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <ReportingForecastGraphs scope={reportingStoryScope} />
    </StoryProviders>,
  );
  expect(
    await screen.findByRole("combobox", { name: "Frozen edition" }),
  ).toBeVisible();
  await pickOption(
    user,
    screen.getByRole("combobox", { name: "Pipeline" }),
    "Sales",
  );
  await waitFor(() =>
    expect(
      fetch.mock.calls.some(
        ([input]) =>
          input instanceof Request && input.url.includes("pipeline_id=sales"),
      ),
    ).toBe(true),
  );
  expect(
    screen.queryByRole("combobox", { name: "Frozen edition" }),
  ).not.toBeInTheDocument();
  await pickOption(
    user,
    screen.getByRole("combobox", { name: "Pipeline" }),
    "All pipelines",
  );
  expect(
    await screen.findByRole("combobox", { name: "Frozen edition" }),
  ).toBeVisible();
});

it("shows missing capture history as unavailable instead of drawing a fabricated bridge", () => {
  installFetchStub(reportingStoryRoutes());
  const evaluation = {
    ...forecastEvaluation,
    charts: forecastEvaluation.charts
      .filter((chart) => chart.kind === "pipeline_movement")
      .map((chart) => ({
        ...chart,
        opening: undefined,
        closing: undefined,
        points: [],
        coverage: {
          status: "unavailable",
          withheld: false,
          reason: "Collecting movement history",
        },
      })),
  } satisfies typeof forecastEvaluation;
  render(
    <StoryProviders>
      <ReportingCharts evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  expect(
    screen.getAllByText("Collecting movement history").length,
  ).toBeGreaterThan(0);
  expect(
    screen.queryByRole("button", { name: /Opening/ }),
  ).not.toBeInTheDocument();
});

it("opens forecast source evidence and returns to the chart", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub(reportingStoryRoutes(forecastEvaluation));
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <ReportingForecastGraphs scope={reportingStoryScope} />
    </StoryProviders>,
  );
  await user.click(await screen.findByRole("button", { name: /Opening.*760/ }));
  expect(await screen.findByText("Northstar rollout")).toBeVisible();
  expect(
    fetch.mock.calls.some(
      ([input]) =>
        input instanceof Request &&
        input.url.includes("context_id=movement_opening"),
    ),
  ).toBe(true);
  await user.click(screen.getByRole("button", { name: "Close" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it.each(["Capture failed; retry the scheduled capture", undefined])(
  "shows capture health without inventing a successful capture: %s",
  (failure) => {
    const evaluation = {
      ...forecastEvaluation,
      charts: forecastEvaluation.charts
        .filter((chart) => chart.kind === "pipeline_movement")
        .map((chart) => ({
          ...chart,
          capture_status: {
            last_attempt_at: "2026-09-22T07:00:00Z",
            next_capture_at: "2026-09-23T07:00:00Z",
            failure,
          },
        })),
    } satisfies typeof forecastEvaluation;
    installFetchStub(reportingStoryRoutes(evaluation));
    render(
      <StoryProviders>
        <ReportingCharts evaluation={evaluation} onEvidence={() => {}} />
      </StoryProviders>,
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      failure ?? "Unavailable",
    );
  },
);

it("opens the at-risk worklist from stage age while frozen editions remain historical", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub(reportingStoryRoutes(forecastEvaluation));
  const view = render(
    <StoryProviders>
      <ReportingCharts evaluation={forecastEvaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  await user.click(
    screen.getByRole("button", { name: /Review at-risk deals/ }),
  );
  expect(window.location.hash).toContain("deals_at_risk");
  view.rerender(
    <StoryProviders>
      <ReportingCharts
        evaluation={forecastEvaluation}
        editionId="frozen"
        onEvidence={() => {}}
      />
    </StoryProviders>,
  );
  expect(
    screen.queryByRole("button", { name: /Review at-risk deals/ }),
  ).not.toBeInTheDocument();
});
