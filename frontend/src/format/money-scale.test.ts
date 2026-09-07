import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";

// The TypeScript half of the money-scale census: an amount already in minor
// units, scaled by a hard-coded power of ten.
//
// A currency with no minor unit (VND, JPY, KRW) is understated a hundredfold by
// `/ 100`, and a three-decimal one (KWD) is overstated tenfold. `format/minorunits`
// owns the conversion, mirroring the Go table, and this refuses the second
// spelling of it.
//
// This was awk until it was not. Six defects shipped in that machinery and none
// was in the rule — a backslash meaning two different things in two grammars, a
// flat flag that cannot express a nested template, a `//` inside a regex literal
// read as a comment. Every one is answered for nothing by the parser the
// project already depends on.
//
// The RULE is shared with the Go half and so is the corpus: the cases below are
// read from backend/gates/testdata/sourcecensus.json, and a Go gate refuses a
// rule arm planted in one language and not the other. Two parsers, one rule.

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(here, "..");
const repoRoot = join(here, "..", "..", "..");
const corpusPath = join(
  repoRoot,
  "backend",
  "gates",
  "testdata",
  "sourcecensus.json",
);

// The floor below which the walk has plainly stopped reaching the tree. Not a
// target: the tripwire for the day a moved directory turns this into a census
// of nothing that still passes.
const FILE_FLOOR = 200;

// How long the whole-tree scan is allowed. Generous on purpose: it parses every
// hand-written file in src/, and a runner slower than a laptop must fail on a
// FINDING rather than on a clock.
const TREE_SCAN_TIMEOUT_MS = 120_000;

const MINOR_UNIT_NAME = /[Mm]inor[A-Za-z_]*|MINOR[A-Z_]*/;
const HARD_CODED_SCALES = new Set(["10", "100", "1000", "10000"]);
const WAIVER = "money-scale-exempt:";

type Finding = { line: number; text: string };

type PlantedCase = {
  gate: string;
  lang: string;
  arm: string;
  want: "fires" | "silent" | "unclosed";
  name: string;
  body: string;
};

