/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { forgetSeat } from "../../lib/storage";
import { jsonResponse } from "../../testing/story-utils";
import {
  metric,
  openOverview as open,
  readings,
  SELF,
  TEAM,
  teamEvaluation,
  teamHandoffs,
  teamReview,
} from "./analytics.overview.testkit";

const originalFetch = globalThis.fetch;
beforeEach(() => forgetSeat());
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

const TEAM_ADDRESS = "#/analytics/overview?overview=team";
const served = (review: ReturnType<typeof teamReview>) => () =>
  jsonResponse(review);

// A qualification team: Riley has a monthly target, Sam has none.
const qualificationReview = (over: Parameters<typeof teamReview>[0] = {}) =>
  teamReview({
    workflow: "qualification",
    handoffs: undefined,
    member_targets: {
      metric: "leads_promoted",
      window: {
        start_at: "2026-08-31T22:00:00Z",
        end_at: "2026-09-30T22:00:00Z",
      },
    },
    members: teamReview().members.map((member, index) => ({
      ...member,
      ...(index === 0 ? { target: { value: 8, actual: 5 } } : {}),
    })),
    ...over,
  });

const qualificationTeam = () => {
  const base = teamEvaluation();
  return {
    ...base,
    metrics: base.metrics.map((reading) =>
      reading.id === "leads_promoted"
        ? metric("leads_promoted", 5, { denominator: 12 })
        : reading,
    ),
  };
};

async function standing() {
  return within(
    await screen.findByRole("region", { name: "Where the team stands" }),
  );
}

function headers(table: HTMLElement) {
  return within(table)
    .getAllByRole("columnheader")
    .map((cell) => cell.textContent);
}

