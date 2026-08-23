// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  LEAD_LIST_KEY,
  leadKey,
  leadManualSignalsKey,
  leadPromotePreviewKey,
  leadScoreKey,
  leadWriteKeys,
} from "./leadkeys";

// Two halves. Below, what the keys ARE; here, that nobody spells one anywhere
// else — because the defect this module exists for is invisible to a test of
// the module itself.
//
// A lead's list is `["leads", query]` and its detail page is the SIBLING
// `["lead", id]`. React Query invalidates by PREFIX, and a prefix does not
// walk sideways, so a mutation naming only the list is a write that succeeds
// and shows nothing on the page the reader is looking at. That is not a
// theory: the board drag and the bulk assign both did exactly this, and the
// second one did it forty rows at a time.
//
// A hand-spelled key cannot be caught by asserting what the module returns —
// the whole failure is a site that never called it. So the subject set is
// derived: every array literal under src/ whose first element is one of the
// lead roots, found by parsing, with this module and its own tests excluded.
//
// WHAT THIS DOES NOT CATCH, and the scope is worth stating precisely because
// "one place" is a strong claim:
//
//   - a key built from a variable ROOT (`[root, id]`), or a literal assigned to
//     a variable first (`const key = ["lead", id]; q({ queryKey: key })`).
//     Following either needs the type checker; nothing in this tree writes one.
//   - the list root spelled as a BARE STRING rather than an array, which
//     `leads.list.tsx` really does: `useListQuery({ key: "leads" })` and
//     `<CreateAction invalidate="leads" />` both feed the same cache root
//     through helpers that take a string. Those are read sites of the list, not
//     per-lead write sites, and it is the per-lead half that had the defect —
//     but the claim above is true of the ARRAY spellings, and a reader deserves
//     to know that rather than discover it.
//
// A cast, a parenthesis and a non-null assertion do NOT escape: see
// isQueryKeyPosition.

const screensDir = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(screensDir, "..");

// The roots a lead's cached reads are filed under. `record-history` is NOT one
// of them: it is the shared history key every record kind uses, and history.tsx
// spells it generically from (kind, id). Only the `"lead"` SECOND element makes
// an instance of it a lead key, which the arm below reads.
const LEAD_ROOTS = ["lead", "leads", "lead-promote-preview"];

// The two positions a query key actually occupies. An array literal anywhere
// else is not one — `import.tsx` passes `["lead", "organization"]` as a record
// KIND list, and a gate that read it as a key would be reporting a defect that
// is not there, which is how a gate gets waived rather than fixed.
//
// The second arm is not speculative: this tree keys `setQueryData` calls
// positionally in ten places (deals, company, brief). No lead does today, and
// that is precisely why the arm has to exist — otherwise the one spelling this
// gate does not read is the escape hatch.
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
  // Climb through the wrappers that leave the literal in place. `as const` is
  // THE react-query key idiom, so a rule anchored on the literal's direct
  // parent would be blind to the spelling a new site is most likely to use —
  // and the caught-error census in this same sweep already unwraps casts, so
  // not doing it here would be an asymmetry inside one change.
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
function spelledIn(fileName: string, text: string): string[] {
  const source = ts.createSourceFile(
    fileName,
    text,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    if (ts.isArrayLiteralExpression(node) && node.elements.length > 0) {
      const [first, second] = node.elements;
      const head = ts.isStringLiteral(first) ? first.text : null;
      const isLeadRoot = head !== null && LEAD_ROOTS.includes(head);
      const isRecordHistoryOfALead =
        head === "record-history" &&
        second !== undefined &&
        ts.isStringLiteral(second) &&
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

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    return entry.isDirectory()
      ? sourceFiles(path)
      : /\.tsx?$/.test(entry.name)
        ? [path]
        : [];
  });
}

// The module itself, and this file. Named rather than pattern-matched: a
// pattern is how a second module quietly joins the exemption.
const OWNERS = ["screens/leadkeys.ts", "screens/leadkeys.test.ts"];

function handSpelled(): string[] {
  const files = sourceFiles(srcRoot)
    .map((file) => [relative(srcRoot, file), file] as const)
    .filter(([name]) => !OWNERS.includes(name))
    // A file that never says "lead" cannot spell one of these keys, and
    // parsing it anyway is what this gate would otherwise cost.
    .filter(([, file]) => readFileSync(file, "utf8").includes('"lead'));
  // A sweep that found no files and a tree with no defects look identical.
  expect(files.length).toBeGreaterThan(0);
  return files
    .flatMap(([name, file]) => spelledIn(name, readFileSync(file, "utf8")))
    .sort();
}

