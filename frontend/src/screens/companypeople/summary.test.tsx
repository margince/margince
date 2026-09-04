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
import userEvent from "@testing-library/user-event";
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

/** What the writes answered, and every URL the component actually sent. */
type Writes = {
  proposals?: unknown;
  proposalStatus?: number;
  calls: string[];
};

function coverageBody(coverage: Partial<Coverage>): Coverage {
  return {
    as_of: "2026-08-31T09:00:00Z",
    summary: {
      contacts_total: 3,
      waiting: 0,
      answered: 1,
      no_reply: 0,
      untried: 2,
    },
    deals: [],
    completeness: { committee_read: true },
    ...coverage,
  } as Coverage;
}

function stub(coverage: Partial<Coverage>, writes?: Writes) {
  const body = coverageBody(coverage);
  vi.stubGlobal("fetch", (input: RequestInfo | URL) => {
    const url =
      typeof input === "string"
        ? input
        : input instanceof URL
          ? input.href
          : input.url;
    writes?.calls.push(url);
    if (url.includes("/coverage")) {
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );
    }
    if (url.includes("/role-proposals")) {
      return Promise.resolve(
        new Response(JSON.stringify(writes?.proposals ?? {}), {
          status: writes?.proposalStatus ?? 200,
          headers: {
            "content-type":
              (writes?.proposalStatus ?? 200) >= 400
                ? "application/problem+json"
                : "application/json",
          },
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
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );

  expect(await screen.findByText("Dietmar Rietsch")).not.toBeNull();
});

// An account where everyone was written to and nobody replied has no way in.
// Naming the least-cold contact would dress a fourth follow-up up as an
// opening, which is the one thing this reading must not do.
test("says nobody has answered rather than naming a fallback", async () => {
  stub({
    summary: {
      contacts_total: 2,
      waiting: 0,
      answered: 0,
      no_reply: 2,
      untried: 0,
    },
  });
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );

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
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );

  expect(await screen.findByText("No champion")).not.toBeNull();
  // And the gap is shown WHERE it is: in the column that would hold them.
  expect(screen.getByTestId("gap-champion")).not.toBeNull();
});

// A committee the reader may not read is a third state, not an empty one. A
// page that collapsed it into "no champion" would tell a reader a deal has
// nobody carrying it when it has somebody they cannot see.
test("tells an unreadable committee apart from an empty one", async () => {
  stub({ completeness: { committee_read: false } });
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );

  expect(await screen.findByText("Hidden from you")).not.toBeNull();
  expect(screen.queryByText(/No champion/)).toBeNull();
});

// A gap over a committee that could not be read whole is suppressed by the
// server; the band must not invent one from the seats it happens to hold.
test("does not name a gap when seats are hidden", async () => {
  stub({
    committee: { seats: [], gaps: [], unlisted_seats: 2 },
  });
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );

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
  render(
    <CoverageBand
      orgId="o-1"
      accountName="Brandt GmbH"
      onNarrow={(s) => narrowed.push(s)}
    />,
  );

  const untried = await screen.findByRole("button", {
    name: "Show who is untried",
  });
  untried.click();
  expect(narrowed).toContain("untried");
});

test("renders the German words under a German locale", async () => {
  stub({});
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
    "de",
  );

  await waitFor(() => expect(screen.getByText("Abdeckung")).not.toBeNull());
});

// An account with no open deal has no committee to READ, which is not the same
// as one the reader may not read. Collapsing the two accuses the reader's own
// role on every account that simply has nothing open.
test("tells no-open-deal apart from withheld", async () => {
  stub({ deals: [], completeness: { committee_read: true } });
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );

  expect(await screen.findByText("No open deal")).not.toBeNull();
  expect(screen.queryByText("Hidden from you")).toBeNull();
});

// The server empties `gaps` whenever a seat is hidden, because it cannot tell
// whether the role is held. Reading that empty list as "both roles named" turns
// a suppressed answer into a confident one.
test("does not claim a complete committee when seats are hidden", async () => {
  stub({ committee: { seats: [], gaps: [], unlisted_seats: 2 } });
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );

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
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );

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
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );

  expect(await screen.findByText("Sam Consultant")).not.toBeNull();
});

// A read that failed says so. Rendering nothing makes a server error look like
// a band this account does not offer.
test("says the reading failed rather than vanishing", async () => {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(new Response("{}", { status: 500 })),
  );
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );

  expect(await screen.findByText("Could not be read")).not.toBeNull();
});

