// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { heldThreadList } from "../../testing/heldthreads.fixtures";
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { ConnectionOutcomesCard } from "../capture/connection-outcomes";

// Which accounts a member's imported network reaches. The figures ARE the
// answer: how many connections resolved to each account, and how many of those
// are already contacts. The gap between the two is what the import was for.

type LinkedInReach = components["schemas"]["LinkedInReachResponse"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.location.hash = "";
});

// Counts past a thousand, so a figure printed without its grouping reads
// differently from one printed with it.
const REACHED: LinkedInReach = {
  accounts: [
    {
      company_id: "018f3a1b-0000-7000-8000-0000000000a1",
      display_name: "Nordwind Logistik GmbH",
      connections: 1204,
      contacts_on_file: 3,
    },
    {
      company_id: "018f3a1b-0000-7000-8000-0000000000a2",
      display_name: "Havelmann & Söhne",
      connections: 6,
      contacts_on_file: 6,
    },
  ],
  accounts_total: 9,
  unresolved_connections: 1420,
};

// The reach opens from the outcomes card's row, in a drawer.
async function renderReach(
  body: LinkedInReach,
  user: ReturnType<typeof userEvent.setup>,
): Promise<void> {
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture()),
    "GET /me/linkedin-reach": () => jsonResponse(body),
    "GET /capture/senders": () => jsonResponse({ data: [] }),
    "GET /capture/contacts-awaiting-decision": () => jsonResponse({ data: [] }),
    "GET /capture/held-threads": () => jsonResponse(heldThreadList()),
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ConnectionOutcomesCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  if (body.accounts.length > 0) {
    await user.click(
      await screen.findByRole("button", { name: "View LinkedIn network" }),
    );
  }
}

it("shows, per account reached, how many connections work there and how many are already on file", async () => {
  await renderReach(REACHED, userEvent.setup());

  const table = await screen.findByRole("table");
  const [, ...rows] = within(table).getAllByRole("row");
  expect(
    rows.map((row) =>
      within(row)
        .getAllByRole("cell")
        .slice(0, 3)
        .map(
          (cell) =>
            (cell.querySelector(".datatable-unfolded") ?? cell).textContent,
        ),
    ),
  ).toEqual([
    ["Nordwind Logistik GmbH", "1,204", "3 of 1,204"],
    ["Havelmann & Söhne", "6", "6 of 6"],
  ]);
  // Folded on a phone, a bare figure says what it counts.
  expect(
    within(rows[0]).getByText("1,204 connections", { exact: false }),
  ).toBeTruthy();
  // Two listed of nine reached: where the list stops is stated, not implied.
  expect(
    screen.getByText(
      "Showing 2 of 9 companies. 1,420 connections work at companies not on file yet.",
    ),
  ).toBeInTheDocument();
});

// Each account leads to its record, where the gap can be closed: from its
// name, from the row and from the keyboard's chevron.
it("opens the company an account row names", async () => {
  const user = userEvent.setup();
  await renderReach(REACHED, user);

  await user.click(
    await screen.findByRole("button", { name: "Nordwind Logistik GmbH" }),
  );
  expect(globalThis.location.hash).toBe(
    "#/companies/018f3a1b-0000-7000-8000-0000000000a1",
  );
  await user.click(
    screen.getByRole("button", { name: "Open Havelmann & Söhne" }),
  );
  expect(globalThis.location.hash).toBe(
    "#/companies/018f3a1b-0000-7000-8000-0000000000a2",
  );
});

// With nothing reached, the row carries the one fact a reader can act on, and
// offers no empty list to open.
it("says why nothing is reached when every connection is off file", async () => {
  await renderReach(
    { accounts: [], accounts_total: 0, unresolved_connections: 40 },
    userEvent.setup(),
  );

  expect(
    await screen.findByText(
      "All your connections work at companies not on file yet. Connections: 40.",
    ),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "View LinkedIn network" }),
  ).toBeNull();
});

// A network past one page continues from the drawer, from the cursor the
// first page handed back, until the server says it has no more.
it("reaches the accounts past the first page", async () => {
  const user = userEvent.setup();
  const [first, second] = REACHED.accounts;
  const reads: (string | null)[] = [];
  await renderReach(
    {
      ...REACHED,
      accounts: [first],
      accounts_total: 2,
      next_cursor: "reach-2",
    },
    user,
  );
  const routed = globalThis.fetch;
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const request =
      input instanceof Request ? input : new Request(String(input), init);
    const url = new URL(request.url);
    if (!url.pathname.endsWith("/me/linkedin-reach")) {
      return routed(input, init);
    }
    reads.push(url.searchParams.get("cursor"));
    return jsonResponse({
      ...REACHED,
      accounts: [second],
      accounts_total: 2,
    });
  }) as typeof fetch;

  await user.click(await screen.findByRole("button", { name: "Load more" }));

  expect(
    await screen.findByRole("button", { name: "Havelmann & Söhne" }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Nordwind Logistik GmbH" }),
  ).toBeInTheDocument();
  expect(reads).toEqual(["reach-2"]);
  expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  expect(
    screen.getByText(
      "Showing 2 of 2 companies. 1,420 connections work at companies not on file yet.",
    ),
  ).toBeInTheDocument();
});
