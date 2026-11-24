// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { TeamOversight } from "./worklist.gap";

type ForecastFocus = components["schemas"]["ForecastFocus"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const TEAM = "00000000-0000-4000-8000-0000000000a1";

function focus(team: Partial<ForecastFocus["teams"][number]>): ForecastFocus {
  return {
    period_start: "2026-10-01",
    period_end: "2026-12-31",
    teams: [
      {
        team_id: TEAM,
        snapshot_id: "00000000-0000-4000-8000-0000000000b1",
        taken_at: "2026-11-18T01:00:00Z",
        base_currency: "EUR",
        status: "behind",
        deals: [],
        masked_count: 0,
        ...team,
      },
    ],
    withheld: false,
  };
}

// Answers the focus read, and an empty list for every other read. The panel
// then draws only what the stored list says.
function stub(body: ForecastFocus) {
  const fetched = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    let answer: unknown = { data: [], page: { has_more: false } };
    if (url.includes("/forecast/focus")) answer = body;
    if (url.includes("/worklist/exceptions")) {
      answer = {
        as_of: "2026-11-18T01:00:00Z",
        truncated: false,
        exceptions: [],
      };
    }
    return new Response(JSON.stringify(answer), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  });
  vi.stubGlobal("fetch", fetched);
  return fetched;
}

function renderOversight(scope: string) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <TeamOversight scope={scope} enabled onOwner={() => {}} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function openGapTab() {
  const user = userEvent.setup();
  await user.click(
    await screen.findByRole("button", { name: en["worklist.gap.tab"] }),
  );
}

describe("the Close the gap tab", () => {
  it("lists the frozen ranks with the facts behind each", async () => {
    stub(
      focus({
        deals: [
          {
            rank: 1,
            deal_id: "00000000-0000-4000-8000-0000000000d1",
            deal_name: "Big renewal",
            weighted_minor: 30_000_00,
            reason: {
              closes_in_days: -3,
              close_provisional: true,
              signals: [{ code: "close_pushed", count: 4 }],
            },
          },
          {
            rank: 2,
            deal_id: "00000000-0000-4000-8000-0000000000d2",
            deal_name: "Small add-on",
            reason: {
              closes_in_days: 5,
              close_provisional: false,
              signals: [],
            },
          },
        ],
      }),
    );
    renderOversight("team");
    await openGapTab();

    expect(await screen.findByText(en["worklist.gap.behind"])).toBeTruthy();
    const rows = screen.getAllByRole("row").slice(1);
    expect(within(rows[0]).getByText("Big renewal")).toBeTruthy();
    expect(within(rows[0]).getByText("Close date 3 days ago")).toBeTruthy();
    expect(
      within(rows[0]).getByText(en["worklist.gap.provisional"]),
    ).toBeTruthy();
    expect(within(rows[0]).getByText("Close date moved 4 times")).toBeTruthy();
    expect(within(rows[0]).getByText("€30,000.00")).toBeTruthy();
    expect(within(rows[1]).getByText("Small add-on")).toBeTruthy();
    expect(within(rows[1]).getByText("Closes in 5 days")).toBeTruthy();
  });

  it("prints no figure for an amount the server withheld", async () => {
    stub(
      focus({
        deals: [
          {
            rank: 1,
            deal_id: "00000000-0000-4000-8000-0000000000d1",
            deal_name: "Masked deal",
            reason: {
              closes_in_days: 0,
              close_provisional: false,
              signals: [],
            },
          },
        ],
      }),
    );
    renderOversight("team");
    await openGapTab();

    const row = (await screen.findByText("Masked deal")).closest("tr");
    if (row === null) throw new Error("the deal is not in a table row");
    expect(row.textContent).not.toMatch(/€|0\.00/);
    expect(within(row).getByText(en["worklist.gap.closesToday"])).toBeTruthy();
  });

  it("says why a team has no list instead of drawing an empty table", async () => {
    stub(focus({ status: "no_goal" }));
    renderOversight("team");
    await openGapTab();

    expect(await screen.findByText(en["worklist.gap.noGoal"])).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("tells an on-target team with nothing at risk that it is covered", async () => {
    stub(focus({ status: "on_target" }));
    renderOversight("team");
    await openGapTab();

    expect(
      await screen.findByText(en["worklist.gap.nothingAtRisk"]),
    ).toBeTruthy();
  });

  it("offers no tab on a scope wider than one team, and reads no list", async () => {
    const fetched = stub(focus({}));
    renderOversight("all");

    expect(
      await screen.findByText(en["worklist.exceptions.title"]),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["worklist.gap.tab"] }),
    ).toBeNull();
    const urls = fetched.mock.calls.map(([input]) =>
      String(input instanceof Request ? input.url : input),
    );
    expect(urls.some((url) => url.includes("/forecast/focus"))).toBe(false);
  });

  it("says the switch held a team back rather than that no list exists", async () => {
    stub({ ...focus({}), teams: [], withheld: true });
    renderOversight("team");
    await openGapTab();

    expect(await screen.findByText(en["worklist.gap.withheld"])).toBeTruthy();
    expect(screen.queryByText(en["worklist.gap.empty"])).toBeNull();
  });

  it("says which day the list was ranked, and how many masked deals it leaves out", async () => {
    stub(focus({ status: "on_target", masked_count: 2 }));
    renderOversight("team");
    await openGapTab();

    expect(
      await screen.findByText(
        "2 deals are left out because you may not see their amounts.",
      ),
    ).toBeTruthy();
    expect(screen.getByText(/^Ranked on .*2026/)).toBeTruthy();
    // An on-target team whose deals are only masked is not one with nothing
    // at risk.
    expect(screen.queryByText(en["worklist.gap.nothingAtRisk"])).toBeNull();
    expect(screen.getByText(en["worklist.gap.onTarget"])).toBeTruthy();
  });
});