test("offers the board and the map, and switches between them", async () => {
  const user = userEvent.setup();
  stub({
    committee: {
      seats: [
        {
          person_id: "p-1",
          full_name: "Philipp Koenigs",
          role: "economic_buyer",
          engagement: "untried",
          routes: {
            top: [
              {
                user_id: "u-1",
                display_name: "Sofia Meier",
                strength_bucket: "developing",
                last_interaction_at: "2026-08-20T09:00:00Z",
              },
            ],
            remainder: 0,
            untried: false,
          },
        },
      ],
      gaps: ["champion"],
      unlisted_seats: 0,
    },
  });
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );

  await screen.findByRole("button", { name: "Map" });
  await user.click(screen.getByRole("button", { name: "Map" }));

  // The same committee, drawn: the colleague who can reach the seat, and the
  // hole where the champion would be.
  expect(
    await screen.findByRole("button", { name: /Sofia Meier/ }),
  ).not.toBeNull();
  expect(
    screen.getByRole("button", { name: /champion missing/i }),
  ).not.toBeNull();
});

// A seat the product read out of messages carries the AI mark; one a colleague
// typed carries none of it. The mark is the whole reason a reader can tell the
// product's reading from a person's assertion and disagree with the first.
test("marks a seat the product read, and only that one", async () => {
  stub({
    committee: {
      seats: [
        {
          person_id: "p-1",
          full_name: "Dietmar Rietsch",
          role: "champion",
          engagement: "answered",
          ai_suggested: true,
        },
        {
          person_id: "p-2",
          full_name: "Ute Sommer",
          role: "economic_buyer",
          engagement: "untried",
        },
      ],
      gaps: [],
      unlisted_seats: 0,
    },
  });
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );

  await screen.findByText("Dietmar Rietsch");
  const marks = screen.getAllByText("Read from their messages");
  expect(marks).toHaveLength(1);
  // The mark sits on the seat that was read, not the one that was typed.
  expect(marks[0]?.closest("[data-suggested='true']")).not.toBeNull();
});

/** An account with one open deal and one seat the product read. */
function suggestedCoverage(): Partial<Coverage> {
  return {
    deals: [{ deal_id: "d-1", name: "Retrofit 2026" }],
    selected_deal_id: "d-1",
    committee: {
      seats: [
        {
          person_id: "p-1",
          full_name: "Ute Sommer",
          role: "economic_buyer",
          engagement: "answered",
          relationship_id: "r-1",
          relationship_version: 1,
          ai_suggested: true,
        },
      ],
      gaps: [],
      unlisted_seats: 0,
    },
  } as Partial<Coverage>;
}

test("reads the roles from the deal, and refreshes the board after", async () => {
  const writes: Writes = {
    calls: [],
    proposals: {
      written: [
        {
          person_id: "p-1",
          full_name: "Ute Sommer",
          role: "economic_buyer",
          evidence_snippet: "I sign off the budget for this, so send",
          source_activity_id: "a-1",
          confidence: 0.9,
        },
      ],
      skipped: 0,
      generated_by: "model",
    },
  };
  stub(suggestedCoverage(), writes);
  const user = userEvent.setup();
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );

  await user.click(
    await screen.findByRole("button", { name: /Suggest roles/i }),
  );
  await screen.findByText(/Seated 1 from what they wrote/);
  // The POST goes to the DEAL; the refresh reads the ACCOUNT. A component that
  // refreshed the deal instead would show the board as it was before the write.
  expect(
    writes.calls.some((url) => url.includes("/deals/d-1/role-proposals")),
  ).toBe(true);
  expect(
    writes.calls.filter((url) => url.includes("/organizations/o-1/coverage"))
      .length,
  ).toBeGreaterThan(1);
});

// The two empty answers are DIFFERENT facts. "Nothing was said" sends a reader
// to the correspondence; "everything was refused" tells them the product looked
// and would not stand behind what it found.
test("tells nothing-proposed apart from everything-refused", async () => {
  stub(suggestedCoverage(), {
    calls: [],
    proposals: { written: [], skipped: 0, generated_by: "model" },
  });
  const user = userEvent.setup();
  const first = render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );
  await user.click(
    await screen.findByRole("button", { name: /Suggest roles/i }),
  );
  await screen.findByText(/Nothing in their messages says who buys/);
  first.unmount();

  stub(suggestedCoverage(), {
    calls: [],
    proposals: { written: [], skipped: 3, generated_by: "model" },
  });
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );
  await user.click(
    await screen.findByRole("button", { name: /Suggest roles/i }),
  );
  await screen.findByText(/3 reading\(s\) were dropped/);
});

