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

/** An expression with its parentheses taken off. A reader's brackets are not a
 *  different rule, and a scan that stopped at one would read
 *  `lead.full_name ?? (lead.email || "")` as a chain of two terms whose second
 *  is nothing it recognises. */
function unwrap(expression: ts.Expression): ts.Expression {
  return ts.isParenthesizedExpression(expression)
    ? unwrap(expression.expression)
    : expression;
}

/** The operands of one fallback chain, left to right. `a ?? b ?? c` parses as
 *  `(a ?? b) ?? c`, so the chain has to be flattened before the ORDER of the
 *  fields can be read — and the order is the whole distinction between this
 *  rule and the address-first one. BOTH sides are flattened: `??` groups to the
 *  left on its own, but nothing stops an author from writing the nesting out. */
function chainOperands(node: ts.BinaryExpression): ts.Expression[] {
  return [node.left, node.right].flatMap((side) => {
    const inner = unwrap(side);
    return ts.isBinaryExpression(inner) &&
      isFallbackOperator(inner.operatorToken.kind)
      ? chainOperands(inner)
      : [inner];
  });
}

/** Whether this chain is the whole of its own expression. An inner link is
 *  already one of the outer chain's operands, so reporting it again would name
 *  one fallback twice — and the parentheses have to be climbed through, or a
 *  bracketed inner chain reads as its own. */
function isOutermostChain(node: ts.BinaryExpression): boolean {
  let child: ts.Node = node;
  let parent: ts.Node = node.parent;
  while (ts.isParenthesizedExpression(parent)) {
    child = parent;
    parent = parent.parent;
  }
  return !(
    ts.isBinaryExpression(parent) &&
    isFallbackOperator(parent.operatorToken.kind) &&
    (parent.left === child || parent.right === child)
  );
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
      isOutermostChain(node)
    ) {
      const terms = chainOperands(node).map(fieldRead);
      // EVERY name in the chain is asked, not the first one: an
      // organization-first label puts somebody else's `full_name` ahead of the
      // lead's, and a scan that stopped at the first would look for the
      // address under the wrong receiver and find nothing.
      const named = terms.some(
        (term, at) =>
          term?.field === "full_name" &&
          terms
            .slice(at + 1)
            .some(
              (later) =>
                later?.field === "email" && later.receiver === term.receiver,
            ),
      );
      if (named) {
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
    [
      // The reader's own brackets, which change nothing about the rule.
      "a bracketed tail, where the address sits inside its own chain",
      'const name = lead.full_name ?? (lead.email || "");',
      1,
    ],
    [
      // Somebody else's name first. A scan reading only the first `full_name`
      // looks for the address under THAT receiver and reports nothing.
      "another record's name ahead of the lead's own",
      "const name = person.full_name ?? lead.full_name ?? lead.email;",
      1,
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
