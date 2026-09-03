// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The harness the Worklist's test files share.
 *
 * Extracted when `worklist.test.tsx` crossed the 1000-line ceiling and had to
 * split. The alternative was copying `day`, `row` and `stub` into the second
 * file, and two copies of a fixture builder drift: one gains a field the
 * contract added, the other keeps describing a row the server no longer sends,
 * and the tests disagree about what a queue looks like.
 *
 * A `.tsx` module rather than a `.test.tsx` one, so vitest does not collect it
 * as a suite of its own with no cases in it.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type RenderResult, render } from "@testing-library/react";
import { vi } from "vitest";
import type { components } from "../api/schema";
import { type Locale, LocaleProvider } from "../i18n";
import { WorklistScreen } from "./worklist";

export type Worklist = components["schemas"]["Worklist"];
export type WorklistItem = components["schemas"]["WorklistItem"];

export function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

/**
 * The queue, plus the one approval a decision row fetches whole. A row sends a
 * sentence; deciding needs the payload, the stager and the evidence, so the row
 * being decided reads the approval it is showing.
 */
export function stub(day: Worklist, approval?: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/worklist")) {
        return jsonResponse(day);
      }
      if (approval && /\/approvals\/[^/]+$/.test(url.split("?")[0])) {
        return jsonResponse(approval);
      }
      return jsonResponse({ data: [] });
    }),
  );
}

/**
 * A walk: one response per page, served in order, keyed by the cursor.
 *
 * The queue pages with a `cursor`, and a stub that answers the same body to
 * every request would let a "load more" test pass while loading the same rows
 * again. This serves page N+1 only when the request carries page N's cursor,
 * so a client that drops the cursor gets page one forever and the test fails.
 */
export function stubWalk(pages: readonly Worklist[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (!url.includes("/worklist")) {
        return jsonResponse({ data: [] });
      }
      const cursor = new URL(url, "http://localhost").searchParams.get(
        "cursor",
      );
      if (!cursor) {
        return jsonResponse(pages[0]);
      }
      const at = pages.findIndex((page) => page.next_cursor === cursor);
      return jsonResponse(at >= 0 ? pages[at + 1] : pages[0]);
    }),
  );
}

export function renderWorklist(
  locale: Locale = "en",
  opensOn?: string,
): RenderResult {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <WorklistScreen opensOn={opensOn} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

export function day(over: Partial<Worklist> = {}): Worklist {
  return {
    as_of: "2026-08-31T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue: [],
    summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
    sources_unavailable: [],
    reach: [],
    counts: [],
    readings: {
      revenue_at_risk_minor: null,
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
    },
    ...over,
  };
}

export function row(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    id: "row-1",
    source: "task",
    category: "tasks",
    level: 4,
    consequence: "task_slips",
    because: [],
    actions: [],
    ...over,
  };
}
