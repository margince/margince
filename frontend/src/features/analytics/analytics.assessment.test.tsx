// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { AssessmentPanel, type SalesAssessment } from "./analytics.assessment";
import { useReportingEvaluation } from "./reporting.queries";
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
const sentence = {
  text: "The open pipeline is a starting point for the review, while commitments identify the deals owners expect to close.",
  kind: "observation",
  cells: ["metric:open_pipeline"],
} as const;
const written: SalesAssessment = {
  evaluation_key: reportingStoryEvaluation.evaluation_key,
  generated_at: "2026-11-03T09:00:00Z",
  generated_by: "model",
  sentences: [{ ...sentence, cells: [...sentence.cells] }],
  paragraphs: [
    [{ ...sentence, cells: [...sentence.cells] }],
    [
      {
        text: "Review the relevant opportunities with their owners before changing the forecast.",
        kind: "observation",
        cells: ["metric:open_pipeline"],
      },
    ],
  ],
  cells: [
    { id: "metric:open_pipeline", label: "Open pipeline", value: "370000 EUR" },
  ],
  decisions: 0,
};
const action = {
  id: "metric:open_pipeline",
  label: "Review open pipeline",
  count: 4,
  evidence: { metric: "open_pipeline", context_id: "state" },
} as const;

function renderPanel(
  assessment: SalesAssessment,
  view: "sales" | "prospecting" = "sales",
  supported = true,
  answer?: (body: unknown) => Response,
) {
  const onEvidence = vi.fn();
  const post = vi.fn(answer ?? (() => jsonResponse(assessment)));
  const routes = reportingStoryRoutes();
  installFetchStub({
    ...routes,
    "GET /analytics/metrics": () =>
      jsonResponse({
        metrics: [],
        presets: [],
        ...(supported
          ? { assessment_views: ["sales", "prospecting", "team"] }
          : {}),
      }),
    "POST /analytics/assessment": post,
  });
  render(
    <StoryProviders>
      <AssessmentPanel
        view={view}
        selection={reportingStoryEvaluation.selection}
        evaluation={{ ...reportingStoryEvaluation, overview_actions: [action] }}
        onEvidence={onEvidence}
      />
    </StoryProviders>,
  );
  return { post, onEvidence };
}

it("renders connected paragraphs without observation badges or a second summary", async () => {
  renderPanel(written);
  expect(await screen.findByText(sentence.text)).toBeVisible();
  expect(screen.getByText(/^Review the relevant opportunities/)).toBeVisible();
  expect(screen.getByText("Written by Margince")).toBeVisible();
  expect(screen.queryByText("Observation")).toBeNull();
  expect(screen.queryByText("No supported decision.")).toBeNull();
  expect(screen.queryByRole("region", { name: "Summary" })).toBeNull();
});

it("keeps application actions with no AI interpretation or reassurance", async () => {
  const { post, onEvidence } = renderPanel({
    ...written,
    generated_by: "deterministic",
    sentences: [],
    paragraphs: [],
  });
  await waitFor(() => expect(post).toHaveBeenCalledOnce());
  const user = userEvent.setup();
  await user.click(
    screen.getByRole("button", { name: "Review open pipeline (4)" }),
  );
  expect(onEvidence).toHaveBeenCalledWith(action.evidence);
  expect(
    screen.queryByText(
      /Compiled from CRM records|An AI model would add|No supported decision/,
    ),
  ).toBeNull();
});

it("does not ask a legacy server to write a manager interpretation for prospecting", async () => {
  const { post } = renderPanel(written, "prospecting", false);
  await screen.findByRole("button", { name: "Review open pipeline (4)" });
  await waitFor(() => expect(screen.queryByText(sentence.text)).toBeNull());
  expect(post).not.toHaveBeenCalled();
});

it("renders a legacy sales response without paragraph metadata", async () => {
  renderPanel({ ...written, paragraphs: undefined }, "sales", false);
  expect(await screen.findByText(sentence.text)).toBeVisible();
});

it("does not display prose from a different evaluated receipt", async () => {
  const { post } = renderPanel({
    ...written,
    evaluation_key: "a-different-evaluation",
  });
  await waitFor(() => expect(post).toHaveBeenCalledOnce());
  expect(screen.queryByText(sentence.text)).toBeNull();
  expect(
    screen.getByRole("button", { name: "Review open pipeline (4)" }),
  ).toBeVisible();
});

it("retries only a precise unsupported-field refusal with the legacy sales body", async () => {
  const calls: unknown[] = [];
  renderPanel(written, "sales", true, (body) => {
    calls.push(body);
    return calls.length === 1
      ? jsonResponse(
          {
            status: 422,
            code: "validation_error",
            details: {
              errors: [
                {
                  field: "body",
                  code: "unknown_field",
                  message: 'unknown field "receipt"',
                },
              ],
            },
          },
          422,
        )
      : jsonResponse({ ...written, paragraphs: undefined });
  });
  expect(await screen.findByText(sentence.text)).toBeVisible();
  expect(calls).toHaveLength(2);
  expect(calls[0]).toHaveProperty(
    "receipt.key",
    reportingStoryEvaluation.evaluation_key,
  );
  expect(calls[1]).not.toHaveProperty("receipt");
  expect(calls[1]).not.toHaveProperty("view");
});

it.each([403, 503])(
  "does not treat a %s failure as legacy support",
  async (status) => {
    const { post } = renderPanel(written, "sales", true, () =>
      jsonResponse({ status, code: "permission_denied" }, status),
    );
    expect(await screen.findByRole("button", { name: "Retry" })).toBeVisible();
    expect(post).toHaveBeenCalledOnce();
    expect(screen.queryByText(sentence.text)).toBeNull();
    expect(
      screen.getByRole("button", { name: "Review open pipeline (4)" }),
    ).toBeVisible();
  },
);

function EvaluatedAssessment() {
  const result = useReportingEvaluation(reportingStoryEvaluation.selection);
  return result.data ? (
    <AssessmentPanel
      selection={result.data.selection}
      evaluation={result.data}
      onEvidence={() => {}}
    />
  ) : null;
}

it("reloads the evaluation after a stale receipt without replaying its old POST", async () => {
  let evaluations = 0;
  const receipts: string[] = [];
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/metrics": () =>
      jsonResponse({ metrics: [], presets: [], assessment_views: ["sales"] }),
    "GET /analytics/evaluate": () =>
      jsonResponse({
        ...reportingStoryEvaluation,
        evaluation_key: ++evaluations === 1 ? "first" : "second",
      }),
    "POST /analytics/assessment": (body) => {
      const serialized = JSON.stringify(body);
      receipts.push(serialized);
      return serialized.includes('"first"')
        ? jsonResponse({ status: 409, code: "version_skew" }, 409)
        : jsonResponse({ ...written, evaluation_key: "second" });
    },
  });
  render(
    <StoryProviders>
      <EvaluatedAssessment />
    </StoryProviders>,
  );
  await userEvent
    .setup()
    .click(await screen.findByRole("button", { name: "Retry" }));
  expect(await screen.findByText(sentence.text)).toBeVisible();
  expect(receipts).toHaveLength(2);
  expect(receipts[0]).toContain('"first"');
  expect(receipts[1]).toContain('"second"');
});
