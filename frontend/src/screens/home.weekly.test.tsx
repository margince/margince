/** @vitest-environment jsdom */
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RecordZoneProvider } from "../app/recordzone";
import { formatDateTime } from "../format/format";
import { en } from "../i18n/en";
import { HomeScreen } from "./home";
import { fleetDeal, jsonResponse, render, run, stubApi } from "./home.testkit";

// The week just gone, split from home.test.tsx when that file crossed the
// 1000-line ceiling frontend/AGENTS.md sets.
//
// One subject, and the seam the file already had: the retrospective and the
// sentence about it are what a reader opens on a Monday, and they read against
// a week of runs rather than against this morning's queue. Everything the two
// suites share is in home.testkit.tsx, so an unrouted read answers the same way
// on both sides of the split.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  window.location.hash = "";
});

// The weekly is a VIEW of the Brief now, not a panel at the foot of the
// morning. These cases are about what the retrospective says, so each one opens
// on the address that shows it — the dial itself is tested in
// home.dials.test.tsx.
beforeEach(() => {
  window.location.hash = "#/home?view=weekly";
});

// ── The week just gone ──

describe("HomeScreen — the weekly retrospective", () => {
  const review = {
    id: "01a04000-0000-7000-8000-00000000000a",
    local_week_start: "2026-06-29",
    generated_at: "2026-07-06T06:00:00Z",
    as_of: "2026-07-06T06:00:00Z",
    counts: {
      tasks_due: 5,
      tasks_done: 4,
      tasks_carried_over: 2,
      deals_moved: 3,
      deals_won: 1,
      deals_lost: 1,
      proposals_accepted: 7,
      proposals_rejected: 2,
      brief_items_acted: 6,
      brief_items_dismissed: 3,
      leads_routed: 9,
      leads_answered_in_target: 7,
      leads_breached: 2,
      meetings_held: 5,
      meetings_with_next_step: 3,
      commitments_due: 4,
      commitments_kept: 3,
    },
    deals: [
      {
        deal_id: "01a04000-0000-7000-8000-00000000000b",
        label: "Weber Rahmenvertrag",
        outcome: "won",
        occurred_at: "2026-07-02T14:00:00Z",
      },
    ],
  };

  it("shows the week's tallies and its frozen deal lines", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () => jsonResponse(review),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    // The label is what the deal was CALLED that week, served from the frozen
    // row rather than looked up — which is why it renders although the review's
    // own deal is absent from the deals payload, where only Fleet retrofit is.
    await screen.findByText("Weber Rahmenvertrag");
    expect(screen.getByText(en["home.weekly.tasksDelivered"])).toBeTruthy();
  });

  // What the wins were WORTH, beside how many there were.
  //
  // The count alone says a week of five small renewals and a week of one
  // company-making deal are the same week. The money was computed, converted
  // and stored — and read by no screen until now.
  it("says what the week's wins were worth", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () =>
        jsonResponse({
          ...review,
          pipeline: {
            created_minor: 4500000,
            won_minor: 1250000,
            lost_minor: 0,
            currency: "EUR",
          },
        }),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    // The review's OWN currency, not the installation's current setting: base
    // currency is operator-mutable, and re-reading it would re-label an old
    // week with a currency its numbers were never in.
    expect(await screen.findByText(/12.500,00\s*€|€12,500\.00/)).toBeTruthy();
  });

  // And a week with no pipeline block draws no money at all.
  //
  // The block is optional on the wire — a week assembled before the money
  // columns existed, or one whose FX rate was missing, has no honest figure.
  // "0 €" is a claim about a week nobody measured.
  it("draws no money for a week that carries none", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () => jsonResponse(review),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    await screen.findByText("Weber Rahmenvertrag");
    const strip = document.querySelector('[data-testid="weekly-strip"]');
    expect(strip).not.toBeNull();
    expect(strip?.textContent ?? "").not.toMatch(/€|EUR/);
  });

  // The panel says its numbers can no longer move.
  //
  // That claim is what separates the weekly from every other panel on Home. A
  // rep who reads it on Tuesday, acts, and re-reads on Thursday is looking at a
  // record rather than a stale figure — and without the mark they have no way
  // to tell those apart. The TEAM weekly has said so since it shipped; the
  // rep's, which is the one a rep actually opens, did not.
  it("marks the week frozen, and says when it was written", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () => jsonResponse(review),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    // A zone the TEST chooses, and not the product's fallback.
    //
    // The fallback is importable by its own reader alone (held by
    // format/zone-by-purpose.test.ts), and rightly: a test asserting against it
    // would be checking the component against the same constant the component
    // reads, which passes however wrong the zone decision is. Naming one here
    // means the assertion fails if the panel ever renders in the viewer's zone
    // instead of the installation's.
    const installationZone = "Asia/Ho_Chi_Minh";
    render(
      <RecordZoneProvider zone={installationZone}>
        <HomeScreen />
      </RecordZoneProvider>,
    );

    expect(await screen.findByText(en["home.weekly.frozen"])).toBeTruthy();
    const written = en["home.weekly.written"].replace(
      "{at}",
      formatDateTime(review.generated_at, "en", installationZone),
    );
    expect(screen.getByText(written)).toBeTruthy();
  });

  // And nothing is certified when there is no review to certify. A badge over
  // an absent week claims a record nobody wrote.
  it("marks nothing frozen when there is no review", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () =>
        jsonResponse({ title: "Not Found" }, 404),
      "GET /weekly-reviews": () => jsonResponse({ weeks: [] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    await screen.findByText(en["home.weekly.none"]);
    expect(screen.queryByText(en["home.weekly.frozen"])).toBeNull();
  });

  it("says there is no review yet rather than drawing a week of zeroes", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () =>
        jsonResponse({ title: "Not Found" }, 404),
      "GET /weekly-reviews": () => jsonResponse({ weeks: [] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    // A page of zeroes would claim a week that was measured and empty.
    await screen.findByText(en["home.weekly.none"]);
  });

  it("survives a payload that is not a review", async () => {
    stubApi({
      // The shape an unrouted read answers with: a list page, not a review.
      "GET /weekly-reviews/latest": () =>
        jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    // The panel formats local_week_start immediately, so a half-shaped answer
    // used to take the whole render down with it rather than drawing one honest
    // empty section.
    //
    // What "the rest of the page survives" MEANS moved with the dials: the
    // morning's queue and this panel are no longer on screen together, so the
    // surviving surface to look for is the weekly's own — the dials and the
    // section around the panel that could not draw.
    expect(
      await screen.findByRole("group", { name: en["brief.view.label"] }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: en["home.panel.weekly"] }),
    ).toBeTruthy();
  });
});

describe("HomeScreen — the week's sentence", () => {
  const narrated = {
    id: "01a04000-0000-7000-8000-00000000000a",
    local_week_start: "2026-06-29",
    generated_at: "2026-07-06T06:00:00Z",
    as_of: "2026-07-06T06:00:00Z",
    counts: {
      tasks_due: 5,
      tasks_done: 4,
      tasks_carried_over: 2,
      deals_moved: 3,
      deals_won: 1,
      deals_lost: 1,
      proposals_accepted: 7,
      proposals_rejected: 2,
      brief_items_acted: 6,
      brief_items_dismissed: 3,
    },
    deals: [],
  };

  it("shows the sentence, marked as agent-authored", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () =>
        jsonResponse({
          ...narrated,
          narrative: "Weber signed; two promises slipped to this week.",
          narrated_at: "2026-07-06T06:01:00Z",
        }),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    await screen.findByText("Weber signed; two promises slipped to this week.");
    // Model-authored prose sitting beside numbers a deterministic pass
    // computed; nothing else on the panel would tell them apart.
    expect(
      screen.getAllByText(en["trust.agentUnnamed"]).length,
    ).toBeGreaterThan(0);
  });

  it("says no pass ran, rather than showing nothing", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () =>
        jsonResponse({ ...narrated, narrative: null, narrated_at: null }),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    // Never a blank week, never a silent one: the counts are still the week's,
    // and a rep reading silence would conclude there was nothing to remark on.
    await screen.findByText(en["home.weekly.noNarrative"]);
  });

  it("stays silent when a pass ran and had nothing to add", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () =>
        jsonResponse({
          ...narrated,
          narrative: null,
          narrated_at: "2026-07-06T06:01:00Z",
        }),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);

    await screen.findByText(en["home.weekly.tasksDelivered"]);
    // A pass that honestly found nothing is not a pass that never ran, and
    // claiming otherwise would tell the rep their week was never looked at.
    expect(screen.queryByText(en["home.weekly.noNarrative"])).toBeNull();
  });
});

