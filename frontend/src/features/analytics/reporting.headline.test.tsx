/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { reportingStoryCatalog } from "../../testing/reporting.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ReportingOverview } from "./reporting.overview";
import { reportingCatalogKey } from "./reporting.queries";
import {
  reportingCatalogClient,
  reportingStoryEvaluation,
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";
import { ResultsSummary } from "./reporting.summary";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
  window.location.hash = "";
});

const landing = {
  id: "forecast_landing",
  label: "Forecast landing",
  version: "1",
  unit: "money",
  temporal_basis: "state_at",
  supports_target: false,
  definition: "The deal forecast's expected finish for this fiscal quarter.",
  blocks: ["forecast_support", "metric_reading"],
} as const;

// The metrics each evaluation asked for, read off the request address.
function recordEvaluations(): string[][] {
  const asked: string[][] = [];
  const stub = globalThis.fetch;
  globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(
      input instanceof Request ? input.url : String(input),
      "http://test",
    );
    if (url.pathname.endsWith("/analytics/evaluate")) {
      asked.push(url.searchParams.getAll("metrics"));
    }
    return stub(input, init);
  }) as typeof fetch;
  return asked;
}

it.each([
  { period: "this_quarter", asksLanding: true },
  { period: "this_month", asksLanding: false },
])(
  "asks for the expected finish only when the page reads this quarter ($period)",
  async ({ period, asksLanding }) => {
    window.location.hash = `#/analytics/performance?period=${period}`;
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/metrics": () =>
        jsonResponse({
          ...reportingStoryCatalog,
          metrics: [...reportingStoryCatalog.metrics, landing],
          schedule_ready: true,
        }),
    });
    const asked = recordEvaluations();
    render(
      <StoryProviders>
        <ReportingOverview view="sales" scope={reportingStoryScope} />
      </StoryProviders>,
    );
    await vi.waitFor(() => expect(asked.length).toBeGreaterThan(0));
    const metrics = asked.at(-1) ?? [];
    expect(metrics).toContain("open_pipeline");
    expect(metrics.includes("forecast_landing")).toBe(asksLanding);
  },
);

it("says open pipeline is a count now and the expected finish is this quarter's", async () => {
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/metrics": () =>
      jsonResponse({
        ...reportingStoryCatalog,
        metrics: [...reportingStoryCatalog.metrics, landing],
      }),
  });
  const evaluation: typeof reportingStoryEvaluation = {
    ...reportingStoryEvaluation,
    metrics: [
      ...reportingStoryEvaluation.metrics.map((metric) =>
        metric.id === "open_pipeline"
          ? { ...metric, coverage: { ...metric.coverage, eligible_count: 3 } }
          : metric,
      ),
      {
        id: "forecast_landing",
        version: "1",
        unit: "EUR",
        value: 76000000,
        coverage: { status: "ok", withheld: false },
        evidence: { metric: "forecast_landing", context_id: "state" },
        forecast_basis: {
          landing: {
            amount_minor: 76000000,
            measure: "weighted",
            won_minor: 42000000,
            remaining_minor: 34000000,
          },
          period: reportingStoryEvaluation.context.interval,
          eligible_count: 3,
          priced_count: 3,
          fx_missing_count: 0,
        },
      },
    ],
  };
  render(
    <StoryProviders>
      <ResultsSummary evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  expect(await screen.findByText("Forecast landing")).toBeVisible();
  expect(screen.getByText(/^Open deals: 3 · as of /)).toBeVisible();
  expect(
    screen.getByText(
      "This fiscal quarter, from the deal forecast · Won + weighted open deals",
    ),
  ).toBeVisible();
  const labels = screen
    .getAllByText(/^(Sales won|Forecast landing|Open pipeline)$/)
    .map((node) => node.textContent);
  expect(labels).toEqual(["Sales won", "Forecast landing", "Open pipeline"]);
});

it("keeps a withheld deal count unknown instead of printing zero", () => {
  const evaluation: typeof reportingStoryEvaluation = {
    ...reportingStoryEvaluation,
    metrics: reportingStoryEvaluation.metrics.map((metric) =>
      metric.id === "open_pipeline"
        ? {
            ...metric,
            coverage: { ...metric.coverage, eligible_count: undefined },
          }
        : metric,
    ),
  };
  render(
    <StoryProviders>
      <ResultsSummary evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  expect(screen.getByText(/^As of /)).toBeVisible();
  expect(screen.queryByText(/Open deals: 0/)).toBeNull();
});

it("opens the forecast section from the forecast landing, not an empty evidence list", async () => {
  const onEvidence = vi.fn();
  const evaluation: typeof reportingStoryEvaluation = {
    ...reportingStoryEvaluation,
    metrics: [
      {
        id: "forecast_landing",
        version: "1",
        unit: "EUR",
        value: 76000000,
        coverage: { status: "ok", withheld: false },
        evidence: { metric: "forecast_landing", context_id: "state" },
      },
    ],
  };
  window.location.hash = "#/analytics/performance";
  render(
    <StoryProviders client={reportingCatalogClient()}>
      <ResultsSummary evaluation={evaluation} onEvidence={onEvidence} />
    </StoryProviders>,
  );
  await userEvent.setup().click(screen.getByRole("link", { name: "Open" }));
  expect(onEvidence).not.toHaveBeenCalled();
  expect(window.location.hash).toMatch(/^#\/analytics\/forecast/);
});

it("heads the sales figures with the win rate and new qualified pipeline beside sales won", () => {
  const evaluation: typeof reportingStoryEvaluation = {
    ...reportingStoryEvaluation,
    metrics: [
      ...reportingStoryEvaluation.metrics,
      {
        id: "closed_win_rate",
        version: "1",
        unit: "percent",
        value: 42,
        coverage: { status: "ok", withheld: false },
        evidence: { metric: "closed_win_rate", context_id: "period" },
      },
      {
        id: "qualified_pipeline_created",
        version: "1",
        unit: "EUR",
        value: 12000000,
        coverage: { status: "ok", withheld: false },
        evidence: {
          metric: "qualified_pipeline_created",
          context_id: "period",
        },
      },
    ],
  };
  // Every reading is named from the catalog, so the win rate needs its entry.
  const client = reportingCatalogClient();
  client.setQueryData(
    reportingCatalogKey,
    (catalog: typeof reportingStoryCatalog) => ({
      ...catalog,
      metrics: [
        ...catalog.metrics,
        { ...landing, id: "closed_win_rate", label: "Closed win rate" },
      ],
    }),
  );
  render(
    <StoryProviders client={client}>
      <ResultsSummary evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  const labels = screen
    .getAllByText(
      /^(Sales won|Closed win rate|Qualified pipeline created|Open pipeline)$/,
    )
    .map((node) => node.textContent);
  expect(labels).toEqual([
    "Sales won",
    "Closed win rate",
    "Qualified pipeline created",
    "Open pipeline",
  ]);
  expect(screen.getByText("42%")).toBeVisible();
});
