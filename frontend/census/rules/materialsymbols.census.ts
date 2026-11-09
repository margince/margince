// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One module imports Material Symbols: design-system/materialsymbols.tsx,
// which reads the copied files and says how to add a missing glyph.

import { join } from "node:path";
import { type CensusFile, type CensusRule, srcDir } from "../tree";

export const OWNER = join(srcDir, "design-system", "materialsymbols.tsx");
// Its render test names the copied files' folder to hash them.
const OWNER_TEST = join(srcDir, "design-system", "materialsymbols.test.tsx");

// Raw text, not a parsed import: a template specifier, a `require`, an `.mdx`
// page or a CSS `url()` names the files where no string-literal import shows.
const MATERIAL_REFERENCE = /material-symbols|@material-["'`$]/g;
export const CORPUS_FILE = /\.(ts|tsx|mts|cts|js|jsx|mjs|cjs|mdx|css|html)$/;

export function materialReferenceLines(text: string): number[] {
  return [...text.matchAll(MATERIAL_REFERENCE)].map(
    (match) => text.slice(0, match.index).split("\n").length,
  );
}

const isCorpus = (file: CensusFile) =>
  (file.area === "src" ||
    file.area === "extensions" ||
    file.rel === "index.html") &&
  CORPUS_FILE.test(file.path);

export const rule: CensusRule = {
  name: "materialsymbols",
  corpus: (tree) => tree.files.filter(isCorpus),
  floors: (corpus) => {
    const owner = corpus.find((file) => file.path === OWNER)?.text ?? "";
    return [
      ...(corpus.length > 100 ? [] : ["100 or fewer files were read"]),
      ...(corpus.some((file) => file.path.endsWith(".css"))
        ? []
        : ["the census read no stylesheet"]),
      ...(corpus.some((file) => file.path.endsWith(".html"))
        ? []
        : ["the census read no HTML page"]),
      ...(materialReferenceLines(owner).length > 0
        ? []
        : ["the owner imports no mark, so the census reads the wrong module"]),
      ...(/["'`]@material-symbols\//.test(owner)
        ? ["the owner imports the npm package"]
        : []),
    ];
  },
  visit: (file) =>
    file.path === OWNER || file.path === OWNER_TEST
      ? []
      : materialReferenceLines(file.text).map((line) => `${file.rel}:${line}`),
  advice:
    "Import the glyph from src/design-system/materialsymbols.tsx, which says how to add a missing one.",
};
