/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { ReportingCharts, wideCharts } from "./reporting.charts";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import { ReportingForecastGraphs } from "./reporting.forecast";
import { forecastEvaluation, sdrEvaluation } from "./reporting.scenarios";
import {
  reportingCatalogClient,
  reportingStoryEvaluation,
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";

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
    <StoryProviders client={reportingCatalogClient()}>
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
    <StoryProviders client={reportingCatalogClient()}>
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
    <StoryProviders client={reportingCatalogClient()}>
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
    <StoryProviders client={reportingCatalogClient()}>
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

it("names every movement bar in words, never by the server's bucket key", () => {
  const keys = ["new", "reopened_or_archived", "stage_weight", "fx"];
  const evaluation = {
    ...forecastEvaluation,
    charts: forecastEvaluation.charts
      .filter((chart) => chart.kind === "pipeline_movement")
      .map((chart) => ({
        ...chart,
        points: keys.map((key) => ({
          key,
          label: key,
          value: 1000000,
          status: "ok",
        })),
      })),
  } satisfies typeof forecastEvaluation;
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  for (const word of [
    "New deals",
    "Reopened or archived",
    "Stage weight changes",
    "Exchange rate changes",
  ]) {
    expect(screen.getAllByText(word).length).toBeGreaterThan(0);
  }
  for (const key of keys) {
    expect(screen.queryByText(key)).not.toBeInTheDocument();
  }
});

it("opens forecast source evidence and returns to the chart", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub(reportingStoryRoutes(forecastEvaluation));
  const fetch = vi.spyOn(globalThis, "fetch");
  render(
    <StoryProviders client={reportingCatalogClient()}>
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
      <StoryProviders client={reportingCatalogClient()}>
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
    <StoryProviders client={reportingCatalogClient()}>
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
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  expect(
    await screen.findByRole("heading", { name: "Accepted handoffs" }),
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
    <StoryProviders client={reportingCatalogClient()}>
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
    <StoryProviders client={reportingCatalogClient()}>
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

it("withholds stage-age marks for a small cohort without printing zero days", async () => {
  const user = userEvent.setup({ delay: null });
  const evaluation = {
    ...reportingStoryEvaluation,
    charts: reportingStoryEvaluation.charts
      .filter((chart) => chart.kind === "stage_age")
      .map((chart) => ({
        ...chart,
        points: chart.points.slice(0, 1).map((point) => ({
          ...point,
          value: null,
          upper: null,
          observations: 2,
          status: "insufficient_sample",
        })),
      })),
  } satisfies typeof reportingStoryEvaluation;
  installFetchStub(reportingStoryRoutes(evaluation));
  const { container } = render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  expect(
    screen.getByRole("button", {
      name: /Discovery: Median days —; 75th percentile —/,
    }),
  ).toBeVisible();
  expect(container.querySelector(".report-chart-range-dot")).toBeNull();
  expect(container.querySelector(".report-chart-range-band")).toBeNull();
  await user.click(screen.getByRole("button", { name: "Reporting details" }));
  expect(
    screen.getByText(/Observations: 2 · Too few observations/),
  ).toBeVisible();
});

it("spans a half-width chart across its row when no chart can share it", () => {
  const kinds = forecastEvaluation.charts.map((chart) => chart.kind);
  expect(kinds).toEqual([
    "forecast_support",
    "pipeline_movement",
    "stage_distribution",
    "stage_age",
  ]);
  // The support chart is followed by the full-width waterfall, so it has no
  // partner; the two stage charts pair up under it.
  expect(wideCharts(forecastEvaluation.charts)).toEqual([
    true,
    true,
    false,
    false,
  ]);
  // Three half-width charts: the first two share a row, the third takes one.
  const halves = forecastEvaluation.charts.filter(
    (chart) => chart.kind !== "pipeline_movement",
  );
  expect(wideCharts(halves)).toEqual([false, false, true]);
});

it("keeps the movement label readable in its evidence drawer", async () => {
  const reference = {
    metric: "open_pipeline",
    context_id: "movement_delta",
    group_key: "reopened_or_archived",
  } as const;
  const evaluation = {
    ...forecastEvaluation,
    charts: forecastEvaluation.charts
      .filter((chart) => chart.kind === "pipeline_movement")
      .map((chart) => ({
        ...chart,
        points: [
          {
            key: "reopened_or_archived",
            label: "reopened_or_archived",
            value: 100,
            status: "ok",
            evidence: reference,
          },
        ],
      })),
  } satisfies typeof forecastEvaluation;
  installFetchStub(reportingStoryRoutes(evaluation));
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingEvidenceDrawer
        evaluation={evaluation}
        reference={reference}
        onClose={() => {}}
      />
    </StoryProviders>,
  );
  expect(await screen.findByText(/Reopened or archived/)).toBeInTheDocument();
  expect(screen.queryByText(/reopened_or_archived/)).not.toBeInTheDocument();
});

it("marks one failed chart and keeps drawing the others", () => {
  installFetchStub(reportingStoryRoutes());
  const baseline = render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts
        evaluation={reportingStoryEvaluation}
        onEvidence={() => {}}
      />
    </StoryProviders>,
  );
  const figuresBefore = screen.getAllByRole("figure").length;
  baseline.unmount();

  const [first, ...others] = reportingStoryEvaluation.charts;
  const evaluation = {
    ...reportingStoryEvaluation,
    charts: [
      {
        ...first,
        points: [],
        coverage: {
          status: "failed",
          withheld: false,
          reason: "This figure could not be calculated just now.",
        },
      },
      ...others,
    ],
  } satisfies typeof reportingStoryEvaluation;
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  expect(screen.getAllByText("Could not be calculated")).toHaveLength(1);
  expect(
    screen.getByText("This figure could not be calculated just now."),
  ).toBeInTheDocument();
  expect(screen.getAllByRole("figure").length).toBeGreaterThanOrEqual(
    figuresBefore - 1,
  );
});

it("keeps the accepted-opportunities series when only the meetings reading failed", () => {
  installFetchStub(reportingStoryRoutes(sdrEvaluation));
  const evaluation = {
    ...sdrEvaluation,
    charts: sdrEvaluation.charts.map((chart) =>
      chart.kind === "sdr_outcomes" && chart.metric === "meetings_held"
        ? {
            ...chart,
            points: [],
            coverage: { status: "failed", withheld: false },
          }
        : chart,
    ),
  } satisfies typeof sdrEvaluation;
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  expect(screen.getAllByText("Could not be calculated").length).toBeGreaterThan(
    0,
  );
  expect(screen.getAllByText(/Week 1/).length).toBeGreaterThan(0);
});

it("shows how many priced deals make each stage's value", () => {
  installFetchStub(reportingStoryRoutes());
  const evaluation = {
    ...reportingStoryEvaluation,
    charts: reportingStoryEvaluation.charts
      .filter((chart) => chart.kind === "stage_distribution")
      .map((chart) => ({
        ...chart,
        points: chart.points.map((point, index) => ({
          ...point,
          observations: index === 0 ? 1 : 7,
        })),
      })),
  } satisfies typeof reportingStoryEvaluation;
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  expect(
    screen.getByRole("button", {
      name: /Discovery: .+ · 1 priced deal, 27%$/,
    }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", {
      name: /Proposal: .+ · 7 priced deals, 41%$/,
    }),
  ).toBeInTheDocument();
});

function ownerChartEvaluation(withTargets: boolean) {
  return {
    ...reportingStoryEvaluation,
    context: {
      ...reportingStoryEvaluation.context,
      evaluated_at: "2026-10-16T00:00:00Z",
      target_interval: {
        start_at: "2026-10-01T00:00:00Z",
        end_at: "2026-10-31T00:00:00Z",
      },
    },
    charts: reportingStoryEvaluation.charts
      .filter(
        (chart) =>
          chart.kind === "owner_attainment" && chart.metric === "bookings_won",
      )
      .map((chart) => ({
        ...chart,
        points: chart.points.map((point) => ({
          ...point,
          target: withTargets ? 100_000 : undefined,
        })),
      })),
  } satisfies typeof reportingStoryEvaluation;
}

it("says whom an owner chart credits and how much of the target period has passed", () => {
  installFetchStub(reportingStoryRoutes());
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts
        evaluation={ownerChartEvaluation(true)}
        onEvidence={() => {}}
      />
    </StoryProviders>,
  );
  expect(
    screen.getByText("Credited to each deal’s current owner"),
  ).toBeInTheDocument();
  expect(
    screen.getByText("50% of the target period has passed"),
  ).toBeInTheDocument();
});

it("says nothing about the target period where no target applies to the chart", () => {
  installFetchStub(reportingStoryRoutes());
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts
        evaluation={ownerChartEvaluation(false)}
        onEvidence={() => {}}
      />
    </StoryProviders>,
  );
  expect(screen.queryByText(/of the target period has passed/)).toBeNull();
});

it("credits a saved edition's sales to the owner it captured", () => {
  installFetchStub(reportingStoryRoutes());
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts
        evaluation={ownerChartEvaluation(true)}
        editionId="e-1"
        onEvidence={() => {}}
      />
    </StoryProviders>,
  );
  expect(
    screen.getByText(
      "Credited to each deal’s owner when this edition was captured",
    ),
  ).toBeInTheDocument();
  expect(
    screen.queryByText("Credited to each deal’s current owner"),
  ).toBeNull();
});
