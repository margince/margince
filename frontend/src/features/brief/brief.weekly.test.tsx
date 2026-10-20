/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { formatDateAbbrev, formatDateTime } from "../../format/format";
import { en } from "../../i18n/en";
import { RecordZoneProvider } from "../../lib/recordzone";
import { BriefScreen } from "./brief";
import {
  beforeHistoryWeeklyNumbers,
  partlyRecordedWeeklyNumbers,
  sharedWeeklyNumbers,
  zeroWeekCounts,
} from "./brief.fixtures";
import { fleetDeal, jsonResponse, render, run, stubApi } from "./brief.testkit";

// The week just gone, split from brief.test.tsx when that file crossed the
// 1000-line ceiling frontend/AGENTS.md sets.
//
// One subject, and the seam the file already had: the retrospective and the
// sentence about it are what a reader opens on a Monday, and they read against
// a week of runs rather than against this morning's queue. Everything the two
// suites share is in brief.testkit.tsx, so an unrouted read answers the same way
// on both sides of the split.

/** One row of the figures table, as the text of its cells. */
function row(table: HTMLElement, label: string): (string | null)[] {
  const found = within(table)
    .getAllByRole("row")
    .find(
      (each) => within(each).queryAllByRole("cell")[0]?.textContent === label,
    );
  if (!found) throw new Error(`no row labelled ${label}`);
  return within(found)
    .getAllByRole("cell")
    .map((cell) => cell.textContent);
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  window.location.hash = "";
});

// The weekly is a VIEW of the Brief now, not a panel at the foot of the
// morning. These cases are about what the retrospective says, so each one opens
// on the address that shows it — the dial itself is tested in
// brief.dials.test.tsx.
beforeEach(() => {
  window.location.hash = "#/home?view=weekly";
});

// ── The week just gone ──

describe("BriefScreen — the weekly retrospective", () => {
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
    render(<BriefScreen />);

    // The label is what the deal was CALLED that week, served from the frozen
    // row rather than looked up — which is why it renders although the review's
    // own deal is absent from the deals payload, where only Fleet retrofit is.
    await screen.findByText("Weber Rahmenvertrag");
    expect(screen.getByText(en["brief.weekly.tasksDelivered"])).toBeTruthy();
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
    render(<BriefScreen />);

    // The review's OWN currency, not the installation's current setting: base
    // currency is operator-mutable, and re-reading it would re-label an old
    // week with a currency its numbers were never in.
    expect(
      (await screen.findAllByText(/12.500,00\s*€|€12,500\.00/)).length,
    ).toBeGreaterThan(0);
  });

  // THE PACE READING. What the wins were worth, and whether that beat the week
  // before — the question a rep opens a closed week to ask.
  it("says whether the week's money beat the week before", async () => {
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
          prior: {
            local_week_start: "2026-06-22",
            counts: review.counts,
            pipeline: {
              created_minor: 3000000,
              won_minor: 1000000,
              lost_minor: 0,
              currency: "EUR",
            },
          },
        }),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<BriefScreen />);

    // +2.500,00 € on 12.500 against 10.000, and the SIGN is always drawn: a
    // bare figure beside last week's leaves the direction to be guessed.
    expect(
      await screen.findByText(/\+2\.500,00\s*€|\+€2,500\.00/),
    ).toBeTruthy();
  });

  // A week that matched the one before says so, rather than reading as growth.
  it("marks a level week level rather than dressing it as a gain", async () => {
    const money = {
      created_minor: 4500000,
      won_minor: 1250000,
      lost_minor: 0,
      currency: "EUR",
    };
    stubApi({
      "GET /weekly-reviews/latest": () =>
        jsonResponse({
          ...review,
          pipeline: money,
          prior: {
            local_week_start: "2026-06-22",
            counts: review.counts,
            pipeline: money,
          },
        }),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<BriefScreen />);

    expect(await screen.findByText(/±0,00\s*€|±€0\.00/)).toBeTruthy();
  });

  // A PRIOR WEEK NOBODY COULD PRICE yields no comparison — not a change
  // measured against a zero that was never a figure. The value still draws.
  it("compares nothing against a prior week that carries no money", async () => {
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
          prior: { local_week_start: "2026-06-22", counts: review.counts },
        }),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<BriefScreen />);

    expect(
      (await screen.findAllByText(/12.500,00\s*€|€12,500\.00/)).length,
    ).toBeGreaterThan(0);
    expect(screen.queryByText(/vs prior|zur Vorwoche/)).toBeNull();
  });

  // TWO CURRENCIES ARE NOT COMPARABLE. Each week is stored in the currency it
  // was measured in, so an operator who changed the base currency mid-quarter
  // leaves two weeks whose subtraction would print a change nobody can act on.
  it("refuses to subtract across a change of currency", async () => {
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
          prior: {
            local_week_start: "2026-06-22",
            counts: review.counts,
            pipeline: {
              created_minor: 3000000,
              won_minor: 1000000,
              lost_minor: 0,
              currency: "USD",
            },
          },
        }),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<BriefScreen />);

    expect(
      (await screen.findAllByText(/12.500,00\s*€|€12,500\.00/)).length,
    ).toBeGreaterThan(0);
    expect(screen.queryByText(/vs prior|zur Vorwoche/)).toBeNull();
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
    render(<BriefScreen />);

    await screen.findByText("Weber Rahmenvertrag");
    const strip = document.querySelector('[data-testid="weekly-strip"]');
    expect(strip).not.toBeNull();
    expect(strip?.textContent ?? "").not.toMatch(/€|EUR/);
  });

  // The panel says its numbers can no longer move.
  //
  // That claim is what separates the weekly from every other panel on Brief. A
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
        <BriefScreen />
      </RecordZoneProvider>,
    );

    expect(await screen.findByText(en["brief.weekly.frozen"])).toBeTruthy();
    const written = en["brief.weekly.written"].replace(
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
    render(<BriefScreen />);

    await screen.findByText(en["brief.weekly.none"]);
    expect(screen.queryByText(en["brief.weekly.frozen"])).toBeNull();
  });

  it("says there is no review yet rather than drawing a week of zeroes", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () =>
        jsonResponse({ title: "Not Found" }, 404),
      "GET /weekly-reviews": () => jsonResponse({ weeks: [] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<BriefScreen />);

    // A page of zeroes would claim a week that was measured and empty.
    await screen.findByText(en["brief.weekly.none"]);
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
    render(<BriefScreen />);

    // The report formats local_week_start immediately, so a half-shaped answer
    // used to take the whole render down with it rather than drawing one honest
    // empty section. What survives is the dial and the report's own title.
    expect(
      await screen.findByRole("group", { name: en["brief.view.label"] }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: en["brief.panel.weekly"], level: 1 }),
    ).toBeTruthy();
  });
});