function expectInOrder(elements: HTMLElement[]) {
  for (let index = 1; index < elements.length; index++) {
    expect(
      elements[index - 1].compareDocumentPosition(elements[index]) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  }
}

function rowOf(text: HTMLElement): HTMLElement {
  const row = text.closest<HTMLElement>(".analytics-attention-row");
  if (!row) throw new Error("the text is in no bottleneck row");
  return row;
}

describe("Team prospecting", () => {
  it("is offered to a manager and not to an SDR measuring their own work", async () => {
    open("#/analytics", teamEvaluation);
    expect(
      await screen.findByRole("button", { name: "Team prospecting" }),
    ).toBeVisible();
    cleanup();
    open(TEAM_ADDRESS, teamEvaluation, { seat: "sdr" });
    expect(
      await screen.findByRole("button", { name: "My prospecting" }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(
      screen.queryByRole("button", { name: "Team prospecting" }),
    ).toBeNull();
  });

  it("leads a handoff team with accepted opportunities and lists what holds it up", async () => {
    const asked = open(TEAM_ADDRESS, teamEvaluation, { template: "sdr" });
    await readings();
    // The catalog's own names, in the order the cards stand.
    expect(
      screen.getByRole("region", { name: "Opening readings" }).textContent,
    ).toMatch(/Accepted.*Meetings held.*Leads qualified.*Leads still waiting/);
    const panel = await standing();
    expect(
      panel.getByText(/Of the 6 rounds handed on in this period/),
    ).toBeVisible();
    expect(panel.getByText("2 handoffs wait for a decision")).toBeVisible();
    expect(panel.getByText(/Most often: Wrong territory/)).toBeVisible();
    expect(panel.getByText(/Next 14 days, through/)).toBeVisible();
    const evaluations = asked.filter(({ url }) =>
      url.pathname.endsWith("/analytics/evaluate"),
    );
    expect(evaluations).toHaveLength(1);
  });

  it("shows a rate only over the cohort of rounds, never over two period counts", async () => {
    // Accepted in the period over submitted in it is 3 of 6, which is no rate:
    // the cohort is 2 of the 8 rounds handed on.
    open(TEAM_ADDRESS, teamEvaluation, {
      template: "sdr",
      team: served(
        teamReview({
          handoffs: teamHandoffs({
            cohort: {
              rounds: 8,
              accepted: 2,
              rejected: 1,
              recycled: 1,
              waiting: 4,
            },
          }),
        }),
      ),
    });
    const panel = await standing();
    expect(
      panel.getByText(/25% of these 8 rounds were accepted\./),
    ).toBeVisible();
    expect(panel.queryByText(/50%/)).toBeNull();
    expect(panel.getByText(/4 still waiting/)).toBeVisible();
  });

  it("shows no rate for a cohort below the floor", async () => {
    open(TEAM_ADDRESS, teamEvaluation, {
      template: "sdr",
      team: served(
        teamReview({
          handoffs: teamHandoffs({
            cohort: {
              rounds: 3,
              accepted: 1,
              rejected: 0,
              recycled: 1,
              waiting: 1,
            },
          }),
        }),
      ),
    });
    const panel = await standing();
    expect(panel.getByText(/Fewer than 5 is too few for a rate/)).toBeVisible();
    expect(panel.queryByText(/%/)).toBeNull();
  });

  it("opens the reasons for returns on the period's decisions, adding up to the count", async () => {
    const user = userEvent.setup();
    open(TEAM_ADDRESS, teamEvaluation, {
      template: "sdr",
      team: served(
        teamReview({
          handoffs: teamHandoffs({
            outcomes: { submitted: 6, accepted: 3, rejected: 1, recycled: 3 },
            reasons: [
              { outcome: "recycled", label: "", given: false, count: 2 },
              {
                outcome: "recycled",
                label: "Needs more qualification",
                given: true,
                count: 1,
              },
              {
                outcome: "rejected",
                label: "Wrong territory",
                given: true,
                count: 1,
              },
            ],
          }),
        }),
      ),
    });
    const panel = await standing();
    const row = rowOf(
      panel.getByText("3 handoffs were sent back in this period"),
    );
    await user.click(within(row).getByRole("button", { name: "Show reasons" }));
    const drawer = await screen.findByRole("dialog", {
      name: "Why handoffs were sent back",
    });
    expect(
      within(drawer).getByText(/whenever the handoff was handed on: 3 in all/),
    ).toBeVisible();
    const table = within(drawer).getByRole("table", { name: "Reasons given" });
    const cells = within(table)
      .getAllByRole("row")
      .slice(1)
      .map((line) => line.textContent);
    expect(cells).toEqual(["No reason recorded2", "Needs more qualification1"]);
    expect(within(drawer).queryByText("Wrong territory")).toBeNull();
  });

  it("opens the leads still owed a reply as the reading's own evidence", async () => {
    const user = userEvent.setup();
    const asked = open(TEAM_ADDRESS, teamEvaluation, { template: "sdr" });
    const panel = await standing();
    await user.click(panel.getByRole("button", { name: "Show leads" }));
    expect(
      asked.some(
        ({ url }) =>
          url.pathname.endsWith("/analytics/evidence") &&
          url.searchParams.get("metric") === "leads_awaiting_response",
      ),
    ).toBe(true);
  });

  it("gives a handoff team its handoff columns, in name order, with what each counts", async () => {
    const review = teamReview();
    open(TEAM_ADDRESS, teamEvaluation, {
      template: "sdr",
      team: served({ ...review, members: [...review.members].reverse() }),
    });
    const members = await screen.findByRole("table", { name: "Members" });
    expect(headers(members)).toEqual([
      "Member",
      "Meetings held",
      "Submitted",
      "Accepted",
      "Returned",
      "Rejected",
      "Waiting now",
    ]);
    const rows = within(members).getAllByRole("row");
    expect(within(rows[1]).getByText("Riley Rep")).toBeVisible();
    expect(within(rows[2]).getByText("Sam SDR")).toBeVisible();
    expect(screen.getByText(/Accepted counts each deal once/)).toBeVisible();
    expect(screen.getByText(/not a ranking/)).toBeVisible();
  });

  it("gives a qualification team qualified leads first, and each member's own monthly target", async () => {
    open(TEAM_ADDRESS, qualificationTeam, {
      template: "sales",
      team: served(qualificationReview()),
    });
    const strip = await readings();
    expectInOrder([
      strip.getByText("Leads qualified"),
      strip.getByText("Meetings held"),
    ]);
    const panel = await standing();
    expect(
      panel.getByText(
        /5 of the 12 leads that arrived in this period are qualified so far\..*That is 42% of the leads that arrived\./,
      ),
    ).toBeVisible();
    const members = await screen.findByRole("table", { name: "Members" });
    expect(headers(members)).toEqual([
      "Member",
      "Qualified",
      "Meetings held",
      "Unanswered",
      "Monthly target",
    ]);
    const rows = within(members).getAllByRole("row");
    expect(within(rows[1]).getByText("5 of 8")).toBeVisible();
    expect(within(rows[2]).getByText("No target")).toBeVisible();
    expect(screen.getByText(/Targets are monthly\./)).toBeVisible();
    expect(screen.queryByText("Submitted")).toBeNull();
  });

  it("says which figures are hidden from the reader, and never shows them as zero", async () => {
    const review = qualificationReview({
      withheld_figures: ["meetings", "leads", "targets"],
      member_targets: undefined,
      members: teamReview().members.map(
        ({ meetings_held: _m, qualified: _q, unanswered: _u, ...row }) => row,
      ),
    });
    open(TEAM_ADDRESS, qualificationTeam, {
      template: "sales",
      team: served(review),
    });
    const members = await screen.findByRole("table", { name: "Members" });
    expect(headers(members)).toEqual(["Member"]);
    expect(screen.getByText("Meetings are hidden from you.")).toBeVisible();
    expect(
      screen.getByText("Qualified and unanswered leads are hidden from you."),
    ).toBeVisible();
  });

  it("says handoffs are hidden from a reader without the grant", async () => {
    const review = teamReview({
      handoffs: undefined,
      withheld_figures: ["handoffs", "leads"],
      members: teamReview().members.map(
        ({
          submitted: _s,
          accepted: _a,
          rejected: _r,
          recycled: _c,
          waiting: _w,
          ...row
        }) => row,
      ),
    });
    open(TEAM_ADDRESS, teamEvaluation, {
      template: "sdr",
      team: served(review),
    });
    const panel = await standing();
    expect(panel.getByText("Handoffs are hidden from you.")).toBeVisible();
    const members = await screen.findByRole("table", { name: "Members" });
    expect(headers(members)).toEqual(["Member", "Meetings held"]);
    expect(panel.queryByText(/Nothing waits on the team/)).toBeNull();
  });

  describe("the leads still owed a reply keep four states apart", () => {
    // A qualification team with nothing else waiting, so the unanswered row
    // alone decides whether the panel may say nothing waits.
    const quiet = qualificationReview();
    const zero = () => {
      const base = qualificationTeam();
      return {
        ...base,
        metrics: base.metrics.map((reading) =>
          reading.id === "leads_awaiting_response"
            ? metric("leads_awaiting_response", 0)
            : reading,
        ),
      };
    };
    const NOTHING = "Nothing waits on the team in this period.";

    it("says nothing waits only over a measured zero", async () => {
      open(TEAM_ADDRESS, zero, { template: "sales", team: served(quiet) });
      expect((await standing()).getByText(NOTHING)).toBeVisible();
    });

    it("names a figure the review withholds, and claims nothing", async () => {
      open(TEAM_ADDRESS, zero, {
        template: "sales",
        team: served({ ...quiet, withheld_figures: ["unanswered"] }),
      });
      const panel = await standing();
      expect(
        panel.getByText("Leads waiting for a reply are hidden from you."),
      ).toBeVisible();
      expect(panel.queryByText(NOTHING)).toBeNull();
    });

    it("says the readings failed, and claims nothing", async () => {
      open(
        TEAM_ADDRESS,
        () => jsonResponse({ title: "Unavailable", status: 503 }, 503),
        { template: "sales", team: served(quiet) },
      );
      const panel = await standing();
      expect(
        panel.getByText("Leads waiting for a reply could not be read."),
      ).toBeVisible();
      expect(panel.queryByText(NOTHING)).toBeNull();
    });

    it("says the readings are still being read, and claims nothing", async () => {
      open(
        TEAM_ADDRESS,
        zero,
        { template: "sales", team: served(quiet) },
        undefined,
        (url) => url.pathname.endsWith("/analytics/evaluate"),
      );
      const panel = await standing();
      expect(
        panel.getByText("Leads waiting for a reply are still being read."),
      ).toBeVisible();
      expect(panel.queryByText(NOTHING)).toBeNull();
    });
  });

  it("leaves out a figure a field mask reaches, and the target held against it", async () => {
    open(TEAM_ADDRESS, qualificationTeam, {
      template: "sales",
      team: served(
        qualificationReview({
          withheld_figures: ["qualified", "targets"],
          member_targets: undefined,
          members: teamReview().members.map(({ qualified: _q, ...row }) => row),
        }),
      ),
    });
    const members = await screen.findByRole("table", { name: "Members" });
    expect(headers(members)).toEqual(["Member", "Meetings held", "Unanswered"]);
    expect(
      screen.getByText("Qualified leads are hidden from you."),
    ).toBeVisible();
  });

  it("names no member while individual performance is off, and keeps the bottlenecks", async () => {
    open(TEAM_ADDRESS, teamEvaluation, {
      template: "sdr",
      team: served(teamReview({ members_withheld: true, members: [] })),
    });
    expect(
      await screen.findByText(
        "Individual performance is off, so no member’s figures are shown.",
      ),
    ).toBeVisible();
    const panel = await standing();
    expect(panel.getByText("2 handoffs wait for a decision")).toBeVisible();
    expect(screen.queryByRole("table", { name: "Members" })).toBeNull();
    expect(screen.queryByText("Riley Rep")).toBeNull();
    expect(screen.queryByText("Sam SDR")).toBeNull();
  });

  it("asks both reads for the one team the scope names", async () => {
    const asked = open(TEAM_ADDRESS, teamEvaluation, { template: "sdr" });
    await screen.findByRole("table", { name: "Members" });
    const scopes = asked
      .filter(({ url }) =>
        ["/v1/analytics/sdr-team", "/v1/analytics/evaluate"].includes(
          url.pathname,
        ),
      )
      .map(({ url }) => [
        url.searchParams.get("scope_kind"),
        url.searchParams.get("scope_id"),
      ]);
    expect(scopes).toEqual([
      ["team", TEAM.id],
      ["team", TEAM.id],
    ]);
    expect(
      screen.getByText(/These figures follow today’s team\./),
    ).toBeVisible();
  });

  it("reads nothing for one colleague's records and says to choose a team", async () => {
    const asked = open(`${TEAM_ADDRESS}&scope=owner:${SELF}`, teamEvaluation, {
      template: "sdr",
    });
    expect(await screen.findByText("Choose a team")).toBeVisible();
    expect(
      asked.some(({ url }) =>
        ["/v1/analytics/sdr-team", "/v1/analytics/evaluate"].includes(
          url.pathname,
        ),
      ),
    ).toBe(false);
  });

  it("withholds the figures a lone colleague makes up", async () => {
    open(TEAM_ADDRESS, teamEvaluation, {
      template: "sdr",
      team: served(
        teamReview({
          withheld: true,
          handoffs: undefined,
          meetings: undefined,
          members: [],
          members_withheld: true,
        }),
      ),
    });
    expect(
      await screen.findByText(/these figures would show one colleague’s work/),
    ).toBeVisible();
    expect(
      screen.queryByRole("region", { name: "Where the team stands" }),
    ).toBeNull();
  });
});
