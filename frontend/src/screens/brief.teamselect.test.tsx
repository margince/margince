/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { BriefTeamSelect, morningTeams, weekTeams } from "./brief.teamselect";
import { jsonResponse, render, stubApi } from "./brief.testkit";

// Each view's picker lists the teams that view's server rule serves. Morning's
// team board is the team's live work and follows row scope; Weekly's team week
// is a lead's verdict and follows who leads or oversees. One shared list would
// offer a team one of the two refuses.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("which teams each view lists", () => {
  it("lists every team on Morning to a seat wider than team scope", () => {
    expect(morningTeams("all")).toBe("every");
    expect(morningTeams("team")).toBe("mine");
  });

  it("lists every team on Weekly only where the server says it oversees them", () => {
    expect(weekTeams("every_team")).toBe("every");
    expect(weekTeams("teams_led")).toBe("mine");
  });
});

// A manager whose row scope an operator widened to `all` and who holds no
// oversight: Morning serves every team's board, Weekly only the teams they lead.
describe("one seat, two views", () => {
  const widenedManager = () => ({
    ...meFixture({ rowScope: "all" }),
    teams: ["t1"],
  });

  function stubTeams() {
    stubApi({
      "GET /me": () => jsonResponse(widenedManager()),
      "GET /teams": () =>
        jsonResponse({
          data: [
            { id: "t1", name: "Nord" },
            { id: "t2", name: "Sued" },
          ],
          page: { next_cursor: null, has_more: false },
        }),
    });
  }

  it("offers both teams on Morning", async () => {
    stubTeams();
    render(
      <BriefTeamSelect lists={morningTeams("all")}>
        {(team) => <p>{team}</p>}
      </BriefTeamSelect>,
    );

    await userEvent.setup().click(await screen.findByRole("combobox"));
    expect(await screen.findByRole("option", { name: "Nord" })).toBeTruthy();
    expect(screen.getByRole("option", { name: "Sued" })).toBeTruthy();
  });

  it("offers only the team they lead on Weekly", async () => {
    stubTeams();
    render(
      <BriefTeamSelect lists={weekTeams("teams_led")}>
        {(team) => <p>{team}</p>}
      </BriefTeamSelect>,
    );

    await screen.findByText("t1");
    expect(screen.queryByText("t2")).toBeNull();
  });
});
