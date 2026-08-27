// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { filesUnder, scriptKindFor } from "../../scripts/lib/source-tree";

// Fitness function for a screen that works out for itself what a lead is
// called.
//
// The rule is one fact: a lead's own name if it has one, otherwise the email
// address that is the only other thing naming it. `format/leadname.ts` is where
// this application answers it, mirroring `leadIdentityName` in the people
// module and the SQL census that holds the query side.
//
// It is not a tidiness argument. The spelling this gate refuses —
// `lead.full_name ?? lead.email` — fires on `null` alone, so a lead carrying a
// present-but-empty `full_name` renders BLANK on every screen that spells it,
// while the server promotes the same lead into a person named by its address.
// Nothing between a `CreateLead` body and the stored row refuses an empty name,
// so that lead is one the product makes. Eight screens carried the spelling.
//
// The subject is DERIVED: every `??` or `||` chain under `src/` that falls back
// from a `full_name` to an `email`, found by parsing rather than by matching
// lines. Both operators, because the second is the same rule in a coat that
// happens to answer correctly today — and a second author of one rule is what
// this gate is about, not one operator.
//
// WHAT THIS DOES NOT CATCH, deliberately:
//
//  - `full_name ?? ""` with no address behind it. Seeding a form field with the
//    stored value is a different question, and empty is the right answer there.
//  - `email ?? full_name`. Address-first is a different rule, and a gate that
//    conflated the two would push somebody to "fix" a surface that meant it.
//  - a name resolved through a variable or a helper of the caller's own. That
//    needs the type checker, and a gate whose rules are fiddly is a gate that
//    gets worked around rather than fixed.

const formatDir = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(formatDir, "..");

/** The operators a fallback is spelled with. `||` is included because it is the
 *  same rule; `??` is the one that carries the defect. */
function isFallbackOperator(kind: ts.SyntaxKind): boolean {
  return (
    kind === ts.SyntaxKind.QuestionQuestionToken ||
    kind === ts.SyntaxKind.BarBarToken
  );
}

/** The operands of one fallback chain, left to right. `a ?? b ?? c` parses as
 *  `(a ?? b) ?? c`, so the chain has to be flattened before the ORDER of the
 *  two fields can be read — and the order is the whole distinction between this
 *  rule and the address-first one. */
function chainOperands(node: ts.BinaryExpression): ts.Expression[] {
  const left = node.left;
  const head =
    ts.isBinaryExpression(left) && isFallbackOperator(left.operatorToken.kind)
      ? chainOperands(left)
      : [left];
  return [...head, node.right];
}

/** One term of a chain: WHOSE field it reads, and which. The receiver is half
 *  the finding — `person.full_name ?? lead.email` reads two records and is not
 *  this rule, so a gate comparing property names alone would send somebody to
 *  "fix" a fallback that means what it says. */
type FieldRead = Readonly<{ receiver: string; field: string }>;

function fieldRead(expression: ts.Expression): FieldRead | null {
  const inner = ts.isParenthesizedExpression(expression)
    ? expression.expression
    : expression;
  if (ts.isPropertyAccessExpression(inner)) {
    return { receiver: inner.expression.getText(), field: inner.name.text };
  }
  if (ts.isElementAccessExpression(inner)) {
    const argument = inner.argumentExpression;
    return ts.isStringLiteral(argument)
      ? { receiver: inner.expression.getText(), field: argument.text }
      : null;
  }
  return null;
}

