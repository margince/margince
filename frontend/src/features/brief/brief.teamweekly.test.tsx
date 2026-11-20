/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { en } from "../../i18n/en";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { meFixture, READS_RECORDS } from "../../testing/mefixture";
import { BriefScreen } from "./brief";
import { agendaRows } from "./brief.teamweeklyagenda";
import type { TeamWeeklyReview, TeamWeekReach } from "./teamweekly.queries";

// The team week on Home, Weekly: who is offered the choice, and what each
// answer of GET /weekly-reviews/team draws.

const NORD = "00000000-0000-4000-8000-0000000000a1";
const SUED = "00000000-0000-4000-8000-0000000000a2";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

beforeEach(() => {
  globalThis.location.hash = "#/home?view=weekly";
});

function review(over: Partial<TeamWeeklyReview> = {}): TeamWeeklyReview {
  return {
    id: "00000000-0000-4000-8000-0000000000f1",
    team_id: NORD,
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
      leads_answered_in_target: 9,
      leads_breached: 1,
      meetings_held: 10,
      meetings_with_next_step: 5,
      commitments_due: 4,
      commitments_kept: 3,
    },
    reps: [
      {
        user_id: "u-lena",
        display_name: "Lena Fischer",
        deals_won: 2,
        leads_breached: 0,
        meetings_held: 6,
        commitments_due: 2,
        commitments_kept: 2,
        help_requested: 0,
        focus_kind: "strong_week",
        focus_label: "Fastest first response on the team",
        patterns: [],
      },
      {
        user_id: "u-tobias",
        display_name: "Tobias Kern",
        deals_won: 1,
        leads_breached: 1,
        meetings_held: 4,
        commitments_due: 2,
        commitments_kept: 1,
        help_requested: 1,
        focus_kind: "help_requested",
        focus_label: "Asked for help on the Hansa renewal",
        patterns: [],
      },
    ],
    agenda: ["u-tobias", "u-lena"],
    ...over,
  };
}

type TeamAnswer = (body: unknown, query: URLSearchParams) => Response;

/** Stub the Weekly page for a reader with this reach, on these teams. */
function stubWeekly(
  reach: TeamWeekReach,
  memberOf: readonly string[],
  team: TeamAnswer = () => jsonResponse(review()),
) {
  return stubApi({
    "GET /me": () =>
      jsonResponse({
        ...meFixture({ allow: READS_RECORDS }),
        teams: memberOf,
        team_week: reach,
      }),
    "GET /teams": () =>
      jsonResponse({
        data: [
          { id: NORD, name: "Nord" },
          { id: SUED, name: "Süd" },
        ],
        page: { next_cursor: null, has_more: false },
      }),
    "GET /weekly-reviews/latest": () =>
      jsonResponse({ title: "Not Found" }, 404),
    "GET /weekly-plans/current": () =>
      jsonResponse({ title: "Not Found" }, 404),
    "GET /weekly-reviews/team": team,
  });
}

const teamReads = (calls: ReturnType<typeof stubApi>) =>
  calls.filter((call) => call.path === "/weekly-reviews/team");

const whoseWeek = () =>
  screen.findByRole("combobox", { name: en["teamweekly.whose"] });

