// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  filesUnder,
  scriptKindFor,
} from "../../scripts/lib/source-tree";
import { en } from "./en";

// A count that decides between two wordings decides it in SOME language's plural
// rule, and the only question is whose. This product's answer is the reader's
// own, through `Intl.PluralRules` in `format/plural.ts`. A `count === 1` at the
// call site answers it differently — it answers "a language with exactly two
// forms, splitting at one", which is English, German and Vietnamese and not
// Polish, Russian or Arabic.
//
// Fifteen sites were each answering it for themselves, correctly, and would each
// have been wrong the day a fourth catalogue shipped — individually, with no
// single place to fix them. That is the shape this gate holds closed: outside
// `i18n/`, a count does not pick a message key.
//
// THE SUBJECT IS DERIVED, which is the load-bearing part, because every gate in
// this tree written against a rule like this was later found to have hard-coded
// a fragment of its own subject and gone blind exactly there. What makes a
// conditional a plural choice here is not a naming convention and not a suffix
// pattern: it is that BOTH ARMS ARE KEYS THE CATALOGUE CARRIES. The catalogue is
// imported, so a key added tomorrow is in scope tomorrow, and a pair of
// arbitrary strings that happen to look key-shaped is not in scope at all.
//
// Two things that are deliberately NOT findings, because neither answers the
// plural question:
//
//   - a conditional between two keys on something other than a count
//     (`isPaid ? "x" : "y"`) — that is a state choice, and states have no
//     plural rule;
//   - a count comparison that produces anything but a pair of message keys —
//     a width, an index, a boolean.
//
// What IS a finding is narrow and specific: a comparison of something against
// the literal 1, whose two branches are both keys in the catalogue.

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(here, "..");
const frontendRoot = resolve(srcRoot, "..");
const extensionsDir = resolve(frontendRoot, "..", "extensions");

// The one home for the rule, and so the one directory allowed to compare a
// count against 1 to reach a key. The DIRECTORY here rather than a single file,
// because the rule is split across two modules on purpose — `format/plural.ts`
// owns the locale's categories and `i18n/index.tsx` owns the catalogue lookup —
// and both are its implementation.
const ruleHome = here;
const pluralModule = join(srcRoot, "format", "plural.ts");
const thisGate = fileURLToPath(import.meta.url);

const CATALOG_KEYS: ReadonlySet<string> = new Set(Object.keys(en));

type Finding = Readonly<{ file: string; line: number; text: string }>;

function sourceFiles(): string[] {
  return [...filesUnder(srcRoot), ...extensionFrontendFiles(extensionsDir)]
    .filter(
      (path) =>
        !path.startsWith(`${ruleHome}/`) &&
        path !== pluralModule &&
        path !== thisGate,
    )
    .sort();
}

/** Whether this expression is a comparison of something against the literal 1. */
function comparesAgainstOne(test: ts.Expression): boolean {
  if (!ts.isBinaryExpression(test)) {
    return false;
  }
  const op = test.operatorToken.kind;
  const isEquality =
    op === ts.SyntaxKind.EqualsEqualsEqualsToken ||
    op === ts.SyntaxKind.EqualsEqualsToken ||
    op === ts.SyntaxKind.ExclamationEqualsEqualsToken ||
    op === ts.SyntaxKind.ExclamationEqualsToken ||
    op === ts.SyntaxKind.GreaterThanToken ||
    op === ts.SyntaxKind.LessThanToken ||
    op === ts.SyntaxKind.GreaterThanEqualsToken ||
    op === ts.SyntaxKind.LessThanEqualsToken;
  if (!isEquality) {
    return false;
  }
  // Either side, because `1 === count` says the same thing and a gate that read
  // only one order would be a gate with a way around it that costs a keystroke.
  const one = (node: ts.Node): boolean =>
    ts.isNumericLiteral(node) && node.text === "1";
  return one(test.left) || one(test.right);
}

/**
 * The catalogue key this expression is, or null.
 *
 * A string literal or a no-substitution template, since both are keys a caller
 * can write. Anything computed is not a key this gate can rule on, and is
 * therefore not a finding — the census below states that limit rather than
 * leaving it to be discovered.
 */
