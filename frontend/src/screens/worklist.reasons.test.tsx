// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { WorklistScreen } from "./worklist";

// Why a row is worth the reader's morning, drawn.
//
// Two lanes could state only how LONG something had been true. A lapsed
// relationship said "quiet for 63 days" whether it was the rep's strongest
// contact with an open deal behind them or a cc who had drifted, and a task
// nobody had taken read exactly like the reader's own. Both facts were already
// in the server's hand and dropped on the way out.
//
// These assert the drawn phrase rather than the wire field: a reason that
// reaches the client and has no sentence for it is silently omitted, so a test
// on the payload would pass over a row that says nothing.

type Worklist = components["schemas"]["Worklist"];
type WorklistItem = components["schemas"]["WorklistItem"];

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

function day(queue: WorklistItem[]): Worklist {
  return {
    as_of: "2026-08-31T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue,
    summary: {
      urgent: 0,
      due: queue.length,
      lower_priority: 0,
      total: queue.length,
    },
    sources_unavailable: [],
    readings: {
      revenue_at_risk_minor: null,
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
    },
    reach: [],
    counts: [],
  };
}

function draw(queue: WorklistItem[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(day(queue))),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <WorklistScreen />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function decayRow(because: WorklistItem["because"]): WorklistItem {
  return {
    id: "d1",
    source: "relationship_decay",
    category: "system",
    level: 3,
    consequence: "data_drifts",
    title: "Dana Weiss",
    because,
    actions: [],
  };
}

describe("a lapsed relationship says what it was worth", () => {
  it("says an open deal rests on the contact", async () => {
    draw([
      decayRow([
        { kind: "quiet_days", value: { kind: "days", days: 63 } },
        { kind: "expected_revenue" },
      ]),
    ]);

    expect(await screen.findByText(/an open deal rests on this/)).toBeTruthy();
    // Beside the span, not instead of it: the rep reads both — how long, and
    // why it matters that long.
    expect(await screen.findByText(/quiet for 63 days/)).toBeTruthy();
  });

  // The row that carries nothing says only how long. This is what makes the
  // line above mean something: if every decay row claimed a deal, the claim
  // would be decoration rather than a fact.
  it("claims no deal when none rests on the contact", async () => {
    draw([decayRow([{ kind: "quiet_days", value: { kind: "days", days: 63 } }])]);

    expect(await screen.findByText(/quiet for 63 days/)).toBeTruthy();
    expect(screen.queryByText(/an open deal rests on this/)).toBeNull();
  });
});

describe("a task says who holds it", () => {
  it("says nobody owns the one nobody has taken", async () => {
    draw([
      {
        id: "t1",
        source: "task",
        category: "tasks",
        level: 3,
        consequence: "task_slips",
        title: "Send the retrofit quote",
        because: [{ kind: "due_today" }, { kind: "unassigned" }],
        actions: [],
      },
    ]);

    expect(await screen.findByText(/nobody owns it/)).toBeTruthy();
  });

  it("says nothing about ownership on a task somebody holds", async () => {
    draw([
      {
        id: "t2",
        source: "task",
        category: "tasks",
        level: 3,
        consequence: "task_slips",
        title: "Send the retrofit quote",
        because: [{ kind: "due_today" }],
        actions: [],
      },
    ]);

    expect(await screen.findByText("Send the retrofit quote")).toBeTruthy();
    expect(screen.queryByText(/nobody owns it/)).toBeNull();
  });
});