describe("the team choice on Weekly", () => {
  it("offers a lead their own teams and opens the chosen team's week", async () => {
    const calls = stubWeekly("teams_led", [NORD]);
    const user = userEvent.setup();
    render(<BriefScreen />);

    await pickOption(user, await whoseWeek(), "Nord");

    await waitFor(() =>
      expect(globalThis.location.hash).toContain(`team=${NORD}`),
    );
    expect(
      await screen.findByText(
        en["teamweekly.outcomes"]
          .replace("{won}", "3")
          .replace("{lost}", "1")
          .replace("{moved}", "5")
          .replace("{leads}", "10"),
      ),
    ).toBeTruthy();
    expect(screen.getByText(en["teamweekly.agenda.title"])).toBeTruthy();
    expect(teamReads(calls)[0]?.path).toBe("/weekly-reviews/team");
  });

  it("lists only the teams a lead is on, and every team for an overseer", async () => {
    stubWeekly("teams_led", [NORD]);
    const user = userEvent.setup();
    render(<BriefScreen />);
    await user.click(await whoseWeek());
    expect(screen.getByRole("option", { name: "Nord" })).toBeTruthy();
    expect(screen.queryByRole("option", { name: "Süd" })).toBeNull();
    cleanup();

    stubWeekly("every_team", []);
    render(<BriefScreen />);
    await user.click(await whoseWeek());
    expect(screen.getByRole("option", { name: "Nord" })).toBeTruthy();
    expect(screen.getByRole("option", { name: "Süd" })).toBeTruthy();
  });

  it("offers a rep no choice, and opens a lead's team link as the rep's own week", async () => {
    globalThis.location.hash = `#/home?view=weekly&team=${NORD}`;
    const calls = stubWeekly("none", [NORD]);
    const { client } = render(<BriefScreen />);

    await waitFor(() =>
      expect(document.querySelector("#brief-weekly")).not.toBeNull(),
    );
    await waitFor(() =>
      expect(client.getQueryState(["teams", "all"])?.status).toBe("success"),
    );
    expect(
      screen.queryByRole("combobox", { name: en["teamweekly.whose"] }),
    ).toBeNull();
    expect(teamReads(calls)).toHaveLength(0);
  });

  it("offers a lead on no team no choice", async () => {
    stubWeekly("teams_led", []);
    const { client } = render(<BriefScreen />);

    // The choice waits on the team list, so its absence means something only
    // once that list has answered.
    await waitFor(() =>
      expect(client.getQueryState(["teams", "all"])?.status).toBe("success"),
    );
    expect(
      screen.queryByRole("combobox", { name: en["teamweekly.whose"] }),
    ).toBeNull();
  });
});

describe("the team week's states", () => {
  beforeEach(() => {
    globalThis.location.hash = `#/home?view=weekly&team=${NORD}`;
  });

  it("says a week nobody was counted in is unmeasured, and draws no agenda", async () => {
    stubWeekly("teams_led", [NORD], () =>
      jsonResponse(
        review({
          counts: { ...review().counts, reps_counted: 0 },
          reps: [],
          agenda: [],
        }),
      ),
    );
    render(<BriefScreen />);

    expect(
      await screen.findByText(en["teamweekly.headline.unmeasured"]),
    ).toBeTruthy();
    expect(screen.queryByText(en["teamweekly.agenda.title"])).toBeNull();
  });

  it("names the members whose week could not be read", async () => {
    stubWeekly("teams_led", [NORD], () =>
      jsonResponse(review({ reps_unread: 1 })),
    );
    render(<BriefScreen />);

    expect(
      await screen.findByText(
        en["teamweekly.repsUnread_one"]
          .replace("{count}", "1")
          .replace("{counted}", "2"),
      ),
    ).toBeTruthy();
  });

  it("says a team has no closed week yet on a 404", async () => {
    stubWeekly("teams_led", [NORD], () =>
      jsonResponse({ title: "Not Found" }, 404),
    );
    render(<BriefScreen />);

    expect(await screen.findByText(en["teamweekly.noSnapshot"])).toBeTruthy();
  });

  it("says the reader can no longer open the team on a 403", async () => {
    stubWeekly("teams_led", [NORD], () =>
      jsonResponse({ title: "Forbidden" }, 403),
    );
    render(<BriefScreen />);

    expect(await screen.findByText(en["teamweekly.forbidden"])).toBeTruthy();
  });

  it("asks for the team and the week in the address", async () => {
    globalThis.location.hash = `#/home?view=weekly&team=${NORD}&week=2026-06-01`;
    const asked: string[] = [];
    stubWeekly("teams_led", [NORD], (_body, query) => {
      asked.push(query.toString());
      return jsonResponse(review());
    });
    render(<BriefScreen />);

    await screen.findByText(en["teamweekly.agenda.title"]);
    expect(asked).toContain(`team=${NORD}&week=2026-06-01`);
  });
});

describe("the Monday agenda", () => {
  it("follows the server's order and drops an id no member carries", () => {
    const rows = agendaRows(
      review({ agenda: ["u-tobias", "u-gone", "u-lena"] }),
    );
    expect(rows.map((rep) => rep.display_name)).toEqual([
      "Tobias Kern",
      "Lena Fischer",
    ]);
  });
});
