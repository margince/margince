// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A `.sort()` or `.toSorted()` with no comparator orders every element by its
// text. For strings that is code-unit order; for numbers it puts 10 before 9.
// Neither says which order the author meant. So every sort names one:
// `stable` or `forReader` from format/collate.ts for strings, a numeric
// comparator for numbers.
//
// The census reads the call in the syntax tree, so a comment or a string that
// mentions a bare sort is no finding. The corpus is the app, its scripts, and
// the census's own files, which the walked tree leaves out.

import { join, relative, sep } from "node:path";
import ts from "typescript";
import {
  filesMatching,
  MODULE_FILE,
  parseSource,
  sourceTextAt,
} from "../../scripts/lib/source-tree";
import { type CensusFile, type CensusRule, frontendRoot } from "../tree";

const censusDir = join(frontendRoot, "census");

/** The census's own modules, read as files the walked tree does not hold. */
function censusFiles(): CensusFile[] {
  return filesMatching(censusDir, MODULE_FILE).map((path) => {
    const text = sourceTextAt(path);
    return {
      path,
      rel: relative(frontendRoot, path).split(sep).join("/"),
      area: "scripts",
      text,
      source: parseSource(path, text),
    };
  });
}

/** The line of every `.sort()` or `.toSorted()` called with no comparator. */
export function bareSorts(source: ts.SourceFile): number[] {
  const lines: number[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isCallExpression(node) &&
      node.arguments.length === 0 &&
      ts.isPropertyAccessExpression(node.expression) &&
      (node.expression.name.text === "sort" ||
        node.expression.name.text === "toSorted")
    ) {
      const at = node.expression.name.getStart(source);
      lines.push(source.getLineAndCharacterOfPosition(at).line + 1);
    }
    node.forEachChild(visit);
  };
  visit(source);
  return lines;
}

export const rule: CensusRule = {
  name: "bare-sort",
  corpus: (tree) => [
    ...tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "scripts") &&
        file.source !== undefined,
    ),
    ...censusFiles(),
  ],
  floors: (corpus) =>
    corpus.some((file) => file.rel.startsWith("census/"))
      ? []
      : ["the corpus holds none of the census's own files"],
  visit: (file) =>
    file.source === undefined
      ? []
      : bareSorts(file.source).map(
          (line) => `${file.rel}:${line}: sorts with no comparator`,
        ),
  advice:
    "These sort by each element's text because no comparator is given. Pass " +
    "`stable` for a key or `forReader` for a list somebody reads " +
    "(format/collate.ts), or `(a, b) => a - b` for numbers.",
};
