/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, test, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { CoverageBand } from "./summary";

type Coverage = components["schemas"]["OrganizationCoverage"];

afterEach(cleanup);

function render(ui: ReactNode, locale: "en" | "de" = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function stub(coverage: Partial<Coverage>) {
  const body: Coverage = {
    as_of: "2026-08-31T09:00:00Z",
    summary: { contacts_total: 3, answered: 1, no_reply: 0, untried: 2 },
    deals: [],
    completeness: { committee_read: true },
    ...coverage,
  } as Coverage;
  vi.stubGlobal("fetch", (input: RequestInfo | URL) => {
    const url =
      typeof input === "string"
        ? input
        : input instanceof URL
          ? input.href
          : input.url;
    if (url.includes("/coverage")) {
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );
    }
    return Promise.resolve(new Response("{}", { status: 200 }));
  });
}

test("names the person worth writing to", async () => {
  stub({
    best_way_in: {
      person_id: "p-1",
      full_name: "Dietmar Rietsch",
      title: "Managing Director",
      engagement: "answered",
    },
  });
  render(<CoverageBand orgId="o-1" onNarrow={() => {}} />);

  expect(await screen.findByText("Dietmar Rietsch")).not.toBeNull();
});

// An account where everyone was written to and nobody replied has no way in.
// Naming the least-cold contact would dress a fourth follow-up up as an
// opening, which is the one thing this reading must not do.
test("says nobody has answered rather than naming a fallback", async () => {
  stub({
    summary: { contacts_total: 2, answered: 0, no_reply: 2, untried: 0 },
  });
  render(<CoverageBand orgId="o-1" onNarrow={() => {}} />);

  expect(await screen.findByText("Nobody has answered")).not.toBeNull();
});

test("names the missing critical role", async () => {
  stub({
    committee: {
      seats: [
        {
          person_id: "p-2",
          full_name: "Ute Sommer",
          role: "economic_buyer",
          engagement: "untried",
        },
      ],
      gaps: ["champion"],
      unlisted_seats: 0,
    },
  });
  render(<CoverageBand orgId="o-1" onNarrow={() => {}} />);

  expect(await screen.findByText("No champion")).not.toBeNull();
  // And the gap is shown WHERE it is: in the column that would hold them.
  expect(screen.getByTestId("gap-champion")).not.toBeNull();
});

// A committee the reader may not read is a third state, not an empty one. A
// page that collapsed it into "no champion" would tell a reader a deal has
// nobody carrying it when it has somebody they cannot see.
test("tells an unreadable committee apart from an empty one", async () => {
  stub({ completeness: { committee_read: false } });
  render(<CoverageBand orgId="o-1" onNarrow={() => {}} />);

  expect(await screen.findByText("Hidden from you")).not.toBeNull();
  expect(screen.queryByText(/No champion/)).toBeNull();
});

// A gap over a committee that could not be read whole is suppressed by the
// server; the band must not invent one from the seats it happens to hold.
test("does not name a gap when seats are hidden", async () => {
  stub({
    committee: { seats: [], gaps: [], unlisted_seats: 2 },
  });
  render(<CoverageBand orgId="o-1" onNarrow={() => {}} />);

  expect(await screen.findByText("2 more you cannot see")).not.toBeNull();
  expect(screen.queryByText(/No champion/)).toBeNull();
});

test("each door narrows the list to what it describes", async () => {
  const narrowed: (string | null)[] = [];
  stub({
    best_way_in: {
      person_id: "p-1",
      full_name: "Dietmar Rietsch",
      engagement: "answered",
    },
  });
  render(<CoverageBand orgId="o-1" onNarrow={(s) => narrowed.push(s)} />);

  const untried = await screen.findByRole("button", {
    name: "Show who is untried",
  });
  untried.click();
  expect(narrowed).toContain("untried");
});

test("renders the German words under a German locale", async () => {
  stub({});
  render(<CoverageBand orgId="o-1" onNarrow={() => {}} />, "de");

  await waitFor(() => expect(screen.getByText("Abdeckung")).not.toBeNull());
});

// An account with no open deal has no committee to READ, which is not the same
// as one the reader may not read. Collapsing the two accuses the reader's own
// role on every account that simply has nothing open.
test("tells no-open-deal apart from withheld", async () => {
  stub({ deals: [], completeness: { committee_read: true } });
  render(<CoverageBand orgId="o-1" onNarrow={() => {}} />);

  expect(await screen.findByText("No open deal")).not.toBeNull();
  expect(screen.queryByText("Hidden from you")).toBeNull();
});

// The server empties `gaps` whenever a seat is hidden, because it cannot tell
// whether the role is held. Reading that empty list as "both roles named" turns
// a suppressed answer into a confident one.
test("does not claim a complete committee when seats are hidden", async () => {
  stub({ committee: { seats: [], gaps: [], unlisted_seats: 2 } });
  render(<CoverageBand orgId="o-1" onNarrow={() => {}} />);

  expect(await screen.findByText("Cannot be judged")).not.toBeNull();
  expect(screen.queryByText("Champion and economic buyer named")).toBeNull();
});

// The door's label has to name what pressing it does. A way in who has not
// answered clears the filter, so promising "who answered" would be a label for
// a different press.
test("labels the way-in door by what it actually does", async () => {
  stub({
    best_way_in: {
      person_id: "p-1",
      full_name: "Philipp Koenigs",
      engagement: "untried",
    },
  });
  render(<CoverageBand orgId="o-1" onNarrow={() => {}} />);

  expect(
    await screen.findByRole("button", { name: "Show everyone" }),
  ).not.toBeNull();
  expect(
    screen.queryByRole("button", { name: "Show who answered" }),
  ).toBeNull();
});

// A seat carrying a role this board has no column for is still a person the
// summary counted. Dropping it silently sends a reader looking for somebody
// the board never drew.
test("draws a seat whose role has no column of its own", async () => {
  stub({
    committee: {
      seats: [
        {
          person_id: "p-9",
          full_name: "Sam Consultant",
          role: "technical_advisor",
          engagement: "answered",
        },
      ],
      gaps: [],
      unlisted_seats: 0,
    },
  });
  render(<CoverageBand orgId="o-1" onNarrow={() => {}} />);

  expect(await screen.findByText("Sam Consultant")).not.toBeNull();
});

// A read that failed says so. Rendering nothing makes a server error look like
// a band this account does not offer.
test("says the reading failed rather than vanishing", async () => {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(new Response("{}", { status: 500 })),
  );
  render(<CoverageBand orgId="o-1" onNarrow={() => {}} />);

  expect(await screen.findByText("Could not be read")).not.toBeNull();
});
