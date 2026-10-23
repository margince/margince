/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider, translate } from "../../i18n";
import { jsonResponse, stubWithSession } from "../../testing/story-utils";
import { LeadHistoryTab } from "./leadhistory";

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
  actor_name: "Demo Admin",
  action: "update",
  occurred_at: "2026-07-13T10:00:00Z",
  summary: "Lead score changed",
};
const CHANGE_LINE = translate("en", "history.verb.update", {
  subject: CHANGE_ROW.actor_name,
});

// The lead's History tab reads the same ONE chronology every other record
// page does (contacttabs.test.tsx pins the contact's copy): what was said and
// what was changed, in one order, under a filter row that starts on All.

describe("LeadHistoryTab", () => {
  it("opens on the whole chronology, with the cuts in the card's head", async () => {
    stubWithSession(
      {
        "GET /activities": () =>
          jsonResponse({ data: [EMAIL_ACTIVITY], page: { has_more: false } }),
      },
      {},
    );
    withProviders(<LeadHistoryTab lead={LEAD} onOpenEmail={() => {}} />);

    expect(await screen.findByText("Fleet renewal")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "All" }).getAttribute("aria-pressed"),
    ).toBe("true");
    for (const cut of ["Email", "Meetings", "Calls", "Notes", "Changes"]) {
      expect(screen.getByRole("button", { name: cut })).toBeTruthy();
    }
    // The search and date row waits behind its toggle.
    expect(screen.queryByLabelText("Activity kind")).toBeNull();
  });

  // A kind cut is a server parameter: the list is read again for that kind,
  // never narrowed from the page already held.
  it("reads the activities again for the kind a cut names", async () => {
    stubWithSession(
      {
        "GET /activities": () =>
          jsonResponse({ data: [EMAIL_ACTIVITY], page: { has_more: false } }),
      },
      {},
    );
    const stub = globalThis.fetch;
    const kinds: (string | null)[] = [];
    vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(
        input instanceof Request ? input.url : String(input),
        "https://test.local",
      );
      if (url.pathname.endsWith("/activities")) {
        kinds.push(url.searchParams.get("kind"));
      }
      return stub(input, init);
    });
    withProviders(<LeadHistoryTab lead={LEAD} onOpenEmail={() => {}} />);

    await screen.findByText("Fleet renewal");
    await userEvent.click(screen.getByRole("button", { name: "Calls" }));

    await waitFor(() => expect(kinds).toContain("call"));
    expect(
      screen
        .getByRole("button", { name: "Calls" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
  });

  it("opens the search and date row from its toggle", async () => {
    stubWithSession(
      {
        "GET /activities": () =>
          jsonResponse({ data: [EMAIL_ACTIVITY], page: { has_more: false } }),
      },
      {},
    );
    withProviders(<LeadHistoryTab lead={LEAD} onOpenEmail={() => {}} />);

    await screen.findByText("Fleet renewal");
    const toggle = screen.getByRole("button", {
      name: translate("en", "timeline.filters.search"),
    });
    await userEvent.click(toggle);

    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByLabelText("Activity kind")).toBeTruthy();
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

    await waitFor(() => expect(screen.getByText(CHANGE_LINE)).toBeTruthy());
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
      await screen.findByText("No activity logged on this lead yet."),
    ).toBeTruthy();
  });
});
