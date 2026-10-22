/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { LeadScreen } from "./leads";
import {
  jsonResponse,
  lead,
  openLeadActions,
  render,
  stubFetchWithMe,
} from "./leads.testkit";

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
  window.location.hash = "";
});

describe("LeadScreen — archived/terminal is read-only (P-3)", () => {
  it("a disqualified lead exposes no enabled mutation anywhere on the page", async () => {
    // A header-only check misses the score override, the clear-override and
    // the owner picker, each able to fire a PATCH the server refuses. So every
    // button on the page is checked, and a control added later is covered.
    stubFetchWithMe(async (url) => {
      if (url.includes("/score")) {
        return jsonResponse({ score: 72, explained: false });
      }
      return jsonResponse({
        ...lead,
        status: "disqualified",
        score_override_reason: "set by hand",
        score_computed: 46,
        archived_at: "2026-07-13T00:00:00Z",
      });
    });
    render(<LeadScreen id="l-1" />);

    await waitFor(() =>
      // Two elements say "Disqualified" (the status badge and the terminal
      // badge), so the wait names the badge it meant.
      expect(screen.getAllByText("Disqualified").length).toBeGreaterThan(0),
    );
    // The sweep opens the menu: a census blind to half the header's buttons
    // reports PASS on a page it never read.
    await openLeadActions();
    // A control that changes what is drawn does not change the record, and a
    // terminal lead is read-only rather than unreadable. The tab strip picks
    // which body is open and the toggle puts the context column away. These are
    // named, since nothing in the DOM tells a view control from a write. So the
    // list stays short, and a mutation added later is still caught.
    const viewControls = new Set([
      "Overview",
      "Deals and projects",
      "History",
      "Hide details",
      // The trigger discloses; the open above sweeps the verbs behind it.
      "More actions",
      // The day's work opens the task LIST: a door to another screen, which
      // writes nothing on this record.
      en["today.workQueue"],
      // A reading's receipt: it opens what the figure was computed from and
      // writes nothing.
      "Evidence",
    ]);
    // The one write a terminal lead offers, in its own set so a reader cannot
    // mistake it for a control that writes nothing. Reopening is the undo of
    // the state that makes the record read-only. Disabled, the page would say
    // "Disqualified: <reason>" with no way back. An operator then re-keys the
    // lead, losing its history and its score with it.
    const closureReversals = new Set(["Reopen"]);
    // The pane's own controls are skipped structurally rather than by label.
    // What folds a disclosure in it is a view control, and "Hide" is too
    // ordinary a word to exempt everywhere it might appear.
    const contextColumn = document.querySelector(".page-zones-aside-column");
    for (const button of screen.getAllByRole("button")) {
      // The accessible name, not the text: the panel switch is icon-only, so
      // its text is empty. A set keyed on text would exempt every unlabelled
      // button on the page along with it.
      const name =
        button.textContent?.trim() || (button.getAttribute("aria-label") ?? "");
      // The call's own disclosure is skipped the same way. It opens the
      // readings a verdict rests on and writes nothing. Its name carries a
      // count that changes with the record.
      if (
        viewControls.has(name) ||
        closureReversals.has(name) ||
        contextColumn?.contains(button) ||
        button.classList.contains("r360-rests-toggle") ||
        // A reading's door, skipped structurally: every stat card says the same
        // word, so a set keyed on "Open" would exempt any control sharing it.
        // The class names a door out of the reading (another screen, a read,
        // nothing written on this record).
        button.classList.contains("stat-card-open")
      ) {
        continue;
      }
      expect(
        (button as HTMLButtonElement).disabled,
        `"${name}" is still live on a terminal lead`,
      ).toBe(true);
    }
  });

  // The way back from "Disqualified: <reason>". Nothing else in the suite
  // fails if the button stops calling the server. The read-only sweep above
  // passes with it inert, because an exempt control is exempt whether or not
  // it works.
  it("offers Reopen on a disqualified lead, and it calls the reopen endpoint", async () => {
    const calls: string[] = [];
    stubFetchWithMe(async (url, method) => {
      if (method !== "GET") {
        calls.push(`${method} ${new URL(url, "http://localhost").pathname}`);
        return jsonResponse({
          ...lead,
          status: "contacted",
          archived_at: null,
        });
      }
      if (url.includes("/score")) {
        return jsonResponse({ score: 72, explained: false });
      }
      return jsonResponse({
        ...lead,
        status: "disqualified",
        archived_at: "2026-07-13T00:00:00Z",
      });
    });
    render(<LeadScreen id="l-1" />);

    const reopen = await screen.findByRole("button", { name: "Reopen" });
    expect((reopen as HTMLButtonElement).disabled).toBe(false);
    await userEvent.click(reopen);
    // The dialog says what reopening does before it is done: the status comes
    // back, the reason goes, the history stays.
    expect(screen.getByText(en["lead.reopenExplain"] as string)).toBeTruthy();
    await userEvent.click(
      screen.getByRole("button", { name: en["lead.reopenConfirm"] as string }),
    );

    await waitFor(() => expect(calls).toContain("POST /v1/leads/l-1/reopen"));
  });

  it("a disqualified lead keeps its controls DISABLED with the reason, never hidden", async () => {
    // Blocked by state rather than permission means visible and disabled
    // with the reason, since hiding the control hides the fact the reader
    // needs. A promoted lead never reaches this page; it redirects to the
    // contact it became.
    stubFetchWithMe(async () =>
      jsonResponse({
        ...lead,
        status: "disqualified",
        archived_at: "2026-07-13T00:00:00Z",
      }),
    );
    render(<LeadScreen id="l-1" />);

    await waitFor(() =>
      // Two elements say "Disqualified" (the status badge and the terminal
      // badge), so the wait names the badge it meant.
      expect(screen.getAllByText("Disqualified").length).toBeGreaterThan(0),
    );
    const reason = "Disqualified. This lead is read-only.";
    await openLeadActions();
    for (const testId of ["lead-disqualify"]) {
      const control = screen.getByTestId(testId) as HTMLButtonElement;
      expect(control.disabled).toBe(true);
      const describedBy = control.getAttribute("aria-describedby");
      expect(describedBy).toBeTruthy();
      expect(document.getElementById(describedBy as string)?.textContent).toBe(
        reason,
      );
    }
    // Qualify is gone rather than disabled: a disqualified lead is not a
    // qualifiable one, and the header's primary action is for live leads.
    expect(screen.queryByRole("button", { name: "Qualify" })).toBeNull();
  });

  it("shows an 'overridden' badge when the score is human-overridden", async () => {
    stubFetchWithMe(async () =>
      jsonResponse({
        ...lead,
        score_override_reason: "Strong buying signal",
        score_computed: 50,
      }),
    );
    render(<LeadScreen id="l-1" />);
    // The header's word stays lower-case because it sits inside a line the
    // score begins. The readings card writes its own line and says so in
    // sentence case.
    await waitFor(() =>
      expect(screen.getAllByText(/· overridden$/).length).toBeGreaterThan(0),
    );
  });

  it("explains the score with its factors and the arithmetic that reconciles them", async () => {
    stubFetchWithMe(async (url) => {
      if (url.includes("/score")) {
        return jsonResponse({
          score: 46,
          explained: true,
          current: {
            score: 46,
            score_computed: 46,
            raw_sum: 45.6,
            rounded_sum: 46,
            computed_at: "2026-06-04T00:00:00Z",
            factors: [
              { factor: "decision_maker_title", points: 15 },
              { factor: "reply", points: 22.6, base_points: 25 },
            ],
          },
        });
      }
      return jsonResponse(lead);
    });
    render(<LeadScreen id="l-1" />);

    await waitFor(() =>
      expect(screen.getByText("Decision-maker title")).toBeTruthy(),
    );
    expect(screen.getByText("Lead replied")).toBeTruthy();
    // The decay is shown as arithmetic a reader can check, not asserted.
    expect(screen.getByText("25, halved every 14 days")).toBeTruthy();
    expect(screen.getByText("Sum 45.60, rounded to 46, score 46")).toBeTruthy();
  });

  it("shows a manual signal from the breakdown, and offers the questions that add one", async () => {
    // The human half of the score reads its stored band and provenance,
    // rather than guessing them from points (ADR-0105).
    //
    // What the form sends is pinned in leadsignals.test.tsx, which drives every
    // one of its states. This test proves the panel reaches the lead page.
    // The stored signal, its reason and its author render here, with the
    // questions that add another beside them.
    stubFetchWithMe(async (url) => {
      if (url.includes("/users")) {
        return jsonResponse({
          data: [{ id: "u-9", email: "lena@x.test", display_name: "Lena F." }],
          page: { next_cursor: null },
        });
      }
      if (url.includes("/manual-signals")) {
        return jsonResponse({
          data: [
            {
              factor: "budget_hint",
              band: "some",
              points: 4,
              signal_kind: "fact",
              confidence: 0.9,
              reason: "CFO named a Q4 line item",
              set_by: "u-9",
              set_at: "2026-06-04T00:00:00Z",
            },
          ],
        });
      }
      return jsonResponse(lead);
    });
    render(<LeadScreen id="l-1" />);
    await waitFor(() =>
      expect(screen.getByText("CFO named a Q4 line item")).toBeTruthy(),
    );
    for (const question of ["Website traffic?", "Company size?", "Budget?"]) {
      expect(screen.getByLabelText(question)).toBeTruthy();
    }
    expect(screen.getByRole("button", { name: "Add to score" })).toBeTruthy();
  });

  it("a closed lead shows its manual signals read-only, with the reason", async () => {
    stubFetchWithMe(async (url) => {
      if (url.includes("/manual-signals")) {
        return jsonResponse({ data: [] });
      }
      if (url.includes("/score")) {
        return jsonResponse({ score: 0, explained: false });
      }
      return jsonResponse({
        ...lead,
        status: "disqualified",
        archived_at: "2026-06-20T08:00:00Z",
      });
    });
    render(<LeadScreen id="l-1" />);
    await waitFor(() => expect(screen.getByText("Lead signals")).toBeTruthy());
    expect(screen.queryByRole("button", { name: "Add to score" })).toBeNull();
    expect(
      screen.getAllByText("This lead is closed and cannot be changed.").length,
    ).toBeGreaterThan(0);
  });

  it("never prints an absent source into the sentence about it", async () => {
    // The suite let this ship: a lead with no source interpolated the missing
    // value and rendered "Came in as undefined" at a rep.
    stubFetchWithMe(async (url) => {
      if (url.includes("/score")) {
        return jsonResponse({ score: 0, explained: false });
      }
      return jsonResponse({ ...lead, score: 0, source: null, title: null });
    });
    render(<LeadScreen id="l-1" />);

    await waitFor(() =>
      expect(screen.getAllByText("No source on record.").length).toBeTruthy(),
    );
    expect(screen.queryByText(/undefined/)).toBeNull();
  });

  it("never claims nothing counts when the score is not zero", async () => {
    // The render caught this: a lead scoring 72 with no retained breakdown
    // was told "Nothing counts toward this score yet", which is the opposite
    // of true. Something counted; this client cannot say what.
    stubFetchWithMe(async (url) => {
      if (url.includes("/score")) {
        return jsonResponse({ score: 72, explained: false });
      }
      return jsonResponse({ ...lead, score: 72 });
    });
    render(<LeadScreen id="l-1" />);

    // Said wherever the score is. The score card states it in the open; the
    // reading beside it keeps its receipt behind its "Evidence" door. So the
    // sentence counts once before that door opens and twice after, and
    // neither surface can drop it unnoticed.
    const notStored =
      "The breakdown for this score is not stored yet. The next update shows it.";
    await waitFor(() => expect(screen.getAllByText(notStored)).toHaveLength(1));
    const grid = document.querySelector(".readings-grid");
    if (!(grid instanceof HTMLElement)) {
      throw new Error("the lead's readings row is not on the page");
    }
    const scoreCard = within(grid).getByText("Score").closest(".stat-card");
    if (!(scoreCard instanceof HTMLElement)) {
      throw new Error("the score reading is not a card in the readings row");
    }
    const user = userEvent.setup();
    await user.click(
      within(scoreCard).getByRole("button", { name: "Evidence" }),
    );
    await waitFor(() => expect(screen.getAllByText(notStored)).toHaveLength(2));
    expect(screen.queryByText("Score inputs:")).toBeNull();
    expect(
      screen.queryByText("No factors counted toward this score yet."),
    ).toBeNull();
  });

  it("says why a lead scores nothing rather than explaining our storage history", async () => {
    stubFetchWithMe(async (url) => {
      if (url.includes("/score")) {
        return jsonResponse({ score: 0, explained: false });
      }
      return jsonResponse({ ...lead, score: 0, title: null });
    });
    render(<LeadScreen id="l-1" />);

    // The reasons a lead earns nothing are derivable from the lead itself, and
    // they are what the reader came for. "This score predates the breakdown"
    // answers a question nobody asked and leaves a 0 looking like a bad
    // prospect rather than an unassessed one (ADR-0108).
    await waitFor(() => expect(screen.getByText("Score inputs:")).toBeTruthy());
    // Not "no reply yet": engagement lives in linked activities this client
    // never reads. So the page states what moves the score rather than
    // asserting the prospect has done nothing.
    expect(
      screen.getByText("A reply or a meeting raises the score most."),
    ).toBeTruthy();
  });

  it("names the owner 'You' when the lead is owned by the current user", async () => {
    stubFetchWithMe(
      async () => jsonResponse({ ...lead, owner_id: "u-9" }),
      "u-9",
    );
    render(<LeadScreen id="l-1" />);
    await waitFor(() => expect(screen.getByText("You")).toBeTruthy());
  });

  it("names an owner who is not the viewer, rather than printing their id", async () => {
    stubFetchWithMe(async (url) => {
      if (url.includes("/users")) {
        return jsonResponse({
          data: [{ id: "u-42", display_name: "Dana Fischer" }],
          page: { next_cursor: null, has_more: false },
        });
      }
      return jsonResponse({ ...lead, owner_id: "u-42" });
    }, "u-9");
    render(<LeadScreen id="l-1" />);

    await waitFor(() => expect(screen.getByText("Dana Fischer")).toBeTruthy());
    // A bare uuid names nobody a reader can recognize. The id stays as the
    // `title` and as the fallback face until the roster lands. So the test
    // asserts that no id is left standing as the visible text.
    expect(screen.queryByText("u-42")).toBeNull();
  });

  it("assigns the lead to another user picked from the roster", async () => {
    let patchBody: Record<string, unknown> | null = null;
    stubFetchWithMe(async (url, method, request) => {
      if (url.includes("/users")) {
        return jsonResponse({
          data: [{ id: "u-42", display_name: "Dana Fischer" }],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (method === "PATCH") {
        patchBody = (await request.json()) as Record<string, unknown>;
        return jsonResponse({ ...lead, owner_id: "u-42" });
      }
      return undefined;
    }, "u-9");
    render(<LeadScreen id="l-1" />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Assign" }),
    );
    // The listbox is opened once and the option awaited inside it. Clicking
    // the trigger again would toggle it shut, and the popup re-renders in
    // place when the roster lands.
    await userEvent.click(
      await screen.findByRole("combobox", { name: "Assign this lead to" }),
    );
    await userEvent.click(
      await screen.findByRole("option", { name: "Dana Fischer" }),
    );

    await waitFor(() => expect(patchBody).toBeTruthy());
    expect(patchBody).toMatchObject({ owner_id: "u-42" });
  });
});

describe("LeadScreen — an archived lead does not ask for what the server refuses", () => {
  function leadBackend(archivedAt: string | null) {
    return stubFetchWithMe(
      async (url) => {
        if (
          url.includes("/score") ||
          url.includes("/lists") ||
          url.includes("/tags")
        ) {
          return jsonResponse(
            { status: 404, code: "not_found", title: "Not found" },
            404,
          );
        }
        return jsonResponse({
          ...lead,
          status: "disqualified",
          archived_at: archivedAt,
        });
      },
      "u-9",
      { data: [] },
      { lists: true },
    );
  }

  it("never reads the score breakdown, the lists or the tags and reports no failed section", async () => {
    const { urls } = leadBackend("2026-07-13T00:00:00Z");
    render(<LeadScreen id="l-1" />);

    await waitFor(() =>
      expect(screen.getAllByText("Disqualified").length).toBeGreaterThan(0),
    );
    // The timeline is read in the same pass as the two refused reads, so it
    // has been asked by the time it is seen.
    await waitFor(() =>
      expect(urls.some((url) => url.includes("/activities"))).toBe(true),
    );

    expect(urls.filter((url) => /\/(score|lists|tags)/.test(url))).toEqual([]);
    expect(screen.queryByText(en["lead.scoreFactorsFailed"])).toBeNull();
  });

  it("reads all three for a live lead", async () => {
    const { urls } = leadBackend(null);
    render(<LeadScreen id="l-1" />);

    await waitFor(() => {
      expect(urls.some((url) => url.includes("/score"))).toBe(true);
      expect(urls.some((url) => url.includes("/lists"))).toBe(true);
      expect(urls.some((url) => url.includes("/tags"))).toBe(true);
    });
  });
});