const PARSE_MS = 60_000;

describe("a lead's cached reads", () => {
  it(
    "are keyed in exactly one place",
    () => {
      const found = handSpelled();
      expect(found, `\n${found.join("\n")}\n`).toEqual([]);
    },
    PARSE_MS,
  );

  // The census's own defect test: a census of zero cannot tell a clean tree
  // from a blind detector. Each case is keyed on how a real site spelled it.
  const planted: ReadonlyArray<readonly [string, string, number]> = [
    ["the list", 'q({ queryKey: ["leads"] });', 1],
    ["the detail page", 'q({ queryKey: ["lead", id] });', 1],
    [
      "a child of the detail page",
      'q({ queryKey: ["lead", id, "score"] });',
      1,
    ],
    [
      "the promote preview's sibling root",
      'q({ queryKey: ["lead-promote-preview", id] });',
      1,
    ],
    [
      "the shared history key, narrowed to a lead",
      'q({ queryKey: ["record-history", "lead", id] });',
      1,
    ],
    [
      "two in one call, which a per-file flag would count once",
      'x({ queryKey: ["leads"] }); y({ queryKey: ["lead", id] });',
      2,
    ],
    // What must stay invisible.
    [
      "the module's own name in an import",
      'import { leadWriteKeys } from "./leadkeys";',
      0,
    ],
    [
      "the shared history key for another record kind",
      'q({ queryKey: ["record-history", "deal", id] });',
      0,
    ],
    [
      "a word that merely starts with lead",
      'q({ queryKey: ["leadership", id] });',
      0,
    ],
    [
      // The false positive the first draft of this gate reported. A gate that
      // cries about a record-kind list is a gate that gets waived.
      "a record-KIND list, which import.tsx really passes",
      'p({ options: ["lead", "organization"] as const });',
      0,
    ],
    [
      // The escape a reviewer found in the first draft: `as const` is the
      // standard react-query key idiom, so this is the spelling a new site is
      // MOST likely to use, not a corner.
      "a key wearing `as const`",
      'q({ queryKey: ["lead", id] as const });',
      1,
    ],
    [
      "a key in parentheses, and one behind a non-null assertion",
      'a({ queryKey: (["leads"]) }); b({ queryKey: ["lead", id] });',
      2,
    ],
    [
      "the positional spelling ten setQueryData calls in this tree use",
      'queryClient.setQueryData(["lead", id], next);',
      1,
    ],
    [
      "the adopted spelling",
      "for (const key of leadWriteKeys(id)) q({ queryKey: key });",
      0,
    ],
  ];
  for (const [name, source, expected] of planted) {
    it(`sees ${name}`, () => {
      expect(spelledIn("planted.tsx", source)).toHaveLength(expected);
    });
  }
});

describe("the keys themselves", () => {
  it("files the detail page as a SIBLING of the list, which is the whole problem", () => {
    // Stated as an assertion rather than a comment: if these two ever share a
    // prefix, the module is solving a problem that no longer exists and the
    // next reader deserves to be told by a failure, not by archaeology.
    expect(leadKey("l-1")).toEqual(["lead", "l-1"]);
    expect(LEAD_LIST_KEY).toEqual(["leads"]);
    expect(leadKey("l-1")[0]).not.toEqual(LEAD_LIST_KEY[0]);
  });

  it("reaches the detail page's children by prefix", () => {
    for (const child of [leadScoreKey("l-1"), leadManualSignalsKey("l-1")]) {
      expect(child.slice(0, 2)).toEqual(leadKey("l-1"));
    }
  });

  it("carries the list, the detail page and the history on every write", () => {
    expect(leadWriteKeys("l-1")).toEqual([
      ["leads"],
      ["lead", "l-1"],
      ["record-history", "lead", "l-1"],
    ]);
  });

  it("leaves the promote preview out, because invalidating it would change nothing", () => {
    // staleTime 0 + enabled-on-open: it refetches every time the dialog opens,
    // and an inactive query is refetched on its next mount either way.
    expect(leadWriteKeys("l-1")).not.toContainEqual(
      leadPromotePreviewKey("l-1"),
    );
  });
});
