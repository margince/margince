/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { removeStored, STORAGE_KEYS } from "../../lib/storage";
import { jsonResponse } from "../../testing/story-utils";
import {
  openOverview as open,
  readings,
  SELF,
  TEAM,
  teamEvaluation,
  teamReview,
} from "./analytics.overview.testkit";

const originalFetch = globalThis.fetch;
beforeEach(() => removeStored(STORAGE_KEYS.analyticsOverviewView));
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

const TEAM_ADDRESS = "#/analytics/overview?overview=team";

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

  it("shows the team's figures once, how its handoffs ended, and one row per member", async () => {
    const asked = open(TEAM_ADDRESS, teamEvaluation, { template: "sdr" });
    const strip = await readings();
    expect(strip.getByText("7")).toBeVisible();
    expect(await screen.findByText("Handoff feedback")).toBeVisible();
    expect(
      screen.getByText(
        /Of the 4 rounds handed on in this period: 2 accepted, 0 returned, 1 rejected and 1 still waiting\./,
      ),
    ).toBeVisible();
    // Four rounds is below the comparable size, so the page says so.
    expect(screen.getByText("A small sample")).toBeVisible();
    expect(screen.getByText("Wrong territory")).toBeVisible();
    const members = screen.getByRole("table", { name: "Members" });
    const rows = within(members).getAllByRole("row");
    expect(rows).toHaveLength(3);
    expect(within(rows[1]).getByText("Riley Rep")).toBeVisible();
    expect(
      screen.getByText(/the rows can add up to more than the team/),
    ).toBeVisible();
    const review = asked.find(({ url }) =>
      url.pathname.endsWith("/analytics/sdr-team"),
    );
    expect(review?.url.searchParams.get("scope_kind")).toBe("team");
    expect(review?.url.searchParams.get("period")).toBe("this_week");
    const evaluations = asked.filter(({ url }) =>
      url.pathname.endsWith("/analytics/evaluate"),
    );
    expect(evaluations).toHaveLength(1);
    expect(evaluations[0].url.searchParams.getAll("metrics")).toEqual([
      "meetings_held",
      "accepted_opportunities",
      "leads_promoted",
      "leads_awaiting_response",
    ]);
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
    expect(scopes).toHaveLength(2);
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

  it("names no member while individual performance is off", async () => {
    open(TEAM_ADDRESS, teamEvaluation, {
      template: "sdr",
      team: () =>
        jsonResponse(teamReview({ members_withheld: true, members: [] })),
    });
    expect(
      await screen.findByText(
        "Individual performance is off, so no member’s figures are shown.",
      ),
    ).toBeVisible();
    expect(screen.queryByRole("table", { name: "Members" })).toBeNull();
  });

  it("withholds the handoff figures a lone colleague makes up", async () => {
    open(TEAM_ADDRESS, teamEvaluation, {
      template: "sdr",
      team: () =>
        jsonResponse(
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
    expect(screen.queryByText("Handoff feedback")).toBeNull();
  });
});
