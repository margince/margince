/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { forgetSeat } from "../../lib/storage";
import { reportingFixtureFrames } from "../../testing/reporting.fixtures";
import { jsonResponse } from "../../testing/story-utils";
import { upcomingWindow } from "./analytics.overview.fixtures";
import {
  isWeeks,
  prospectingWeeks,
} from "./analytics.overview.prospectingdata";
import { workday, workRows } from "./analytics.overview.scenariodata";
import {
  metric,
  openOverview as open,
  prospectingEvaluation,
  readings,
} from "./analytics.overview.testkit";
import type { ReportingEvaluation } from "./reporting.model";

const originalFetch = globalThis.fetch;
beforeEach(() => forgetSeat());
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

const ADDRESS = "#/analytics/overview?overview=prospecting";
const week = reportingFixtureFrames.weekAcrossMonths;
const failed = () => jsonResponse({ title: "Unavailable", status: 503 }, 503);

// The week's answer, and the recent weeks answered on their own request.
function weekAnd(
  evaluation: ReportingEvaluation,
  weeks: ReportingEvaluation = prospectingWeeks(
    week,
    "accepted_opportunities",
    [1, 0, 3, 1, 2, 2],
  ),
) {
  return (url: URL) => (isWeeks(url) ? weeks : evaluation);
}

async function results() {
  return within(await screen.findByRole("region", { name: "Your results" }));
}

