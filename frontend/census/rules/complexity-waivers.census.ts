// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A waiver of the complexity or function-length rule in production code names
// the issue that splits the function. Without one, the waiver is where an
// oversized function stops being anybody's work. Test code carries no cap.

import type { CensusRule } from "../tree";

const WAIVER =
  /biome-ignore\s+[^:]*lint\/complexity\/noExcessive(?:CognitiveComplexity|LinesPerFunction)[^:]*:(.*)$/;
const ISSUE = /#\d+\b|\/issues\/\d+\b/;
const TEST_CODE = /\.(test|testkit|stories)\.tsx?$|\/testing\//;

/** The lines holding a complexity waiver whose reason names no issue. */
export function unnamedWaivers(source: string): number[] {
  return source.split("\n").flatMap((line, i) => {
    const waiver = WAIVER.exec(line);
    return waiver && !ISSUE.test(waiver[1]) ? [i + 1] : [];
  });
}

export const rule: CensusRule = {
  name: "complexity-waivers",
  corpus: (tree) =>
    tree.files.filter(
      (file) => file.area === "src" && /\.tsx?$/.test(file.path),
    ),
  floors: (corpus) =>
    corpus.length > 1000 ? [] : ["1000 source files or fewer were read"],
  visit: (file) =>
    TEST_CODE.test(file.path)
      ? []
      : unnamedWaivers(file.text).map((line) => `${file.rel}:${line}`),
  advice:
    "A biome-ignore of noExcessiveCognitiveComplexity or " +
    "noExcessiveLinesPerFunction in production code names no issue. Name " +
    "the issue that splits the function in its reason.",
};
