/** @vitest-environment happy-dom */
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { readingsDay } from "./brief.fixtures";
import { BriefGlance } from "./brief.glance";
import type { BriefView } from "./brief.view";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  window.location.hash = "";
});

// ── The deck: staging is local, the commit is the only thing that sends ──

describe("BriefGlance — the greeting follows the reader's own hour", () => {
  // A local-time instant, so the hour the case names is the hour the reader's
  // own zone reports whatever machine this runs on.
  function greetingAt(hour: number, firstName: string | null): string {
    const view = rtlRender(
      <LocaleProvider initial="en">
        <BriefGlance
          view="morning"
          firstName={firstName}
          now={new Date(2026, 6, 5, hour, 30, 0)}
          // These cases are about the GREETING's hour. An unread day is the
          // right value for them: the sentence is then absent, and the greeting
          // is what the assertion reads.
          day={undefined}
          week={undefined}
        />
      </LocaleProvider>,
    );
    const greeting =
      screen.getByRole("heading", { level: 1 }).textContent ?? "";
    view.unmount();
    return greeting;
  }

  it("names four bands, at the boundaries of each", () => {
    expect(greetingAt(5, "Ada")).toBe("Good morning, Ada.");
    expect(greetingAt(11, "Ada")).toBe("Good morning, Ada.");
    expect(greetingAt(12, "Ada")).toBe("Good afternoon, Ada.");
    expect(greetingAt(17, "Ada")).toBe("Good afternoon, Ada.");
    expect(greetingAt(18, "Ada")).toBe("Good evening, Ada.");
    expect(greetingAt(21, "Ada")).toBe("Good evening, Ada.");
    expect(greetingAt(22, "Ada")).toBe("Good evening, Ada.");
    expect(greetingAt(4, "Ada")).toBe("Good evening, Ada.");
  });

  // The hour is known before the name is. Greeting nobody until /me answers
  // would move the heading under the reader a moment after they read it.
  it("greets the hour with no name at all while the session is in flight", () => {
    expect(greetingAt(9, null)).toBe("Good morning.");
    expect(greetingAt(2, null)).toBe("Good evening.");
  });

  it("draws no line for a reading it was not given", () => {
    rtlRender(
      <LocaleProvider initial="en">
        <BriefGlance
          view="morning"
          firstName="Ada"
          now={new Date(2026, 6, 5, 9, 0, 0)}
          day={undefined}
          week={undefined}
        />
      </LocaleProvider>,
    );
    // Not one of them, and in particular not the "nothing is waiting" claim: an
    // unread queue is not an empty one.
    // The header never states a count. Every fact those lines carried is drawn
    // by the section that owns it — the deck, the readings strip, the rail's
    // panels — so an unread queue leaves the header saying only the greeting.
    expect(screen.queryByTestId("glance-sentence")).toBeNull();
    expect(screen.queryByText("Nothing is waiting on you.")).toBeNull();
  });
});

// ── The weekly speaks about its own week ──

describe("BriefGlance — the weekly's sentence comes from the closed week", () => {
  // Only what the sentence reads. A fuller review would let this suite pass
  // over a composer reaching for a figure the weekly does not actually carry.
  const CLOSED_WEEK = {
    local_week_start: "2026-06-29",
    counts: {
      tasks_due: 0,
      tasks_done: 0,
      tasks_carried_over: 0,
      deals_moved: 0,
      deals_won: 2,
      deals_lost: 0,
      proposals_accepted: 0,
      proposals_rejected: 0,
      brief_items_acted: 0,
      brief_items_dismissed: 0,
      commitments_due: 3,
      commitments_kept: 1,
      leads_routed: 0,
      leads_answered_in_target: 0,
      leads_breached: 0,
      meetings_held: 0,
      meetings_with_next_step: 0,
    },
  } as unknown as Parameters<typeof BriefGlance>[0]["week"];

  function sentenceOf(
    view: BriefView,
    week: Parameters<typeof BriefGlance>[0]["week"],
  ): string | null {
    const rendered = rtlRender(
      <LocaleProvider initial="en">
        <BriefGlance
          view={view}
          firstName="Ada"
          now={new Date(2026, 6, 5, 9, 0, 0)}
          // A read day, so the MORNING case has a sentence of its own to draw.
          // Without it both views would fall silent and the assertion that they
          // say different things would pass over a component saying neither.
          day={readingsDay({})}
          week={week}
        />
      </LocaleProvider>,
    );
    const text = screen.queryByTestId("glance-sentence")?.textContent ?? null;
    rendered.unmount();
    return text;
  }

  it("states the week's result and what it left behind", () => {
    const said = sentenceOf("weekly", CLOSED_WEEK);
    expect(said).toContain("2 deals won");
    expect(said).toContain("2 promises");
  });

  // The two views compose from different reads. Over the weekly the morning's
  // sentence would be describing today under a heading about a closed week.
  it("does not put the morning's sentence under the weekly's heading", () => {
    expect(sentenceOf("weekly", CLOSED_WEEK)).not.toBe(
      sentenceOf("morning", CLOSED_WEEK),
    );
  });

  // A week still in flight, or one that failed to read. The heading names the
  // view and claims nothing about it.
  it("draws no sentence at all over a week it has not read", () => {
    expect(sentenceOf("weekly", undefined)).toBeNull();
  });
});

// ── The opening block is two lines, and one of them is a control ──