function catalogKey(node: ts.Expression): string | null {
  const text = ts.isStringLiteral(node)
    ? node.text
    : ts.isNoSubstitutionTemplateLiteral(node)
      ? node.text
      : null;
  return text !== null && CATALOG_KEYS.has(text) ? text : null;
}

function findingsIn(path: string, source: string): Finding[] {
  const parsed = ts.createSourceFile(
    path,
    source,
    ts.ScriptTarget.Latest,
    true,
    scriptKindFor(path),
  );
  const rel = relative(srcRoot, path).split("\\").join("/");
  const found: Finding[] = [];
  const walk = (node: ts.Node): void => {
    if (
      ts.isConditionalExpression(node) &&
      comparesAgainstOne(node.condition)
    ) {
      const whenTrue = catalogKey(node.whenTrue);
      const whenFalse = catalogKey(node.whenFalse);
      if (whenTrue && whenFalse) {
        found.push({
          file: rel,
          line:
            parsed.getLineAndCharacterOfPosition(node.getStart(parsed)).line +
            1,
          text: `${whenTrue} / ${whenFalse}`,
        });
      }
    }
    ts.forEachChild(node, walk);
  };
  walk(parsed);
  return found;
}

describe("one plural rule", () => {
  it("finds no count-picks-the-key outside i18n/", () => {
    const files = sourceFiles();
    // Fail closed. A walk pointed at the wrong tree reports PASS over nothing,
    // and under-recognition is the one way a gate must not break.
    expect(files.length).toBeGreaterThan(100);

    const findings = files.flatMap((path) =>
      findingsIn(path, readFileSync(path, "utf8")),
    );

    expect(
      findings.map((f) => `${f.file}:${f.line} — ${f.text}`),
      "A count picks a message key outside i18n/. Use usePlural() and a " +
        "<base>_one / <base>_other pair, so the form comes from the reader's " +
        "own plural rule rather than from a comparison with 1.",
    ).toEqual([]);
  });

  // The gate's own census. It reads a smaller tree than it claims, or matches a
  // shape the real defect does not take, and it reports PASS either way — so
  // both halves are measured against planted source rather than assumed.
  it("sees the defect in every spelling it takes", () => {
    const both = Object.keys(en).slice(0, 2);
    const [first, second] = both;
    const cases = [
      `const label = n === 1 ? "${first}" : "${second}";`,
      `const label = 1 === n ? "${first}" : "${second}";`,
      `const label = n !== 1 ? "${first}" : "${second}";`,
      `const label = n > 1 ? "${first}" : "${second}";`,
      `const label = t(rows === 1 ? "${first}" : "${second}", {});`,
      `const label = n === 1 ? \`${first}\` : \`${second}\`;`,
    ];
    for (const source of cases) {
      expect(
        findingsIn(join(srcRoot, "planted.ts"), source),
        `not seen: ${source}`,
      ).toHaveLength(1);
    }
  });

  it("rules on nothing it has no standing to rule on", () => {
    const both = Object.keys(en).slice(0, 2);
    const [first, second] = both;
    const allowed = [
      // A state choice, not a plural one.
      `const label = isPaid ? "${first}" : "${second}";`,
      // A count comparison that produces no keys.
      `const width = n === 1 ? 10 : 20;`,
      // Key-shaped strings the catalogue does not carry: not this gate's
      // business, and pretending otherwise would make it fail on any two
      // strings with a dot in them.
      `const label = n === 1 ? "not.a.key" : "also.not.a.key";`,
      // Half a pair. One arm being a key is not a choice between wordings.
      `const label = n === 1 ? "${first}" : someOtherThing;`,
    ];
    for (const source of allowed) {
      expect(
        findingsIn(join(srcRoot, "planted.ts"), source),
        `wrongly flagged: ${source}`,
      ).toHaveLength(0);
    }
  });

  it("reads the extension tier, not frontend/src alone", () => {
    // The unit trees ship UI in the same bundle, in the same three languages. A
    // gate stopping at frontend/src would hold the core to a standard the
    // extension tier escapes, which is the wrong way round — and it is the
    // failure this repo has already had, twice, in the design-system gates.
    const files = sourceFiles();
    const extensionFiles = extensionFrontendFiles(extensionsDir);
    for (const path of extensionFiles) {
      expect(files, `${path} is not being read`).toContain(path);
    }
  });
});
