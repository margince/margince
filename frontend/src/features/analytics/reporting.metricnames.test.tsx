/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
  storyQueryClient,
} from "../../testing/story-utils";
import { ReportingCharts } from "./reporting.charts";
import { reportingCatalogKey } from "./reporting.queries";
import { forecastEvaluation } from "./reporting.scenarios";
import {
  reportingStoryCatalog,
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
});

const stageAge = {
  ...forecastEvaluation,
  metrics: forecastEvaluation.metrics.filter(
    (metric) => metric.id === "stage_age",
  ),
  charts: forecastEvaluation.charts.filter(
    (chart) => chart.kind === "stage_age",
  ),
};

it("draws no metric until the catalog answers, so no id shows", () => {
  installFetchStub({
    ...reportingStoryRoutes(stageAge),
    "GET /analytics/metrics": () => new Promise<Response>(() => {}),
  });
  render(
    <StoryProviders>
      <ReportingCharts evaluation={stageAge} onEvidence={() => {}} />
    </StoryProviders>,
  );
  expect(document.body).not.toHaveTextContent("stage_age");
  expect(screen.queryByRole("heading")).not.toBeInTheDocument();
});

it("names the stage-age chart as the catalog names its metric", async () => {
  const client = storyQueryClient();
  client.setQueryDefaults(reportingCatalogKey, {
    staleTime: Number.POSITIVE_INFINITY,
  });
  client.setQueryData(reportingCatalogKey, {
    ...reportingStoryCatalog,
    metrics: reportingStoryCatalog.metrics.map((metric) =>
      metric.id === "stage_age"
        ? { ...metric, label: "Days in stage" }
        : metric,
    ),
  });
  installFetchStub(reportingStoryRoutes(stageAge));
  render(
    <StoryProviders client={client}>
      <ReportingCharts evaluation={stageAge} onEvidence={() => {}} />
    </StoryProviders>,
  );
  expect(
    await screen.findByRole("heading", { name: "Days in stage" }),
  ).toBeVisible();
});

it("names a metric the catalog no longer offers as unavailable, never by id", async () => {
  installFetchStub({
    ...reportingStoryRoutes(reportingStoryEvaluation),
    "GET /analytics/metrics": () =>
      jsonResponse({
        ...reportingStoryCatalog,
        metrics: reportingStoryCatalog.metrics.filter(
          (metric) => metric.id !== "bookings_won",
        ),
      }),
  });
  render(
    <StoryProviders>
      <ReportingCharts
        evaluation={reportingStoryEvaluation}
        onEvidence={() => {}}
      />
    </StoryProviders>,
  );
  expect(
    (await screen.findAllByText(en["reporting.metricUnavailable"])).length,
  ).toBeGreaterThan(0);
  expect(document.body).not.toHaveTextContent("bookings_won");
});
