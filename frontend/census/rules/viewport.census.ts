// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A stylesheet cannot read a width src/lib/viewport.ts names, so each block
// spells the number. A width that is neither named nor on the list of unnamed
// ones fails, so a named block that drifts fails at any distance.

import * as viewport from "../../src/lib/viewport";
import type { CensusFile, CensusRule } from "../tree";

type NamedWidth = { name: string; px: number };

/** Every `*_MAX_WIDTH` the viewport module exports. */
export function namedWidths(): NamedWidth[] {
  return Object.entries(viewport).flatMap(([name, px]) =>
    name.endsWith("_MAX_WIDTH") && typeof px === "number" ? [{ name, px }] : [],
  );
}

export type UpperWidth = { prelude: string; px: number; exclusive: boolean };

// `max-width: N`, `width <= N` and `width < N`, and the reversed `N >= width`
// and `N > width`. A lower bound (`min-width`, `width >= N`) never names a fold.
const UPPER_BOUND =
  /max-width\s*:\s*(?<max>[\d.]+)(?<maxUnit>px|rem|em)|(?<![\w-])width\s*(?<lt><=?)\s*(?<below>[\d.]+)(?<belowUnit>px|rem|em)|(?<above>[\d.]+)(?<aboveUnit>px|rem|em)\s*(?<gt>>=?)\s*width(?![\w-])/g;

function toPx(size: string | undefined, unit: string | undefined): number {
  return Number(size) * (unit === "px" ? 1 : 16);
}

/** The upper widths a sheet's `@media` preludes test, in px, comments left out. */
export function mediaWidths(css: string): UpperWidth[] {
  const code = css.replaceAll(/\/\*[\s\S]*?\*\//g, "");
  return [...code.matchAll(/@media([^{]+)\{/g)].flatMap(([, prelude = ""]) =>
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

// Every unnamed upper width the tree spells today. A listed width no sheet
// spells fails, so the list cannot hide a drift onto a stale entry.
export const UNNAMED_WIDTHS = [
  400, 480, 520, 560, 600, 640, 760, 768, 800, 899, 900, 960, 1000, 1024, 1200,
];

/** What is wrong with a block's upper width, or nothing. */
export function misspelling(found: UpperWidth): string | undefined {
  const named = namedWidths();
  const width = named.find(({ px }) => px === found.px);
  if (width) {
    return found.exclusive
      ? `spells a strict "<", which leaves ${width.px}px itself out of ${width.name}`
      : undefined;
  }
  if (UNNAMED_WIDTHS.includes(found.px)) {
    return undefined;
  }
  const [nearest] = [...named].sort(
    (one, two) => Math.abs(found.px - one.px) - Math.abs(found.px - two.px),
  );
  return `spells ${found.px}px, which is not ${nearest?.name} (${nearest?.px}px) or any named width, and is not in UNNAMED_WIDTHS`;
}

const spelledWidths = (corpus: readonly CensusFile[]) =>
  new Set(
    corpus.flatMap((sheet) => mediaWidths(sheet.text).map(({ px }) => px)),
  );

export const rule: CensusRule = {
  name: "viewport",
  corpus: (tree) =>
    tree.files.filter(
      (file) => file.area === "src" && file.path.endsWith(".css"),
    ),
  // A walk that found no sheet, or no block for a width, would pass a drifted tree.
  floors: (corpus) => {
    const spelled = spelledWidths(corpus);
    return [
      ...(corpus.length > 100 ? [] : ["100 stylesheets or fewer were read"]),
      ...namedWidths()
        .filter(({ px }) => !spelled.has(px))
        .map(({ name }) => `no stylesheet block names ${name}`),
    ];
  },
  across: (corpus) => {
    const spelled = spelledWidths(corpus);
    return [
      ...corpus.flatMap((sheet) =>
        mediaWidths(sheet.text).flatMap((block) => {
          const wrong = misspelling(block);
          return wrong
            ? [`${sheet.rel}: @media ${block.prelude} ${wrong}`]
            : [];
        }),
      ),
      ...UNNAMED_WIDTHS.filter((px) => !spelled.has(px)).map(
        (px) =>
          `${px}px is in UNNAMED_WIDTHS but no stylesheet spells it any more`,
      ),
    ];
  },
  advice:
    "A block spells the width src/lib/viewport.ts names for it. Use the named " +
    "width, or add a deliberate unnamed one to UNNAMED_WIDTHS.",
};
