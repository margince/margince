/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import {
  headlineReadings,
  TeamWeeklyPanel,
  TeamWeeklySection,
} from "./brief.teamweekly";
import { jsonResponse, render, stubApi } from "./home.testkit";
import type { TeamWeeklyRep, TeamWeeklyReview } from "./teamweekly.queries";

// A team's frozen week. Every figure came off the snapshot, so the tests that
// matter are the ones proving the page cannot say more than the snapshot holds.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function rep(over: Partial<TeamWeeklyRep> = {}): TeamWeeklyRep {
  return {
    user_id: "u1",
    display_name: "Lena Fischer",
    deals_won: 1,
    leads_breached: 0,
    meetings_held: 3,
    commitments_due: 2,
    commitments_kept: 2,
    help_requested: 0,
    focus_kind: "strong_week",
    focus_label: "Fastest first response on the team",
    ...over,
  } as TeamWeeklyRep;
}

function review(
  counts: Partial<TeamWeeklyReview["counts"]> = {},
  over: Partial<TeamWeeklyReview> = {},
): TeamWeeklyReview {
  return {
    id: "r1",
    team_id: "t1",
    team_name: "Nord",
    local_week_start: "2026-06-01",
    generated_at: "2026-06-08T06:00:00Z",
    as_of: "2026-06-08T06:00:00Z",
    reps_unread: 0,
    counts: {
      reps_counted: 2,
      deals_won: 3,
      deals_lost: 1,
      deals_moved: 5,
      leads_routed: 10,
      leads_answered_in_target: 10,
      leads_breached: 0,
      meetings_held: 10,
      meetings_with_next_step: 5,
      commitments_due: 4,
      commitments_kept: 3,
      ...counts,
    },
    reps: [rep()],
    ...over,
  } as TeamWeeklyReview;
}

describe("the headline states the bar it measured against", () => {
  // A verdict that does not name its bar is an opinion. Both clauses come off
  // the stored counts, so the sentence cannot disagree with the figures below.
  it("picks the healthiest reading and the weakest", () => {
    const { best, worst } = headlineReadings(review());

    expect(best?.key).toBe("teamweekly.reading.firstResponse");
    expect(worst?.key).toBe("teamweekly.reading.nextStep");
  });

  // Zero of zero is not zero per cent. A team that routed no leads has no
  // first-response reading, and inventing one at 0% would report a failure
  // where nothing was attempted.
  it("has no reading where nothing was due", () => {
    const { best, worst } = headlineReadings(
      review({
        leads_routed: 0,
        leads_answered_in_target: 0,
        meetings_held: 0,
        meetings_with_next_step: 0,
        commitments_due: 0,
        commitments_kept: 0,
      }),
    );

    expect(best).toBeNull();
    expect(worst).toBeNull();
  });

  // Nothing stood out. Saying so is the honest answer; manufacturing a verdict
  // from a middling number is not.
  it("says the plain thing when no reading is decided either way", async () => {
    stubApi({
      "GET /weekly-reviews/team": () =>
        jsonResponse(
          review({
            leads_routed: 10,
            leads_answered_in_target: 8,
            meetings_held: 10,
            meetings_with_next_step: 8,
            commitments_due: 10,
            commitments_kept: 8,
          }),
        ),
    });
    render(<TeamWeeklySection teamId="t1" />);

    expect(
      await screen.findByText(en["teamweekly.headline.plain"]),
    ).toBeTruthy();
  });
});

