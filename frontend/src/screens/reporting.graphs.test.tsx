/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { ReportingCharts } from "./reporting.charts";
import { ReportingForecastGraphs } from "./reporting.forecast";
import { forecastEvaluation, sdrEvaluation } from "./reporting.scenarios";
import {
  reportingStoryEvaluation,
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
  expect(screen.getByText(/Manager forecast/)).toBeVisible();
});

it("labels snapshot selection as sharing and keeps the forecast at one quarterly scope", async () => {
  installFetchStub(reportingStoryRoutes(forecastEvaluation));
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders>
      <ReportingForecastGraphs scope={reportingStoryScope} />
    </StoryProviders>,
  );
  expect(
    await screen.findByRole("combobox", { name: "Snapshot to share" }),
  ).toBeVisible();
  expect(
    screen.queryByRole("combobox", { name: "Pipeline" }),
  ).not.toBeInTheDocument();
  expect(
    fetch.mock.calls.some(
      ([input]) =>
        input instanceof Request && input.url.includes("period=this_quarter"),
    ),
  ).toBe(true);
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
    if (failure) expect(screen.getByRole("status")).toHaveTextContent(failure);
    else expect(screen.queryByRole("status")).not.toBeInTheDocument();
  },
);

it("opens the stage-age chart evidence context", async () => {
  const user = userEvent.setup({ delay: null });
  const evidence = vi.fn();
  const evaluation = {
    ...forecastEvaluation,
    charts: forecastEvaluation.charts.filter(
      (chart) => chart.kind === "stage_age",
    ),
  };
  installFetchStub(reportingStoryRoutes(evaluation));
  render(
    <StoryProviders>
      <ReportingCharts evaluation={evaluation} onEvidence={evidence} />
    </StoryProviders>,
  );
  expect(
    screen.queryByRole("button", { name: /Open my worklist/ }),
  ).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "View records" }));
  expect(evidence).toHaveBeenCalledWith({
    metric: evaluation.charts[0].metric,
    context_id: evaluation.charts[0].context_id,
  });
});

it("shows accepted handoffs alone and hides targets without an allocation", async () => {
  const evaluation = {
    ...sdrEvaluation,
    metrics: sdrEvaluation.metrics.filter(
      (metric) => metric.id === "accepted_opportunities",
    ),
    charts: sdrEvaluation.charts
      .filter((chart) => chart.metric === "accepted_opportunities")
      .map((chart) => ({
        ...chart,
        points: chart.points.map((point) => ({ ...point, target: undefined })),
      })),
  };
  installFetchStub(reportingStoryRoutes(evaluation));
  render(
    <StoryProviders>
      <ReportingCharts evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  expect(
    screen.getByRole("heading", { name: "Accepted handoffs" }),
  ).toBeVisible();
  expect(
    document.querySelectorAll(".report-chart-column").length,
  ).toBeGreaterThan(0);
  expect(screen.queryByText("Progress against target")).not.toBeInTheDocument();
});

it.each([
  { actual: 21600000, label: "€84k remaining" },
  { actual: 34000000, label: "€40k above target" },
])("shows target attainment with $label", ({ actual, label }) => {
  const evaluation = {
    ...reportingStoryEvaluation,
    metrics: [
      {
        ...reportingStoryEvaluation.metrics[0],
        target: 30000000,
        target_actual: actual,
      },
    ],
  };
  installFetchStub(reportingStoryRoutes(evaluation));
  render(
    <StoryProviders>
      <ReportingCharts evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  expect(screen.getByText(label, { exact: false })).toBeVisible();
});

it("omits unassigned target columns for SDR outcomes", async () => {
  const user = userEvent.setup({ delay: null });
  const evaluation = {
    ...sdrEvaluation,
    charts: sdrEvaluation.charts.map((chart) => ({
      ...chart,
      points: chart.points.map((point) => ({ ...point, target: undefined })),
    })),
    metrics: sdrEvaluation.metrics.map((metric) => ({
      ...metric,
      target: undefined,
      target_actual: undefined,
    })),
  };
  installFetchStub(reportingStoryRoutes(evaluation));
  render(
    <StoryProviders>
      <ReportingCharts evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  await user.click(screen.getByText("All metrics"));
  expect(
    screen.queryByRole("columnheader", { name: "Target" }),
  ).not.toBeInTheDocument();
  expect(
    screen.getByText("34", { selector: ".stat-card-value" }),
  ).toBeVisible();
});