describe("My prospecting's results", () => {
  it("leads with accepted handoffs where SDRs hand prospects on", async () => {
    open(ADDRESS, weekAnd(prospectingEvaluation()), { seat: "sdr" });
    const lead = await results();
    expect(lead.getByText("Accepted handoffs")).toBeVisible();
    expect(lead.queryByText("Leads qualified")).toBeNull();
    expect(
      lead.getByText("2", { selector: ".analytics-hero-value" }),
    ).toBeVisible();
  });

  it("leads with qualified leads where nobody hands prospects on", async () => {
    open(
      ADDRESS,
      weekAnd(
        prospectingEvaluation(),
        prospectingWeeks(week, "leads_promoted", [1, 2, 0, 4, 2, 3]),
      ),
      { seat: "sdr", template: "sales" },
    );
    const lead = await results();
    expect(lead.getByText("Leads qualified")).toBeVisible();
    expect(lead.queryByText("Accepted handoffs")).toBeNull();
    expect(lead.getByText(/^Of 11 leads that arrived /)).toBeVisible();
  });

  it("names no qualified figure for a week nobody arrived in, as its column does", async () => {
    open(
      ADDRESS,
      weekAnd(
        prospectingEvaluation([
          metric("leads_promoted", 0, { denominator: 0 }),
        ]),
        prospectingWeeks(week, "leads_promoted", [1, 2, 0, 4, 2, null]),
      ),
      { seat: "sdr", template: "sales" },
    );
    const lead = await results();
    expect(
      lead.getByText("—", { selector: ".analytics-hero-value" }),
    ).toBeVisible();
    expect(lead.getByText(/^No lead arrived /)).toBeVisible();
    const columns = await lead.findAllByRole("button", {
      name: /, Leads qualified: /,
    });
    expect(columns.at(-1)).toHaveAccessibleName(/: —$/);
  });

  it("dates the week's figure and the month's targets apart, and divides neither by the other", async () => {
    open(ADDRESS, weekAnd(prospectingEvaluation()), { seat: "sdr" });
    const lead = await results();
    expect(lead.getByText(/^Handoffs accepted into a deal, /)).toBeVisible();
    expect(lead.getByText(/^Monthly targets, 1 Sept/)).toBeVisible();
    // The month's own actual against the month's target, never the week's 4.
    expect(lead.getByText("Meetings held: 18 of 24")).toBeVisible();
    expect(screen.queryByText(/\b4 of 24\b/)).toBeNull();
    expect(screen.queryByText(/\b2 of 8\b/)).toBeNull();
  });

  it("says hidden targets are hidden, never that none is set", async () => {
    const hidden = { coverage: { status: "ok", withheld: true } } as const;
    open(
      ADDRESS,
      weekAnd(
        prospectingEvaluation([
          metric("meetings_held", null, hidden),
          metric("accepted_opportunities", null, hidden),
        ]),
      ),
      { seat: "sdr" },
    );
    const lead = await results();
    expect(lead.getByText("Meetings held: Hidden from you")).toBeVisible();
    expect(lead.getByText("Accepted handoffs: Hidden from you")).toBeVisible();
    expect(lead.queryByText(/No monthly target set/)).toBeNull();
    expect(lead.queryByRole("link", { name: "Set targets" })).toBeNull();
  });

  it("shows a hidden target beside one it may read", async () => {
    open(
      ADDRESS,
      weekAnd(
        prospectingEvaluation([
          metric("meetings_held", 4, { target: 24, target_actual: 18 }),
          metric("accepted_opportunities", null, {
            coverage: { status: "ok", withheld: true },
          }),
        ]),
      ),
      { seat: "sdr" },
    );
    const lead = await results();
    expect(lead.getByText("Meetings held: 18 of 24")).toBeVisible();
    expect(lead.getByText("Accepted handoffs: Hidden from you")).toBeVisible();
    expect(lead.queryByText(/No monthly target set/)).toBeNull();
  });

  it("agrees with its last week, and leaves a week with no reading empty", async () => {
    open(
      ADDRESS,
      weekAnd(
        prospectingEvaluation(),
        prospectingWeeks(week, "accepted_opportunities", [
          null,
          null,
          3,
          1,
          0,
          2,
        ]),
      ),
      { seat: "sdr" },
    );
    const lead = await results();
    const columns = await lead.findAllByRole("button", {
      name: /, Accepted handoffs: /,
    });
    expect(columns.at(-1)).toHaveAccessibleName(/: 2$/);
    expect(columns[0]).toHaveAccessibleName(/: —$/);
    expect(columns[4]).toHaveAccessibleName(/: 0$/);
  });

  it("says the last week counts up to now only while it is under way", async () => {
    const done = prospectingWeeks(
      week,
      "accepted_opportunities",
      [1, 0, 3, 1, 2, 2],
    );
    const caption = async (weeks: ReportingEvaluation) => {
      open(ADDRESS, weekAnd(prospectingEvaluation(), weeks), { seat: "sdr" });
      const history = within(
        await screen.findByRole("region", {
          name: "How your outcomes developed",
        }),
      );
      const text = (await history.findByText(/^Week by week since /))
        .textContent;
      cleanup();
      return text;
    };
    expect(await caption(done)).not.toContain("up to now");
    const ongoing = {
      ...done,
      context: {
        ...done.context,
        interval: { ...done.context.interval, end_at: "2026-11-01T23:00:00Z" },
      },
    };
    expect(await caption(ongoing)).toContain(
      "Each handoff counts in the week it was accepted. The last week counts up to now.",
    );
  });

  it("names no outcome while the workflow is unknown, and offers the read again when it fails", async () => {
    open(ADDRESS, weekAnd(prospectingEvaluation()), {
      seat: "sdr",
      framework: failed,
      summary: failed,
    });
    const lead = await results();
    expect(
      await lead.findByText(
        "Could not read whether this company uses SDR handoffs.",
      ),
    ).toBeVisible();
    expect(lead.getByRole("button", { name: "Retry" })).toBeVisible();
    expect(screen.queryByText("Accepted handoffs")).toBeNull();
    expect(screen.queryByText(/does not use SDR handoffs/)).toBeNull();
  });
});