// ── What CHANGED, not only what happened ──

// A count with no bar against it is a fact a rep cannot act on. "12 deals
// moved" only becomes a review beside the week that came before it.
describe("HomeScreen — the week against the one before", () => {
  const counts = {
    tasks_due: 5,
    tasks_done: 4,
    tasks_carried_over: 2,
    deals_moved: 3,
    deals_won: 3,
    deals_lost: 1,
    proposals_accepted: 7,
    proposals_rejected: 2,
    brief_items_acted: 6,
    brief_items_dismissed: 3,
    leads_routed: 9,
    leads_answered_in_target: 7,
    leads_breached: 2,
    meetings_held: 5,
    meetings_with_next_step: 3,
    commitments_due: 4,
    commitments_kept: 3,
  };
  const review = {
    id: "01a04000-0000-7000-8000-00000000000a",
    local_week_start: "2026-06-29",
    generated_at: "2026-07-06T06:00:00Z",
    as_of: "2026-07-06T06:00:00Z",
    counts,
    deals: [],
  };
  const withPrior = {
    ...review,
    prior: {
      local_week_start: "2026-06-22",
      // One won last week against three this week: a delta of +2 that a
      // reader can check against the figure beside it.
      counts: { ...counts, deals_won: 1 },
    },
  };

  const mount = async (fixture: unknown) => {
    stubApi({
      "GET /weekly-reviews/latest": () => jsonResponse(fixture),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<HomeScreen />);
    return screen.findByTestId("weekly-strip");
  };

  it("names the change since the week before", async () => {
    const strip = await mount(withPrior);

    expect(strip.textContent).toContain(
      en["home.weekly.sincePrior"].replace("{delta}", "+2"),
    );
  });

  // A week that stayed exactly level is a real answer. Printing "+0" dresses it
  // as an increase, which is a small lie told every Monday.
  it("says a level week stayed level", async () => {
    const strip = await mount(withPrior);

    expect(strip.textContent).toContain(
      en["home.weekly.sincePrior"].replace("{delta}", "±0"),
    );
  });

  // A rep's first week did not stay level — it had nothing to stay level
  // against, and a "±0" beside every figure would claim a comparison that was
  // never made.
  it("draws no comparison for a first week", async () => {
    const strip = await mount(review);

    const marker = en["home.weekly.sincePrior"].replace("{delta}", "").trim();
    expect(strip.textContent).not.toContain(marker);
  });

  // ── Five outcomes, and the workings under them ──

  // The strip is read ACROSS as one comparison, so its width is the claim. At
  // ten slots it folded into two ranks at 1280 and stopped being one reading —
  // which is what #3709 reported.
  it("draws five slots, not the ten the week has figures for", async () => {
    const strip = await mount(withPrior);

    expect(strip.querySelectorAll(".stat-card")).toHaveLength(5);
  });

  // The five are the week's OUTCOMES: what the rep planned and kept, what
  // closed, how fast new business was answered, whether meetings led anywhere,
  // and what did not get finished.
  it("gives the strip the week's outcomes", async () => {
    const strip = await mount(withPrior);

    for (const key of [
      "home.weekly.planCommitmentsKept",
      "home.weekly.dealsWon",
      "home.weekly.leadsAnswered",
      "home.weekly.meetingsHeld",
      "home.weekly.carriedOver",
    ] as const) {
      expect(within(strip).getByText(en[key])).toBeTruthy();
    }
  });

  // Two readings, two names — and neither of them a promise.
  //
  // commitments_* counts what a rep wrote into their weekly PLAN and settled;
  // tasks_* counts tasks that fell due in the week. Both render through
  // home.weekly.ofDue, so they arrive on one screen in the same "{n} of {m}"
  // shape six lines apart, and they used to arrive under names one word apart
  // too: "Promises kept" heading the strip, "Promised, delivered" in the list
  // below. On a seat that keeps no weekly plan the first reads 0 of 0 for ever
  // while the figure that reflects the week's delivered work sits under the
  // near-synonym, and the likely reading of a leading 0 of 0 is "I kept
  // nothing" rather than "I never wrote a plan".
  //
  // "Promise" is the wrong word for either. The Morning rail reserves it for
  // something the product does not track yet and says so on screen, so a
  // headline figure wearing it names a third thing again.
  it("names the plan and the task figures apart, and neither as a promise", async () => {
    await mount(withPrior);

    const planned = en["home.weekly.planCommitmentsKept"];
    const delivered = en["home.weekly.tasksDelivered"];
    expect(planned).not.toBe(delivered);
    expect(screen.getByText(planned)).toBeTruthy();
    expect(screen.getByText(delivered)).toBeTruthy();

    // Asserted rather than assumed: the reservation is what makes "promise"
    // wrong here, so if the rail ever starts tracking them this rule wants
    // rereading instead of quietly continuing to hold.
    expect(en["home.promises.untracked"]).toContain("not tracked yet");
    for (const label of [planned, delivered]) {
      expect(label.toLowerCase()).not.toContain("promise");
    }
  });

  // Narrowing the strip must not LOSE the other five. They are still the week's
  // figures and a reader who wants them has to be able to find them — a strip
  // that got shorter by dropping readings would be a worse answer than the row
  // that folded.
  it("keeps the other five figures, under the strip", async () => {
    const strip = await mount(withPrior);

    for (const key of [
      "home.weekly.tasksDelivered",
      "home.weekly.dealsMoved",
      "home.weekly.dealsLost",
      "home.weekly.decided",
      "home.weekly.queueWorked",
    ] as const) {
      const reading = screen.getByText(en[key]);
      expect(reading).toBeTruthy();
      // Under the strip, not in it: in a slot they would be back to competing
      // with the outcomes for the one comparison the row makes.
      expect(strip.contains(reading)).toBe(false);
    }
  });
});
