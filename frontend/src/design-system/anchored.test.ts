// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

// Which side of its trigger a portalled panel opens on, and how tall it may be.
//
// Stated over the measurements themselves, because the test environment gives
// every element a zero-sized rectangle: there is no laid-out page here to read
// a placement off. Beside the module that owns the rule rather than inside any
// one suite, because three components share it — the popover, the evidence
// mark's receipt and the overflow menu — and stating it inside the menu's tests
// is what made it read as a rule about menus.

import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { type VerticalPlacement, verticalPlacement } from "./anchored";

const VIEWPORT = 800;

beforeEach(() => {
  vi.stubGlobal("innerHeight", VIEWPORT);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

// A real DOMRect, not a two-field literal cast into the shape: a box whose
// `top` was supplied and whose height was not is a box no element has, and
// `verticalPlacement` is free to read a field the literal never spelled.
const near = (top: number) => new DOMRect(0, top, 0, 30);

// The two edges a reader actually sees, for a panel that wants `natural` px.
// What it draws is the smaller of what it wants and what it was allowed, and
// which edge is anchored decides where the other one lands — so this is the
// only way to ask the question the panels are contracted on: is it on screen?
function panelEdges(at: VerticalPlacement, natural: number, view = VIEWPORT) {
  const drawn = Math.min(natural, at.maxHeight);
  if (at.bottom !== undefined) {
    const lower = view - at.bottom;
    return { upper: lower - drawn, lower };
  }
  if (at.top !== undefined) {
    return { upper: at.top, lower: at.top + drawn };
  }
  throw new Error("a placement anchors the panel by its top or by its bottom");
}

// The rule all three panels rest on. They are FIXED, so the viewport is all the
// room there is: a panel hanging past the bottom edge puts its own controls
// where no amount of page scrolling reaches them.
it("keeps the panel inside the viewport from every trigger position", () => {
  // Taller than the VIEWPORT, not merely taller than the room. A fixture that
  // only outgrows the room is bounded by whatever cap the placement returns,
  // so it cannot reach an edge the cap itself overshot — and the one trigger
  // position able to break the rule is then the one the fixture is too small to
  // exercise. This panel is larger than every cap there can be.
  const TALL = VIEWPORT + 100;
  // The last two are a trigger scrolled OFF the bottom and off the top: the
  // panel stays open while the page moves under it, so these are ordinary
  // frames rather than freak ones.
  for (const top of [0, 100, 400, 680, 760, 790, 820, -130]) {
    const at = verticalPlacement(near(top));
    const edges = panelEdges(at, TALL);
    expect(edges.upper, `trigger at ${top}`).toBeGreaterThanOrEqual(0);
    expect(edges.lower, `trigger at ${top}`).toBeLessThanOrEqual(VIEWPORT);
    // And the cap itself, stated rather than inferred from where a panel of one
    // particular height happened to land: a cap bigger than the screen is the
    // defect whatever is drawn under it.
    expect(at.maxHeight, `the cap at ${top}`).toBeLessThanOrEqual(VIEWPORT);
    expect(at.maxHeight, `the cap at ${top}`).toBeGreaterThanOrEqual(0);
  }
});

// The technique, not just the outcome. A flipped panel positioned by its `top`
// needs its own rendered height — and at placement time the only height there
// is to read is the one this hook has already imposed on the panel, which is
// how a panel came to be placed 26px above a trigger and drawn 143px tall.
it("anchors a flipped panel by its bottom, never by a height it cannot know", () => {
  const at = verticalPlacement(near(VIEWPORT - 40));

  expect(at.top).toBeUndefined();
  if (at.bottom === undefined) {
    throw new Error("a flipped panel must be anchored by its own bottom edge");
  }
  // Its lower edge lands above the trigger's own top, whatever it grows to.
  expect(VIEWPORT - at.bottom).toBeLessThanOrEqual(VIEWPORT - 40);
  expect(at.maxHeight).toBeLessThanOrEqual(VIEWPORT - 40);
});

it("hangs the panel from the trigger while the room below can hold one", () => {
  const at = verticalPlacement(near(100));

  expect(at.bottom).toBeUndefined();
  if (at.top === undefined) {
    throw new Error("a panel with room below it must be anchored by its top");
  }
  expect(at.top).toBeGreaterThanOrEqual(130);
  expect(at.top + at.maxHeight).toBeLessThanOrEqual(VIEWPORT);
});

// Neither side can hold a panel worth reading, so it takes the roomier one and
// is capped to it — a panel allowed more room than there is runs off the screen
// with the controls it was opened for below the edge.
it("takes the roomier side when neither side has room for a panel", () => {
  vi.stubGlobal("innerHeight", 200);

  const at = verticalPlacement(near(90));

  expect(at.maxHeight).toBeGreaterThan(0);
  expect(at.maxHeight).toBeLessThan(200);
  expect(panelEdges(at, 240, 200).upper).toBeGreaterThanOrEqual(0);
  expect(panelEdges(at, 240, 200).lower).toBeLessThanOrEqual(200);
});

// A negative max-height is not a length: the browser drops the declaration
// entirely, and the panel it was meant to cap opens uncapped.
//
// Reached from a trigger LARGER than the room it sits in — a popover opened by
// a whole sentence, wrapped over several lines at 200% zoom, whose own box is
// taller than the 400px viewport that zoom leaves it. There is no room on
// either side of such a trigger, so nought is the honest answer and the panel
// is drawn empty; what this pins is that the cap is a LENGTH, because the
// alternative is the declaration being dropped and the panel opening uncapped
// over a screen that had no room for it at all.
it("never asks for a negative height", () => {
  vi.stubGlobal("innerHeight", 400);

  const at = verticalPlacement(new DOMRect(0, 4, 0, 404));

  expect(at.maxHeight).toBe(0);
  expect(panelEdges(at, 240, 400).lower).toBeLessThanOrEqual(400);
});
