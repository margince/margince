/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { jsonResponse } from "../../testing/company.fixtures";
import { ForecastReview } from "./analytics.forecast.review";

// A reading the records moved past never reads as current. The panel names
// what changed and what to do, and the verdict loses its tone. The changed
// deal's finding says so on its own row. A reading nobody can vouch for says
// that instead.

type Assurance = components["schemas"]["ForecastAssurance"];
type InputCheck = components["schemas"]["InputCheck"];

const RUN: Assurance = {
  run_id: "r-1",
  as_of: "2026-09-07T03:00:00Z",
  status: "complete",
  readiness: "ready",
  eligible_deals: 2,
  sources: [{ source: "mail", state: "checked" }],
  freshness: {
    state: "current",
    changed_deals: 0,
    changed_signals: 0,
    workspace_changed: false,
  },
};

function check(id: string, label: string, changed: boolean | null): InputCheck {
  return {
    id,
    type: "close_past",
    subject_kind: "deal",
    subject_id: `d-${id}`,
    subject: { type: "deal", id: `d-${id}`, label },
    severity: "high",
    status: "open",
    first_seen_at: "2026-09-01T09:00:00Z",
    last_seen_at: "2026-09-07T09:00:00Z",
    inputs_changed: changed,
  };
}

function show(run: Assurance, checks: InputCheck[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/forecast/assurance/exceptions")) {
        return jsonResponse({ data: checks });
      }
      if (url.includes("/forecast/assurance/enforcement")) {
        return jsonResponse({}, 404);
      }
      if (url.includes("/forecast/assurance")) {
        return jsonResponse(run);
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

async function rowOf(deal: string): Promise<HTMLElement> {
  const row = (await screen.findByRole("link", { name: deal })).closest("tr");
  if (row === null) {
    throw new Error(`no table row holds the link to ${deal}`);
  }
  return row;
}

function badgeOf(text: string): string {
  return screen.getByText(text).closest(".badge")?.className ?? "";
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("an input check the records moved past", () => {
  it("says the reading is out of date and when it refreshes", async () => {
    show(
      {
        ...RUN,
        freshness: {
          state: "out_of_date",
          changed_deals: 1,
          changed_signals: 0,
          workspace_changed: false,
        },
      },
      [check("e-1", "Fleet retrofit", true), check("e-2", "Harbor", false)],
    );

    expect(
      await screen.findByText(
        "1 deal changed since this check. Choose Check again to read the changes now, or wait for the next nightly run.",
      ),
    ).toBeTruthy();
    expect(
      within(await rowOf("Fleet retrofit")).getByText("Out of date"),
    ).toBeTruthy();
    expect(within(await rowOf("Harbor")).queryByText("Out of date")).toBeNull();
    // The verdict keeps its word and drops the healthy tone.
    expect(badgeOf("Ready")).not.toContain("badge-success");
  });

  it("names a change that reaches every deal", async () => {
    show(
      {
        ...RUN,
        freshness: {
          state: "out_of_date",
          changed_deals: 0,
          changed_signals: 0,
          workspace_changed: true,
        },
      },
      [check("e-1", "Fleet retrofit", true)],
    );

    expect(
      await screen.findByText(
        "A stage or forecast call changed since this check. Choose Check again to read the changes now, or wait for the next nightly run.",
      ),
    ).toBeTruthy();
  });

  it("names a workspace change beside the deals and signals that changed", async () => {
    show(
      {
        ...RUN,
        freshness: {
          state: "out_of_date",
          changed_deals: 2,
          changed_signals: 1,
          workspace_changed: true,
        },
      },
      [check("e-1", "Fleet retrofit", true)],
    );

    expect(
      await screen.findByText(
        "2 deals changed since this check. 1 buying signal changed since this check. A stage or forecast call changed since this check. Choose Check again to read the changes now, or wait for the next nightly run.",
      ),
    ).toBeTruthy();
  });

  it("says when nobody knows whether the records moved, and claims no healthy verdict", async () => {
    show(
      {
        ...RUN,
        freshness: {
          state: "unknown",
          changed_deals: 0,
          changed_signals: 0,
          workspace_changed: false,
        },
      },
      [check("e-1", "Fleet retrofit", null)],
    );

    expect(
      await screen.findByText(
        "It is not known whether the records changed since this check: no check that read every source has run since Margince started tracking changes. Choose Check again to run one.",
      ),
    ).toBeTruthy();
    expect(
      within(await rowOf("Fleet retrofit")).queryByText("Out of date"),
    ).toBeNull();
    expect(badgeOf("Ready")).not.toContain("badge-success");
  });

  it("draws a current reading as it always did", async () => {
    show(RUN, [check("e-1", "Fleet retrofit", false)]);

    await screen.findByRole("link", { name: "Fleet retrofit" });
    expect(screen.queryByText("Out of date")).toBeNull();
    expect(badgeOf("Ready")).toContain("badge-success");
  });
});
