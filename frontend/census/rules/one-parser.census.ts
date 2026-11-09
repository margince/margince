// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// scripts/lib/source-tree.ts is the one place the frontend calls the
// compiler's parser. Two parse sites can disagree about the dialect: one read
// a `.ts` file as TSX and reported a pass over code it had not parsed. A
// program parses every file it reads, so it counts as a parser too.

import type { CensusFile, CensusRule } from "../tree";

const OWNER = "scripts/lib/source-tree.ts";

// Spelled in two halves so this file does not name itself a parser.
const CALLS = ["ts.createSourceFile", "ts.createProgram"].map((name) =>
  [name, "("].join(""),
);

const inScope = (file: CensusFile) =>
  file.area === "src" ||
  file.area === "scripts" ||
  file.area === "e2e" ||
  file.area === "extensions";

export const rule: CensusRule = {
  name: "one-parser",
  corpus: (tree) => tree.files.filter(inScope),
  floors: (corpus) => [
    ...(corpus.length > 100 ? [] : ["fewer than 100 files were read"]),
    ...(corpus.some((file) => file.rel === OWNER)
      ? []
      : [`${OWNER} was not read`]),
  ],
  visit: (file) =>
    file.rel !== OWNER && CALLS.some((call) => file.text.includes(call))
      ? [`${file.rel} calls the compiler's parser`]
      : [],
  advice:
    "Parse through parseSource, sourceFileAt or the programs in " +
    "scripts/lib/source-tree.ts, so every gate reads one tree in one dialect.",
};
