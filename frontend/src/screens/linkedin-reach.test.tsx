// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { meFixture } from "../testing/mefixture";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import { LinkedInReachSection } from "./linkedin-reach";

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

function renderReach(body: LinkedInReach): void {
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture()),
    "GET /me/linkedin-reach": () => jsonResponse(body),
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <LinkedInReachSection />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

it("shows, per account reached, how many connections work there and how many are already on file", async () => {
  renderReach(REACHED);

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
  renderReach(REACHED);
  const user = userEvent.setup();

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

it("says how many connections match nothing yet, instead of an empty sentence", async () => {
  renderReach({
    accounts: [],
    accounts_total: 0,
    unresolved_connections: 5210,
  });

  expect(
    await screen.findByText(
      "All your connections work at companies not on file yet. Connections: 5,210.",
    ),
  ).toBeInTheDocument();
  expect(screen.getByText("No companies reached yet")).toBeInTheDocument();
  expect(screen.queryByRole("table")).toBeNull();
});

it("explains what will appear when nothing is imported yet", async () => {
  renderReach({ accounts: [], accounts_total: 0, unresolved_connections: 0 });

  expect(
    await screen.findByText(
      "After the import, companies on file where you know someone appear here as a table, most connections first.",
    ),
  ).toBeInTheDocument();
});
