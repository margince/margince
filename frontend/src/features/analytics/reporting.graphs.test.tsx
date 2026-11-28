/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, expect, it, vi } from "vitest";
import { en as englishCopy } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ReportingEvidenceDrawer } from "./analytics.drawers";
import { ReportingCharts, wideCharts } from "./reporting.charts";
import { ReportingForecastGraphs } from "./reporting.forecast";
import { forecastEvaluation, sdrEvaluation } from "./reporting.scenarios";
import {
  reportingCatalogClient,
  reportingStoryEvaluation,
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;

// The chart bodies are lazy, so the first chart drawn in a file suspends. Many
// cases read the chart at once, so the chunks load here, before any case.
beforeAll(async () => {
  installFetchStub(reportingStoryRoutes());
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts
        evaluation={reportingStoryEvaluation}
        onEvidence={() => {}}
      />
    </StoryProviders>,
  );
  await screen.findAllByRole("figure");
  cleanup();
  globalThis.fetch = originalFetch;
});

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
  await waitFor(() =>
    expect(container.querySelectorAll(".report-chart-column")).toHaveLength(8),
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
    screen.getByRole("heading", {
      name: englishCopy["reporting.sdr_outcomes"],
    }),
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
  await user.click(await screen.findByRole("button", { name: /Opening.*760/ }));
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
    await screen.findByRole("combobox", {
      name: englishCopy["reporting.shareSnapshot"],
    }),
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

it("keeps the quarter's cards and Shared links on a week, and says why Share view waits for the quarter", async () => {
  installFetchStub({
    ...reportingStoryRoutes(forecastEvaluation),
    "GET /me": meRoute({
      report_definition: ["read"],
      forecast: ["read", "create"],
    }),
    "GET /forecast/shares": () => jsonResponse({ data: [] }),
  });
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingForecastGraphs scope={reportingStoryScope} period="week" />
    </StoryProviders>,
  );
  expect(
    await screen.findByRole("button", {
      name: englishCopy["analytics.share.listOpen"],
    }),
  ).toBeVisible();
  expect(screen.getByRole("button", { name: "Share view" })).toHaveAttribute(
    "disabled",
  );
  expect(
    screen.getAllByText(englishCopy["analytics.share.quarterOnly"]).length,
  ).toBeGreaterThan(0);
  expect(screen.getByText(englishCopy["forecast.quarterCards"])).toBeVisible();
  expect(
    await screen.findByRole("button", { name: /Opening.*760/ }),
  ).toBeVisible();
  expect(
    screen.queryByRole("combobox", {
      name: englishCopy["reporting.shareSnapshot"],
    }),
  ).not.toBeInTheDocument();
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

it("names every movement bar in words, never by the server's bucket key", async () => {
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
    expect((await screen.findAllByText(word)).length).toBeGreaterThan(0);
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
  await user.click(
    screen.getByRole("button", { name: englishCopy["reporting.viewRecords"] }),
  );
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
  expect(
    screen.queryByText(englishCopy["reporting.target_progress"]),
  ).not.toBeInTheDocument();
});

// The delta and the share are the server's; the card only words them.
it.each([
  { actual: 21600000, delta: -8400000, label: "€84,000 remaining" },
  { actual: 34000000, delta: 4000000, label: "€40,000 above target" },
])("shows target attainment with $label", ({ actual, delta, label }) => {
  const evaluation = {
    ...reportingStoryEvaluation,
    metrics: [
      {
        ...reportingStoryEvaluation.metrics[0],
        target: 30000000,
        target_actual: actual,
        target_delta: delta,
        attainment: (actual / 30000000) * 100,
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

// The progress line words what the server sent, which here is not what the
// target and actual would give (6 and 85%). A zero target has a gap and no
// share.
it.each([
  { delta: -10, attainment: 80, line: "Remaining: 10 · 80% of target" },
  { delta: 34, attainment: undefined, line: "Remaining: 0" },
])("words the server's figures as $line", ({ delta, attainment, line }) => {
  const evaluation = {
    ...sdrEvaluation,
    metrics: sdrEvaluation.metrics.map((metric) =>
      metric.id === "meetings_held"
        ? {
            ...metric,
            target: attainment == null ? 0 : 40,
            target_delta: delta,
            attainment,
          }
        : metric,
    ),
  };
  installFetchStub(reportingStoryRoutes(evaluation));
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  const lines = [...document.querySelectorAll("p.t-caption")].map(
    (node) => node.textContent,
  );
  expect(lines).toContain(line);
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
  await user.click(
    screen.getByRole("button", {
      name: englishCopy["reporting.contextDetails"],
    }),
  );
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
  expect(
    screen.getAllByText(englishCopy["reporting.status.failed"]),
  ).toHaveLength(1);
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
  expect(
    screen.getAllByText(englishCopy["reporting.status.failed"]).length,
  ).toBeGreaterThan(0);
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
    screen.getByRole("button", { name: /Discovery: .+ · 1 priced deal$/ }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: /Proposal: .+ · 7 priced deals$/ }),
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
    screen.getByText(englishCopy["reporting.creditCurrentOwner"]),
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
    screen.getByText(englishCopy["reporting.creditCapturedOwner"]),
  ).toBeInTheDocument();
  expect(
    screen.queryByText(englishCopy["reporting.creditCurrentOwner"]),
  ).toBeNull();
});

// With individual performance off, a reader sees only their own bar on the
// owner chart. The chart says why above its bars, or one bar reads as a team
// of one.
it("says why an owner chart drawn from the reader's own figures has one bar", () => {
  installFetchStub(reportingStoryRoutes());
  const reason =
    "An administrator switched off individual performance, so this shows your own figures only. Team totals stay complete.";
  const base = ownerChartEvaluation(false);
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingCharts
        evaluation={{
          ...base,
          charts: base.charts.map((chart) => ({
            ...chart,
            points: chart.points.slice(0, 1),
            coverage: { status: "partial", withheld: true, reason },
          })),
        }}
        onEvidence={() => {}}
      />
    </StoryProviders>,
  );
  expect(screen.getByText(reason)).toBeInTheDocument();
});

it("names the forecast's open pipeline by the quarter its deals close in", async () => {
  installFetchStub(reportingStoryRoutes(forecastEvaluation));
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingForecastGraphs scope={reportingStoryScope} />
    </StoryProviders>,
  );
  expect(
    (await screen.findAllByText(englishCopy["analytics.overview.openClosing"]))
      .length,
  ).toBeGreaterThan(0);
  expect(screen.queryByText("Open pipeline")).not.toBeInTheDocument();
});

it("titles the drawer after the tile that opened it, and dates mix rows by close day", async () => {
  const reference = {
    metric: "bookings_mix",
    context_id: "interval",
    group_key: "kind:software",
  } as const;
  const won = reportingStoryEvaluation.metrics.find(
    (metric) => metric.id === "bookings_won",
  );
  if (!won) throw new Error("the story evaluation reads sales won");
  const evaluation = {
    ...reportingStoryEvaluation,
    metrics: [
      ...reportingStoryEvaluation.metrics,
      { ...won, id: "bookings_mix", evidence: reference },
    ],
  } satisfies typeof reportingStoryEvaluation;
  installFetchStub(reportingStoryRoutes(evaluation));
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <ReportingEvidenceDrawer
        evaluation={evaluation}
        reference={reference}
        title="Product sales"
        onClose={() => {}}
      />
    </StoryProviders>,
  );
  expect(
    await screen.findByRole("heading", { name: "Product sales" }),
  ).toBeInTheDocument();
  expect(
    await screen.findByRole("columnheader", { name: "Closed on" }),
  ).toBeInTheDocument();
});