// A role is recorded on a deal. Hidden, the button teaches nothing; disabled
// with the reason, it says what the account is missing.
test("says why it cannot read roles when the account has no open deal", async () => {
  stub({ committee: { seats: [], gaps: ["champion"], unlisted_seats: 0 } });
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );
  const button = await screen.findByRole("button", { name: /Suggest roles/i });
  expect((button as HTMLButtonElement).disabled).toBe(true);
  expect(await screen.findByText(/Roles are recorded on a deal/)).toBeTruthy();
});

// Confirming writes the SAME role back. That looks like a no-op and is not:
// the store reassigns captured_by to the editing human, which is what turns
// the machine's reading into a person's answer.
test("confirming patches the seat's own row with the role unchanged", async () => {
  const writes: Writes = { calls: [] };
  stub(suggestedCoverage(), writes);
  const user = userEvent.setup();
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );

  await user.click(await screen.findByRole("button", { name: /^Confirm$/ }));
  await waitFor(() =>
    expect(writes.calls.some((url) => url.includes("/relationships/r-1"))).toBe(
      true,
    ),
  );
});

// The verbs belong to an UNCONFIRMED seat only. A seat a colleague typed is
// already theirs and nothing about it is waiting for an answer, so offering to
// confirm it would be agreeing with them on their behalf.
test("offers no verbs on a seat a person typed", async () => {
  stub({
    deals: [{ deal_id: "d-1", name: "Retrofit 2026" }],
    selected_deal_id: "d-1",
    committee: {
      seats: [
        {
          person_id: "p-2",
          full_name: "Jan Roth",
          role: "champion",
          engagement: "answered",
          relationship_id: "r-2",
          relationship_version: 1,
        },
      ],
      gaps: [],
      unlisted_seats: 0,
    },
  } as Partial<Coverage>);
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );
  await screen.findByText("Jan Roth");
  expect(screen.queryByRole("button", { name: /^Confirm$/ })).toBeNull();
  expect(screen.queryByRole("button", { name: /Change role/i })).toBeNull();
});

// A seat with no version cannot be patched conditionally, and an unconditional
// confirm would overwrite whatever a colleague changed. The verbs are not
// offered rather than offered and silently dropped: a button that reports
// nothing and does nothing is worse than one that is not there.
test("offers no verbs when the seat carries no version to pin the write", async () => {
  stub({
    deals: [{ deal_id: "d-1", name: "Retrofit 2026" }],
    selected_deal_id: "d-1",
    committee: {
      seats: [
        {
          person_id: "p-1",
          full_name: "Ute Sommer",
          role: "economic_buyer",
          engagement: "answered",
          relationship_id: "r-1",
          ai_suggested: true,
        },
      ],
      gaps: [],
      unlisted_seats: 0,
    },
  } as Partial<Coverage>);
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );
  await screen.findByText("Ute Sommer");
  expect(screen.queryByRole("button", { name: /^Confirm$/ })).toBeNull();
});

// A concurrent edit must say so in words a reader can act on. The sentinel's
// own text is "version skew", which tells them nothing about what to do.
test("says a concurrent edit happened rather than printing the sentinel", async () => {
  const writes: Writes = { calls: [] };
  stub(suggestedCoverage(), writes);
  vi.stubGlobal("fetch", (input: RequestInfo | URL) => {
    const url =
      typeof input === "string"
        ? input
        : input instanceof URL
          ? input.href
          : input.url;
    if (url.includes("/relationships/")) {
      return Promise.resolve(
        new Response(
          JSON.stringify({ code: "version_skew", detail: "version skew" }),
          {
            status: 409,
            headers: { "content-type": "application/problem+json" },
          },
        ),
      );
    }
    return Promise.resolve(
      new Response(JSON.stringify(coverageBody(suggestedCoverage())), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );
  });
  const user = userEvent.setup();
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );
  await user.click(await screen.findByRole("button", { name: /^Confirm$/ }));
  await screen.findByText(/changed since you opened it/);
  expect(screen.queryByText("version skew")).toBeNull();
});

// An installation with no model lane answers 501, and the handler's own words
// name a Go function. A reader is owed a sentence about the product.
test("says the reading needs a model rather than naming a handler", async () => {
  stub(suggestedCoverage(), {
    calls: [],
    proposalStatus: 501,
    proposals: {
      code: "not_implemented",
      detail:
        "operation ProposeDealRoles (no model path configured) is specified but not yet implemented",
    },
  });
  const user = userEvent.setup();
  render(
    <CoverageBand orgId="o-1" accountName="Brandt GmbH" onNarrow={() => {}} />,
  );
  await user.click(
    await screen.findByRole("button", { name: /Suggest roles/i }),
  );
  await screen.findByText(/Reading roles needs a model/);
  expect(screen.queryByText(/ProposeDealRoles/)).toBeNull();
});
