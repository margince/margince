/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { RecordTeamAssign } from "./recordteamassign";

// Who the picker is allowed to offer, and on what terms it asks for them.
// Both rules here are ones the server already holds: offering somebody the
// save would refuse, or hiding somebody it would accept, are the same defect
// seen from either side.

const RECORD_ID = "11111111-1111-1111-1111-111111111111";

const ROLES = [
  {
    id: "r1",
    key: "account_manager",
    label: "Account manager",
    record_types: ["deal", "company", "project"],
    assignee_kinds: ["user", "team"],
    sort_order: 1,
    active: true,
    system: true,
    version: 1,
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
  },
];

const USERS = [
  { id: "u1", display_name: "Mara Feld", status: "active", is_agent: false },
  { id: "u2", display_name: "Jonas Reed", status: "invited", is_agent: false },
  {
    id: "u3",
    display_name: "Ines Kraft",
    status: "suspended",
    is_agent: false,
  },
  { id: "u4", display_name: "Margince", status: "active", is_agent: true },
];

function stubFetch(seen: URL[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      seen.push(new URL(req.url));
      const body = req.url.includes("/record-roles")
        ? { data: ROLES }
        : req.url.includes("/teams")
          ? { data: [] }
          : { data: USERS };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

function render(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return rtlRender(
    <QueryClientProvider client={qc}>
      <LocaleProvider>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("offers an invited colleague and withholds the ones the server refuses", async () => {
  const seen: URL[] = [];
  stubFetch(seen);
  render(
    <RecordTeamAssign
      open
      onClose={() => {}}
      recordType="deal"
      recordId={RECORD_ID}
    />,
  );
  await userEvent.type(screen.getByPlaceholderText("Find a colleague"), "a");
  expect(await screen.findByText("Mara Feld")).toBeTruthy();
  // Invited is deliberately assignable server-side: staffing a record is part
  // of onboarding somebody, and refusing it would mean nobody could be given
  // work until their first login.
  expect(screen.getByText("Jonas Reed")).toBeTruthy();
  // Both of these the server itself refuses.
  expect(screen.queryByText("Ines Kraft")).toBeNull();
  expect(screen.queryByText("Margince")).toBeNull();
});

it("asks the server to search the roster, and for a full page of it", async () => {
  const seen: URL[] = [];
  stubFetch(seen);
  render(
    <RecordTeamAssign
      open
      onClose={() => {}}
      recordType="deal"
      recordId={RECORD_ID}
    />,
  );
  await userEvent.type(screen.getByPlaceholderText("Find a colleague"), "reed");
  await screen.findByText("Mara Feld");
  const users = seen.filter((u) => u.pathname.endsWith("/users")).at(-1);
  // Narrowing one page here instead would leave a colleague past the page
  // boundary unassignable however precisely their name was typed.
  expect(users?.searchParams.get("q")).toBe("reed");
  expect(users?.searchParams.get("limit")).toBe("200");
});
