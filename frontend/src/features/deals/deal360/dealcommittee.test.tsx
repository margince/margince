/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../../api/schema";
import { LocaleProvider, translate } from "../../../i18n";
import { en } from "../../../i18n/en";
import { CommitteeReading } from "./dealcommittee";

type DealCoverage = components["schemas"]["DealCoverage"];

// Every mark the card draws is explained in words beside it and in the
// legend. The withheld, loading and empty reads each say their own sentence,
// so a check that never ran never reads as a covered deal.

const DEAL_ID = "01a03000-0000-7000-8000-000000000001";

const coverage = (over: Partial<DealCoverage> = {}): DealCoverage => ({
  deal_id: DEAL_ID,
  stakeholders: [
    {
      contact_id: "01a03000-0000-7000-8000-0000000000b1",
      contact_name: "Dana Weiss",
      role: "champion",
      engaged: true,
    },
    {
      contact_id: "01a03000-0000-7000-8000-0000000000b3",
      contact_name: "Ines Kraft",
      role: "evaluator",
      engaged: false,
    },
  ],
  our_side: [
    {
      user_id: "01a03000-0000-7000-8000-0000000000c1",
      display_name: "Lena Fischer",
      strength_bucket: "strong",
      interactions_90d: 24,
      last_at: "2026-08-20T09:00:00Z",
    },
  ],
  risks: [],
  sections_omitted: [],
  ...over,
});

const TEAM = [
  {
    key: "u1",
    name: "Lena Fischer",
    jobs: "Owner, Sales owner",
    carries: false,
  },
  { key: "u2", name: "Tomás Reyes", jobs: "Delivery owner", carries: false },
];

function draw(
  props: Omit<Parameters<typeof CommitteeReading>[0], "roleLabel" | "team"> & {
    team?: Parameters<typeof CommitteeReading>[0]["team"];
  },
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrap = (node: ReactNode) => (
    <QueryClientProvider client={client}>
      <LocaleProvider>{node}</LocaleProvider>
    </QueryClientProvider>
  );
  return render(
    wrap(
      <CommitteeReading team={TEAM} {...props} roleLabel={(role) => role} />,
    ),
  );
}

const side = (name: string) => screen.getByRole("region", { name });

afterEach(cleanup);

