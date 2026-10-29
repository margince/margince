/** @vitest-environment happy-dom */
import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { dayInZone, viewerZone } from "../../format/timezone";
import { en } from "../../i18n/en";
import { readStored, removeStored, STORAGE_KEYS } from "../../lib/storage";
import { digest, readingsDay } from "../../testing/brief.fixtures";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { BriefIntro, FIGURES_STAND_MS, LEAVE_MS, SHOW_MS } from "./brief.intro";

// The morning's greeting plays once a local day, from the page's first render.
// Its figures come only from today's read, and join it when that read lands.
// It never plays under reduced motion, and any key or press ends it.

// Eight in the morning on the viewer's own clock, which the greeting reads.
const NOW = new Date(2026, 8, 3, 8, 0, 0);
const TODAY = dayInZone(NOW.getTime(), viewerZone());
const SKIP = en["brief.intro.skip"];
// The curtain is named by everything it says, the way out included.
const curtain = { name: (name: string) => name.includes(SKIP) };

/** A day read this morning, the only read the greeting plays over. */
function todaysRead(read = readingsDay()) {
  return { ...read, as_of: NOW.toISOString() };
}

beforeEach(() => {
  removeStored(STORAGE_KEYS.briefIntroShown);
  stubApi({});
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function intro(day = todaysRead()) {
  return render(<BriefIntro firstName="Ada" day={day} now={NOW} />);
}

describe("BriefIntro", () => {
  it("plays on the first visit of the day and records the day", () => {
    intro();
    expect(screen.getByRole("button", curtain).textContent).toContain(
      "Good morning, Ada.",
    );
    expect(readStored(STORAGE_KEYS.briefIntroShown)).toBe(TODAY);
  });

  it("lifts away on its own, then leaves the page", () => {
    vi.useFakeTimers();
    try {
      intro();
      act(() => vi.advanceTimersByTime(SHOW_MS));
      expect(screen.getByRole("button", curtain).dataset.state).toBe("leaving");
      act(() => vi.advanceTimersByTime(LEAVE_MS));
      expect(screen.queryByRole("button", curtain)).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not play a second time on the same day", () => {
    intro().unmount();
    intro();
    expect(screen.queryByRole("button", curtain)).toBeNull();
  });

  it("rises before the day's read, so the page never shows first", () => {
    render(<BriefIntro firstName="Ada" day={undefined} now={NOW} />);
    const shown = screen.getByRole("button", curtain);
    expect(shown.textContent).toContain("Good morning, Ada.");
    expect(
      shown.querySelector(".brief-intro-figures[data-pending]"),
    ).not.toBeNull();
    expect(readStored(STORAGE_KEYS.briefIntroShown)).toBe(TODAY);
  });

  it("adds the figures when the day's read lands", () => {
    const view = render(
      <BriefIntro firstName="Ada" day={undefined} now={NOW} />,
    );
    view.rerender(
      <BriefIntro
        firstName="Ada"
        day={todaysRead(readingsDay({}, undefined, undefined, { total: 7 }))}
        now={NOW}
      />,
    );
    const figures = screen
      .getByRole("button", curtain)
      .querySelector(".brief-intro-figures");
    expect(figures?.hasAttribute("data-pending")).toBe(false);
    expect(figures?.textContent).toContain("7");
  });

  it("holds late figures long enough to read, then lifts", () => {
    vi.useFakeTimers();
    try {
      const view = render(
        <BriefIntro firstName="Ada" day={undefined} now={NOW} />,
      );
      act(() => vi.advanceTimersByTime(SHOW_MS - 500));
      view.rerender(
        <BriefIntro firstName="Ada" day={todaysRead()} now={NOW} />,
      );
      act(() => vi.advanceTimersByTime(FIGURES_STAND_MS - 1));
      expect(screen.getByRole("button", curtain).dataset.state).toBeUndefined();
      act(() => vi.advanceTimersByTime(1));
      expect(screen.getByRole("button", curtain).dataset.state).toBe("leaving");
    } finally {
      vi.useRealTimers();
    }
  });

  it("lifts on time when the day's read never lands", () => {
    vi.useFakeTimers();
    try {
      render(<BriefIntro firstName="Ada" day={undefined} now={NOW} />);
      act(() => vi.advanceTimersByTime(SHOW_MS));
      act(() => vi.advanceTimersByTime(LEAVE_MS));
      expect(screen.queryByRole("button", curtain)).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("leaves out the figures of a read from an earlier day", () => {
    const yesterday = new Date(NOW.getTime() - 24 * 60 * 60 * 1000);
    intro({
      ...readingsDay({}, undefined, undefined, { total: 7 }),
      as_of: yesterday.toISOString(),
    });
    const shown = screen.getByRole("button", curtain);
    expect(
      shown.querySelector(".brief-intro-figures[data-pending]"),
    ).not.toBeNull();
    expect(shown.textContent).not.toContain("7");
  });

  it("reads the greeting and its figures out, not only the way out", () => {
    intro();
    const name = screen.getByRole("button", curtain).textContent ?? "";
    expect(name).toContain("Good morning, Ada.");
    expect(name).toContain(en["brief.intro.waiting"]);
  });

  it("keeps nightly capture separate from current waiting work", async () => {
    stubApi({
      "GET /digest": () =>
        jsonResponse({
          ...digest,
          capture: { ...digest.capture, messages_synced: 42 },
          review: { ...digest.review, approvals_pending: 3 },
        }),
    });
    intro(todaysRead(readingsDay({}, undefined, undefined, { total: 1 })));
    await screen.findByText("Last nightly run: 42 emails synced.");
    const shown = screen.getByRole("button", curtain);
    const figures = shown.querySelector(".brief-intro-figures");
    expect(figures?.textContent).toContain("1");
    expect(figures?.textContent).not.toContain("3");
    expect(figures?.textContent).toContain(en["brief.intro.waiting"]);
  });

  it("never plays for a reader who asked for less motion", () => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query.includes("reduce"),
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
    }));
    intro();
    expect(screen.queryByRole("button", curtain)).toBeNull();
  });

  it("ends on any key", () => {
    intro();
    act(() => {
      globalThis.dispatchEvent(new KeyboardEvent("keydown", { key: "a" }));
    });
    expect(screen.queryByRole("button", curtain)).toBeNull();
  });

  it("ends on a press", async () => {
    const user = userEvent.setup();
    intro();
    await user.click(screen.getByRole("button", curtain));
    expect(screen.queryByRole("button", curtain)).toBeNull();
  });

  it("states the day's figures the page states", () => {
    intro(todaysRead(readingsDay({}, undefined, undefined, { total: 7 })));
    const shown = screen.getByRole("button", curtain);
    expect(shown.textContent).toContain("7");
    expect(shown.textContent).toContain(en["brief.intro.waiting"]);
  });
});
