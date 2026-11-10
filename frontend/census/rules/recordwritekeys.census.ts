// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A record is served under several query keys. A restore callback that
// invalidates by hand names one of them and leaves the others stale, so every
// onRestored goes through invalidateRecord in src/lib/recordwritekeys.ts.

import ts from "typescript";
import type { CensusFile, CensusRule } from "../tree";

/** Every onRestored in one file whose callback invalidates by hand. */
export function handInvalidations(source: ts.SourceFile): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isPropertyAssignment(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === "onRestored" &&
      node.initializer.getText().includes("invalidateQueries")
    ) {
      found.push(node.initializer.getText().replace(/\s+/g, " "));
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

// Stories and tests supply their own no-op callbacks and are not writers.
const inCorpus = (file: CensusFile) =>
  file.area === "src" &&
  /\.tsx?$/.test(file.path) &&
  !/\.(test|stories)\.tsx?$/.test(file.path);

export const rule: CensusRule = {
  name: "recordwritekeys",
  corpus: (tree) => tree.files.filter(inCorpus),
  visit: (file) =>
    file.source
      ? handInvalidations(file.source).map((text) => `${file.rel}: ${text}`)
      : [],
  advice:
    "A restore callback spelling its own query key invalidates one read of a " +
    "record that is served under several; call invalidateRecord instead.",
};