describe("BriefScreen — the week's sentence", () => {
  // The retrospective's own section. Scoped rather than page-wide: Brief draws
  // several readings, and a query for the AI mark across the whole screen would
  // answer about whichever one happened to come first in the document.
  function weeklySection(): HTMLElement {
    const section = document.getElementById("brief-weekly");
    expect(section).not.toBeNull();
    return section as HTMLElement;
  }

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
    render(<BriefScreen />);

    const sentence = await screen.findByText(
      "Weber signed; two promises slipped to this week.",
    );
    // Model-authored prose sitting beside numbers a deterministic pass
    // computed; nothing else on the panel would tell them apart.
    expect(
      screen.getAllByText(en["trust.agentUnnamed"]).length,
    ).toBeGreaterThan(0);
    const weekly = within(weeklySection());
    expect(weekly.getByText(en["co.assistant.aiTag"])).toBeTruthy();
    // The mark is on the sentence, not the report. The figures are worked out
    // without a model, and a mark over them would claim a model wrote them.
    const marked = weeklySection().querySelector(".brief-report-narrative");
    expect(marked?.contains(sentence)).toBe(true);
    expect(
      marked?.textContent?.includes(en["brief.weekly.tasksDelivered"]),
    ).toBe(false);
  });

  it("says no pass ran, rather than showing nothing", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () =>
        jsonResponse({ ...narrated, narrative: null, narrated_at: null }),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /brief": () => jsonResponse(run),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<BriefScreen />);

    // Never a blank week, never a silent one: the counts are still the week's,
    // and a rep reading silence would conclude there was nothing to remark on.
    await screen.findAllByText(en["brief.weekly.dealsWon"]);
    expect(screen.queryByTestId("weekly-narrative")).toBeNull();
    // The mirror of the marked case: no model wrote anything here, so nothing
    // wears the mark that says one did.
    expect(weeklySection().querySelector(".brief-report-narrative")).toBeNull();
    expect(
      within(weeklySection()).queryByText(en["co.assistant.aiTag"]),
    ).toBeNull();
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
    render(<BriefScreen />);

    await screen.findByText(en["brief.weekly.tasksDelivered"]);
    // A pass that honestly found nothing is not a pass that never ran, and
    // claiming otherwise would tell the rep their week was never looked at.
    expect(screen.queryByTestId("weekly-narrative")).toBeNull();
  });
});

