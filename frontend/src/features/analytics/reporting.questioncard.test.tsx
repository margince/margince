import { REPORTING_FIXTURE_ZONE } from "../../testing/reporting.fixtures";
/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { RecordZoneProvider } from "../../lib/recordzone";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ANSWER, SAVED_RUNS } from "./analytics.questions.testkit";
import { ReportingCharts } from "./reporting.charts";
import type { KeptQuestions } from "./reporting.questioncard";
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

function draw(editionId?: string, rolePreview?: boolean, kept?: KeptQuestions) {
  render(
    <StoryProviders>
      <RecordZoneProvider zone={REPORTING_FIXTURE_ZONE}>
        <ReportingCharts
          evaluation={withQuestionCard}
          editionId={editionId}
          kept={kept}
          onEvidence={() => {}}
          rolePreview={rolePreview}
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

it("shows the answer a sent edition kept, and asks nothing", async () => {
  const ask = vi.fn(() => jsonResponse(ANSWER));
  installFetchStub({ ...reportingStoryRoutes(), "POST /analytics/query": ask });
  draw("edition-1", false, {
    redacted: false,
    questions: [{ card_id: "by-stage", status: "kept", answer: ANSWER }],
  });
  expect(
    await screen.findByRole("table", { name: "Answer" }),
  ).toBeInTheDocument();
  expect(screen.queryByText(/Not kept/)).not.toBeInTheDocument();
  expect(ask).not.toHaveBeenCalled();
});

it("says why a sent edition kept no answer, and asks nothing", async () => {
  const ask = vi.fn(() => jsonResponse(ANSWER));
  installFetchStub({ ...reportingStoryRoutes(), "POST /analytics/query": ask });
  draw("edition-1", false, {
    redacted: false,
    questions: [
      { card_id: "by-stage", status: "not_kept", reason: "too_many_records" },
    ],
  });
  expect(
    await screen.findByText(/counted more records than a sent report keeps/),
  ).toBeInTheDocument();
  expect(ask).not.toHaveBeenCalled();
});

it("shows nothing of a kept answer this reader may not ask for", async () => {
  installFetchStub(reportingStoryRoutes());
  draw("edition-1", false, {
    redacted: false,
    questions: [{ card_id: "by-stage", status: "withheld" }],
  });
  expect(
    await screen.findByText(/Your access does not cover this question/),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("table", { name: "Answer" }),
  ).not.toBeInTheDocument();
});

it("says an expired edition's answer is gone, not that it was never kept", async () => {
  installFetchStub(reportingStoryRoutes());
  draw("edition-1", false, { redacted: true, expired: true });
  expect(await screen.findByText(/No longer available/)).toBeInTheDocument();
});

it("asks nothing under a role preview, which this session cannot ask as", async () => {
  const ask = vi.fn(() => jsonResponse(ANSWER));
  installFetchStub({ ...reportingStoryRoutes(), "POST /analytics/query": ask });
  draw(undefined, true);
  expect(
    await screen.findByText(/Not shown in a role preview/),
  ).toBeInTheDocument();
  expect(ask).not.toHaveBeenCalled();
});

it("binds a filtered saved question from its link, and sends none of its filters", async () => {
  const user = userEvent.setup({ delay: null });
  const write = vi.fn(() => jsonResponse(reportingStoryReport));
  const runId = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b";
  installFetchStub({
    ...reportingStoryRoutes(),
    [`GET /analytics/runs/${runId}`]: () =>
      jsonResponse({
        id: runId,
        query: {
          ...question,
          filters: [{ field: "company_id", op: "eq", value: "c-1" }],
        },
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
    runId,
  );
  await user.click(screen.getByRole("button", { name: "Add question" }));
  await user.click(screen.getByRole("button", { name: "Save report" }));
  await waitFor(() => expect(write).toHaveBeenCalled());
  expect(JSON.stringify(write.mock.calls)).not.toContain("c-1");
  expect(write).toHaveBeenCalledWith(
    expect.objectContaining({
      selection: expect.objectContaining({
        layout: [
          expect.objectContaining({
            cards: expect.arrayContaining([
              expect.objectContaining({
                kind: "question",
                question: { ...question, saved_question_id: runId },
              }),
            ]),
          }),
        ],
      }),
    }),
  );
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
              expect.objectContaining({
                kind: "question",
                question: { ...question, saved_question_id: runId },
              }),
            ]),
          }),
        ],
      }),
    }),
  );
});

