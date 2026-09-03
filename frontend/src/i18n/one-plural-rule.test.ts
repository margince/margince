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
 *
 * A call carrying a key is the same choice one level down:
 * `n === 1 ? t("a") : t("b")` picks a wording exactly as `t(n === 1 ? …)` does,
 * and reading only the second spelling is how this gate first went blind to the
 * more common one. The CALLEE IS NOT MATCHED — not `t`, not a screen's own
 * `plural()` — because a gate that names its subject's spellings has hard-coded
 * part of its subject, and renaming the translator would be the way around it.
 * Any argument may be the key, since a translator that takes the locale first
 * (`translate(locale, "a")`) carries it second.
 */
function catalogKey(node: ts.Expression): string | null {
  if (ts.isParenthesizedExpression(node)) {
    return catalogKey(node.expression);
  }
  if (ts.isCallExpression(node)) {
    for (const argument of node.arguments) {
      const key = catalogKey(argument);
      if (key !== null) {
        return key;
      }
    }
    return null;
  }
  const text = ts.isStringLiteral(node)
    ? node.text
    : ts.isNoSubstitutionTemplateLiteral(node)
      ? node.text
      : null;
  return text !== null && CATALOG_KEYS.has(text) ? text : null;
}

// A count compared with 1 that picks between two catalogue keys is USUALLY a
// plural choice, and sometimes is not: "one open deal, so naming it adds
// nothing" chooses between two different sentences rather than two forms of one.
// Nothing derivable separates those — the tree's pairs are spelled `_one`,
// `One`, `Many`, and bare singular/plural nouns, so keying on the NAMES is the
// hard-coded-subject mistake this gate exists to avoid.
//
// So the author declares it, in the source, with a reason, the way
// `craft:ignore` does. A waiver with no reason is not a waiver: it reads as a
// keystroke that turns the gate off, which is exactly what the reason is for.
const WAIVER = /plural-rule:allow[ \t]+(?<reason>\S.*)/;

/**
 * Whether the comment attached above this site waives it, with a reason.
 *
 * The comment is read off the AST, from the STATEMENT the site sits in, rather
 * than by counting lines up from the expression. The formatter owns where lines
 * break: a waiver matched by line arithmetic comes undone the next time
 * anything either side of it is reflowed, and it comes undone SILENTLY — the
 * comment still reads as an exemption while the gate has started failing.
 * Anchoring on the statement is what a reader means by "the comment above this".
 */
function waivedAt(source: string, node: ts.Node): boolean {
  let statement: ts.Node = node;
  while (statement.parent && !ts.isStatement(statement)) {
    statement = statement.parent;
  }
  const comments = [
    ...(ts.getLeadingCommentRanges(source, statement.getFullStart()) ?? []),
    ...(ts.getLeadingCommentRanges(source, node.getFullStart()) ?? []),
  ];
  return comments.some((range) =>
    WAIVER.test(source.slice(range.pos, range.end)),
  );
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
      const line =
        parsed.getLineAndCharacterOfPosition(node.getStart(parsed)).line + 1;
      if (whenTrue && whenFalse && !waivedAt(source, node)) {
        found.push({
          file: rel,
          line,
          text: `${whenTrue} / ${whenFalse}`,
        });
      }
    }
    ts.forEachChild(node, walk);
  };
  walk(parsed);
  return found;
}

/**
 * This gate's own ceiling, because it does not belong to the population
 * `vitest.budget.ts` measures.
 *
 * That ceiling is arithmetic over WAITING — the longest chain of one-second
 * waiters a test legitimately composes, plus the slowest measured test. This
 * gate waits for nothing. It type-parses every source file in the tree, so its
 * cost is the corpus, and the two numbers have no relationship at all. It sat
 * under that ceiling only while the tree was small enough for the coincidence
 * to hold, and stopped when the tree grew: `Test timed out in 13437ms` on main,
 * on two consecutive runs, with every assertion in it passing.
 *
 * Derived per FILE and then multiplied out, not picked whole: 1600 files at
 * 40ms each. The corpus is ~1410 today, so the headroom is ~190 files — chosen
 * against this tree's merge rate rather than as a round margin, because a
 * budget with thirty files of slack is one that fails next week. Measured at ~3.3ms per file on an idle ten-core machine and
 * unchanged under eight spinners — the cost is the parse, not contention for a
 * core. A CI runner is smaller and saturated by the rest of the suite running
 * in parallel, and needed more than 12.5ms per file there, so the allowance is
 * an order of magnitude over the local measurement rather than a margin
 * trimmed to fit.
 *
 * Written as the product so the derivation is in the code and not only here.
 * scripts/test-budget.test.ts refuses a timeout it cannot statically fold —
 * correctly, since a ceiling no reader can evaluate is one no reader can audit
 * — and it rejected a first version that multiplied by `sourceFiles().length`
 * at runtime. Two constants fold; a directory walk does not.
 *
 * What that costs is that the number cannot scale ITSELF, so the corpus size it
 * assumes is asserted in the test body instead: the tree outgrowing this budget
 * fails by name and count rather than returning as an opaque timeout.
 */