// ── What CHANGED, not only what happened ──

// A count with no bar against it is a fact a rep cannot act on. "12 deals
// moved" only becomes a review beside the week that came before it.
describe("BriefScreen — the week against the one before", () => {
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
    render(<BriefScreen />);
    return screen.findByTestId("weekly-strip");
  };

  it("names the change since the week before", async () => {
    const strip = await mount(withPrior);

    expect(strip.textContent).toContain(
      en["brief.weekly.sincePrior"]
        .replace("{delta}", "+2")
        .replace("{week}", "22/06/2026"),
    );
  });

  it("names an older comparison week when a report is missing in between", async () => {
    const strip = await mount({
      ...withPrior,
      prior: { ...withPrior.prior, local_week_start: "2026-06-01" },
    });
    expect(strip.textContent).toContain(
      en["brief.weekly.sincePrior"]
        .replace("{delta}", "+2")
        .replace("{week}", "01/06/2026"),
    );
    expect(strip.textContent).not.toContain("vs last week");
  });

  // A week that stayed exactly level is a real answer. Printing "+0" dresses it
  // as an increase, which is a small lie told every Monday.
  it("says a level week stayed level", async () => {
    const strip = await mount(withPrior);

    expect(strip.textContent).toContain(
      en["brief.weekly.sincePrior"]
        .replace("{delta}", "±0")
        .replace("{week}", "22/06/2026"),
    );
  });

  // A rep's first week did not stay level — it had nothing to stay level
  // against, and a "±0" beside every figure would claim a comparison that was
  // never made.
  it("draws no comparison for a first week", async () => {
    const strip = await mount(review);

    const marker = en["brief.weekly.sincePrior"].replace("{delta}", "").trim();
    expect(strip.textContent).not.toContain(marker);
  });

  // ── The headline, the table and the workings ──

  // The report opens on the four figures a week is judged by, in that order.
  it("leads with four headline figures", async () => {
    const strip = await mount(withPrior);

    const labels = [...strip.querySelectorAll("dt")].map(
      (label) => label.textContent,
    );
    expect(labels).toEqual([
      en["brief.weekly.dealsWon"],
      en["brief.weekly.planCommitmentsKept"],
      en["brief.weekly.leadsAnswered"],
      en["brief.weekly.meetingsHeld"],
    ]);
  });

  // One fact, one surface. Deals that moved without closing are a working, and
  // the list under the table reports them.
  it("reports the deals that moved in the workings and not in the headline", async () => {
    const strip = await mount(withPrior);

    expect(strip.textContent).not.toContain(en["brief.weekly.dealsMoved"]);
    expect(screen.getByText(en["brief.weekly.dealsMoved"])).toBeTruthy();
  });

  // Every figure the week carries is in the table, beside the week before.
  it("keeps every figure in the figures table", async () => {
    await mount(withPrior);

    const table = screen.getByRole("table", {
      name: en["brief.report.chapter.figures"],
    });
    for (const key of [
      "brief.weekly.dealsWon",
      "brief.week.lostLabel",
      "brief.weekly.planCommitmentsKept",
      "brief.weekly.leadsAnswered",
      "brief.weekly.meetingsHeld",
      "brief.weekly.carriedOver",
    ] as const) {
      expect(within(table).getByText(en[key])).toBeTruthy();
    }
    expect(row(table, en["brief.weekly.dealsWon"])).toEqual([
      en["brief.weekly.dealsWon"],
      "3",
      "1",
      "+2",
    ]);
  });

  // Two readings, two names — and neither of them a promise.
  //
  // commitments_* counts what a rep wrote into their weekly PLAN and settled;
  // tasks_* counts tasks that fell due in the week. Both render through
  // brief.weekly.ofDue, so they arrive on one screen in the same "{n} of {m}"
  // shape six lines apart, and they used to arrive under names one word apart
  // too: "Promises kept" heading the strip, "Promised, delivered" in the list
  // below. On a seat that keeps no weekly plan the first reads 0 of 0 for ever
  // while the figure that reflects the week's delivered work sits under the
  // near-synonym, and the likely reading of a leading 0 of 0 is "I kept
  // nothing" rather than "I never wrote a plan".
  //
  // "Promise" is the wrong word for either. A promise made in conversation is
  // not something this product tracks — only tasks are — so a headline figure
  // wearing the word names a third thing again.
  it("names the plan and the task figures apart, and neither as a promise", async () => {
    await mount(withPrior);

    const planned = en["brief.weekly.planCommitmentsKept"];
    const delivered = en["brief.weekly.tasksDelivered"];
    expect(planned).not.toBe(delivered);
    expect(screen.getAllByText(planned).length).toBeGreaterThan(0);
    expect(screen.getByText(delivered)).toBeTruthy();

    for (const label of [planned, delivered]) {
      expect(label.toLowerCase()).not.toContain("promise");
    }
  });

  // The workings stay findable, under the headline rather than in it.
  it("keeps supporting activity below the headline", async () => {
    const strip = await mount(withPrior);

    for (const key of [
      "brief.weekly.tasksDelivered",
      "brief.weekly.decided",
      "brief.weekly.queueWorked",
    ] as const) {
      const reading = screen.getByText(en[key]);
      expect(reading).toBeTruthy();
      expect(strip.contains(reading)).toBe(false);
    }
  });
});

