// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
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

function pipeline(current: string | null): StageStep[] {
  const here = current === null ? -1 : OPEN.indexOf(current);
  const open = OPEN.map((label, index) => ({
    key: label,
    label,
    done: here >= 0 && index < here,
    current: label === current,
    onPick: () => undefined,
  }));
  const exits = ["Won", "Lost"].map((label) => ({
    key: label,
    label,
    terminal: true,
    current: label === current,
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
      if (this.tagName === "OL") {
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

function show(current: string | null) {
  render(<StageLadder label="Stage" steps={pipeline(current)} />);
  const [run, exits] = within(
    screen.getByRole("group", { name: "Stage" }),
  ).getAllByRole("list");
  return { run, exits };
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
    const { run } = show("Negotiation");

    expect(run.scrollLeft).toBe(centredOn(OPEN.indexOf("Negotiation")));
  });

  it("rests a record that took a way out on the last stage it climbed", () => {
    const { run } = show("Won");

    expect(run.scrollLeft).toBe(centredOn(OPEN.length - 1));
  });

  it("leaves a record the pipeline cannot place at the start", () => {
    const { run } = show(null);

    expect(run.scrollLeft).toBe(0);
  });
});
