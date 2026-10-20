import { REPORTING_FIXTURE_ZONE } from "./reporting.fixtures";
/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { RecordZoneProvider } from "../../lib/recordzone";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ANSWER } from "./analytics.questions.testkit";
import { ReportingCharts } from "./reporting.charts";
import { SaveReportingDialog } from "./reporting.save";
import {
  reportingStoryEvaluation,
  reportingStoryReport,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

const question = {
  entity: "deals-by-stage",
  group_by: ["stage_id"],
  measures: [{ fn: "count" as const }],
};

const withQuestionCard = {
  ...reportingStoryEvaluation,
  selection: {
    ...reportingStoryEvaluation.selection,
    layout: [
      {
        cards: [
          {
            id: "by-stage",
            kind: "question" as const,
            width: "full" as const,
            question,
          },
        ],
      },
    ],
  },
};

function draw(editionId?: string) {
  render(
    <StoryProviders>
      <RecordZoneProvider zone={REPORTING_FIXTURE_ZONE}>
        <ReportingCharts
          evaluation={withQuestionCard}
          editionId={editionId}
          onEvidence={() => {}}
        />
      </RecordZoneProvider>
    </StoryProviders>,
  );
}

it("asks a question card over the report's scope, for this reader", async () => {
  const ask = vi.fn(() => jsonResponse(ANSWER));
  installFetchStub({ ...reportingStoryRoutes(), "POST /analytics/query": ask });
  draw();
  await waitFor(() =>
    expect(ask).toHaveBeenCalledWith(
      expect.objectContaining({
        entity: "deals-by-stage",
        scope_kind: reportingStoryEvaluation.selection.scope.kind,
        scope_id: reportingStoryEvaluation.selection.scope.id,
      }),
    ),
  );
});

it("says a sent edition keeps no answer, and asks nothing", async () => {
  const ask = vi.fn(() => jsonResponse(ANSWER));
  installFetchStub({ ...reportingStoryRoutes(), "POST /analytics/query": ask });
  draw("edition-1");
  expect(
    await screen.findByText(/Not kept in sent reports/),
  ).toBeInTheDocument();
  expect(ask).not.toHaveBeenCalled();
});

it("adds a saved question from its link, without its own scope", async () => {
  const user = userEvent.setup({ delay: null });
  const write = vi.fn(() => jsonResponse(reportingStoryReport));
  const runId = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b";
  installFetchStub({
    ...reportingStoryRoutes(),
    [`GET /analytics/runs/${runId}`]: () =>
      jsonResponse({
        id: runId,
        query: { ...question, scope_kind: "team", scope_id: "t-9" },
        answer: ANSWER,
        asked_by: "u-1",
        stored_floor: 5,
      }),
    "PATCH /analytics/reports/report": write,
  });
  render(
    <StoryProviders>
      <SaveReportingDialog
        selection={reportingStoryReport.selection}
        report={reportingStoryReport}
        onClose={() => {}}
        onSaved={() => {}}
      />
    </StoryProviders>,
  );
  await user.click(
    screen.getByText("Charts and order", { selector: ".disclosure-label" }),
  );
  await user.type(
    await screen.findByRole("textbox", {
      name: "Link to a saved custom report",
    }),
    `https://crm.example/app#/analytics/questions/${runId}`,
  );
  await user.click(screen.getByRole("button", { name: "Add question" }));
  await user.click(screen.getByRole("button", { name: "Save report" }));
  await waitFor(() => expect(write).toHaveBeenCalled());
  expect(write).toHaveBeenCalledWith(
    expect.objectContaining({
      selection: expect.objectContaining({
        layout: [
          expect.objectContaining({
            cards: expect.arrayContaining([
              expect.objectContaining({ kind: "question", question }),
            ]),
          }),
        ],
      }),
    }),
  );
});
