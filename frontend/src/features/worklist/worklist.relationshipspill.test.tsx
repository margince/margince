/** @vitest-environment happy-dom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { WorklistScreen } from "./worklist";

// A relationship gone quiet is outreach to someone the reader has worked with.
// The server files it under its own `relationships` category. The pill row
// offered no way to ask for that cut. The rows were reachable only under All.
// A reader looking for work with contacts had nothing to press.

type Worklist = components["schemas"]["Worklist"];

const QUIET_ROW = {
  id: "decay-1",
  source: "relationship_decay" as const,
  category: "relationships" as const,
  level: 3,
  consequence: "data_drifts" as const,
  title: "Mai Tran · quiet for 60 days",
  occurred_precision: "instant" as const,
  because: [],
  actions: [],
};
const DELIVERY_ROW = {
  id: "bounce-1",
  source: "bounce" as const,
  category: "system" as const,
  level: 2,
  consequence: "customer_never_received" as const,
  title: "A message to Globex bounced",
  occurred_precision: "instant" as const,
  because: [],
  actions: [],
};

function day(queue: Worklist["queue"]): Worklist {
  return {
    as_of: "2026-10-26T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue,
    summary: { urgent: 0, due: 0, lower_priority: 0, total: queue.length },
    sources_unavailable: [],
    readings: {
      changed_since_brief: 0,
      revenue_at_risk_minor: null,
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
    },
    reach: [],
    counts: [
      {
        category: "relationships",
        considered: 1,
        shown: 1,
        more_available: false,
      },
      { category: "system", considered: 1, shown: 1, more_available: false },
    ],
  };
}

// The server does the narrowing; the stub answers each filter with the rows
// the server would, and records which filter was asked for.
function stubServer() {
  const asked: (string | null)[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(String(input instanceof Request ? input.url : input));
      if (!url.pathname.endsWith("/worklist")) {
        return Response.json({ data: [] });
      }
      const filter = url.searchParams.get("filter");
      asked.push(filter);
      const queue =
        filter === "relationships"
          ? [QUIET_ROW]
          : filter === "system"
            ? [DELIVERY_ROW]
            : [QUIET_ROW, DELIVERY_ROW];
      return Response.json(day(queue));
    }),
  );
  return asked;
}

function renderWorklist() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <WorklistScreen />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

it("offers a Quiet relationships pill with its count, and pressing it asks the server for that cut", async () => {
  const user = userEvent.setup();
  const asked = stubServer();
  renderWorklist();
  const pill = await screen.findByRole("button", {
    name: new RegExp(`${en["worklist.filter.relationships"]}.*1`),
  });
  await user.click(pill);
  await waitFor(() => expect(asked).toContain("relationships"));
  expect(await screen.findByText("Mai Tran · quiet for 60 days")).toBeTruthy();
  await waitFor(() =>
    expect(screen.queryByText("A message to Globex bounced")).toBeNull(),
  );
});

it("keeps upkeep under System, without the quiet relationship", async () => {
  const user = userEvent.setup();
  const asked = stubServer();
  renderWorklist();
  await user.click(
    await screen.findByRole("button", {
      name: new RegExp(`^${en["worklist.filter.system"]}`),
    }),
  );
  await waitFor(() => expect(asked).toContain("system"));
  expect(await screen.findByText("A message to Globex bounced")).toBeTruthy();
  await waitFor(() =>
    expect(screen.queryByText("Mai Tran · quiet for 60 days")).toBeNull(),
  );
});
