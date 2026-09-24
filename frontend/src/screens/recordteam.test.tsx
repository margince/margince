/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { RecordTeam } from "./recordteam";

// The panel used to be a read surface over a write API: every hook for naming,
// moving and ending a responsibility existed and no control called one. What
// these assert is that a reader who can write sees a way to, a reader who
// cannot sees none, and that a refused read never offers to add a duplicate of
// a responsibility that may already be there.

const RECORD_ID = "11111111-1111-1111-1111-111111111111";

const ROW = {
  id: "a1",
  record_type: "deal",
  record_id: RECORD_ID,
  subject_kind: "user",
  subject_id: "u1",
  subject_name: "Mara Feld",
  subject_inactive: false,
  role_id: "r1",
  role_key: "account_manager",
  role_label: "Account manager",
  role_active: true,
  version: 1,
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-01T10:00:00Z",
};

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

function stubFetch(
  rows: unknown[],
  opts: Readonly<{
    failRead?: boolean;
    failDelete?: boolean;
    onDelete?: (id: string) => void;
  }> = {},
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.method === "DELETE") {
        opts.onDelete?.(req.url.split("/").pop() ?? "");
        if (opts.failDelete) {
          return new Response(
            JSON.stringify({ title: "Forbidden", detail: "not yours" }),
            { status: 403, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response(null, { status: 204 });
      }
      if (req.url.includes("/assignments") && opts.failRead) {
        return new Response(JSON.stringify({ title: "nope" }), {
          status: 500,
          headers: { "Content-Type": "application/json" },
        });
      }
      const body = req.url.includes("/record-roles")
        ? { data: ROLES }
        : { data: rows };
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

it("invites an assignment on a record that has none", async () => {
  stubFetch([]);
  render(<RecordTeam recordType="deal" recordId={RECORD_ID} />);
  expect(await screen.findByText(/No one is assigned yet/)).toBeTruthy();
  expect(screen.getByRole("button", { name: "Assign" })).toBeTruthy();
});

it("draws no panel of its own when a section already names it", async () => {
  stubFetch([]);
  render(<RecordTeam recordType="company" recordId={RECORD_ID} bare />);
  expect(await screen.findByText(/No one is assigned yet/)).toBeTruthy();
  // The company rail is one pane of headed slices and names this one itself;
  // a second heading and a titled region inside it would be a card in a card.
  expect(screen.queryByRole("heading", { name: "Responsible" })).toBeNull();
  expect(screen.queryByRole("region")).toBeNull();
  // The verb still stands: the shape changed, not what a writer may do.
  expect(screen.getByRole("button", { name: "Assign" })).toBeTruthy();
});

it("offers no verbs at all on a record this reader cannot write", async () => {
  stubFetch([ROW]);
  render(<RecordTeam recordType="deal" recordId={RECORD_ID} readOnly />);
  // The row still renders: who is responsible is a fact a read-only reader is
  // entitled to, and hiding it would read as nobody being assigned.
  expect(await screen.findByText(/Mara Feld/)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Assign" })).toBeNull();
  expect(screen.queryByRole("button", { name: /Change/ })).toBeNull();
});

it("offers no way to add over a read that failed", async () => {
  stubFetch([], { failRead: true });
  render(<RecordTeam recordType="deal" recordId={RECORD_ID} />);
  expect(await screen.findByText(/did not load/i)).toBeTruthy();
  // A failed read is not an empty record. Inviting an assignment here invites
  // a duplicate of one that may already be on the record.
  expect(screen.queryByRole("button", { name: "Assign" })).toBeNull();
});

it("ends a responsibility through the row's own verb", async () => {
  const deleted: string[] = [];
  stubFetch([ROW], { onDelete: (id) => deleted.push(id) });
  render(<RecordTeam recordType="deal" recordId={RECORD_ID} />);
  const remove = await screen.findByRole("button", {
    name: "Remove assignee: Mara Feld",
  });
  await userEvent.click(remove);
  expect(deleted).toEqual(["a1"]);
});

it("names the row in each verb, so one of four is tellable from the rest", async () => {
  stubFetch([ROW, { ...ROW, id: "a2", subject_name: "Jonas Reed" }]);
  render(<RecordTeam recordType="deal" recordId={RECORD_ID} />);
  expect(
    await screen.findByRole("button", {
      name: "Change assignee: Mara Feld",
    }),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Change assignee: Jonas Reed" }),
  ).toBeTruthy();
});

it("says so when ending a responsibility is refused", async () => {
  stubFetch([ROW], { failDelete: true });
  render(<RecordTeam recordType="deal" recordId={RECORD_ID} />);
  const remove = await screen.findByRole("button", {
    name: "Remove assignee: Mara Feld",
  });
  await userEvent.click(remove);
  // The button re-enables either way, so a refusal that said nothing would
  // read exactly like a responsibility that ended.
  expect(await screen.findByRole("alert")).toBeTruthy();
  // Exact match: the row's own name, not the removal verb's tooltip, which
  // also carries "Mara Feld" as part of its own sentence.
  expect(screen.getByText("Mara Feld")).toBeTruthy();
});
