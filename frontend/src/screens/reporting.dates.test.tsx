/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ReportingOverview } from "./reporting.overview";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";
import { ResultsSummary } from "./reporting.summary";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
  window.location.hash = "";
});

it("shows the server cutoff when a custom range includes future days", async () => {
  window.location.hash =
    "#/analytics/performance?period=custom&from=2026-01-01&through=2026-10-31&pipeline=all";
  installFetchStub(reportingStoryRoutes(reportingStoryEvaluation));
  render(
    <StoryProviders>
      <ReportingOverview scope={reportingStoryScope} />
    </StoryProviders>,
  );
  expect(await screen.findByText(/^Results through /)).toHaveTextContent(
    "22/09/2026, 14:00",
  );
  expect(
    screen.queryByRole("button", { name: "Retry" }),
  ).not.toBeInTheDocument();
});

it.each([
  {
    name: "oversized",
    from: "2025-01-01",
    through: "2026-10-31",
    message: "choose an interval of no more than twelve months",
  },
  {
    name: "reversed",
    from: "2026-10-31",
    through: "2026-01-01",
    message: "choose an end date after the start date",
  },
])(
  "asks for corrected $name dates without retrying",
  async ({ from, through, message }) => {
    window.location.hash = `#/analytics/performance?period=custom&from=${from}&through=${through}`;
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/evaluate": () =>
        jsonResponse(
          {
            status: 400,
            code: "reporting_interval_invalid",
            detail: message,
          },
          400,
        ),
    });
    render(
      <StoryProviders>
        <ReportingOverview scope={reportingStoryScope} />
      </StoryProviders>,
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(message);
    expect(
      screen.queryByRole("button", { name: "Retry" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/Reload the page/)).not.toBeInTheDocument();
    expect(screen.queryByText(/invalid argument/)).not.toBeInTheDocument();
  },
);

it("labels an empty sales period without calling unknown values zero", () => {
  const evaluation: typeof reportingStoryEvaluation = {
    ...reportingStoryEvaluation,
    metrics: reportingStoryEvaluation.metrics
      .filter((m) => m.id === "bookings_won")
      .map((m) => ({
        ...m,
        value: 0,
        coverage: { ...m.coverage, status: "no_data", eligible_count: 0 },
      })),
  };
  const { rerender } = render(
    <StoryProviders>
      <ResultsSummary evaluation={evaluation} onEvidence={() => {}} />
    </StoryProviders>,
  );
  expect(screen.getByText(/^No sales won · /)).toBeVisible();
  rerender(
    <StoryProviders>
      <ResultsSummary
        evaluation={{
          ...evaluation,
          metrics: evaluation.metrics.map((m) => ({
            ...m,
            value: 0,
            coverage: {
              ...m.coverage,
              status: "partial",
              eligible_count: 2,
              priced_count: 0,
            },
          })),
        }}
        onEvidence={() => {}}
      />
    </StoryProviders>,
  );
  expect(screen.queryByText(/^No sales won/)).not.toBeInTheDocument();
  expect(screen.getByText("Partial coverage")).toBeVisible();
});

it("preserves the server explanation for a broken installation calendar", async () => {
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/evaluate": () =>
      jsonResponse(
        {
          status: 400,
          code: "invalid_argument",
          detail: "reporting timezone is unavailable",
        },
        400,
      ),
  });
  render(
    <StoryProviders>
      <ReportingOverview scope={reportingStoryScope} />
    </StoryProviders>,
  );
  expect(
    await screen.findByText("reporting timezone is unavailable"),
  ).toBeVisible();
  expect(screen.queryByText(/twelve months/)).not.toBeInTheDocument();
});