// scaleFindings is the rule. It answers a parse failure separately from a clean
// read, because a scanner that could not finish a file has gone blind and
// reporting OK over the rest of it is the one failure a census must not have.
function scaleFindings(
  path: string,
  source: string,
): { findings: Finding[]; unreadable: boolean } {
  const file = parseSource(path, source);
  if (unfinished(file)) return { findings: [], unreadable: true };
  const waived = waivedLines(source, file);
  const findings: Finding[] = [];
  const visit = (node: ts.Node): void => {
    if (ts.isBinaryExpression(node) && scalesByAPowerOfTen(node)) {
      const unit = enclosing(node);
      if (namesAMinorUnit(unit)) {
        const line = lineOf(file, unit.getStart(file));
        const last = lineOf(file, unit.getEnd());
        if (!waivedBetween(waived, line, last)) {
          findings.push({
            line,
            text: node.getText(file).replace(/\s+/g, " "),
          });
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(file, visit);
  return { findings, unreadable: false };
}

// unfinished reports a file the scanner could not finish reading, which is a
// narrower thing than a file with a syntax error.
//
// Only two constructs swallow the REST of a file: a template literal and a
// block comment, both of which run to the end once opened. An unterminated
// single-line string ends at the newline, so the parser reads on and the code
// after it is still judged — which is why the awk's own "a backtick in a regex
// literal" case is no longer a refusal but a finding: the parser reads the
// regex as a regex.
function unfinished(file: ts.SourceFile): boolean {
  const diagnostics = (
    file as unknown as { parseDiagnostics?: ts.DiagnosticWithLocation[] }
  ).parseDiagnostics;
  if (!diagnostics) return false;
  return diagnostics.some((d) => {
    const message = ts.flattenDiagnosticMessageText(d.messageText, " ");
    return (
      message.includes("Unterminated template literal") ||
      message.includes("'*/' expected")
    );
  });
}

function scalesByAPowerOfTen(node: ts.BinaryExpression): boolean {
  const operator = node.operatorToken.kind;
  const scaling =
    operator === ts.SyntaxKind.SlashToken ||
    operator === ts.SyntaxKind.AsteriskToken ||
    operator === ts.SyntaxKind.PercentToken;
  if (!scaling) return false;
  return isHardCodedScale(node.left) || isHardCodedScale(node.right);
}

// A grouped literal is the house style for a four-digit scale, and 10_000 is the
// same finding as 10000 because the scanner has already read it as a value —
// `.text` is "10000" where the source says "10_000". The awk had to spell both
// forms, and spelled one of them wrong for a while.
function isHardCodedScale(node: ts.Expression): boolean {
  if (!ts.isNumericLiteral(node)) return false;
  return HARD_CODED_SCALES.has(node.text);
}

// enclosing is the span a minor-unit name may be found in: the nearest
// statement, but never past a literal that holds SIBLINGS.
//
// The statement alone is too wide. A vector of five fields divides a percentage
// by 100 in one of them and names a minor-unit base in another, and a rule
// reading the whole statement calls that money — a census whose escape hatch
// gets used routinely stops being read.
//
// An array is the same shape as an object and was missed: `[valueMinor,
// seconds * 1000]` walked past the elements to the declaration, read the whole
// of it, and reported an unrelated power because a NEIGHBOUR named a minor
// unit. Both stop here, for one reason — an element is its own span, and what
// sits beside it is not part of what it says.
function enclosing(node: ts.Node): ts.Node {
  let current: ts.Node = node;
  while (current.parent) {
    if (
      ts.isObjectLiteralExpression(current.parent) ||
      ts.isArrayLiteralExpression(current.parent)
    ) {
      return current;
    }
    if (
      ts.isStatement(current.parent) ||
      ts.isVariableDeclaration(current.parent)
    ) {
      return current.parent;
    }
    current = current.parent;
  }
  return current;
}

// namesAMinorUnit reads IDENTIFIERS, never source text: a comment describing
// the defect and a string quoting it are not code, and neither is an
// identifier. The awk had to be taught both, twice, once per grammar.
function namesAMinorUnit(unit: ts.Node): boolean {
  let named = false;
  const visit = (node: ts.Node): void => {
    if (named) return;
    if (ts.isIdentifier(node) && MINOR_UNIT_NAME.test(node.text)) {
      named = true;
      return;
    }
    ts.forEachChild(node, visit);
  };
  if (ts.isIdentifier(unit) && MINOR_UNIT_NAME.test(unit.text)) return true;
  ts.forEachChild(unit, visit);
  return named;
}

// waivedLines are the lines carrying the marker in a COMMENT. The marker
// written inside a string waives nothing, which is a distinction the scanner
// makes for free and the awk had to be told.
function waivedLines(source: string, file: ts.SourceFile): Set<number> {
  const waived = new Set<number>();
  const scanner = ts.createScanner(
    ts.ScriptTarget.Latest,
    false,
    ts.LanguageVariant.Standard,
    source,
  );
  let token = scanner.scan();
  while (token !== ts.SyntaxKind.EndOfFileToken) {
    const isComment =
      token === ts.SyntaxKind.SingleLineCommentTrivia ||
      token === ts.SyntaxKind.MultiLineCommentTrivia;
    if (isComment && scanner.getTokenText().includes(WAIVER)) {
      const from = lineOf(file, scanner.getTokenStart());
      const to = lineOf(file, scanner.getTokenEnd());
      for (let line = from; line <= to; line += 1) waived.add(line);
    }
    token = scanner.scan();
  }
  return waived;
}

function waivedBetween(waived: Set<number>, from: number, to: number): boolean {
  for (let line = from; line <= to; line += 1)
    if (waived.has(line)) return true;
  return false;
}

function lineOf(file: ts.SourceFile, position: number): number {
  return file.getLineAndCharacterOfPosition(position).line + 1;
}

// sourceFiles is every hand-written TypeScript file the census judges. Tests are
// out of scope — they plant these defects on purpose — and schema.d.ts is
// generated from the contract.
function sourceFiles(root: string): string[] {
  const found: string[] = [];
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    const path = join(root, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === "node_modules") continue;
      found.push(...sourceFiles(path));
      continue;
    }
    if (!/\.tsx?$/.test(entry.name)) continue;
    if (/\.test\.tsx?$/.test(entry.name) || entry.name === "schema.d.ts")
      continue;
    found.push(path);
  }
  return found;
}

const planted: PlantedCase[] = JSON.parse(
  readFileSync(corpusPath, "utf8"),
).filter((one: PlantedCase) => one.gate === "money-scale" && one.lang === "ts");

describe("the money-scale census, over the cases planted for it", () => {
  it("has cases to run", () => {
    expect(planted.length).toBeGreaterThan(0);
  });

  for (const one of planted) {
    it(`${one.want}: ${one.name}`, () => {
      const { findings, unreadable } = scaleFindings(
        "probe.ts",
        `${one.body}\n`,
      );
      if (one.want === "unclosed") {
        // Its own expectation, not a flavour of `fires`: the two mean opposite
        // things about the run. `fires` says the census READ the code and found
        // the defect; `unclosed` says it could not read it at all and refused
        // rather than pretend.
        expect(unreadable).toBe(true);
        return;
      }
      expect(unreadable).toBe(false);
      if (one.want === "fires") {
        expect(
          findings.length,
          `the planted defect was not found`,
        ).toBeGreaterThan(0);
      } else {
        expect(
          findings,
          `the census refused code that is not the defect`,
        ).toEqual([]);
      }
    });
  }
});

describe("the money-scale census, over the tree", () => {
  const files = sourceFiles(srcRoot);

  it("reaches the tree it is supposed to read", () => {
    expect(files.length).toBeGreaterThanOrEqual(FILE_FLOOR);
  });

  // A census over every hand-written file in the tree, parsed by the real
  // TypeScript parser. That is not a ten-second unit test on a cold runner, and
  // the alternative — sampling the tree — is the under-recognition failure a
  // census must not have.
  it(
    "finds no amount in minor units scaled by a hard-coded power of ten",
    () => {
      const findings: string[] = [];
      for (const path of files) {
        // format/minorunits OWNS the conversion — the ISO-4217 digit table is
        // applied there, mirroring the Go one, and refusing the one
        // implementation would be refusing the answer.
        if (path.includes(join("format", "minorunits"))) continue;
        const source = readFileSync(path, "utf8");
        const { findings: found, unreadable } = scaleFindings(path, source);
        expect(
          unreadable,
          `${path}: the parser could not finish this file`,
        ).toBe(false);
        for (const one of found)
          findings.push(`${path}:${one.line}: ${one.text}`);
      }
      expect(
        findings,
        "use format/minorunits toMinorUnits / toMajorUnits",
      ).toEqual([]);
    },
    TREE_SCAN_TIMEOUT_MS,
  );
});
