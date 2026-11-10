// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One module says how a filter operator reads: filtersentence.ts. A second map
// from an operator to its words drifts until a Live List and the builder's
// picker name one clause differently. So outside that module no shipped source
// holds a string literal naming a `filters.op.` message.
//
// The extension frontend layers ship in the same bundle and speak the same
// catalog, so they are read too.

import { relative, sep } from "node:path";
import ts from "typescript";
import { type CensusFile, type CensusRule, repoRoot } from "../tree";

export const OWNER = "frontend/src/features/lists/filtersentence.ts";
const OPERATOR_FAMILY = "filters.op";

// The analytics question engine has operators of its own (`ne`, `is_null`,
// `is_not_null`) that no filter tree carries, so it words them itself.
export const EXEMPT: ReadonlyMap<string, string> = new Map([
  [
    "frontend/src/features/analytics/analytics.questions.vocab.ts",
    "maps the analytics engine's own operator enum, not FilterOp",
  ],
]);

// Tests, stories and kits render nothing a reader sees, and the catalogs are
// where the words are defined.
export function shipped(path: string): boolean {
  return (
    !/\.(test|stories|testkit)\.[cm]?[jt]sx?$/.test(path) &&
    !path.startsWith("frontend/src/i18n/")
  );
}

/** A whole key, or the bare head a key is composed from, but no neighbour. */
function namesOperatorFamily(text: string): boolean {
  return text === OPERATOR_FAMILY || text.startsWith(`${OPERATOR_FAMILY}.`);
}

/** Every string literal in this source that names an operator's message. */
export function operatorLiterals(source: ts.SourceFile): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node): void => {
    if (
      (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) &&
      namesOperatorFamily(node.text)
    ) {
      found.push(node.text);
    }
    // A key composed at run time still names the family by its head.
    if (ts.isTemplateExpression(node) && namesOperatorFamily(node.head.text)) {
      found.push(node.head.text);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

const fromRepo = (file: CensusFile) =>
  relative(repoRoot, file.path).split(sep).join("/");

const writersIn = (corpus: readonly CensusFile[]) =>
  new Map(
    corpus.flatMap((file): [string, string[]][] => {
      const literals = file.source ? operatorLiterals(file.source) : [];
      return literals.length > 0 ? [[fromRepo(file), literals]] : [];
    }),
  );

export const rule: CensusRule = {
  name: "filtersentence",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        file.source !== undefined &&
        shipped(fromRepo(file)),
    ),
  floors: (corpus) => [
    ...(writersIn(corpus).get(OWNER)?.includes("filters.op.eq")
      ? []
      : [`${OWNER} spells no filters.op.eq, so the scan does not reach it`]),
    ...(corpus.some((file) => file.area === "extensions")
      ? []
      : ["the walk reached no extension frontend module"]),
  ],
  across: (corpus) => {
    const writers = writersIn(corpus);
    return [
      ...[...writers.keys()]
        .filter((path) => path !== OWNER && !EXEMPT.has(path))
        .map((path) => `${path}: spells a filters.op. message`),
      ...[...EXEMPT]
        .filter(([path]) => !writers.has(path))
        .map(
          ([path, why]) =>
            `${path}: no longer spells a filters.op. message, so its exemption (${why}) is stale and must go`,
        ),
    ];
  },
  advice:
    "Say the clause through frontend/src/features/lists/filtersentence.ts " +
    "(operatorKey, clauseWords, filterSentence) instead of spelling a " +
    "filters.op. message.",
};