describe("My prospecting's readings", () => {
  it("tells a measured zero, a hidden figure and a failed one apart", async () => {
    const shown = async (held: ReturnType<typeof metric>) => {
      open(
        ADDRESS,
        weekAnd(
          prospectingEvaluation([held, metric("accepted_opportunities", 1)]),
        ),
        { seat: "sdr" },
      );
      const strip = await readings();
      const text = strip.getAllByText(/./).map((node) => node.textContent);
      cleanup();
      return text.join("|");
    };
    const zero = await shown(metric("meetings_held", 0));
    const hidden = await shown(
      metric("meetings_held", null, {
        coverage: { status: "ok", withheld: true },
      }),
    );
    const broken = await shown(
      metric("meetings_held", null, {
        coverage: { status: "failed", withheld: false },
      }),
    );
    expect(zero).toContain("No meetings recorded as held this week");
    expect(hidden).toContain("Hidden from you");
    expect(broken).toContain("Could not be read");
    expect(new Set([zero, hidden, broken]).size).toBe(3);
  });

  it("keeps a valid median when only the unanswered count failed, and says so", async () => {
    open(
      ADDRESS,
      weekAnd(
        prospectingEvaluation([
          metric("lead_first_response", 1.7),
          metric("leads_awaiting_response", null, {
            coverage: { status: "failed", withheld: false },
          }),
        ]),
      ),
      { seat: "sdr" },
    );
    const strip = await readings();
    expect(strip.getByText(/^1[.,]7/)).toBeVisible();
    expect(
      strip.getByText("Unanswered leads could not be counted"),
    ).toBeVisible();
  });

  it("says what is missing when too few leads were answered, and still counts who waits", async () => {
    open(
      ADDRESS,
      weekAnd(
        prospectingEvaluation([
          metric("lead_first_response", null, {
            coverage: { status: "insufficient_sample", withheld: false },
          }),
          metric("leads_awaiting_response", 3),
        ]),
      ),
      { seat: "sdr" },
    );
    const strip = await readings();
    expect(strip.getByText("Too few")).toBeVisible();
    expect(
      strip.getByText("Too few answered leads to give a typical time."),
    ).toBeVisible();
    expect(strip.getByText("3 leads still waiting for a reply")).toBeVisible();
  });

  it("counts booked meetings in the window it names, and lists the same meetings", async () => {
    const asOf = "2026-09-25T09:00:00Z";
    const asked = open(ADDRESS, weekAnd(prospectingEvaluation()), {
      seat: "sdr",
      summary: () =>
        jsonResponse({
          as_of: asOf,
          scope: { kind: "owner" },
          workflow: "qualification",
          upcoming_meetings: 1,
          upcoming_window: upcomingWindow(asOf),
        }),
      meetings: () =>
        jsonResponse({
          as_of: asOf,
          window: upcomingWindow(asOf),
          rows: [
            {
              id: "m-1",
              starts_at: "2026-09-26T10:00:00Z",
              starts_precision: "instant",
              title: "Discovery call with Orchard Freight",
              restricted: false,
              record: {
                type: "lead",
                id: "l-orchard",
                label: "Orchard Freight",
              },
            },
          ],
        }),
      template: "sales",
    });
    const strip = await readings();
    expect(strip.getByText("Booked meetings")).toBeVisible();
    expect(strip.getByText("Next 14 days, through 8 Oct 2026")).toBeVisible();
    const list = within(
      await screen.findByRole("region", { name: "Meetings to prepare" }),
    );
    expect(
      await list.findByText("Discovery call with Orchard Freight"),
    ).toBeVisible();
    // The list is read in the window the count declared, not one from now.
    const page = asked.find(({ url }) =>
      url.pathname.endsWith("/analytics/sdr-summary/meetings"),
    )?.url.searchParams;
    expect(page?.get("window_start")).toBe(asOf);
    expect(page?.get("window_end")).toBe(upcomingWindow(asOf).end_at);
  });
});