describe("the week's movement counts what advanced", () => {
  // A team that moved eleven deals and closed none read as a team where nothing
  // happened. Every member's weekly has counted deals_moved since the review
  // shipped; the team total dropped it on the way up, so the panel could not
  // draw the row at all.
  //
  // Asserted through the METER ROLE rather than by text: the bar carries its
  // label as `aria-label`, which is how a screen reader gets it and the only
  // place it exists. A getByText here finds nothing however right the code is.
  it("draws the advanced row beside won and lost", async () => {
    stubApi({
      "GET /weekly-reviews/team": () =>
        jsonResponse(review({ deals_won: 0, deals_lost: 0, deals_moved: 11 })),
    });
    render(<TeamWeeklySection teamId="t1" />);

    await screen.findByText(en["teamweekly.movement.title"]);
    const advanced = screen.getByRole("meter", {
      name: en["teamweekly.movement.moved"],
    });
    expect(advanced.getAttribute("aria-valuenow")).toBe("11");
  });

  // ONE BASELINE for every bar, and the advanced count must be part of what
  // sets it. A week whose largest fact is what moved would otherwise draw that
  // row against a scale taken from the closed deals alone — the biggest number
  // on the panel rendering as the shortest bar.
  it("lets the advanced count set the scale when it is the week's largest", async () => {
    stubApi({
      "GET /weekly-reviews/team": () =>
        jsonResponse(review({ deals_won: 2, deals_lost: 1, deals_moved: 20 })),
    });
    render(<TeamWeeklySection teamId="t1" />);
    await screen.findByText(en["teamweekly.movement.title"]);

    const advanced = screen.getByRole("meter", {
      name: en["teamweekly.movement.moved"],
    });
    const won = screen.getByRole("meter", {
      name: en["teamweekly.movement.won"],
    });
    // Every bar shares the maximum, and it is the advanced figure.
    expect(advanced.getAttribute("aria-valuemax")).toBe("20");
    expect(won.getAttribute("aria-valuemax")).toBe("20");
  });

  // A snapshot written before the column existed carries zero, and zero is a
  // COUNT rather than an absence — the row still draws, saying the team
  // advanced nothing that week.
  it("draws the row at zero rather than dropping it", async () => {
    stubApi({
      "GET /weekly-reviews/team": () =>
        jsonResponse(review({ deals_moved: 0 })),
    });
    render(<TeamWeeklySection teamId="t1" />);
    await screen.findByText(en["teamweekly.movement.title"]);

    expect(
      screen
        .getByRole("meter", { name: en["teamweekly.movement.moved"] })
        .getAttribute("aria-valuenow"),
    ).toBe("0");
  });
});

describe("the scorecard says what the wins were worth", () => {
  // The count alone says a week of five small renewals and a week of one
  // company-making deal are the same week. The money was computed, converted
  // and stored when the snapshot was written — and read by nothing until now.
  it("prices the wins beside the losses when the snapshot recorded the money", async () => {
    stubApi({
      "GET /weekly-reviews/team": () =>
        jsonResponse(
          review({}, {
            pipeline: {
              created_minor: 9000000,
              won_minor: 2500000,
              lost_minor: 0,
              currency: "EUR",
            },
          } as Partial<TeamWeeklyReview>),
        ),
    });
    render(<TeamWeeklySection teamId="t1" />);

    // The review's OWN currency, not the installation's current setting: base
    // currency is operator-mutable, and re-reading it would re-label a closed
    // week with a currency its numbers were never in.
    expect(await screen.findByText(/25.000,00\s*€|€25,000\.00/)).toBeTruthy();
    // The lost count survives the money arriving: it is a different fact, not a
    // delta the value replaces.
    expect(screen.getByText(/1 lost|1 verloren/)).toBeTruthy();
  });

  // The block is ABSENT whenever any member's week could not be converted, and
  // summing only the reps who did convert would be a confident number quietly
  // missing one. A zero here would claim a team that won three deals won
  // nothing.
  it("draws no money at all over a week that could not be converted", async () => {
    stubApi({ "GET /weekly-reviews/team": () => jsonResponse(review()) });
    render(<TeamWeeklySection teamId="t1" />);

    expect(
      await screen.findByText(
        en["teamweekly.card.wonBasis"].replace("{lost}", "1"),
      ),
    ).toBeTruthy();
    expect(screen.queryByText(/€|EUR/)).toBeNull();
  });
});

