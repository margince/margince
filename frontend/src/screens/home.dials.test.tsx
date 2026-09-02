/** @vitest-environment jsdom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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
    expect(await screen.findByText(en["brief.eyebrow"])).toBeTruthy();
    // findBy: the sentence is composed from the worklist read, so it appears
    // when that lands rather than on the first paint.
    expect(await screen.findByTestId("glance-sentence")).toBeTruthy();

    cleanup();
    vi.unstubAllGlobals();
    globalThis.location.hash = "#/home?view=weekly";
    stubHome(["mine"]);
    render(<HomeScreen />);

    expect(await screen.findByText(en["brief.eyebrow.weekly"])).toBeTruthy();
    expect(screen.queryByText(en["brief.eyebrow"])).toBeNull();
    expect(screen.queryByTestId("glance-sentence")).toBeNull();
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
