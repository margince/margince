/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { wholeWeek } from "../../testing/brief.fixtures";
import {
  fleetDeal,
  jsonResponse,
  render,
  stubApi,
} from "../../testing/brief.testkit";
import { BriefScreen } from "./brief";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

it.each([
  { current: true, prior: false },
  { current: false, prior: true },
  { current: true, prior: true },
])(
  "keeps withheld lead figures out of current and prior comparisons: %j",
  async ({ current, prior }) => {
    window.location.hash = "#/home?view=weekly";
    const review = {
      ...wholeWeek,
      leads_withheld: current,
      narrative: null,
      narrated_at: null,
      scorecard: { ...wholeWeek.scorecard, lead: undefined },
      counts: {
        ...wholeWeek.counts,
        leads_routed: current ? 0 : wholeWeek.counts.leads_routed,
        leads_answered_in_target: current
          ? 0
          : wholeWeek.counts.leads_answered_in_target,
        leads_breached: current ? 0 : wholeWeek.counts.leads_breached,
      },
      prior: {
        local_week_start: "2026-06-22",
        leads_withheld: prior,
        counts: wholeWeek.counts,
      },
    };
    stubApi({
      "GET /weekly-reviews/latest": () => jsonResponse(review),
      "GET /weekly-reviews": () =>
        jsonResponse({ weeks: [review.local_week_start] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<BriefScreen />);
    const table = await screen.findByRole("table", {
      name: en["brief.report.chapter.figures"],
    });
    const leadRow = within(table)
      .getByText(en["brief.weekly.leadsAnswered"])
      .closest("tr");
    if (!leadRow) throw new Error("lead figure row is absent");
    const cells = within(leadRow)
      .getAllByRole("cell")
      .map((cell) => cell.textContent);
    if (current) expect(cells[1]).toBe(en["reporting.unavailable"]);
    else expect(cells[1]).not.toBe(en["reporting.unavailable"]);
    if (prior) expect(cells[2]).toBe(en["reporting.unavailable"]);
    expect(cells[3]).toBe("—");
    expect(within(table).getByText(en["brief.weekly.dealsWon"])).toBeTruthy();
    if (current)
      expect(screen.queryByText(en["brief.weekly.noLeads"])).toBeNull();
  },
);
