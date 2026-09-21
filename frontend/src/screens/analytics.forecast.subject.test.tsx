/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { ForecastReview } from "./analytics.forecast.review";
import { jsonResponse } from "./company.fixtures";

// A finding in the forecast review says WHICH deal it is about.
//
// The row named the check and the money and left the record to a uuid on the
// wire, so a manager reading the review before a call knew something was wrong
// and not what it was wrong about.

type InputCheck = components["schemas"]["InputCheck"];

const CHECK = {
  id: "e-1",
  type: "close_past",
  subject_kind: "deal",
  subject_id: "d-1",
  severity: "high",
  status: "open",
  first_seen_at: "2026-09-01T09:00:00Z",
  last_seen_at: "2026-09-07T09:00:00Z",
} satisfies InputCheck;

function show(check: InputCheck) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/forecast/assurance/exceptions")) {
        return jsonResponse({ data: [check] });
      }
      if (url.includes("/forecast/assurance")) {
        return jsonResponse({
          ran_at: "2026-09-07T03:00:00Z",
          coverage: { deals_checked: 10, deals_total: 10 },
        });
      }
      return jsonResponse({}, 404);
    }),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ForecastReview />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a forecast finding", () => {
  it("names the deal and links to it", async () => {
    show({
      ...CHECK,
      subject: { type: "deal", id: "d-1", label: "Fleet retrofit" },
    });

    const link = await screen.findByRole("link", { name: "Fleet retrofit" });
    // The href, not a click: it is also what a new tab and a middle-click
    // follow, and a manager triaging a review opens several deals at once.
    expect(link.getAttribute("href")).toBe("#/deals/d-1");
  });

  it("still draws the row when the server names no subject", async () => {
    // Version skew, and the reader who may not see the deal's name. The row
    // said what was wrong before any of this and still does.
    show(CHECK);

    expect(await screen.findByText(/close date/i)).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Fleet retrofit" })).toBeNull();
  });
});
