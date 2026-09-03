/** @vitest-environment jsdom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { formatTimeOfDay } from "../format/format";
import { viewerZone } from "../format/timezone";
import { en } from "../i18n/en";
import { HomeScreen } from "./home";
import { readingsDay, waitingRow } from "./home.fixtures";
import { jsonResponse, render, stubApi } from "./home.testkit";
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
  globalThis.location.hash = "#/home";
});

// One waiting customer, under the scopes this case is about. Built on the shared
// day so the whole answer stays in one spelling: the strip, the sentence and Do
// next are all drawn from it, and a hand-built copy here would drift from theirs
// one edited field at a time.
function worklist(scopeOptions: Worklist["scope_options"]) {
  const day = readingsDay({}, [waitingRow()]);
  return { ...day, scope_options: scopeOptions };
}

/** Stub every read the Brief fans out to, for a reader with the given scopes. */
function stubHome(scopeOptions: Worklist["scope_options"]) {
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
    stubHome(["mine"]);
    render(<HomeScreen />);

    await screen.findByRole("group", { name: en["brief.view.label"] });
    expect(
      screen.queryByRole("group", { name: en["brief.scope.label"] }),
    ).toBeNull();
  });

  it("draws both dials for a reader whose scope reaches a team", async () => {
    stubHome(["mine", "team"]);
    render(<HomeScreen />);

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
    stubHome(["mine"]);
    render(<HomeScreen />);

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
    stubHome(["mine", "team"]);
    render(<HomeScreen />);

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
    globalThis.location.hash = "#/home?view=weekly";
    stubHome(["mine"]);
    render(<HomeScreen />);

    await screen.findByRole("group", { name: en["brief.view.label"] });
    await waitFor(() =>
      expect(document.querySelector("#home-weekly")).not.toBeNull(),
    );
    expect(document.querySelector("#home-schedule")).toBeNull();
    expect(document.querySelector("#home-watch")).toBeNull();
    // And the TRACK is gone with them. The <aside> element is gated on having
    // content, but the grid template is on the wrapper and driven by `shape` —
    // so dropping only the contents leaves the weekly at seventy per cent
    // width beside a third of nothing, which reads as a rail that failed to
    // load rather than one that was never there. `page-zones-aside` is the
    // class that reserves the column.
    expect(document.querySelector(".page-zones-aside")).toBeNull();
  });

  // And it is still there on the morning, or the assertion above passes over a
  // rail that was deleted rather than placed.
  it("keeps the rail on the morning", async () => {
    stubHome(["mine"]);
    render(<HomeScreen />);

    await waitFor(() =>
      expect(document.querySelector("#home-schedule")).not.toBeNull(),
    );
    expect(document.querySelector(".page-zones-aside")).not.toBeNull();
  });

  // DECISION 5, ON THE PAGE. Every combination the dials offer must draw
  // something. An empty work column under a selectable dial is the defect the
  // rule exists to prevent, and it is invisible to a test that only checks the
  // address resolved.
  it("draws a surface under every combination it offers", async () => {
    for (const hash of [
      "#/home",
      "#/home?view=weekly",
      "#/home?scope=team",
      "#/home?scope=team&view=weekly",
    ]) {
      globalThis.location.hash = hash;
      stubHome(["mine", "team"]);
      const view = render(<HomeScreen />);

      await screen.findByRole("group", { name: en["brief.view.label"] });
      await waitFor(() => {
        const main = view.container.querySelector(".home-main");
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
  it("names the view in the eyebrow, and keeps the sentence to the morning", async () => {
    stubHome(["mine"]);
    render(<HomeScreen />);
    // The expected time is DERIVED, not written down: the runner's zone is not
    // the fixture's, so a literal "07:00" here passes in Berlin and fails in CI.
    expect(
      await screen.findByText(
        `${en["brief.eyebrow"]} · as of ${formatTimeOfDay(
          readingsDay({}).as_of,
          "en",
          viewerZone(),
        )}`,
      ),
    ).toBeTruthy();
    // findBy: the sentence is composed from the worklist read, so it appears
    // when that lands rather than on the first paint.
    expect(await screen.findByTestId("glance-sentence")).toBeTruthy();

    cleanup();
    vi.unstubAllGlobals();
    globalThis.location.hash = "#/home?view=weekly";
    stubHome(["mine"]);
    render(<HomeScreen />);

    expect(await screen.findByText(en["brief.eyebrow.weekly"])).toBeTruthy();
    // Exact match: the morning's eyebrow now composes the scope with an as-of,
    // so a substring read would call the weekly clean while the morning's own
    // words were on the page.
    expect(
      screen.queryByText((text) => text === en["brief.eyebrow"]),
    ).toBeNull();
    expect(screen.queryByTestId("glance-sentence")).toBeNull();
    // And the line that stands in for it belongs to the week too. The weekly
    // NEVER composes a sentence, so the fallback is the only line under its
    // heading — and the morning's "this is your day" read as the wrong week
    // entirely beneath "YOUR WEEK".
    expect(screen.getByText(en["home.glance.introWeekly"])).toBeTruthy();
    expect(screen.queryByText(en["home.glance.intro"])).toBeNull();
  });

  // The morning shows what waits; the weekly shows the week. Neither shows the
  // other, or the dial would not be a dial.
  it("shows the morning's work only on the morning", async () => {
    stubHome(["mine"]);
    render(<HomeScreen />);
    expect(await screen.findByText(en["brief.donext.title"])).toBeTruthy();

    cleanup();
    vi.unstubAllGlobals();
    globalThis.location.hash = "#/home?view=weekly";
    stubHome(["mine"]);
    render(<HomeScreen />);

    await screen.findByRole("group", { name: en["brief.view.label"] });
    await waitFor(() =>
      expect(screen.queryByText(en["brief.donext.title"])).toBeNull(),
    );
  });
});
