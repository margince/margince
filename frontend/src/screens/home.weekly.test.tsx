/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
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
    expect(screen.getByText(en["home.weekly.promised"])).toBeTruthy();
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
    // used to take Home's whole render down with it — the queue, the deck and
    // everything else — rather than drawing one honest empty section.
    await screen.findByText("Fleet retrofit");
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

    await screen.findByText(en["home.weekly.promised"]);
    // A pass that honestly found nothing is not a pass that never ran, and
    // claiming otherwise would tell the rep their week was never looked at.
    expect(screen.queryByText(en["home.weekly.noNarrative"])).toBeNull();
  });
});
