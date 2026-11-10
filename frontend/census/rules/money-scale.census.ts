// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The TypeScript half of the money-scale census: an amount already in minor
// units, scaled by a hard-coded power of ten.
//
// A currency with no minor unit (VND, JPY, KRW) is understated a hundredfold by
// `/ 100`, and a three-decimal one (KWD) is overstated tenfold.
// `format/minorunits` owns the conversion, mirroring the Go table.
//
// The rule and its planted cases are shared with the Go half through
// backend/gates/testdata/sourcecensus.json. A Go gate refuses an arm planted in
// one language and not the other.

import { join } from "node:path";
import ts from "typescript";
import { type CensusFile, type CensusRule, repoRoot } from "../tree";

export const corpusPath = join(
  repoRoot,
  "backend",
  "gates",
  "testdata",
  "sourcecensus.json",
);

// Below this many files the walk has stopped reaching the tree.
const FILE_FLOOR = 200;

const MINOR_UNIT_NAME = /[Mm]inor[A-Za-z_]*|MINOR[A-Z_]*/;
const HARD_CODED_SCALES = new Set(["10", "100", "1000", "10000"]);
const WAIVER = "money-scale-exempt:";

type Finding = { line: number; text: string };

// A file the parser could not finish is reported apart from a clean read. A
// scanner that went blind must not report the rest of the file as clean.
export function scaleFindings(file: ts.SourceFile): {
  findings: Finding[];
  unreadable: boolean;
} {
  if (unfinished(file)) return { findings: [], unreadable: true };
  const waived = waivedLines(file);
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

// Only a template literal and a block comment run to the end of the file once
// opened. An unterminated single-line string ends at the newline, and the code
// after it is still judged.
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

// The scanner reads `10_000` as the value "10000", so both spellings match.
function isHardCodedScale(node: ts.Expression): boolean {
  if (!ts.isNumericLiteral(node)) return false;
  return HARD_CODED_SCALES.has(node.text);
}

// The span a minor-unit name may be found in: the nearest statement, but never
// past an object or array literal. An element is its own span, and a neighbour
// naming a minor unit says nothing about it.
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

// Identifiers only: a comment or a string quoting the defect is not code.
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

// The lines carrying the marker in a comment. The marker inside a string
// waives nothing.
function waivedLines(file: ts.SourceFile): Set<number> {
  const waived = new Set<number>();
  const scanner = ts.createScanner(
    ts.ScriptTarget.Latest,
    false,
    ts.LanguageVariant.Standard,
    file.text,
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

// Hand-written TypeScript under src/. Tests plant the defect,
// schema.d.ts is generated, and format/minorunits owns the conversion.
const judged = (file: CensusFile) =>
  file.area === "src" &&
  /\.tsx?$/.test(file.path) &&
  !/\.test\.tsx?$/.test(file.path) &&
  !file.rel.endsWith("/schema.d.ts") &&
  !file.rel.startsWith("src/format/minorunits");

export const rule: CensusRule = {
  name: "money-scale",
  corpus: (tree) => tree.files.filter(judged),
  floors: (corpus) =>
    corpus.length >= FILE_FLOOR
      ? []
      : [`fewer than ${FILE_FLOOR} source files were read`],
  visit: (file) => {
    if (!file.source) return [];
    const { findings, unreadable } = scaleFindings(file.source);
    if (unreadable) return [`${file.rel}: the parser could not finish it`];
    return findings.map(({ line, text }) => `${file.rel}:${line}: ${text}`);
  },
  advice: "use format/minorunits toMinorUnits / toMajorUnits",
};
