/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";
import type { components } from "../../../api/schema";
import { pickOption } from "../../../design-system/select-testing";
import { LocaleProvider } from "../../../i18n";
import { en } from "../../../i18n/en";
import { CoverageBand } from "./summary";

type Coverage = components["schemas"]["CompanyCoverage"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const DEALS = [
  { deal_id: "d-fleet", name: "Fleet 2027" },
  { deal_id: "d-retrofit", name: "Retrofit 2026" },
];

// Each deal's own committee: the newest has nobody on it, the older one a
// champion. The server reads the newest unless `deal_id` names another.
function committeeOf(dealId: string): Coverage {
  const staffed = dealId === "d-retrofit";
  return {
    as_of: "2026-08-31T09:00:00Z",
    summary: {
      contacts_total: 2,
      waiting: 0,
      answered: 1,
      no_reply: 0,
      untried: 1,
      lapsed: 0,
    },
    deals: DEALS,
    selected_deal_id: dealId,
    completeness: { committee_read: true },
    committee: {
      seats: staffed
        ? [
            {
              contact_id: "p-1",
              full_name: "Ute Sommer",
              role: "champion",
              engagement: "answered",
            },
          ]
        : [],
      gaps: staffed ? ["economic_buyer"] : ["economic_buyer", "champion"],
      unlisted_seats: 0,
    },
  };
}

function stubCoverage(): string[] {
  const asked: string[] = [];
  vi.stubGlobal("fetch", (input: RequestInfo | URL) => {
    const url = new URL(new Request(input).url);
    if (url.pathname.endsWith("/coverage")) {
      asked.push(url.searchParams.get("deal_id") ?? "");
      return Promise.resolve(
        Response.json(
          committeeOf(url.searchParams.get("deal_id") ?? "d-fleet"),
        ),
      );
    }
    return Promise.resolve(Response.json({}));
  });
  return asked;
}

function renderBand() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CoverageBand
          companyId="o-1"
          accountName="Brandt GmbH"
          onNarrow={() => {}}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

test("names the deal whose committee it reads, and switches to another", async () => {
  const user = userEvent.setup();
  const asked = stubCoverage();
  renderBand();

  expect(
    await screen.findByText("Buying committee · Fleet 2027"),
  ).not.toBeNull();
  await pickOption(
    user,
    screen.getByRole("combobox", {
      name: en["co.contacts.board.deal"],
    }),
    "Retrofit 2026",
  );

  expect(
    await screen.findByText("Buying committee · Retrofit 2026"),
  ).not.toBeNull();
  expect(await screen.findByText("Ute Sommer")).not.toBeNull();
  await waitFor(() => expect(asked).toEqual(["", "d-retrofit"]));
});

test("offers no deal picker when the account has one open deal", async () => {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(
      Response.json({ ...committeeOf("d-fleet"), deals: [DEALS[0]] }),
    ),
  );
  renderBand();

  expect(
    await screen.findByText("Buying committee · Fleet 2027"),
  ).not.toBeNull();
  expect(
    screen.queryByRole("combobox", {
      name: en["co.contacts.board.deal"],
    }),
  ).toBeNull();
});

test("falls back to the newest deal's committee when the picked deal has closed", async () => {
  const user = userEvent.setup();
  const asked: string[] = [];
  let closed = false;
  vi.stubGlobal("fetch", (input: RequestInfo | URL) => {
    const named = new URL(new Request(input).url).searchParams.get("deal_id");
    asked.push(named ?? "");
    if (named === "d-retrofit") {
      closed = true;
      return Promise.resolve(
        Response.json({ title: "Not found", status: 404 }, { status: 404 }),
      );
    }
    // Once Retrofit has closed, the server's own answer no longer lists it.
    const deals = closed ? [DEALS[0]] : DEALS;
    return Promise.resolve(Response.json({ ...committeeOf("d-fleet"), deals }));
  });
  renderBand();

  await screen.findByText("Buying committee · Fleet 2027");
  await pickOption(
    user,
    screen.getByRole("combobox", {
      name: en["co.contacts.board.deal"],
    }),
    "Retrofit 2026",
  );

  await waitFor(() => expect(asked).toContain("d-retrofit"));
  // The refreshed answer lists one deal, so its picker leaves only once it lands.
  await waitFor(() =>
    expect(
      screen.queryByRole("combobox", {
        name: en["co.contacts.board.deal"],
      }),
    ).toBeNull(),
  );
  expect(screen.getByText("Buying committee · Fleet 2027")).not.toBeNull();
  expect(screen.queryByText("Unavailable")).toBeNull();
});
