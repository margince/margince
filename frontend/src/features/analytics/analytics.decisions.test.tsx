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
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { DecisionsPanel, type SalesDecision } from "./analytics.decisions";
import { reportingStoryRoutes } from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;

afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

const open: SalesDecision = {
  id: "11111111-1111-4111-8111-111111111111",
  decision: "Call the buyer about the budget",
  deal_id: "22222222-2222-4222-8222-222222222222",
  owner_id: "00000000-0000-4000-8000-000000000001",
  review_on: "2026-11-10",
  expected_result: "Budget owner named",
  status: "open",
  done_at: null,
  result: null,
  created_at: "2026-11-03T09:00:00Z",
};

it("lists the open decision and records its outcome without touching the deal", async () => {
  const user = userEvent.setup({ delay: null });
  const outcome = vi.fn((_body: unknown) =>
    jsonResponse({
      ...open,
      status: "done",
      done_at: "2026-11-04T09:00:00Z",
      result: "Buyer met",
    }),
  );
  installFetchStub({
    ...reportingStoryRoutes(),
    // Recording a decision or its outcome is writing a task.
    "GET /me": meRoute({
      report_definition: ["read"],
      deal: ["read"],
      activity: ["read", "create", "update"],
    }),
    "GET /analytics/decisions": () => jsonResponse({ data: [open] }),
    "POST /analytics/decisions/11111111-1111-4111-8111-111111111111/outcome":
      outcome,
  });
  render(
    <StoryProviders>
      <DecisionsPanel />
    </StoryProviders>,
  );
  expect(
    await screen.findByText("Call the buyer about the budget"),
  ).toBeInTheDocument();
  expect(screen.getByText("Budget owner named")).toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Record outcome" }));
  const save = screen.getByRole("button", { name: "Save outcome" });
  expect(save).toBeDisabled();
  await user.type(
    screen.getByRole("textbox", { name: "What work was completed?" }),
    "Buyer met",
  );
  await user.click(save);
  await waitFor(() => expect(outcome).toHaveBeenCalledTimes(1));
  expect(outcome.mock.calls[0]?.[0]).toEqual({ result: "Buyer met" });
});

it("asks for the decisions of the review's deals", async () => {
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /me": meRoute({ report_definition: ["read"], activity: ["read"] }),
    "GET /analytics/decisions": () => jsonResponse({ data: [open] }),
  });
  const routed = globalThis.fetch;
  const asked: URL[] = [];
  globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
    asked.push(
      new URL(
        input instanceof Request ? input.url : String(input),
        "http://app.test",
      ),
    );
    return routed(input, init);
  }) as typeof fetch;
  render(
    <StoryProviders>
      <DecisionsPanel
        review={{
          scope: { kind: "team", id: "33333333-3333-4333-8333-333333333333" },
          product_id: "44444444-4444-4444-8444-444444444444",
          country: "DE",
        }}
      />
    </StoryProviders>,
  );
  expect(
    await screen.findByText("Call the buyer about the budget"),
  ).toBeInTheDocument();
  const list = asked.find((url) =>
    url.pathname.endsWith("/analytics/decisions"),
  );
  expect(list?.searchParams.get("scope_kind")).toBe("team");
  expect(list?.searchParams.get("scope_id")).toBe(
    "33333333-3333-4333-8333-333333333333",
  );
  expect(list?.searchParams.get("product_id")).toBe(
    "44444444-4444-4444-8444-444444444444",
  );
  expect(list?.searchParams.get("country")).toBe("DE");
  expect(list?.searchParams.has("industry")).toBe(false);
});
