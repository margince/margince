// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { jsonResponse, stubWithSession } from "../story-utils";
import { DealHistoryTab } from "./dealhistorytab";

// The deal's History tab reads the same ONE chronology every other record
// page does (leadhistory.test.tsx pins the lead's copy): what was said and
// what was changed, in one order, under a filter row that starts on All.

type Deal = components["schemas"]["Deal"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function withProviders(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const DEAL: Deal = {
  id: "d-1",
  name: "Fleet retrofit",
  status: "open",
  source: "manual",
  captured_by: "human:u-1",
  version: 3,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-20T08:00:00Z",
} as Deal;

const CALL_ACTIVITY = {
  id: "a-1",
  kind: "call",
  subject: "Renewal check-in",
  direction: "outbound",
  occurred_at: "2026-08-11T12:00:00Z",
  is_done: true,
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-08-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
};

const CHANGE_ROW = {
  id: "h-1",
  actor_type: "human",
  actor_id: "u-1",
  action: "update",
  occurred_at: "2026-07-13T10:00:00Z",
  summary: "Deal amount changed",
};

describe("DealHistoryTab", () => {
  it("opens on the whole chronology, with the kind filter bar over it", async () => {
    stubWithSession(
      {
        "GET /activities": () =>
          jsonResponse({ data: [CALL_ACTIVITY], page: { has_more: false } }),
      },
      {},
    );
    withProviders(<DealHistoryTab deal={DEAL} onOpenEmail={() => {}} />);

    expect(await screen.findByText("Renewal check-in")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "All" }).getAttribute("aria-pressed"),
    ).toBe("true");
  });

  // Changes is the one cut with no exchanges to narrow, and it swaps the rows
  // for the record-change audit — the one surface that can put a change back
  // — rather than drawing the same edits a second time beside it.
  it("swaps in the record-change audit on the Changes cut, and hides the rows", async () => {
    stubWithSession(
      {
        "GET /activities": () =>
          jsonResponse({ data: [CALL_ACTIVITY], page: { has_more: false } }),
        "GET /records/deal/d-1/history": () =>
          jsonResponse({ data: [CHANGE_ROW], page: { next_cursor: null } }),
      },
      {},
    );
    withProviders(<DealHistoryTab deal={DEAL} onOpenEmail={() => {}} />);

    await screen.findByText("Renewal check-in");
    await userEvent.click(screen.getByRole("button", { name: "Changes" }));

    await waitFor(() =>
      expect(screen.getByText("Deal amount changed")).toBeTruthy(),
    );
    expect(screen.queryByText("Renewal check-in")).toBeNull();
    expect(screen.queryByLabelText("Activity kind")).toBeNull();
  });

  it("says there is nothing yet rather than drawing an empty list silently", async () => {
    stubWithSession({}, {});
    withProviders(<DealHistoryTab deal={DEAL} onOpenEmail={() => {}} />);

    expect(
      await screen.findByText("No activity on this deal yet."),
    ).toBeTruthy();
  });
});
