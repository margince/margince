/** @vitest-environment happy-dom */
import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { filesMatching } from "../../scripts/lib/source-tree";
import * as viewport from "./viewport";
import {
  FOLD_MAX_WIDTH,
  NARROW_MAX_WIDTH,
  PHONE_MAX_WIDTH,
  usePhoneViewport,
  widthMedia,
} from "./viewport";

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
    media: widthMedia(PHONE_MAX_WIDTH),
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

// A stylesheet cannot read a width this module names, so each block spells the
// number. A block names the width it sits nearest, within the smallest gap
// between two named widths, and must spell that width exactly.

const sourceRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

type NamedWidth = { name: string; px: number };

/** Every `*_MAX_WIDTH` this module exports. */
function namedWidths(): NamedWidth[] {
  return Object.entries(viewport).flatMap(([name, px]) =>
    name.endsWith("_MAX_WIDTH") && typeof px === "number" ? [{ name, px }] : [],
  );
}

type UpperWidth = { prelude: string; px: number; exclusive: boolean };

// `max-width: N`, `width <= N` and `width < N`, and the reversed `N >= width`
// and `N > width`. A lower bound (`min-width`, `width >= N`) never names a fold.
const UPPER_BOUND =
  /max-width\s*:\s*(?<max>[\d.]+)(?<maxUnit>px|rem|em)|(?<![\w-])width\s*(?<lt><=?)\s*(?<below>[\d.]+)(?<belowUnit>px|rem|em)|(?<above>[\d.]+)(?<aboveUnit>px|rem|em)\s*(?<gt>>=?)\s*width(?![\w-])/g;

function toPx(size: string | undefined, unit: string | undefined): number {
  return Number(size) * (unit === "px" ? 1 : 16);
}

/** The upper widths a sheet's `@media` preludes test, in px, comments left out. */
function mediaWidths(css: string): UpperWidth[] {
  const code = css.replaceAll(/\/\*[\s\S]*?\*\//g, "");
  return [...code.matchAll(/@media([^{]+)\{/g)].flatMap(([, prelude]) =>
    [...prelude.matchAll(UPPER_BOUND)].map(({ groups = {} }) => ({
      prelude: prelude.trim(),
      px: toPx(
        groups.max ?? groups.below ?? groups.above,
        groups.maxUnit ?? groups.belowUnit ?? groups.aboveUnit,
      ),
      exclusive: groups.lt === "<" || groups.gt === ">",
    })),
  );
}

/** The named width nearest `px`, unless it is further off than two named widths. */
function namedWidthOf(px: number): NamedWidth | undefined {
  const named = namedWidths();
  const band = Math.min(
    ...named.flatMap((one) =>
      named
        .filter((two) => two !== one)
        .map((two) => Math.abs(one.px - two.px)),
    ),
  );
  const [nearest] = [...named].sort(
    (one, two) => Math.abs(px - one.px) - Math.abs(px - two.px),
  );
  return nearest && Math.abs(px - nearest.px) < band ? nearest : undefined;
}

/** What is wrong with a block that names `width`, or nothing. */
function drift(found: UpperWidth, width: NamedWidth): string | undefined {
  if (found.exclusive) {
    return `spells a strict "<", which leaves ${width.px}px itself out of ${width.name}`;
  }
  if (found.px !== width.px) {
    return `spells ${found.px}px where ${width.name} is ${width.px}px`;
  }
  return undefined;
}

describe("the named widths in the stylesheets", () => {
  it("reads every upper width a media query can spell", () => {
    const read = (css: string) =>
      mediaWidths(css).map(({ px, exclusive }) => ({ px, exclusive }));
    const upTo = (px: number) => [{ px, exclusive: false }];
    const below = (px: number) => [{ px, exclusive: true }];
    expect(read("@media (max-width: 700px) { a {} }")).toEqual(upTo(700));
    expect(read("@media (width <= 699px) { a {} }")).toEqual(upTo(699));
    expect(read("@media (width < 40rem) { a {} }")).toEqual(below(640));
    expect(read("@media (720px >= width) { a {} }")).toEqual(upTo(720));
    expect(read("@media (45em > width) { a {} }")).toEqual(below(720));
    expect(read("@media (pointer: coarse) and (max-width: 701px) {")).toEqual(
      upTo(701),
    );
    expect(read("@media (600px <= width <= 1100px) { a {} }")).toEqual(
      upTo(1100),
    );
    expect(read("/* @media (max-width: 690px) { */ a {}")).toEqual([]);
    expect(read("@media (min-width: 700px) { a {} }")).toEqual([]);
    expect(read("@media (width >= 700px) { a {} }")).toEqual([]);
    expect(read("@container (max-width: 700px) { a {} }")).toEqual([]);
  });

  it("gives a width to the named width it sits nearest, and leaves the rest", () => {
    const names = namedWidths();
    expect(names.map(({ name }) => name)).toEqual(
      expect.arrayContaining([
        "PHONE_MAX_WIDTH",
        "FOLD_MAX_WIDTH",
        "NARROW_MAX_WIDTH",
      ]),
    );
    for (const width of names) {
      expect(namedWidthOf(width.px)).toEqual(width);
    }
    expect(namedWidthOf(PHONE_MAX_WIDTH - 1)?.name).toBe("PHONE_MAX_WIDTH");
    expect(namedWidthOf(FOLD_MAX_WIDTH + 5)?.name).toBe("FOLD_MAX_WIDTH");
    expect(namedWidthOf(NARROW_MAX_WIDTH + 1)?.name).toBe("NARROW_MAX_WIDTH");
    for (const unnamed of [600, 640, 760, 1024]) {
      expect(namedWidthOf(unnamed)).toBeUndefined();
    }
  });

  it("spells each named width exactly in every block that names it", () => {
    const sheets = filesMatching(sourceRoot, /\.css$/);
    const blocks = sheets.flatMap((sheet) =>
      mediaWidths(readFileSync(sheet, "utf8")).flatMap((found) => {
        const width = namedWidthOf(found.px);
        return width
          ? [{ ...found, width, sheet: relative(sourceRoot, sheet) }]
          : [];
      }),
    );
    // A walk that found no sheet, or no block for a width, would pass a drifted tree.
    expect(sheets.length).toBeGreaterThan(100);
    for (const { name } of namedWidths()) {
      expect(
        blocks.filter(({ width }) => width.name === name).length,
        `no stylesheet block names ${name}`,
      ).toBeGreaterThan(0);
    }
    expect(
      blocks.flatMap((block) => {
        const wrong = drift(block, block.width);
        return wrong
          ? [`${block.sheet}: @media ${block.prelude} ${wrong}`]
          : [];
      }),
      "a block spells the width lib/viewport.ts names for it",
    ).toEqual([]);
  });
});