describe("the team's frozen week", () => {
  // A snapshot covering four of six reps reads exactly like a team of four, and
  // every figure on the page is short by the same two people.
  it("states unread members rather than letting the totals imply full coverage", async () => {
    stubApi({
      "GET /weekly-reviews/team": () =>
        jsonResponse(review({}, { reps_unread: 2 })),
    });
    render(<TeamWeeklySection teamId="t1" />);

    expect(
      await screen.findByText(
        en["teamweekly.repsUnread"]
          .replace("{count}", "2")
          .replace("{counted}", "2"),
      ),
    ).toBeTruthy();
  });

  it("says nothing about coverage when every member was read", async () => {
    stubApi({
      "GET /weekly-reviews/team": () => jsonResponse(review()),
    });
    const { container } = render(<TeamWeeklySection teamId="t1" />);

    await screen.findByText(en["teamweekly.movement.title"]);
    expect(container.querySelector(".teamweekly-coverage")).toBeNull();
  });

  // The two absences are different facts. A screen drawing one plate over both
  // would tell a lead they lack permission on a Tuesday in their team's first
  // week.
  it("tells a refusal apart from a week that has not closed yet", async () => {
    stubApi({
      "GET /weekly-reviews/team": () =>
        jsonResponse({ title: "Forbidden" }, 403),
    });
    render(<TeamWeeklySection teamId="t1" />);

    expect(await screen.findByText(en["teamweekly.forbidden"])).toBeTruthy();
    expect(screen.queryByText(en["teamweekly.noSnapshot"])).toBeNull();
  });

  it("says no week has closed when there is no snapshot", async () => {
    stubApi({
      "GET /weekly-reviews/team": () =>
        jsonResponse({ title: "Not Found" }, 404),
    });
    render(<TeamWeeklySection teamId="t1" />);

    expect(await screen.findByText(en["teamweekly.noSnapshot"])).toBeTruthy();
  });

  // One row per member, including the member whose week went well. A page
  // promising one focus per rep and drawing rows only for the troubled ones
  // reads as a team where only those people exist.
  it("draws a row for every member, the good week included", async () => {
    stubApi({
      "GET /weekly-reviews/team": () =>
        jsonResponse(
          review(
            {},
            {
              reps: [
                rep(),
                rep({
                  user_id: "u2",
                  display_name: "Noah Berger",
                  focus_kind: "leads_breached",
                  focus_label: "Three leads went unanswered",
                }),
              ],
            },
          ),
        ),
    });
    const { container } = render(<TeamWeeklySection teamId="t1" />);

    await screen.findByText("Three leads went unanswered");
    expect(container.querySelectorAll(".teamweekly-rep")).toHaveLength(2);
    // The good week is marked as something to copy, not as a problem.
    expect(screen.getByText(en["teamweekly.focus.strong_week"])).toBeTruthy();
    expect(screen.getByText("Fastest first response on the team")).toBeTruthy();
  });

  it("keeps the scorecard at five slots", async () => {
    stubApi({
      "GET /weekly-reviews/team": () => jsonResponse(review()),
    });
    const { container } = render(<TeamWeeklySection teamId="t1" />);

    await screen.findByText(en["teamweekly.movement.title"]);
    const strip = container.querySelector('[data-testid="teamweekly-strip"]');
    expect(strip?.children).toHaveLength(5);
  });
});

describe("the team picker", () => {
  // Offered on the same tier the team board is. A picker shown to a reader who
  // will be refused every team is a control that exists to fail.
  it("draws nothing at all for a reader whose scope reaches no team", () => {
    const calls = stubApi({});
    const { container } = render(<TeamWeeklyPanel offered={false} />);

    expect(container.firstChild).toBeNull();
    expect(calls.filter((call) => call.path === "/teams")).toHaveLength(0);
  });

  // One team is not a choice: a control whose only option is the one already
  // showing asks the reader to confirm what they cannot change.
  it("reads a single team straight through without a control", async () => {
    stubApi({
      "GET /teams": () =>
        jsonResponse({
          data: [{ id: "t1", name: "Nord" }],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /weekly-reviews/team": () => jsonResponse(review()),
    });
    render(<TeamWeeklyPanel offered />);

    await screen.findByText(en["teamweekly.movement.title"]);
    expect(screen.queryByLabelText(en["teamweekly.pickTeam"])).toBeNull();
  });
});