describe("the buying committee, drawn", () => {
  it("names each colleague with their jobs under our team", () => {
    draw({ coverage: coverage(), withheld: false, pending: false });
    const ours = within(side(en["deal.committee.ourTeam"]));
    expect(ours.getByText("Lena Fischer")).toBeTruthy();
    expect(ours.getByText("Owner, Sales owner")).toBeTruthy();
    expect(ours.getByText("Delivery owner")).toBeTruthy();
  });

  it("names each seat with its role and engagement under their team", () => {
    draw({ coverage: coverage(), withheld: false, pending: false });
    const theirs = within(side(en["deal.committee.theirTeam"]));
    expect(theirs.getByText("Dana Weiss")).toBeTruthy();
    expect(
      theirs.getByText(
        translate("en", "deal.committee.seatSub", {
          role: "champion",
          engagement: en["deal.committee.legendEngaged"],
        }),
      ),
    ).toBeTruthy();
    expect(
      theirs.getByText(
        translate("en", "deal.committee.seatSub", {
          role: "evaluator",
          engagement: en["deal.committee.legendQuiet"],
        }),
      ),
    ).toBeTruthy();
  });

  it("says when a seat last wrote, through the reader that knows a day from an instant", () => {
    draw({
      coverage: coverage({
        stakeholders: [
          {
            contact_id: "01a03000-0000-7000-8000-0000000000b1",
            contact_name: "Dana Weiss",
            role: "",
            engaged: false,
            last_inbound_at: "2026-05-20T12:00:00Z",
            last_inbound_precision: "day",
          },
        ],
      }),
      withheld: false,
      pending: false,
      wroteOn: (at, precision) => `${precision} ${at.slice(0, 10)}`,
    });
    expect(
      screen.getByText(
        translate("en", "deal.committee.seatWrote", {
          role: en["deal.committee.noRole"],
          engagement: en["deal.committee.legendQuiet"],
          date: "day 2026-05-20",
        }),
      ),
    ).toBeTruthy();
  });

  it("names the seat a gap misses and offers the verb for it", () => {
    const offered: string[] = [];
    draw({
      coverage: coverage({
        risks: [
          { kind: "coverage_gap", summary: "no engaged champion" },
          { kind: "going_cold", summary: "quiet", days_since_touch: 30 },
        ],
      }),
      withheld: false,
      pending: false,
      addSeat: (gap, describedBy) => {
        offered.push(gap.role ?? "");
        return (
          <button type="button" aria-describedby={describedBy}>
            Add
          </button>
        );
      },
    });
    const gap = screen.getByText(en["deal.committee.missing.champion"]);
    expect(offered).toEqual(["champion"]);
    expect(
      screen
        .getByRole("button", { name: "Add" })
        .getAttribute("aria-describedby"),
    ).toBe(gap.id);
  });

  it("says a single contact is one contact, and a second engaged one is what is missing past that", () => {
    const thin: DealCoverage["risks"][number] = {
      kind: "single_threaded_theirs",
      summary: "thin",
    };
    draw({
      coverage: coverage({
        stakeholders: [coverage().stakeholders[0]],
        risks: [thin],
      }),
      withheld: false,
      pending: false,
    });
    expect(
      screen.getByText(en["deal.committee.missing.secondContact"]),
    ).toBeTruthy();
    cleanup();
    draw({
      coverage: coverage({ risks: [thin] }),
      withheld: false,
      pending: false,
    });
    expect(
      screen.getByText(en["deal.committee.missing.secondEngaged"]),
    ).toBeTruthy();
  });

  it("names in the legend every mark the card draws, our side included", () => {
    draw({
      coverage: coverage({
        risks: [{ kind: "coverage_gap", summary: "no engaged champion" }],
      }),
      withheld: false,
      pending: false,
    });
    const legend = screen.getByRole("list", {
      name: en["deal.committee.legend"],
    });
    const drawn = new Set(
      [...document.querySelectorAll(".dc-sides .dc-dot")].map((dot) =>
        dot.className.replace("dc-dot ", ""),
      ),
    );
    expect(drawn).toEqual(
      new Set(["dc-dot-ours", "dc-dot-engaged", "dc-dot-quiet", "dc-dot-gap"]),
    );
    expect(
      within(legend)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual([
      en["deal.committee.ourTeam"],
      en["deal.committee.legendEngaged"],
      en["deal.committee.legendQuiet"],
      en["deal.committee.legendGap"],
    ]);
  });

  it("leaves a mark the card does not draw out of the legend", () => {
    draw({ coverage: coverage(), withheld: false, pending: false });
    const legend = screen.getByRole("list", {
      name: en["deal.committee.legend"],
    });
    expect(
      within(legend).queryByText(en["deal.committee.legendGap"]),
    ).toBeNull();
  });

  it("marks the colleague who holds most of the contact", () => {
    draw({
      coverage: coverage(),
      withheld: false,
      pending: false,
      team: [{ ...TEAM[0], carries: true }, TEAM[1]],
    });
    const badge = screen.getByText(en["deal.committee.carriesContact"]);
    expect(badge.closest(".dc-row")?.textContent).toContain("Lena Fischer");
  });

  it("says nobody is assigned rather than drawing an empty side", () => {
    draw({ coverage: coverage(), withheld: false, pending: false, team: [] });
    const ours = within(side(en["deal.committee.ourTeam"]));
    expect(ours.getByText(en["assignments.empty"])).toBeTruthy();
  });

  // Each no-seat state asserts its own sentence, not merely the absence of
  // seats. Absence is what they share, so a withheld lane that regressed into
  // an empty one would stay green on an absence-only check.
  it("says the lane is withheld rather than showing it as empty", () => {
    draw({ coverage: undefined, withheld: true, pending: false });
    expect(screen.getByText("Hidden for your role")).toBeTruthy();
    expect(screen.queryByText("Nobody named")).toBeNull();
  });

  it("says the read is still loading rather than showing it as empty", () => {
    draw({ coverage: undefined, withheld: false, pending: true });
    expect(
      document.querySelector('[role="status"][aria-busy="true"]'),
    ).toBeTruthy();
    expect(screen.queryByText("Nobody named")).toBeNull();
  });

  it("says nobody is named when the read answers with no seat and no gap", () => {
    draw({
      coverage: coverage({ stakeholders: [] }),
      withheld: false,
      pending: false,
    });
    expect(screen.getByText("Nobody named")).toBeTruthy();
    expect(screen.getByText("Lena Fischer")).toBeTruthy();
  });
});
