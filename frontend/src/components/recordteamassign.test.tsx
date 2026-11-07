/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import type { RecordAssignment } from "./recordassignments.queries";
import { RecordTeamAssign } from "./recordteamassign";

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
  {
    id: "u5",
    display_name: "Tessa Holm",
    status: "deactivated",
    is_agent: false,
  },
];

const invitedAssignment: RecordAssignment = {
  id: "a1",
  record_type: "deal",
  record_id: RECORD_ID,
  subject_kind: "user",
  subject_id: "u2",
  subject_name: "Jonas Reed",
  subject_inactive: false,
  role_id: ROLES[0].id,
  role_key: ROLES[0].key,
  role_label: ROLES[0].label,
  role_active: true,
  version: 1,
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-01T10:00:00Z",
};

function stubFetch(
  seen: URL[],
  users: readonly unknown[] = USERS,
  patches?: unknown[],
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      seen.push(new URL(req.url));
      if (req.method === "PATCH") {
        if (!patches) throw new Error("Unexpected assignment write");
        patches.push(await req.json());
        return Response.json(invitedAssignment);
      }
      const body = req.url.includes("/record-roles")
        ? { data: ROLES }
        : req.url.includes("/teams")
          ? { data: [] }
          : { data: users };
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

async function search(placeholder: string, term: string) {
  vi.useFakeTimers();
  try {
    fireEvent.change(screen.getByPlaceholderText(placeholder), {
      target: { value: term },
    });
    await act(() => vi.advanceTimersByTimeAsync(250));
  } finally {
    vi.useRealTimers();
  }
}

afterEach(() => {
  vi.useRealTimers();
  cleanup();
  vi.unstubAllGlobals();
});

it("offers only active human colleagues", async () => {
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
  await search("Find a colleague", "a");
  expect(await screen.findByText("Mara Feld")).toBeTruthy();
  expect(screen.queryByText("Jonas Reed")).toBeNull();
  expect(screen.queryByText("Invited, not signed in yet")).toBeNull();
  expect(screen.queryByText("Ines Kraft")).toBeNull();
  expect(screen.queryByText("Margince")).toBeNull();
  expect(screen.queryByText("Tessa Holm")).toBeNull();
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
  await search("Find a colleague", "reed");
  await screen.findByText("Mara Feld");
  const users = seen.filter((u) => u.pathname.endsWith("/users")).at(-1);
  // Narrowing one page here instead would leave a colleague past the page
  // boundary unassignable however precisely their name was typed.
  expect(users?.searchParams.get("q")).toBe("reed");
  expect(users?.searchParams.get("limit")).toBe("200");
  expect(users?.searchParams.has("include_invited")).toBe(false);
});

it("asks the server to search the teams too", async () => {
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
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Team" }));
  await search("Find a team", "Thắng");
  await screen.findByText("No match");
  const teams = seen.filter((u) => u.pathname.endsWith("/teams")).at(-1);
  expect(teams?.searchParams.get("q")).toBe("Thắng");
});

it("says who can be assigned when no colleague matches", async () => {
  stubFetch([], []);
  render(
    <RecordTeamAssign
      open
      onClose={() => {}}
      recordType="deal"
      recordId={RECORD_ID}
    />,
  );
  await search("Find a colleague", "zz");
  expect(
    await screen.findByText(
      "No active colleague found. Ask an admin to invite them, then ask them to sign in.",
    ),
  ).toBeTruthy();
});

it("keeps an existing invited holder editable without offering them as a new choice", async () => {
  const patches: unknown[] = [];
  stubFetch([], USERS, patches);
  const user = userEvent.setup();
  render(
    <RecordTeamAssign
      open
      onClose={() => {}}
      recordType="deal"
      recordId={RECORD_ID}
      existing={invitedAssignment}
    />,
  );
  expect(await screen.findByText("Jonas Reed")).toBeTruthy();
  await screen.findByText("Account manager");
  await user.click(screen.getByRole("button", { name: "Save change" }));
  await waitFor(() => expect(patches).toHaveLength(1));
  expect(patches[0]).toMatchObject({
    subject_kind: "user",
    subject_id: "u2",
    role_id: "r1",
  });
});