it("reads the reader's saved questions only once asked, and binds the one picked", async () => {
  const user = userEvent.setup({ delay: null });
  const write = vi.fn(() => jsonResponse(reportingStoryReport));
  const list = vi.fn(() => jsonResponse(SAVED_RUNS));
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/runs": list,
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
  const saved = SAVED_RUNS.data[1];
  await screen.findByRole("button", {
    name: "Choose from your saved questions",
  });
  expect(list).not.toHaveBeenCalled();
  await user.click(
    screen.getByRole("button", { name: "Choose from your saved questions" }),
  );
  await pickOption(
    user,
    await screen.findByRole("combobox", { name: "Your saved questions" }),
    /· Count · /,
  );
  await user.click(screen.getByRole("button", { name: "Save report" }));
  await waitFor(() => expect(write).toHaveBeenCalled());
  expect(write).toHaveBeenCalledWith(
    expect.objectContaining({
      selection: expect.objectContaining({
        layout: [
          expect.objectContaining({
            cards: expect.arrayContaining([
              expect.objectContaining({
                kind: "question",
                question: {
                  entity: saved?.entity,
                  measures: saved?.measures,
                  saved_question_id: saved?.id,
                },
              }),
            ]),
          }),
        ],
      }),
    }),
  );
});

it("offers a filtered saved question, and no unfiltered one the report already holds", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/runs": () => jsonResponse(SAVED_RUNS),
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
  await user.click(
    await screen.findByRole("button", {
      name: "Choose from your saved questions",
    }),
  );
  const picker = await screen.findByRole("combobox", {
    name: "Your saved questions",
  });
  await user.click(picker);
  expect(screen.getAllByRole("option")).toHaveLength(3);
  await user.keyboard("{Escape}");
  await pickOption(user, picker, /· Count · /);
  await user.click(
    screen.getByRole("combobox", { name: "Your saved questions" }),
  );
  expect(screen.getAllByRole("option")).toHaveLength(2);
  expect(screen.getByRole("option", { name: /filtered/ })).toBeInTheDocument();
});

const withBoundCard = {
  ...reportingStoryEvaluation,
  selection: {
    ...reportingStoryEvaluation.selection,
    layout: [
      {
        cards: [
          {
            id: "bound",
            kind: "question" as const,
            width: "full" as const,
            question: { ...question, binding: "ref-1" },
          },
        ],
      },
    ],
  },
};

function drawBound() {
  render(
    <StoryProviders>
      <RecordZoneProvider zone={REPORTING_FIXTURE_ZONE}>
        <ReportingCharts
          evaluation={withBoundCard}
          report={{ id: "report", revision: 3 }}
          onEvidence={() => {}}
        />
      </RecordZoneProvider>
    </StoryProviders>,
  );
}

it("asks a bound card on the server and says which calendar its periods keep", async () => {
  const card = vi.fn(() =>
    jsonResponse({
      status: "answered",
      answer: ANSWER,
      binding: {
        filtered: true,
        period_basis: { timezone: "Europe/Berlin", fiscal_year_start_month: 4 },
      },
    }),
  );
  const ask = vi.fn(() => jsonResponse(ANSWER));
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/reports/report/questions/bound/answer": card,
    "POST /analytics/query": ask,
  });
  drawBound();
  expect(
    await screen.findByText(/fiscal year starting in April, in Europe\/Berlin/),
  ).toBeInTheDocument();
  expect(card).toHaveBeenCalled();
  expect(ask).not.toHaveBeenCalled();
});

it("shows a restricted bound card with nothing of its answer", async () => {
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/reports/report/questions/bound/answer": () =>
      jsonResponse({ status: "restricted", binding: { filtered: true } }),
  });
  drawBound();
  expect(
    await screen.findByText(/filters on records your access does not cover/),
  ).toBeInTheDocument();
  expect(screen.queryByText(/This question has filters/)).toBeNull();
});