// ── A zero its source never measured ──

describe("BriefScreen — figures whose source had not started", () => {
  const review = {
    id: "01a04000-0000-7000-8000-00000000000a",
    local_week_start: "2026-06-29",
    generated_at: "2026-07-06T06:00:00Z",
    as_of: "2026-07-06T06:00:00Z",
    counts: { ...zeroWeekCounts },
    deals: [],
  };
  // A record zone the test chooses, so a "recorded from" date printed in the
  // viewer's zone instead would fail rather than happen to agree.
  const recordZone = "Asia/Ho_Chi_Minh";

  const mount = async (fixture: unknown) => {
    stubApi({
      "GET /weekly-reviews/latest": () => jsonResponse(fixture),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(
      <RecordZoneProvider zone={recordZone}>
        <BriefScreen />
      </RecordZoneProvider>,
    );
    return screen.findByTestId("weekly-strip");
  };
  const card = (strip: HTMLElement, label: string) => {
    const found = [
      ...strip.querySelectorAll<HTMLElement>(".brief-report-figure"),
    ].find((each) => each.querySelector("dt")?.textContent === label);
    if (!found) throw new Error(`no figure labelled ${label}`);
    return {
      value: found.querySelector(".brief-report-figure-value")?.textContent,
      details: [...found.querySelectorAll(".t-caption")].map(
        (line) => line.textContent,
      ),
    };
  };
  const figuresTable = () =>
    screen.getByRole("table", { name: en["brief.report.chapter.figures"] });
  const recordedFrom = (iso: string) =>
    en["brief.weekly.recordedFrom"].replace(
      "{date}",
      formatDateAbbrev(iso, "en", recordZone),
    );

  it("says a week before recorded history was not recorded, never 0", async () => {
    const strip = await mount({
      ...review,
      counts: { ...zeroWeekCounts, tasks_completed: 0 },
      numeric_summary: beforeHistoryWeeklyNumbers,
    });

    // Nothing to count draws no figure large: a dash, and the words under it.
    const values = [
      ...strip.querySelectorAll(".brief-report-figure-value"),
    ].map((value) => value.textContent);
    expect(values).toHaveLength(4);
    expect(new Set(values)).toEqual(new Set(["—"]));
    // Every figure in the table says the same, all seven of them.
    const weekCells = within(figuresTable())
      .getAllByRole("row")
      .slice(1)
      .map((each) => within(each).getAllByRole("cell")[1]?.textContent);
    expect(weekCells).toHaveLength(7);
    expect(new Set(weekCells)).toEqual(
      new Set([en["brief.weekly.notRecorded"]]),
    );
    expect(card(strip, en["brief.weekly.dealsWon"]).details).toEqual([
      en["brief.weekly.notRecorded"],
      recordedFrom("2026-09-21T09:00:00Z"),
    ]);
    // The headline names the gap instead of reading the zeros as a quiet week.
    const glance = screen.getByTestId("brief-glance");
    expect(glance.textContent).toContain(en["brief.week.beforeHistory"]);
    expect(glance.textContent).not.toContain(en["brief.week.quiet"]);
  });

  it("keeps a partly recorded figure, its money line and its qualifier", async () => {
    const strip = await mount({
      ...review,
      pipeline: {
        created_minor: 0,
        won_minor: 1250000,
        lost_minor: 0,
        currency: "EUR",
      },
      counts: {
        ...zeroWeekCounts,
        deals_won: 3,
        deals_lost: 1,
        leads_routed: 9,
        leads_answered_in_target: 7,
        meetings_held: 5,
        meetings_with_next_step: 3,
      },
      numeric_summary: partlyRecordedWeeklyNumbers,
    });

    const won = card(strip, en["brief.weekly.dealsWon"]);
    expect(won.value).toBe("3");
    // The money line stays, and the qualifier follows it.
    expect(won.details).toEqual([
      expect.stringMatching(/^(12.500,00\s*€|€12,500\.00)$/),
      en["brief.weekly.partialFrom"].replace(
        "{date}",
        formatDateAbbrev("2026-09-10T08:00:00Z", "en", recordZone),
      ),
    ]);
    expect(card(strip, en["brief.weekly.meetingsHeld"])).toEqual({
      value: "3 of 5",
      details: ["Some meeting history predates confirmed outcomes."],
    });
    expect(card(strip, en["brief.weekly.leadsAnswered"])).toEqual({
      value: "—",
      details: [en["brief.weekly.notRecorded"], "No lead source in scope."],
    });
    expect(
      screen.getByText(en["brief.weekly.dealsMoved"]).nextSibling?.textContent,
    ).toBe(en["brief.weekly.partialValue"].replace("{value}", "0"));
  });

  // Only a count is drawn at headline size. A share with nothing to share out
  // draws a dash and says why under it.
  it("draws a dash, not words, for a share with nothing to count", async () => {
    const strip = await mount({
      ...review,
      counts: { ...zeroWeekCounts, commitments_due: 0, commitments_kept: 0 },
    });

    expect(card(strip, en["brief.weekly.planCommitmentsKept"])).toEqual({
      value: "—",
      details: [en["brief.weekly.noCommitments"]],
    });
  });

  it("draws a review frozen before coverage existed exactly as before", async () => {
    const strip = await mount({
      ...review,
      counts: { ...zeroWeekCounts, deals_lost: 1 },
      numeric_summary: sharedWeeklyNumbers,
    });

    expect(strip.textContent).not.toContain(en["brief.weekly.notRecorded"]);
    expect(row(figuresTable(), en["brief.week.lostLabel"])[1]).toBe("1");
    expect(card(strip, en["brief.weekly.dealsWon"]).value).toBe("0");
  });
});

// ── The report's own shape ──

describe("BriefScreen — the weekly report", () => {
  const review = {
    id: "01a04000-0000-7000-8000-00000000000a",
    local_week_start: "2026-06-29",
    generated_at: "2026-07-06T06:00:00Z",
    as_of: "2026-07-06T06:00:00Z",
    counts: { ...zeroWeekCounts, deals_won: 1 },
    deals: [],
  };

  const openReport = (weeks: readonly string[]) => {
    stubApi({
      "GET /weekly-reviews/latest": () => jsonResponse(review),
      "GET /weekly-reviews": () => jsonResponse({ weeks }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(
      <RecordZoneProvider zone="Europe/Berlin">
        <BriefScreen />
      </RecordZoneProvider>,
    );
  };

  it("titles the report by the week it covers, and names its calendar week", async () => {
    openReport(["2026-06-29"]);

    expect(
      await screen.findByRole("heading", {
        level: 1,
        name: /^29 June\s*[–-]\s*5 July$/,
      }),
    ).toBeTruthy();
    expect(
      screen.getByText(en["brief.report.kicker"].replace("{week}", "27")),
    ).toBeTruthy();
  });

  // A chapter with nothing to say is left out, and the numbers close up: a
  // report that reads 1, 3, 4 looks like it lost a page.
  it("numbers its chapters with no gap where one is left out", async () => {
    openReport(["2026-06-29"]);

    await screen.findByText(en["brief.report.chapter.figures"], {
      selector: "h2",
    });
    const numbers = [
      ...document.querySelectorAll(".brief-report-chapter-number"),
    ].map((number) => number.textContent);
    expect(numbers).toEqual(numbers.map((_, at) => String(at + 1)));
    // No deal closed this week, so there is no chapter for closed deals.
    expect(
      screen.queryByRole("heading", {
        name: new RegExp(en["brief.report.chapter.closed"]),
      }),
    ).toBeNull();
  });

  // The stepper reaches only weeks that have a report, and writes the week
  // into the address so the page can be sent on.
  it("steps to the week before, and not past the newest", async () => {
    openReport(["2026-06-29", "2026-06-22"]);
    const user = userEvent.setup();

    const later = await screen.findByRole("button", {
      name: en["brief.report.later"],
    });
    expect(later).toHaveProperty("disabled", true);
    await user.click(
      screen.getByRole("button", { name: en["brief.report.earlier"] }),
    );
    await waitFor(() =>
      expect(window.location.hash).toContain("week=2026-06-22"),
    );
  });

  it("says when the next report comes", async () => {
    openReport(["2026-06-29"]);

    expect(
      await screen.findByText(
        new RegExp(
          `^${en["brief.report.next"].replace("{date}", "Monday,? 13 July")}$`,
        ),
      ),
    ).toBeTruthy();
  });
});
