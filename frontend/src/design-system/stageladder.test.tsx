// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { clippedEdges } from "./scrollregion";
import { StageLadder, type StageStep } from "./stageladder";

// A pipeline is the workspace's to lengthen, so the ladder has to stay one
// readable row however many stages it is handed: the open stages scroll, the
// ways out stay put, and the row comes to rest where the record stands.

const OPEN = [
  "Inbound",
  "Discovery",
  "Qualified",
  "Demo",
  "Technical review",
  "Proposal",
  "Negotiation",
  "Legal review",
  "Procurement",
  "Signature",
];

function pipeline(current: string | null, refused = false): StageStep[] {
  const here = current === null ? -1 : OPEN.indexOf(current);
  const open = OPEN.map((label, index) => ({
    key: label,
    label,
    done: here >= 0 && index < here,
    current: label === current,
    disabled: refused,
    onPick: () => undefined,
  }));
  const exits = ["Won", "Lost"].map((label) => ({
    key: label,
    label,
    terminal: true,
    current: label === current,
    disabled: refused,
    onPick: () => undefined,
  }));
  return [...open, ...exits];
}

// The layout a browser would give the run: 300px of room, each rung 100px
// wide on a 120px pitch. happy-dom lays nothing out, so the geometry the
// ladder measures has to be supplied; the arithmetic on it is the ladder's.
const RUN_WIDTH = 300;
const PITCH = 120;
const RUNG_WIDTH = 100;

function rect(left: number, width: number) {
  return DOMRect.fromRect({ x: left, y: 0, width, height: 32 });
}

