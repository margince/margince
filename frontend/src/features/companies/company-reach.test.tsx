// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { CompanyReachCard } from "./company-reach";

// Who on the team can get you in at this account. The card names the colleague
// to ask and says how many connections they have here. Never who they are.

type Reach = components["schemas"]["CompanyLinkedInReachResponse"];

const COMPANY = "018f3a1b-0000-7000-8000-0000000000c1";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderCard(body: Reach): void {
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture()),
    [`GET /companies/${COMPANY}/linkedin-reach`]: () => jsonResponse(body),
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CompanyReachCard companyId={COMPANY} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

it("names each colleague and their count, in the order the server sent", async () => {
  renderCard({
    colleagues: [
      {
        user_id: "018f3a1b-0000-7000-8000-0000000000u1",
        display_name: "Dana Okonjo",
        connections: 3,
      },
      {
        user_id: "018f3a1b-0000-7000-8000-0000000000u2",
        display_name: "Minh Tran",
        connections: 1,
      },
    ],
  });

  const table = await screen.findByRole("table");
  const [, ...rows] = within(table).getAllByRole("row");
  expect(
    rows.map((row) =>
      within(row)
        .getAllByRole("cell")
        .map(
          (cell) =>
            (cell.querySelector(".datatable-unfolded") ?? cell).textContent,
        ),
    ),
  ).toEqual([
    ["Dana Okonjo", "3"],
    ["Minh Tran", "1"],
  ]);
});

it("says nobody reaches this account rather than drawing an empty table", async () => {
  renderCard({ colleagues: [] });

  expect(
    await screen.findByText(/No colleague reach is shown for this company/),
  ).toBeInTheDocument();
  expect(screen.queryByRole("table")).toBeNull();
});

// A refusal is not an empty answer. Drawn as "nobody reaches this account" it
// would read as a cold door rather than a list this reader was not given.
it("says the list could not be read when the read is refused", async () => {
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture()),
    [`GET /companies/${COMPANY}/linkedin-reach`]: () =>
      jsonResponse({ title: "Forbidden", detail: "no" }, 403),
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CompanyReachCard companyId={COMPANY} />
      </LocaleProvider>
    </QueryClientProvider>,
  );

  expect(
    await screen.findByText(/reach list could not be read/),
  ).toBeInTheDocument();
  expect(screen.queryByRole("table")).toBeNull();
});

// The reader's own reach is a way in they hold already. It stays, reads "You"
// rather than their own name, and leads the list.
it("names the reader's own row You, first", async () => {
  const me = meFixture();
  renderCard({
    colleagues: [
      {
        user_id: "018f3a1b-0000-7000-8000-0000000000u2",
        display_name: "Minh Tran",
        connections: 1,
      },
      {
        user_id: me.user.id,
        display_name: me.user.display_name,
        connections: 8,
      },
    ],
  });

  const table = await screen.findByRole("table");
  const [, ...rows] = within(table).getAllByRole("row");
  const firstCell = await within(rows[0]).findByText("You");
  expect(firstCell).toBeInTheDocument();
  expect(within(table).queryByText(me.user.display_name)).toBeNull();
  expect(within(rows[1]).getByText("Minh Tran")).toBeInTheDocument();
});
