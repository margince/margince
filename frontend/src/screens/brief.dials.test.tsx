/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { formatTimeOfDay } from "../format/format";
import { viewerZone } from "../format/timezone";
import { en } from "../i18n/en";
import { BriefScreen } from "./brief";
import { meetingRow, readingsDay, waitingRow } from "./brief.fixtures";
import { jsonResponse, render, stubApi } from "./brief.testkit";
import type { Worklist } from "./worklist.queries";

// The dials, on the rendered page.
//
// brief.view.test.ts proves the ADDRESS resolves; none of it proves the page
// draws what the address asked for. These are about the surface: which sections
// exist under each dial, and that no combination lands on an empty page.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

beforeEach(() => {
  globalThis.location.hash = "#/brief";
});

// One waiting customer, under the scopes this case is about. Built on the shared
// day so the whole answer stays in one spelling: the strip, the sentence and Do
// next are all drawn from it, and a hand-built copy here would drift from theirs
// one edited field at a time.
function worklist(scopeOptions: Worklist["scope_options"]) {
  // The row is marked CHANGED and a source is marked UNAVAILABLE, so all three
  // morning elements above the work column actually draw. The default fixture
  // leaves both empty, and both of those elements return null on an empty one —
  // so a weekly case built on it would assert their absence over elements that
  // were never going to appear, and would keep passing if the guard broke.
  // A MEETING as well as the waiting customer, so the rail's first panel
  // actually draws rows. Without it `#brief-schedule` renders its empty state
  // and "keeps the rail on the morning" would pass over a rail that is there
  // but says nothing — which is indistinguishable from the loading state the
  // assertion is meant to rule out.
  const day = readingsDay({}, [
    { ...waitingRow(), changed_since_brief: true },
    meetingRow("m1", false),
  ]);
  return {
    ...day,
    scope_options: scopeOptions,
    sources_unavailable: [{ source: "calendar", reason: "not_connected" }],
  };
}

/** Stub every read the Brief fans out to, for a reader with the given scopes. */
function stubBrief(scopeOptions: Worklist["scope_options"]) {
  return stubApi({
    "GET /worklist": () => jsonResponse(worklist(scopeOptions)),
    "GET /worklist/team": () =>
      jsonResponse({
        as_of: "2026-06-10T06:00:00Z",
        members: [],
        unassigned: { waiting: 0, at_risk: 0, overdue: 0 },
        truncated: false,
      }),
    "GET /weekly-reviews/latest": () =>
      jsonResponse({ title: "Not Found" }, 404),
    "GET /weekly-plans/current": () =>
      jsonResponse({ title: "Not Found" }, 404),
    "GET /teams": () =>
      jsonResponse({ data: [], page: { next_cursor: null, has_more: false } }),
  });
}

