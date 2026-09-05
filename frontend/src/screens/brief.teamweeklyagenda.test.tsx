/** @vitest-environment jsdom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { TeamWeeklySection } from "./brief.teamweekly";
import { agendaRows, agendaText } from "./brief.teamweeklyagenda";
import { jsonResponse, render, stubApi } from "./home.testkit";
import type { TeamWeeklyRep, TeamWeeklyReview } from "./teamweekly.queries";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function rep(over: Partial<TeamWeeklyRep> = {}): TeamWeeklyRep {
  return {
    user_id: "u1",
    display_name: "Lena Fischer",
    deals_won: 0,
    leads_breached: 0,
    meetings_held: 0,
    commitments_due: 0,
    commitments_kept: 0,
    help_requested: 0,
    focus_kind: "quiet_week",
    focus_label: "A quiet week",
    ...over,
  } as TeamWeeklyRep;
}

const lena = rep();
const noah = rep({
  user_id: "u2",
  display_name: "Noah Berger",
  focus_kind: "help_requested",
  focus_label: "Asked for help on 2 commitments",
});

/**
 * A snapshot whose agenda DISAGREES with the order its reps arrived in.
 *
 * That disagreement is the whole point: the server sends reps by name and the
 * agenda by rank, so a fixture where the two agree would pass against a screen
 * that ignored the agenda entirely.
 */
function review(over: Partial<TeamWeeklyReview> = {}): TeamWeeklyReview {
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
      deals_won: 0,
      deals_lost: 0,
      deals_moved: 0,
      leads_routed: 0,
      leads_answered_in_target: 0,
      leads_breached: 0,
      meetings_held: 0,
      meetings_with_next_step: 0,
      commitments_due: 0,
      commitments_kept: 0,
    },
    reps: [lena, noah],
    agenda: ["u2", "u1"],
    ...over,
  } as TeamWeeklyReview;
}

describe("the agenda is the server's order, not the page's", () => {
  it("takes the reps in agenda order rather than the order they arrived", () => {
    const names = agendaRows(review()).map((r) => r.display_name);

    expect(names).toEqual(["Noah Berger", "Lena Fischer"]);
  });

  // The contract says the agenda names every rep exactly once and the server
  // holds it. A client that trusted it anyway would draw a nameless row against
  // an older server, and an anonymous item is worse than a missing one.
  it("drops an id the snapshot carries no rep for", () => {
    const rows = agendaRows(review({ agenda: ["u2", "ghost", "u1"] }));

    expect(rows.map((r) => r.display_name)).toEqual([
      "Noah Berger",
      "Lena Fischer",
    ]);
  });

  it("has no items for a team whose week nobody could read", () => {
    expect(agendaRows(review({ reps: [], agenda: [] }))).toHaveLength(0);
  });
});

describe("what Copy puts on the clipboard", () => {
  // The point of the action in a Monday meeting is that what was copied is what
  // was read, so the text is composed from the rows on screen.
  it("numbers the items in agenda order, with the server's own labels", () => {
    const text = agendaText(agendaRows(review()), "Monday agenda");

    expect(text).toBe(
      [
        "Monday agenda",
        "1. Noah Berger — Asked for help on 2 commitments",
        "2. Lena Fischer — A quiet week",
      ].join("\n"),
    );
  });

  it("is the heading alone when there is nothing to raise", () => {
    expect(agendaText([], "Monday agenda")).toBe("Monday agenda");
  });
});

describe("the agenda on the screen", () => {
  it("draws the items in order and says what opens the meeting", async () => {
    stubApi({ "GET /weekly-reviews/team": () => jsonResponse(review()) });
    const { container } = render(<TeamWeeklySection teamId="t1" />);

    await screen.findByText("Asked for help on 2 commitments");
    const names = [
      ...container.querySelectorAll(".teamweekly-agenda-name"),
    ].map((node) => node.textContent);
    expect(names).toEqual(["Noah Berger", "Lena Fischer"]);
    // The header summary and the list are one derivation, so the summary names
    // the rep the first row does.
    const summary = container.querySelector(
      '[data-testid="teamweekly-agenda-summary"]',
    );
    expect(summary?.textContent).toContain("Noah Berger");
    expect(summary?.textContent).toContain("2");
  });

  it("says the week could not be read rather than drawing an empty meeting", async () => {
    stubApi({
      "GET /weekly-reviews/team": () =>
        jsonResponse(review({ reps: [], agenda: [] })),
    });
    render(<TeamWeeklySection teamId="t1" />);

    expect(await screen.findByText(en["teamweekly.agenda.empty"])).toBeTruthy();
    // No copy control over an agenda with nothing on it.
    expect(screen.queryByText(en["teamweekly.agenda.copy"])).toBeNull();
  });

  it("copies exactly what is on screen", async () => {
    const written: string[] = [];
    vi.stubGlobal("navigator", {
      clipboard: {
        writeText: (text: string) => {
          written.push(text);
          return Promise.resolve();
        },
      },
    });
    stubApi({ "GET /weekly-reviews/team": () => jsonResponse(review()) });
    render(<TeamWeeklySection teamId="t1" />);

    const copy = await screen.findByText(en["teamweekly.agenda.copy"]);
    copy.click();

    await waitFor(() =>
      expect(written).toEqual([
        [
          en["teamweekly.agenda.title"],
          "1. Noah Berger — Asked for help on 2 commitments",
          "2. Lena Fischer — A quiet week",
        ].join("\n"),
      ]),
    );
    await screen.findByText(en["teamweekly.agenda.copied"]);
  });

  // navigator.clipboard is UNDEFINED outside a secure context, so this is a
  // missing capability rather than a rejected promise — and a lead who pressed
  // Copy and got nothing has no way to know the agenda is still on screen to
  // select by hand.
  it("says so when the browser will not hand over a clipboard", async () => {
    vi.stubGlobal("navigator", {});
    stubApi({ "GET /weekly-reviews/team": () => jsonResponse(review()) });
    render(<TeamWeeklySection teamId="t1" />);

    const copy = await screen.findByText(en["teamweekly.agenda.copy"]);
    copy.click();

    expect(
      await screen.findByText(en["teamweekly.agenda.copyFailed"]),
    ).toBeTruthy();
  });
});
