// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { AssessmentPanel, type SalesAssessment } from "./analytics.assessment";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;

afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

const written: SalesAssessment = {
  evaluation_key: "fixture-evaluation",
  generated_at: "2026-11-03T09:00:00Z",
  generated_by: "model",
  confidence: "medium",
  sentences: [
    {
      text: "Sales won stand at 216,000 EUR against 370,000 EUR open.",
      kind: "observation",
      cells: ["metric:bookings_won", "metric:open_pipeline"],
    },
  ],
  next_decision: {
    deal_id: "22222222-2222-4222-8222-222222222222",
    owner_id: "00000000-0000-4000-8000-000000000001",
    text: "Ask the buyer for the budget owner this week.",
  },
  cells: [
    { id: "metric:bookings_won", label: "bookings_won", value: "216000 EUR" },
    { id: "metric:open_pipeline", label: "open_pipeline", value: "370000 EUR" },
  ],
  decisions: 0,
};

function renderPanel(assessment: SalesAssessment) {
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /me": meRoute({
      report_definition: ["read"],
      deal: ["read"],
      activity: ["read", "create"],
    }),
    "POST /analytics/assessment": () => jsonResponse(assessment),
  });
  render(
    <StoryProviders>
      <AssessmentPanel
        selection={reportingStoryEvaluation.selection}
        evaluation={reportingStoryEvaluation}
        onEvidence={() => {}}
      />
    </StoryProviders>,
  );
}

it("shows the model's sentences and offers its next decision", async () => {
  renderPanel(written);
  expect(
    await screen.findByText(
      "Sales won stand at 216,000 EUR against 370,000 EUR open.",
    ),
  ).toBeInTheDocument();
  expect(
    screen.getByText(/Ask the buyer for the budget owner this week\./),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Record decision" }),
  ).toBeInTheDocument();
  expect(screen.getByText("Written by Margince")).toBeInTheDocument();
});

it("shows the page's own summary when the assessment was not written by a model", async () => {
  renderPanel({
    ...written,
    generated_by: "deterministic",
    sentences: [],
    next_decision: undefined,
    confidence: undefined,
  });
  expect(
    await screen.findByText("Compiled from CRM records"),
  ).toBeInTheDocument();
  expect(
    screen.getByText(
      "Written from the figures alone. An AI model would add its reading here once one is set up.",
    ),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Record decision" }),
  ).not.toBeInTheDocument();
});
