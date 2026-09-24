// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { LinkedInReachCard } from "./linkedin-reach";
import { installFetchStub, jsonResponse } from "./story-utils";

// Which accounts a member's imported network reaches. The figures ARE the
// answer: how many connections resolved to each account, and how many of those
// are already contacts — the gap between the two is what the import was for.

type LinkedInReach = components["schemas"]["LinkedInReachResponse"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
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
        <LinkedInReachCard />
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
        .map((cell) => cell.textContent),
    ),
  ).toEqual([
    ["Nordwind Logistik GmbH", "1,204", "3 of 1,204"],
    ["Havelmann & Söhne", "6", "6 of 6"],
  ]);
  // Each account name leads to its record, where the gap can be closed.
  expect(
    within(rows[0]).getByRole("link", { name: "Nordwind Logistik GmbH" }),
  ).toHaveAttribute("href", "#/companies/018f3a1b-0000-7000-8000-0000000000a1");
  // Two listed of nine reached: where the list stops is stated, not implied.
  expect(
    screen.getByText(
      "Showing 2 of 9 companies. 1,420 connections work at companies not on file yet.",
    ),
  ).toBeInTheDocument();
});
