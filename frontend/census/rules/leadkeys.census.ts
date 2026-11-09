// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A lead's cached reads are keyed in one place: src/lib/leadkeys.ts.
//
// The list is `["leads", query]` and the detail page is the sibling
// `["lead", id]`. React Query invalidates by prefix, and a prefix does not
// reach a sibling. So a write that names only the list leaves the open page
// stale. A test of the module cannot see a site that never called it. So this
// rule reads every array literal under src/ whose head is a lead root.
//
// Out of scope: a key built from a variable root, a literal stored in a
// variable first, and the list root as a bare string (`key: "leads"`). The
// last feeds read sites of the list, not per-lead writes.

import ts from "typescript";
import { parseSource } from "../../scripts/lib/source-tree";
import type { CensusFile, CensusRule } from "../tree";

// `record-history` is the shared history key of every record kind. Only a
// `"lead"` second element makes it a lead key.
const LEAD_ROOTS = ["lead", "leads", "lead-promote-preview"];

// The two places a query key sits. Any other array literal is not a key:
// import.tsx passes `["lead", "company"]` as a list of record kinds.
const KEY_TAKING_CALLS = [
  "invalidateQueries",
  "removeQueries",
  "cancelQueries",
  "refetchQueries",
  "setQueryData",
  "getQueryData",
  "fetchQuery",
  "ensureQueryData",
];

function isQueryKeyPosition(node: ts.ArrayLiteralExpression): boolean {
  // Climb through casts, parentheses and `!`: `as const` is the usual key idiom.
  let node_: ts.Node = node;
  while (
    node_.parent !== undefined &&
    (ts.isAsExpression(node_.parent) ||
      ts.isParenthesizedExpression(node_.parent) ||
      ts.isNonNullExpression(node_.parent) ||
      ts.isTypeAssertionExpression(node_.parent))
  ) {
    node_ = node_.parent;
  }
  const parent = node_.parent;
  if (
    parent !== undefined &&
    ts.isPropertyAssignment(parent) &&
    parent.initializer === node_ &&
    ts.isIdentifier(parent.name) &&
    parent.name.text === "queryKey"
  ) {
    return true;
  }
  return (
    parent !== undefined &&
    ts.isCallExpression(parent) &&
    parent.arguments[0] === node_ &&
    ts.isPropertyAccessExpression(parent.expression) &&
    KEY_TAKING_CALLS.includes(parent.expression.name.text)
  );
}

/** `<file>:<line> <literal>` for every hand-spelled lead query key in one file. */
export function spelledIn(fileName: string, text: string): string[] {
  const source = parseSource(fileName, text);
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    if (ts.isArrayLiteralExpression(node) && node.elements.length > 0) {
      const [first, second] = node.elements;
      // A no-substitution template counts as a string, so a backtick does not escape.
      const head = ts.isStringLiteralLike(first) ? first.text : null;
      const isLeadRoot = head !== null && LEAD_ROOTS.includes(head);
      const isRecordHistoryOfALead =
        head === "record-history" &&
        second !== undefined &&
        ts.isStringLiteralLike(second) &&
        second.text === "lead";
      if ((isLeadRoot || isRecordHistoryOfALead) && isQueryKeyPosition(node)) {
        const { line } = source.getLineAndCharacterOfPosition(node.getStart());
        found.push(`${fileName}:${line + 1} ${node.getText()}`);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

// The module and its own tests, named rather than matched by a pattern.
const OWNERS = ["src/lib/leadkeys.ts", "src/lib/leadkeys.test.ts"];

// A file that never says "lead" cannot spell one of these keys.
const inCorpus = (file: CensusFile) =>
  file.area === "src" &&
  /\.tsx?$/.test(file.path) &&
  !OWNERS.includes(file.rel) &&
  file.text.includes("lead");

export const rule: CensusRule = {
  name: "leadkeys",
  corpus: (tree) => tree.files.filter(inCorpus),
  visit: (file) => spelledIn(file.rel, file.text),
  advice:
    "A lead's query key is spelled by hand here. Use the helpers in " +
    "src/lib/leadkeys.ts (leadKey, LEAD_LIST_KEY, leadWriteKeys) so a write " +
    "reaches the list and the detail page together.",
};