beforeEach(() => {
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(
    function (this: Element) {
      if (this.firstElementChild?.tagName === "OL") {
        return rect(0, RUN_WIDTH);
      }
      const rung = Array.prototype.indexOf.call(
        this.parentElement?.children ?? [],
        this,
      );
      return rect(rung * PITCH, RUNG_WIDTH);
    },
  );
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// Both shapes draw the one run, so each claim about it holds for both.
const SHAPES = ["chevrons", "bars"] as const;
type Shape = (typeof SHAPES)[number];

function show(current: string | null, refused = false, variant?: Shape) {
  render(
    <StageLadder
      label="Stage"
      variant={variant}
      steps={pipeline(current, refused)}
    />,
  );
  const group = screen.getByRole("group", { name: "Stage" });
  const run = group.querySelector("ol");
  const exits = group.querySelector("ul");
  if (!run || !exits) {
    throw new Error("The ladder drew no run or no ways out.");
  }
  // The box that scrolls holds the run rather than being it.
  return { run, exits, scroller: run.parentElement };
}

// A run holding four times the room it is given.
function overflowing() {
  vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(
    RUN_WIDTH * 4,
  );
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(
    RUN_WIDTH,
  );
}

// A rung's own words, without the track glyph that introduces it — decoration
// hidden from every reader but `textContent`.
function stageName(item: HTMLElement) {
  return item.textContent?.replace("›", "");
}

// Where the run's scroll would put a rung's centre: the middle of the room.
function centredOn(rung: number) {
  return rung * PITCH + RUNG_WIDTH / 2 - RUN_WIDTH / 2;
}

describe.each(SHAPES)("a long pipeline in the %s shape", (variant) => {
  it("keeps the ways out apart from the run a scroll can hide", () => {
    const { run, exits } = show("Negotiation", false, variant);

    expect(within(run).getAllByRole("listitem").map(stageName)).toEqual(OPEN);
    expect(
      within(exits)
        .getAllByRole("button")
        .map((exit) => exit.textContent),
    ).toEqual(["Won", "Lost"]);
  });

  it("comes to rest with the current stage in the middle of the run", () => {
    const { scroller } = show("Negotiation", false, variant);

    expect(scroller?.scrollLeft).toBe(centredOn(OPEN.indexOf("Negotiation")));
  });

  it("rests a record that took a way out on the last stage it climbed", () => {
    const { scroller } = show("Won", false, variant);

    expect(scroller?.scrollLeft).toBe(centredOn(OPEN.length - 1));
  });

  it("returns to the start once the pipeline cannot place the record", () => {
    const { rerender } = render(
      <StageLadder
        label="Stage"
        variant={variant}
        steps={pipeline("Negotiation")}
      />,
    );
    rerender(
      <StageLadder label="Stage" variant={variant} steps={pipeline(null)} />,
    );

    const run = screen.getByRole("group").querySelector("ol");
    expect(run?.parentElement?.scrollLeft).toBe(0);
  });

  // Every rung of a refused ladder is a disabled button, which no keyboard
  // reaches — so the stages past the edge are reachable only by the run itself.
  it("makes a run past its edge a keyboard stop named for the ladder", () => {
    overflowing();
    const { run } = show("Negotiation", true, variant);

    const region = screen.getByRole("region", { name: "Stage" });
    expect(region.tabIndex).toBe(0);
    expect(region).toContainElement(run);
  });

  it("adds no keyboard stop while the run fits", () => {
    show("Negotiation", true, variant);

    expect(screen.queryByRole("region")).toBeNull();
  });
});

describe("which ends of a sideways run are clipped", () => {
  const box = (scrollLeft: number, scrollWidth = 1200, clientWidth = 300) => ({
    scrollLeft,
    scrollWidth,
    clientWidth,
  });

  it("finds none while the content fits, a rounding pixel included", () => {
    expect(clippedEdges(box(0, 300))).toEqual({ start: false, end: false });
    expect(clippedEdges(box(0, 301))).toEqual({ start: false, end: false });
  });

  it("finds the end at the start, both in the middle, the start at the end", () => {
    expect(clippedEdges(box(0))).toEqual({ start: false, end: true });
    expect(clippedEdges(box(450))).toEqual({ start: true, end: true });
    expect(clippedEdges(box(900))).toEqual({ start: true, end: false });
  });

  it("reads a right-to-left scroll by its size", () => {
    expect(clippedEdges(box(-900))).toEqual({ start: true, end: false });
  });
});

describe.each(SHAPES)(
  "the buttons at a clipped end of the %s shape",
  (variant) => {
    const LATER = { name: "Show later stages" };
    const EARLIER = { name: "Show earlier stages" };
    const scrollBy = vi.fn();
    const browserScrollBy = HTMLElement.prototype.scrollBy;

    beforeEach(() => {
      scrollBy.mockReset();
      HTMLElement.prototype.scrollBy = scrollBy;
    });

    afterEach(() => {
      HTMLElement.prototype.scrollBy = browserScrollBy;
      vi.unstubAllGlobals();
    });

    it("draws none while the run fits", () => {
      show("Negotiation", false, variant);

      expect(screen.queryByRole("button", LATER)).toBeNull();
      expect(screen.queryByRole("button", EARLIER)).toBeNull();
    });

    it("draws only the later one for a run resting at its start", () => {
      overflowing();
      show(null, false, variant);

      expect(screen.getByRole("button", LATER)).toBeInTheDocument();
      expect(screen.queryByRole("button", EARLIER)).toBeNull();
    });

    it("draws both around a current stage in the middle of a long run", () => {
      overflowing();
      show("Negotiation", false, variant);

      expect(screen.getByRole("button", LATER)).toBeInTheDocument();
      expect(screen.getByRole("button", EARLIER)).toBeInTheDocument();
    });

    it("pages most of a row each way, smoothly", () => {
      overflowing();
      show("Negotiation", false, variant);

      fireEvent.click(screen.getByRole("button", LATER));
      fireEvent.click(screen.getByRole("button", EARLIER));

      expect(scrollBy.mock.calls).toEqual([
        [{ left: RUN_WIDTH * 0.75, behavior: "smooth" }],
        [{ left: -RUN_WIDTH * 0.75, behavior: "smooth" }],
      ]);
    });

    it("pages toward each end of a right-to-left run", () => {
      overflowing();
      // happy-dom computes direction from a style, never from `dir`.
      render(
        <div dir="rtl" style={{ direction: "rtl" }}>
          <StageLadder
            label="Stage"
            variant={variant}
            steps={pipeline("Negotiation")}
          />
        </div>,
      );

      fireEvent.click(screen.getByRole("button", LATER));
      fireEvent.click(screen.getByRole("button", EARLIER));

      expect(scrollBy.mock.calls).toEqual([
        [{ left: -RUN_WIDTH * 0.75, behavior: "smooth" }],
        [{ left: RUN_WIDTH * 0.75, behavior: "smooth" }],
      ]);
    });

    it("jumps instead of gliding for a reader who asked for less motion", () => {
      vi.stubGlobal("matchMedia", (query: string) => ({
        matches: query.includes("prefers-reduced-motion"),
        media: query,
        onchange: null,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
        addListener: () => undefined,
        removeListener: () => undefined,
        dispatchEvent: () => false,
      }));
      overflowing();
      show("Negotiation", false, variant);

      fireEvent.click(screen.getByRole("button", LATER));

      expect(scrollBy).toHaveBeenCalledWith({
        left: RUN_WIDTH * 0.75,
        behavior: "auto",
      });
    });

    it("takes the later button away once the run is scrolled to its end", () => {
      overflowing();
      const { scroller } = show("Negotiation", false, variant);
      if (!scroller) {
        throw new Error("no scroller");
      }

      act(() => {
        scroller.scrollLeft = RUN_WIDTH * 3;
        fireEvent.scroll(scroller);
      });

      expect(screen.queryByRole("button", LATER)).toBeNull();
      expect(screen.getByRole("button", EARLIER)).toBeInTheDocument();
    });

    it("hands focus to the last stage when the pressed button goes", () => {
      overflowing();
      const { scroller } = show("Negotiation", false, variant);
      if (!scroller) {
        throw new Error("no scroller");
      }
      const later = screen.getByRole("button", LATER);
      later.focus();
      fireEvent.click(later);

      act(() => {
        scroller.scrollLeft = RUN_WIDTH * 3;
        fireEvent.scroll(scroller);
      });

      expect(document.activeElement).toHaveTextContent("Signature");
    });
  },
);

describe("the bars shape", () => {
  afterEach(cleanup);

  const bars = (onNext = () => undefined): StageStep[] => [
    {
      key: "a",
      label: "New",
      done: true,
      sub: "2 Aug",
      onPick: () => undefined,
    },
    {
      key: "b",
      label: "Contacted",
      current: true,
      sub: "4 Aug · now",
      onPick: () => undefined,
    },
    { key: "c", label: "Engaged", onPick: onNext },
    { key: "won", label: "Qualified", terminal: true, onPick: () => undefined },
  ];

  it("says where the record stands and offers the next stage", () => {
    const onNext = vi.fn();
    render(<StageLadder label="Status" variant="bars" steps={bars(onNext)} />);
    expect(screen.getByText("Stage 2 of 3")).toBeInTheDocument();
    screen.getByRole("button", { name: /Move to Engaged/ }).click();
    expect(onNext).toHaveBeenCalledOnce();
  });

  it("draws the current stage as a marker with its sub line, not a button", () => {
    render(<StageLadder label="Status" variant="bars" steps={bars()} />);
    const here = document.querySelector('[aria-current="step"]');
    expect(here).toHaveTextContent("Contacted4 Aug · now");
    expect(here?.tagName).toBe("SPAN");
    expect(screen.getByText("2 Aug")).toBeInTheDocument();
  });

  it("offers no next move from the last open stage", () => {
    const last = bars().map((step) => ({
      ...step,
      current: step.key === "c",
    }));
    render(<StageLadder label="Status" variant="bars" steps={last} />);
    expect(screen.queryByRole("button", { name: /Move to/ })).toBeNull();
  });

  it("names a way out the record took as a chip, and no position", () => {
    const closed = bars().map((step) => ({
      ...step,
      current: step.key === "won",
    }));
    render(<StageLadder label="Status" variant="bars" steps={closed} />);
    const chip = within(
      screen.getByRole("group", { name: "Status" }),
    ).getByText("Qualified");
    expect(chip.closest('[aria-current="step"]')).not.toBeNull();
    expect(screen.queryByText(/Stage \d of/)).toBeNull();
  });
});