describe("the Brief's dials", () => {
  // A rep has one scope, so the control would have one option — which asks them
  // to confirm what they cannot change.
  it("draws no scope dial for a reader whose scope reaches no team", async () => {
    stubBrief(["mine"]);
    render(<BriefScreen />);

    await screen.findByRole("group", { name: en["brief.view.label"] });
    expect(
      screen.queryByRole("group", { name: en["brief.scope.label"] }),
    ).toBeNull();
  });

  it("draws both dials for a reader whose scope reaches a team", async () => {
    stubBrief(["mine", "team"]);
    render(<BriefScreen />);

    // findBy, not getBy: the scope dial appears only once the worklist read
    // lands, because whether this reader HAS a second scope is that read's
    // answer. Asserting it synchronously tests the first paint, which is
    // always a rep.
    expect(
      await screen.findByRole("group", { name: en["brief.view.label"] }),
    ).toBeTruthy();
    expect(
      await screen.findByRole("group", { name: en["brief.scope.label"] }),
    ).toBeTruthy();
  });

  // The dial writes the address, so a reader can send what they are looking at.
  it("puts the chosen view in the address", async () => {
    stubBrief(["mine"]);
    render(<BriefScreen />);

    await userEvent.click(
      await screen.findByRole("button", { name: en["brief.view.weekly"] }),
    );

    await waitFor(() =>
      expect(globalThis.location.hash).toContain("view=weekly"),
    );
  });

  // And it REPLACES the entry rather than pushing one.
  //
  // Back is the key a reader presses to get out of where they are. A reader
  // turns several dials to reach one view, so pushing per turn would bury the
  // screen they arrived from under a stack of near-identical entries and Back
  // would walk them through it one dial at a time instead of out.
  //
  // app/addressstate.test.ts holds this for replaceParams itself. That proves
  // the mechanism, not that the Brief's dials go through it — a screen writing
  // location.hash directly would satisfy every other assertion in this file
  // while quietly pushing an entry per press.
  it("turns a dial without adding a history entry to press Back through", async () => {
    stubBrief(["mine", "team"]);
    render(<BriefScreen />);

    const before = globalThis.history.length;
    await userEvent.click(
      await screen.findByRole("button", { name: en["brief.view.weekly"] }),
    );
    await waitFor(() =>
      expect(globalThis.location.hash).toContain("view=weekly"),
    );
    await userEvent.click(
      screen.getByRole("button", { name: en["brief.scope.team"] }),
    );
    await waitFor(() =>
      expect(globalThis.location.hash).toContain("scope=team"),
    );

    expect(globalThis.history.length).toBe(before);
  });

  // The rail belongs to the view it is beside.
  //
  // Every panel in it answers a question about TODAY — what the day is booked
  // with, what is owed now, what arrived overnight. Under the weekly they sit
  // beside a week that closed, so the page shows a rep "Today's schedule" next
  // to a retrospective and reads as two screens overlaid.
  //
  // The work column already switches on the view. The rail did not, because it
  // is drawn once outside that branch and nothing asserted otherwise.
  it("leaves the morning's rail off the weekly", async () => {
    globalThis.location.hash = "#/brief?view=weekly";
    stubBrief(["mine"]);
    render(<BriefScreen />);

    await screen.findByRole("group", { name: en["brief.view.label"] });
    await waitFor(() =>
      expect(document.querySelector("#brief-weekly")).not.toBeNull(),
    );
    // `.rail-panel` is what every panel in the rail wears, so this is the rail's
    // own content rather than one panel's id — a panel that collapses on a
    // quiet morning is absent for a reason that has nothing to do with the
    // view, and an id-shaped assertion would pass on that instead. The quiet
    // LINE goes with them: it is the rail reporting on the rail.
    expect(document.querySelectorAll(".rail-panel")).toHaveLength(0);
    expect(document.querySelector("#brief-quiet")).toBeNull();
    // And the TRACK is gone with them. The <aside> element is gated on having
    // content, but the grid template is on the wrapper and driven by `shape` —
    // so dropping only the contents leaves the weekly at seventy per cent
    // width beside a third of nothing, which reads as a rail that failed to
    // load rather than one that was never there. `page-zones-aside` is the
    // class that reserves the column.
    expect(document.querySelector(".page-zones-aside")).toBeNull();
  });

  // THE READINGS AND THE LINE UNDER THEM BELONG TO THE MORNING TOO.
  //
  // The same defect as the rail, one layer up and missed when the rail was
  // fixed: the readings strip and the coverage line are drawn ABOVE the view
  // branch, so they render whatever the dial says. Both read the worklist —
  // today's queue — and the worklist is fetched unconditionally, so under the
  // weekly a rep saw today's urgent count stacked on top of a week that had
  // closed.
  it("leaves the morning's readings and coverage line off the weekly", async () => {
    globalThis.location.hash = "#/brief?view=weekly";
    stubBrief(["mine"]);
    render(<BriefScreen />);

    await screen.findByRole("group", { name: en["brief.view.label"] });
    await waitFor(() =>
      expect(document.querySelector("#brief-weekly")).not.toBeNull(),
    );
    expect(screen.queryByTestId("brief-readings")).toBeNull();
    expect(document.querySelector(".brief-coverage")).toBeNull();
  });

  // The positive control for the case above, and it is the assertion that gives
  // it any force. Both elements are absent on a weekly for two possible
  // reasons — the guard, or a fixture that never made them appear — and only
  // seeing them on the morning tells those apart.
  it("keeps the readings and coverage line on the morning", async () => {
    stubBrief(["mine"]);
    render(<BriefScreen />);

    await screen.findByTestId("brief-readings");
    expect(document.querySelector(".brief-coverage")).not.toBeNull();
  });

  // AND THE LINE SITS UNDER THE FIGURES IT QUALIFIES. Above them it was a
  // caveat a reader met before the numbers it was about, which is the ordering
  // this move exists to fix — and nothing about a coverage line rendering at
  // all would catch it back in the wrong place.
  it("puts the coverage line under the readings, never above them", async () => {
    stubBrief(["mine"]);
    render(<BriefScreen />);

    const strip = await screen.findByTestId("brief-readings");
    const line = document.querySelector(".brief-coverage");
    if (line === null) {
      throw new Error("the coverage line is not on the page");
    }
    // DOCUMENT_POSITION_FOLLOWING: the line comes after the strip.
    expect(
      strip.compareDocumentPosition(line) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  // And it is still there on the morning, or the assertion above passes over a
  // rail that was deleted rather than placed. Asserted on DRAWN ROWS, not on
  // the region: the day carries a meeting, so the schedule panel has something
  // to say, and a rail that rendered its empty states would satisfy a check for
  // the aside while telling a reader nothing.
  it("keeps the rail on the morning", async () => {
    stubBrief(["mine"]);
    render(<BriefScreen />);

    const schedule = await waitFor(() => {
      const panel = document.querySelector("#brief-schedule");
      if (panel === null) {
        throw new Error("the schedule panel is not on the page");
      }
      return panel;
    });
    await waitFor(() =>
      expect(
        schedule.querySelectorAll(".rail-schedule-row").length,
      ).toBeGreaterThan(0),
    );
    expect(document.querySelectorAll(".rail-panel").length).toBeGreaterThan(0);
    expect(document.querySelector(".page-zones-aside")).not.toBeNull();
  });

  // DECISION 5, ON THE PAGE. Every combination the dials offer must draw
  // something. An empty work column under a selectable dial is the defect the
  // rule exists to prevent, and it is invisible to a test that only checks the
  // address resolved.
  it("draws a surface under every combination it offers", async () => {
    for (const hash of [
      "#/brief",
      "#/brief?view=weekly",
      "#/brief?scope=team",
      "#/brief?scope=team&view=weekly",
    ]) {
      globalThis.location.hash = hash;
      stubBrief(["mine", "team"]);
      const view = render(<BriefScreen />);

      await screen.findByRole("group", { name: en["brief.view.label"] });
      await waitFor(() => {
        const main = view.container.querySelector(".brief-main");
        expect(
          main?.querySelectorAll("section, .panel").length ?? 0,
        ).toBeGreaterThan(0);
      });
      cleanup();
      vi.unstubAllGlobals();
    }
  });

  // The opening block belongs to the view it is about. The sentence is composed
  // from the ranked queue — what waits TODAY — so over the weekly it would be
  // describing this morning under a heading about the week that closed.
  //
  // TWO LINES AND NOTHING ELSE, either way. The uppercase eyebrow that named
  // the view and the clock that reported the minute the queue was read are
  // gone: the view is the dial's own state, drawn beside this block, and an
  // as-of that ticked every sixty seconds re-rendered the page's opening for a
  // digit nobody read.
  it("keeps the composed sentence to the morning, and names no view above it", async () => {
    stubBrief(["mine"]);
    render(<BriefScreen />);
    // findBy: the sentence is composed from the worklist read, so it appears
    // when that lands rather than on the first paint.
    expect(await screen.findByTestId("glance-sentence")).toBeTruthy();
    // The greeting, and then the sentence. Nothing between them and nothing
    // above the greeting — which is the whole of what the block draws now.
    const glance = screen.getByTestId("brief-glance");
    expect(glance.children).toHaveLength(2);
    expect(glance.firstElementChild?.tagName).toBe("H1");
    // And no clock. The time the queue was read was the one thing here that
    // moved on its own, and a substring read is what catches it coming back in
    // some other wording.
    expect(glance.textContent).not.toContain(
      formatTimeOfDay(readingsDay({}).as_of, "en", viewerZone()),
    );

    cleanup();
    vi.unstubAllGlobals();
    globalThis.location.hash = "#/brief?view=weekly";
    stubBrief(["mine"]);
    render(<BriefScreen />);

    expect(screen.queryByTestId("glance-sentence")).toBeNull();
    // And the line that stands in for it belongs to the week too. The weekly
    // NEVER composes a sentence, so the fallback is the only line under its
    // heading — and the morning's "this is your day" read as the wrong week
    // entirely beneath "YOUR WEEK".
    expect(screen.getByText(en["brief.glance.introWeekly"])).toBeTruthy();
    expect(screen.queryByText(en["brief.glance.intro"])).toBeNull();
  });

  // The morning shows what waits; the weekly shows the week. Neither shows the
  // other, or the dial would not be a dial.
  it("shows the morning's work only on the morning", async () => {
    stubBrief(["mine"]);
    render(<BriefScreen />);
    expect(await screen.findByText(en["brief.feed.title"])).toBeTruthy();

    cleanup();
    vi.unstubAllGlobals();
    globalThis.location.hash = "#/brief?view=weekly";
    stubBrief(["mine"]);
    render(<BriefScreen />);

    await screen.findByRole("group", { name: en["brief.view.label"] });
    await waitFor(() =>
      expect(screen.queryByText(en["brief.feed.title"])).toBeNull(),
    );
  });
});