/** `<file>:<line>` for every hand-spelled lead name in one file. */
function fallbacksIn(fileName: string, text: string): string[] {
  const source = ts.createSourceFile(
    fileName,
    text,
    ts.ScriptTarget.Latest,
    true,
    // The file's OWN kind, from the one place this tree decides that. Under
    // `TSX` a plain `.ts` generic arrow — `<T>(value: T) => value` — parses as
    // an unclosed JSX element, and everything after it is parse recovery
    // rather than the tree this scan means to walk. A census that reads a
    // smaller tree reports PASS and nothing fails, which is the one way a gate
    // must not break.
    scriptKindFor(fileName),
  );
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    if (
      ts.isBinaryExpression(node) &&
      isFallbackOperator(node.operatorToken.kind) &&
      // The OUTERMOST link of the chain only: an inner one would report the
      // same fallback a second time under a different line.
      !(
        ts.isBinaryExpression(node.parent) &&
        isFallbackOperator(node.parent.operatorToken.kind) &&
        node.parent.left === node
      )
    ) {
      const terms = chainOperands(node).map(fieldRead);
      const name = terms.findIndex((term) => term?.field === "full_name");
      const address = terms.findIndex(
        (term, at) =>
          at > name &&
          term?.field === "email" &&
          term.receiver === terms[name]?.receiver,
      );
      if (name >= 0 && address > name) {
        found.push(
          `${fileName}:${source.getLineAndCharacterOfPosition(node.getStart()).line + 1}`,
        );
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

/** Every TypeScript source under `src/`, through the shared walk — which
 *  descends a symlinked directory a bundler would ship and this scan would
 *  otherwise read straight past. */
function sourceFiles(dir: string): string[] {
  return filesUnder(dir).filter((path) => /\.tsx?$/.test(path));
}

// The files that OWN the rule, named one by one rather than by directory: a
// second helper added beside `leadname.ts` would otherwise pass the census this
// test exists to keep at zero.
const LEAD_NAME_OWNERS = [
  "format/leadname.ts",
  "format/leadname.test.ts",
  "format/leadname-coverage.test.ts",
];

function fallbacks(): string[] {
  const files = sourceFiles(srcRoot)
    .map((file): readonly [string, string] => [relative(srcRoot, file), file])
    .filter(([name]) => !LEAD_NAME_OWNERS.includes(name));
  // A sweep that found no files and a tree with no fallbacks look identical.
  expect(files.length).toBeGreaterThan(0);
  return files
    .flatMap(([name, file]) => fallbacksIn(name, readFileSync(file, "utf8")))
    .sort();
}

const PARSE_MS = 60_000;

describe("what a lead is called", () => {
  it(
    "is worked out in exactly one place",
    () => {
      const found = fallbacks();
      expect(found, `\n${found.join("\n")}\n`).toEqual([]);
    },
    PARSE_MS,
  );

  const planted: ReadonlyArray<readonly [string, string, number]> = [
    [
      "the spelling eight screens carried",
      'const name = lead.full_name ?? lead.email ?? "";',
      1,
    ],
    [
      "the same fallback ending at the id",
      "const name = lead.full_name ?? lead.email ?? lead.id;",
      1,
    ],
    [
      "an organization-first label, which still names the lead at its second term",
      'const label = lead.company_name ?? lead.full_name ?? lead.email ?? "";',
      1,
    ],
    [
      "the rule inside JSX, where it is read rather than assigned",
      "const row = <strong>{lead.full_name ?? lead.email}</strong>;",
      1,
    ],
    [
      // The coat the gate would otherwise miss: correct today, and a second
      // author of the rule the moment either half moves.
      "the same rule spelled with ||",
      'const name = lead.full_name || lead.email || "";',
      1,
    ],
    [
      "a bracketed read, which a property-access-only rule would miss",
      'const name = lead["full_name"] ?? lead["email"];',
      1,
    ],
    [
      "a form field seeded from the stored name, where empty is the right answer",
      'const value = lead.full_name ?? "";',
      0,
    ],
    [
      "a person, who is not a lead and has no address to fall back to",
      "const name = data.full_name ?? null;",
      0,
    ],
    [
      "an address-first label, which is a different rule and means it",
      "const name = lead.email ?? lead.full_name;",
      0,
    ],
    [
      // Two records, one chain: a person's name with a lead's address behind
      // it is a fallback somebody meant, and calling it this rule would send
      // the next author to break it.
      "two different receivers, which is not one record being named",
      "const name = person.full_name ?? lead.email;",
      0,
    ],
  ];

  for (const [what, code, count] of planted) {
    it(`sees ${what}`, () => {
      expect(fallbacksIn("planted.tsx", code)).toHaveLength(count);
    });
  }

  // The shape a census goes blind to rather than the shape it reports: parsed
  // as TSX, a `.ts` generic arrow opens a JSX element that never closes, and
  // the fallback after it is inside parse recovery rather than the tree. The
  // same source under both extensions is the only assertion that can tell.
  const genericArrowThenFallback =
    "const id = <T>(value: T) => value;\nconst name = lead.full_name ?? lead.email;";

  it("reads a .ts generic arrow as TypeScript, so what follows it is still scanned", () => {
    expect(fallbacksIn("planted.ts", genericArrowThenFallback)).toHaveLength(1);
  });
});