const PARSE_BUDGET_PER_FILE_MS = 40;
const BUDGETED_CORPUS_FILES = 1_600;
const SCAN_TIMEOUT_MS = BUDGETED_CORPUS_FILES * PARSE_BUDGET_PER_FILE_MS;

describe("one plural rule", () => {
  it(
    "finds no count-picks-the-key outside i18n/",
    () => {
      const files = sourceFiles();
      // Fail closed. A walk pointed at the wrong tree reports PASS over
      // nothing, and under-recognition is the one way a gate must not break.
      expect(files.length).toBeGreaterThan(100);
      // And fail LOUDLY when the tree outgrows the ceiling's premise. The
      // ceiling above is a literal because scripts/test-budget.test.ts must be
      // able to read it, which means it cannot scale itself — so the premise it
      // rests on is asserted here instead. Without this the next growth spurt
      // returns as `Test timed out`, which names this test and not the reason.
      expect(files.length).toBeLessThanOrEqual(BUDGETED_CORPUS_FILES);

      const findings = files.flatMap((path) =>
        findingsIn(path, readFileSync(path, "utf8")),
      );

      expect(
        findings.map((f) => `${f.file}:${f.line} — ${f.text}`),
        "A count picks a message key outside i18n/. Use usePlural() and a " +
          "<base>_one / <base>_other pair, so the form comes from the reader's " +
          "own plural rule rather than from a comparison with 1.",
      ).toEqual([]);
    },
    SCAN_TIMEOUT_MS,
  );

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
      // The arms carrying the keys rather than the condition being inside the
      // call. This is the spelling the tree actually used, and the one the gate
      // read straight past on its first arming.
      `const label = n === 1 ? t("${first}") : t("${second}", { count });`,
      // A translator that takes the locale first, so the key is not argument
      // one. Matching a fixed position would be hard-coding the subject.
      `const label = n === 1 ? translate(locale, "${first}") : translate(locale, "${second}");`,
      `const label = n === 1 ? (t("${first}")) : (t("${second}"));`,
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
      // A call on both arms carrying no key at all — a count deciding between
      // two computations is not a wording choice.
      `const label = n === 1 ? formatOne(value) : formatMany(value);`,
      // A waived site: the author says why, on the line above.
      `// plural-rule:allow one open deal needs no name\nconst label = n === 1 ? t("${first}") : t("${second}");`,
    ];
    for (const source of allowed) {
      expect(
        findingsIn(join(srcRoot, "planted.ts"), source),
        `wrongly flagged: ${source}`,
      ).toHaveLength(0);
    }
  });

  it("does not accept a waiver with no reason", () => {
    // The waiver has to cost a sentence. A bare marker is a keystroke that
    // turns the gate off, and a gate with one of those is decoration.
    const [first, second] = Object.keys(en).slice(0, 2);
    const reasonless = [
      `// plural-rule:allow\nconst label = n === 1 ? t("${first}") : t("${second}");`,
      `// plural-rule:allow   \nconst label = n === 1 ? t("${first}") : t("${second}");`,
    ];
    for (const source of reasonless) {
      expect(
        findingsIn(join(srcRoot, "planted.ts"), source),
        `a reasonless waiver was honoured: ${source}`,
      ).toHaveLength(1);
    }
  });

  it("waives the site below the comment and not the next one down", () => {
    // One author's exemption must not become everybody's: the walk up stops at
    // the first line of code, so the second site here is still a finding.
    const [first, second] = Object.keys(en).slice(0, 2);
    const source = [
      `// plural-rule:allow this one is a naming choice`,
      `const first = n === 1 ? t("${first}") : t("${second}");`,
      `const second = n === 1 ? t("${first}") : t("${second}");`,
    ].join("\n");
    expect(findingsIn(join(srcRoot, "planted.ts"), source)).toHaveLength(1);
  });

  it("still reads a waiver whose reason wraps, and a split conditional", () => {
    // The formatter owns where lines break. A waiver that only matched when the
    // marker sat exactly one line above would come undone the next time
    // anything either side of it was reflowed, and it would come undone
    // silently — the comment would still read as an exemption.
    const [first, second] = Object.keys(en).slice(0, 2);
    const source = [
      `// plural-rule:allow naming the thing is a choice between two`,
      `// sentences rather than between two forms of one`,
      `const label =`,
      `  n === 1`,
      `    ? t("${first}")`,
      `    : t("${second}");`,
    ].join("\n");
    expect(findingsIn(join(srcRoot, "planted.ts"), source)).toHaveLength(0);
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
