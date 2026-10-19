/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { teamWeek } from "./brief.fixtures";
import { TeamWeeklySection } from "./brief.teamweekly";
import {
  TeamAttentionPanel,
  TeamCoveragePanel,
  TeamQuality,
  TeamSummary,
} from "./brief.teamweeklyattention";
import { jsonResponse, render, stubApi } from "./brief.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the team week's summary, quality and coverage", () => {
  it("prints the server's summary as written", () => {
    render(<TeamSummary review={teamWeek} />);
    expect(screen.getByTestId("teamweekly-summary").textContent).toBe(
      teamWeek.summary,
    );
  });

  it("says a figure was not measured rather than drawing zero", () => {
    render(
      <TeamQuality
        review={{ ...teamWeek, quality: { proposals_accepted: 3 } }}
      />,
    );
    expect(screen.getByText("3")).toBeTruthy();
    expect(
      screen.getAllByText(en["teamweekly.quality.notMeasured"]),
    ).toHaveLength(2);
  });

  it("names the basis a horizon is at risk against, and why one has none", () => {
    render(<TeamCoveragePanel review={teamWeek} />);
    expect(
      screen.getByText("Coverage at risk against the manager’s call"),
    ).toBeTruthy();
    expect(screen.getByText(en["teamweekly.coverage.replan"])).toBeTruthy();
    expect(
      screen.getByText(en["forecast.pipelineAbsent.insufficient_basis"]),
    ).toBeTruthy();
  });

  it("never draws a target the product does not have", () => {
    const { container } = render(<TeamCoveragePanel review={teamWeek} />);
    expect(container.textContent).not.toMatch(/off target|not on track/i);
  });
});

describe("what needs attention now", () => {
  it("lists the stalled deals the read carried", () => {
    render(<TeamAttentionPanel review={teamWeek} />);
    expect(screen.getByText(en["teamweekly.attention.live"])).toBeTruthy();
    expect(screen.getByText("No activity for 66 days")).toBeTruthy();
  });

  it("says nothing needs attention when both lists are empty", () => {
    render(
      <TeamAttentionPanel
        review={{ ...teamWeek, exceptions: [], stale_deals: [] }}
      />,
    );
    expect(screen.getByText(en["teamweekly.attention.empty"])).toBeTruthy();
  });

  it("draws nothing for a read that did not ask", () => {
    const { container } = render(
      <TeamAttentionPanel
        review={{ ...teamWeek, exceptions: undefined, stale_deals: undefined }}
      />,
    );
    expect(container.textContent).toBe("");
  });
});

describe("the live panel on the page", () => {
  it("shows what needs attention even when the frozen week counted nobody", async () => {
    stubApi({
      "GET /weekly-reviews/team": () =>
        jsonResponse({
          ...teamWeek,
          counts: { ...teamWeek.counts, reps_counted: 0 },
          reps: [],
          agenda: [],
        }),
    });
    render(<TeamWeeklySection teamId="t1" />);

    expect(
      await screen.findByText(en["teamweekly.attention.title"]),
    ).toBeTruthy();
    expect(screen.getByText("No activity for 66 days")).toBeTruthy();
  });
});
