/** @vitest-environment happy-dom */
import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { filesMatching } from "../../scripts/lib/source-tree";
import * as viewport from "./viewport";
import { PHONE_MAX_WIDTH, usePhoneViewport } from "./viewport";

// The width the chrome has to KNOW rather than merely be laid out by: at phone
// width the sidebar keeps its four destinations and a section moves into the page
// head, which is a decision in TypeScript rather than a layout a stylesheet makes.

afterEach(() => {
  vi.unstubAllGlobals();
});

// One media query, the way a browser hands it over: `matches` now, plus a
// subscription for when it stops being true.
function stubMatchMedia(matches: boolean) {
  const listeners = new Set<() => void>();
  const query = {
    matches,
    media: `(max-width: ${PHONE_MAX_WIDTH}px)`,
    addEventListener: (_: string, listener: () => void) =>
      listeners.add(listener),
    removeEventListener: (_: string, listener: () => void) =>
      listeners.delete(listener),
  };
  vi.stubGlobal("matchMedia", () => query);
  return {
    resizeTo(nowMatches: boolean) {
      query.matches = nowMatches;
      for (const listener of listeners) {
        listener();
      }
    },
    get subscribers() {
      return listeners.size;
    },
  };
}

describe("usePhoneViewport", () => {
  it("answers from the media query the browser gives it", () => {
    stubMatchMedia(true);
    expect(renderHook(() => usePhoneViewport()).result.current).toBe(true);
  });

  it("answers false above the breakpoint", () => {
    stubMatchMedia(false);
    expect(renderHook(() => usePhoneViewport()).result.current).toBe(false);
  });

  // Subscribed, not measured once: a window is resized and a phone is rotated
  // while the app is open, and chrome that read the width at mount would keep the
  // other width's arrangement for the rest of the session.
  it("follows the viewport across a resize, and lets go on unmount", () => {
    const media = stubMatchMedia(true);
    const { result, unmount } = renderHook(() => usePhoneViewport());
    expect(media.subscribers).toBe(1);

    act(() => media.resizeTo(false));
    expect(result.current).toBe(false);

    act(() => media.resizeTo(true));
    expect(result.current).toBe(true);

    unmount();
    expect(media.subscribers).toBe(0);
  });

  // Some embedded contexts have no `matchMedia` at all. A missing media query is
  // a DEFAULT, never an error: the answer is "not a phone", which is the
  // arrangement that works at any width.
  it("defaults to the wide arrangement where matchMedia is absent", () => {
    vi.stubGlobal("matchMedia", undefined);
    expect(renderHook(() => usePhoneViewport()).result.current).toBe(false);
  });
});

// A stylesheet cannot read PHONE_MAX_WIDTH, so each phone block spells the
// number. A block names the phone width when it is closer to it than the nearest
// other width this module names. Such a block must spell the same number.

const sourceRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

/** Every width this module names: PHONE_MAX_WIDTH and its siblings. */
function namedWidths(): number[] {
  return Object.entries(viewport).flatMap(([name, width]) =>
    name.endsWith("_MAX_WIDTH") && typeof width === "number" ? [width] : [],
  );
}

/** The upper widths a sheet's `@media` preludes test, in px, comments left out. */
function mediaWidths(css: string): { prelude: string; px: number }[] {
  const code = css.replaceAll(/\/\*[\s\S]*?\*\//g, "");
  return [...code.matchAll(/@media([^{]+)\{/g)].flatMap(([, prelude]) =>
    [
      ...prelude.matchAll(
        /(?:max-width\s*:|width\s*<=?)\s*([\d.]+)(px|rem|em)/g,
      ),
    ].map(([, size, unit]) => ({
      prelude: prelude.trim(),
      px: Number(size) * (unit === "px" ? 1 : 16),
    })),
  );
}

/** Whether `px` sits nearer the phone width than any other named width does. */
function namesThePhone(px: number): boolean {
  const gap = Math.min(
    ...namedWidths()
      .filter((width) => width !== PHONE_MAX_WIDTH)
      .map((width) => Math.abs(width - PHONE_MAX_WIDTH)),
  );
  return Math.abs(px - PHONE_MAX_WIDTH) < gap;
}

describe("the phone width in the stylesheets", () => {
  it("reads every upper width a media query can spell", () => {
    const read = (css: string) => mediaWidths(css).map((found) => found.px);
    expect(read("@media (max-width: 700px) { a {} }")).toEqual([700]);
    expect(read("@media (width <= 699px) { a {} }")).toEqual([699]);
    expect(read("@media (width < 40rem) { a {} }")).toEqual([640]);
    expect(read("@media (pointer: coarse) and (max-width: 701px) {")).toEqual([
      701,
    ]);
    expect(read("/* @media (max-width: 690px) { */ a {}")).toEqual([]);
    expect(read("@media (min-width: 700px) { a {} }")).toEqual([]);
  });

  it("takes a width near the phone's as naming it, and the others' as theirs", () => {
    expect(namedWidths()).toContain(PHONE_MAX_WIDTH);
    expect(namesThePhone(PHONE_MAX_WIDTH + 1)).toBe(true);
    expect(namesThePhone(PHONE_MAX_WIDTH - 1)).toBe(true);
    for (const other of namedWidths().filter((w) => w !== PHONE_MAX_WIDTH)) {
      expect(namesThePhone(other)).toBe(false);
    }
  });

  it("spells PHONE_MAX_WIDTH in every block that names the phone width", () => {
    const sheets = filesMatching(sourceRoot, /\.css$/);
    const blocks = sheets.flatMap((sheet) =>
      mediaWidths(readFileSync(sheet, "utf8"))
        .filter(({ px }) => namesThePhone(px))
        .map((found) => ({ ...found, sheet: relative(sourceRoot, sheet) })),
    );
    // A walk that found no sheet or no phone block would pass a drifted tree.
    expect(sheets.length).toBeGreaterThan(100);
    expect(blocks.length).toBeGreaterThan(0);
    expect(
      blocks
        .filter(({ px }) => px !== PHONE_MAX_WIDTH)
        .map(({ sheet, prelude }) => `${sheet}: @media ${prelude}`),
      `a phone block spells ${PHONE_MAX_WIDTH}px, the width lib/viewport.ts names`,
    ).toEqual([]);
  });
});
