/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { LeadHistoryTab } from "./leadhistory";
import { jsonResponse, stubWithSession } from "./story-utils";

type Lead = components["schemas"]["Lead"];

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

const LEAD: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  status: "contacted",
  score: 72,
  captured_by: "human:u-1",
  source: "manual",
  writable: true,
  version: 3,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-20T08:00:00Z",
};

const EMAIL_ACTIVITY = {
  id: "a-1",
  kind: "email",
  subject: "Fleet renewal",
  direction: "inbound",
  occurred_at: "2026-08-11T12:00:00Z",
  is_done: false,
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-08-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
};

const CHANGE_ROW = {
  id: "h1",
  actor_type: "human",
  actor_id: "u-1",
  action: "update",
  occurred_at: "2026-07-13T10:00:00Z",
  summary: "Lead score changed",
};

// The lead's History tab reads the same ONE chronology every other record
// page does (contacttabs.test.tsx pins the contact's copy): what was said and
// what was changed, in one order, under a filter row that starts on All.

describe("LeadHistoryTab", () => {
  it("opens on the whole chronology, with the kind filter bar over it", async () => {
    stubWithSession(
      {
        "GET /activities": () =>
          jsonResponse({ data: [EMAIL_ACTIVITY], page: { has_more: false } }),
      },
      {},
    );
    withProviders(<LeadHistoryTab lead={LEAD} onOpenEmail={() => {}} />);

    expect(await screen.findByText("Fleet renewal")).toBeTruthy();
    expect(screen.getByLabelText("Activity kind")).toBeTruthy();
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
          jsonResponse({ data: [EMAIL_ACTIVITY], page: { has_more: false } }),
        "GET /records/lead/l-1/history": () =>
          jsonResponse({ data: [CHANGE_ROW], page: { next_cursor: null } }),
      },
      {},
    );
    withProviders(<LeadHistoryTab lead={LEAD} onOpenEmail={() => {}} />);

    await screen.findByText("Fleet renewal");
    await userEvent.click(screen.getByRole("button", { name: "Changes" }));

    await waitFor(() =>
      expect(screen.getByText("Lead score changed")).toBeTruthy(),
    );
    expect(screen.queryByText("Fleet renewal")).toBeNull();
    // The narrowing row stands down with the exchanges it would have
    // narrowed — a reader who picked Changes has no kind left to filter.
    expect(screen.queryByLabelText("Activity kind")).toBeNull();
    // The panel this swaps in carries its own by-change / by-field switch.
    expect(screen.getByRole("button", { name: "By field" })).toBeTruthy();
  });

  it("says there is nothing yet rather than drawing an empty list silently", async () => {
    stubWithSession({}, {});
    withProviders(<LeadHistoryTab lead={LEAD} onOpenEmail={() => {}} />);

    expect(
      await screen.findByText("Nothing is logged on this lead yet."),
    ).toBeTruthy();
  });
});
