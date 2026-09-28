// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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

function show(current: string | null, refused = false) {
  render(<StageLadder label="Stage" steps={pipeline(current, refused)} />);
  const [run, exits] = within(
    screen.getByRole("group", { name: "Stage" }),
  ).getAllByRole("list");
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

describe("a long pipeline", () => {
  it("keeps the ways out apart from the run a scroll can hide", () => {
    const { run, exits } = show("Negotiation");

    expect(run.tagName).toBe("OL");
    expect(within(run).getAllByRole("listitem").map(stageName)).toEqual(OPEN);
    expect(
      within(exits)
        .getAllByRole("button")
        .map((exit) => exit.textContent),
    ).toEqual(["Won", "Lost"]);
  });

  it("comes to rest with the current stage in the middle of the run", () => {
    const { scroller } = show("Negotiation");

    expect(scroller?.scrollLeft).toBe(centredOn(OPEN.indexOf("Negotiation")));
  });

  it("rests a record that took a way out on the last stage it climbed", () => {
    const { scroller } = show("Won");

    expect(scroller?.scrollLeft).toBe(centredOn(OPEN.length - 1));
  });

  it("returns to the start once the pipeline cannot place the record", () => {
    const { rerender } = render(
      <StageLadder label="Stage" steps={pipeline("Negotiation")} />,
    );
    rerender(<StageLadder label="Stage" steps={pipeline(null)} />);

    const [run] = screen.getAllByRole("list");
    expect(run.parentElement?.scrollLeft).toBe(0);
  });

  // Every rung of a refused ladder is a disabled button, which no keyboard
  // reaches — so the stages past the edge are reachable only by the run itself.
  it("makes a run past its edge a keyboard stop named for the ladder", () => {
    overflowing();
    const { run } = show("Negotiation", true);

    const region = screen.getByRole("region", { name: "Stage" });
    expect(region.tabIndex).toBe(0);
    expect(region).toContainElement(run);
  });

  it("adds no keyboard stop while the run fits", () => {
    show("Negotiation", true);

    expect(screen.queryByRole("region")).toBeNull();
  });
});
