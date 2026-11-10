// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Every source the queue can draw has a story that draws it.
//
// A story is where a reader looks at a row. A row with a useless verb set or
// no address passes every unit test, and a glance at a story catches it. The
// vocabulary comes from KNOWN_SOURCES, so a second copy cannot go stale.

import { readFileSync } from "node:fs";
import { basename, dirname, join } from "node:path";
import ts from "typescript";
import { storySuffixes } from "../../scripts/lib/story-files";
import { KNOWN_SOURCES } from "../../src/lib/worklistcopy";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
  frontendRoot,
  srcDir,
} from "../tree";

const storiesDir = join(srcDir, "features", "worklist");

// `batch` stands for a pile of other rows, so it has no row of its own to draw.
const standsForOtherRows = "batch";

const sources = Object.keys(KNOWN_SOURCES).filter(
  (source) => source !== standsForOtherRows,
);

// Read off Storybook's own globs, so a new kind of story file joins the corpus.
const mainPath = join(frontendRoot, ".storybook", "main.ts");
const suffixes = storySuffixes(mainPath, readFileSync(mainPath, "utf8")) ?? [];

const isWorklistStory = (file: CensusFile) =>
  dirname(file.path) === storiesDir &&
  basename(file.path).startsWith("worklist.") &&
  suffixes.some((suffix) => file.path.endsWith(suffix));

/** Every queue source a story file sets on a worklist item. */
export function drawnIn(
  corpus: readonly CensusFile[],
  program: ts.Program,
): Set<string> {
  const checker = program.getTypeChecker();
  const drawn = new Set<string>();
  const visit = (node: ts.Node): void => {
    if (
      ts.isPropertyAssignment(node) &&
      node.name.getText() === "source" &&
      ts.isObjectLiteralExpression(node.parent)
    ) {
      // A contextual Partial<WorklistItem> override counts too. A verdict's
      // provenance has no category, so it cannot pass for a queue source.
      const owner =
        checker.getContextualType(node.parent) ??
        checker.getTypeAtLocation(node.parent);
      const value = checker.getTypeAtLocation(node.initializer);
      const hasCategory = node.parent.properties.some(
        (property) => property.name?.getText() === "category",
      );
      if (
        (hasCategory || owner.getProperty("category")) &&
        value.isStringLiteral()
      )
        drawn.add(value.value);
    }
    ts.forEachChild(node, visit);
  };
  for (const file of corpus) {
    const source = program.getSourceFile(file.path);
    if (!source) throw new Error(`Story was not loaded: ${file.rel}`);
    visit(source);
  }
  return drawn;
}

// The floors and the findings read one walk of the stories.
const drawnByTree = new WeakMap<CensusTree, Set<string>>();
function drawnOnce(corpus: readonly CensusFile[], tree: CensusTree) {
  const known = drawnByTree.get(tree);
  if (known) return known;
  const drawn = drawnIn(corpus, tree.checked());
  drawnByTree.set(tree, drawn);
  return drawn;
}

export const rule: CensusRule = {
  name: "worklist.storycensus",
  corpus: (tree) => tree.files.filter(isWorklistStory),
  // A glob that matched nothing, or a KNOWN_SOURCES nobody reads, passes empty.
  floors: (corpus, tree) => [
    ...(sources.length > 15 ? [] : ["KNOWN_SOURCES holds 15 sources or fewer"]),
    ...(drawnOnce(corpus, tree).size > 15
      ? []
      : ["the worklist stories draw 15 sources or fewer"]),
  ],
  across: (corpus, tree) => {
    const drawn = drawnOnce(corpus, tree);
    const known = new Set(Object.keys(KNOWN_SOURCES));
    return [
      ...sources
        .filter((source) => !drawn.has(source))
        .map((source) => `${source}: no worklist story draws it`),
      ...[...drawn]
        .filter((source) => !known.has(source))
        .map(
          (source) => `${source}: a story draws it, the queue does not know it`,
        ),
    ];
  },
  advice:
    "Every source in KNOWN_SOURCES (src/lib/worklistcopy.ts) needs a " +
    "worklist.*.stories.tsx story that draws its row. Add the story, or drop " +
    "a storied source the queue no longer knows.",
};
