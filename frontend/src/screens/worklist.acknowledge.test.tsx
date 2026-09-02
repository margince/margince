// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { WorklistScreen } from "./worklist";

// A notice's one verb settles it through its own read endpoint, not a link
// to whatever it names — split from worklist.test.tsx to hold that ceiling
// (frontend/AGENTS.md) rather than grow the file already at it.

type Worklist = components["schemas"]["Worklist"];
type WorklistItem = components["schemas"]["WorklistItem"];

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

function noticeRow(): WorklistItem {
  return {
    id: "n1",
    source: "notice",
    category: "tasks",
    level: 4,
    consequence: "task_slips",
    title: "A deal you own changed stage",
    detail: "Acme Renewal moved to a new pipeline stage.",
    because: [],
    actions: ["acknowledge"],
  };
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

// Answers the read as accepted and drops the notice from the next `/worklist`
// read — the shape a real invalidated refetch produces once the server has
// actually marked it read, not merely that the POST was sent.
function stubSettling() {
  let read = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const target =
        input instanceof Request ? input : new Request(input, init);
      if (target.url.includes("/notices/n1/read") && target.method === "POST") {
        read = true;
        return new Response(null, { status: 204 });
      }
      if (target.url.includes("/worklist")) {
        return jsonResponse(read ? day([]) : day([noticeRow()]));
      }
      return jsonResponse({ data: [] });
    }),
  );
}

// Answers the read with a refusal, so the button's own failure path — never
// exercised by the happy-path test — has somewhere to run.
function stubRefusing() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const target =
        input instanceof Request ? input : new Request(input, init);
      if (target.url.includes("/notices/n1/read") && target.method === "POST") {
        return new Response(JSON.stringify({ title: "refused" }), {
          status: 409,
          headers: { "content-type": "application/json" },
        });
      }
      if (target.url.includes("/worklist")) {
        return jsonResponse(day([noticeRow()]));
      }
      return jsonResponse({ data: [] });
    }),
  );
}

function renderWorklist() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
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

describe("settling a notice", () => {
  it("leaves the lane once the read is accepted", async () => {
    const user = userEvent.setup();
    stubSettling();
    renderWorklist();

    await screen.findByText("A deal you own changed stage");
    await user.click(screen.getByRole("button", { name: "Got it" }));

    await waitFor(() => {
      expect(screen.queryByText("A deal you own changed stage")).toBeNull();
    });
    await screen.findByText("Nothing is waiting on you.");
  });

  it("says so and leaves the notice in the lane when the read is refused", async () => {
    const user = userEvent.setup();
    stubRefusing();
    renderWorklist();

    await screen.findByText("A deal you own changed stage");
    await user.click(screen.getByRole("button", { name: "Got it" }));

    expect(
      await screen.findByText("That could not be marked as seen."),
    ).toBeTruthy();
    expect(screen.getByText("A deal you own changed stage")).toBeTruthy();
  });
});