describe("My prospecting's next moves", () => {
  it("names each move with its reason and clock, and opens its own record", async () => {
    open(ADDRESS, weekAnd(prospectingEvaluation()), {
      seat: "sdr",
      worklist: () => jsonResponse(workday(week.clock, workRows)),
    });
    const moves = within(
      await screen.findByRole("region", { name: "Do next" }),
    );
    const open_ = await moves.findByRole("link", {
      name: "Open Call Nordhafen Logistik back about the fleet trial",
    });
    expect(open_.getAttribute("href")).toContain("/leads/");
    expect(moves.getAllByText("Overdue")[0]).toBeVisible();
    expect(moves.getByText("All 5 items in your Worklist")).toBeVisible();
  });

  it("says the day's whole total when a card folds several items", async () => {
    const folded = {
      ...workRows[2],
      id: "work-folded",
      title: undefined,
      batch: { key: "likely_automated", count: 8 },
    } as (typeof workRows)[number];
    open(ADDRESS, weekAnd(prospectingEvaluation()), {
      seat: "sdr",
      worklist: () =>
        jsonResponse(
          workday(week.clock, [workRows[0], workRows[1], folded], {
            summary: { urgent: 1, due: 2, lower_priority: 0, total: 10 },
          }),
        ),
    });
    const moves = within(
      await screen.findByRole("region", { name: "Do next" }),
    );
    expect(
      await moves.findByText("All 10 items in your Worklist"),
    ).toBeVisible();
    expect(moves.queryByText(/more in your Worklist/)).toBeNull();
    expect(moves.queryByText(/^7 /)).toBeNull();
  });

  it("reaches a concrete task from the page in two presses", async () => {
    open(ADDRESS, weekAnd(prospectingEvaluation()), {
      seat: "sdr",
      worklist: () => jsonResponse(workday(week.clock, workRows)),
    });
    const user = userEvent.setup({ delay: null });
    const first = await screen.findByRole("link", {
      name: /^Open Call Nordhafen Logistik/,
    });
    await user.click(first);
    expect(window.location.hash).toContain("/leads/");
  });

  it("offers no move and claims no clear day when the day could not be read", async () => {
    open(ADDRESS, weekAnd(prospectingEvaluation()), {
      seat: "sdr",
      worklist: failed,
    });
    const moves = within(
      await screen.findByRole("region", { name: "Do next" }),
    );
    expect(await moves.findByText("This section did not load.")).toBeVisible();
    expect(moves.queryByText("Nothing needs you right now")).toBeNull();
    expect(moves.queryByRole("link", { name: /^Open / })).toBeNull();
  });
});

describe("My prospecting's handoffs", () => {
  it("shows each waiting handoff's recipient, age and state, and one sent back with its reason", async () => {
    open(ADDRESS, weekAnd(prospectingEvaluation()), {
      seat: "sdr",
      summary: () =>
        jsonResponse({
          as_of: week.clock,
          scope: { kind: "owner" },
          workflow: "handoff",
          handoffs: { waiting: 1, unassigned: 0, blocked: 1, returned: 1 },
          upcoming_meetings: 0,
          upcoming_window: upcomingWindow(week.clock),
        }),
      handoffs: () =>
        jsonResponse({
          as_of: week.clock,
          rows: [
            {
              id: "h-1",
              waiting_since: "2026-10-26T09:00:00Z",
              resubmitted: false,
              restricted: false,
              subject: { type: "lead", id: "l-1", label: "Bramwell Werkstatt" },
              recipient: { id: "u-2", label: "Casey Closer", active: false },
            },
          ],
        }),
      returned: () =>
        jsonResponse({
          as_of: week.clock,
          rows: [
            {
              id: "h-2",
              waiting_since: "2026-10-27T09:00:00Z",
              resubmitted: false,
              restricted: false,
              subject: {
                type: "lead",
                id: "l-2",
                label: "Lindqvist Transport",
              },
              reason: "No budget this quarter",
            },
          ],
        }),
    });
    const waiting = within(
      await screen.findByRole("region", { name: "Handoffs waiting on others" }),
    );
    expect(
      await waiting.findByText(/Casey Closer can no longer act/),
    ).toBeVisible();
    expect(waiting.getByText("Recipient cannot act")).toBeVisible();
    expect(
      waiting.getByText(/^Waiting .*Casey Closer can no longer act$/),
    ).toBeVisible();
    expect(
      await waiting.findByText(/^Sent back .* ago · No budget this quarter$/),
    ).toBeVisible();
  });

  it("names the cohort its outcomes count", async () => {
    open(ADDRESS, weekAnd(prospectingEvaluation()), { seat: "sdr" });
    const cohort = within(
      await screen.findByRole("region", {
        name: "What happened to your handoffs",
      }),
    );
    expect(
      await cohort.findByText(/handoffs you submitted .*, as they stand now/),
    ).toBeVisible();
    expect(cohort.getByText("Not decided yet")).toBeVisible();
  });
});
