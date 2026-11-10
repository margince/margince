// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A day count is reckoned in the zone the date beside it is drawn in.
//
// `calendarDaysBetween` requires the zone, so tsc catches a caller that forgets
// it. This rule covers what tsc cannot see: the literal "UTC", and a hand floor
// of a millisecond span (`Math.floor(x / 86_400_000)`) outside the owner.

import ts from "typescript";
import { parseSource } from "../../scripts/lib/source-tree";
import type { CensusFile, CensusRule } from "../tree";

const OWNER = "src/format/format.dates.ts";

// Functions whose day floor counts elapsed time, not a calendar. Each entry
// must still hold a floor, so it cannot outlive its reason.
const elapsedSpans: Readonly<Record<string, string>> = {
  "format/format.ts:formatDuration":
    "formatDuration renders elapsed durations for timers; record ages use formatAge with a zone.",
  "format/now.ts:formatSpan":
    "Countdowns show remaining elapsed time, so midnight cannot consume a day of the deadline.",
  "format/now.ts:formatElapsed":
    "Duration callers count complete days; activityAgo supplies the record calendar count explicitly.",
};

const DAY_LITERAL = "86400000";

function isUtcLiteral(node: ts.Expression): boolean {
  return ts.isStringLiteralLike(node) && node.text === "UTC";
}

function isDayDivisor(node: ts.Expression): boolean {
  if (ts.isNumericLiteral(node)) {
    return [DAY_LITERAL, "24"].includes(node.text.replaceAll("_", ""));
  }
  return (
    ts.isIdentifier(node) && ["DAY_MS", "HOURS_PER_DAY"].includes(node.text)
  );
}

function zonelessDayCounts(file: ts.SourceFile): string[] {
  const found: string[] = [];
  const where = (node: ts.Node) =>
    `${file.fileName}:${file.getLineAndCharacterOfPosition(node.getStart(file)).line + 1}`;
  const visit = (node: ts.Node) => {
    if (
      ts.isCallExpression(node) &&
      ts.isIdentifier(node.expression) &&
      node.expression.text === "calendarDaysBetween" &&
      (node.arguments.length < 3 || isUtcLiteral(node.arguments[2]))
    ) {
      found.push(`${where(node)}: calendarDaysBetween without a real zone`);
    }
    if (
      ts.isBinaryExpression(node) &&
      node.operatorToken.kind === ts.SyntaxKind.SlashToken &&
      isDayDivisor(node.right) &&
      ts.isCallExpression(node.parent) &&
      node.parent.expression.getText(file) === "Math.floor"
    ) {
      found.push(`${where(node)}: hand-written UTC day floor`);
    }
    ts.forEachChild(node, visit);
  };
  visit(file);
  return found;
}

// Every finding has `Math.floor` or `calendarDaysBetween` in its raw text, or
// a `\u` escape spelling one of them.
export function mayHoldFinding(text: string): boolean {
  return (
    text.includes("Math.floor") ||
    text.includes("calendarDaysBetween") ||
    text.includes("\\u")
  );
}

export function findingsIn(path: string, text: string): string[] {
  return mayHoldFinding(text) ? zonelessDayCounts(parseSource(path, text)) : [];
}

// Each top-level statement is judged on its own, so an exemption covers one
// named function and not its file.
function statementFindings(file: CensusFile): string[] {
  if (!file.source || !mayHoldFinding(file.text)) return [];
  const source = file.source;
  const fromSrc = file.rel.replace(/^src\//, "");
  return source.statements.flatMap((statement) => {
    const name = ts.isFunctionDeclaration(statement)
      ? statement.name?.text
      : "";
    if (`${fromSrc}:${name}` in elapsedSpans) return [];
    return findingsIn(file.rel, statement.getText(source));
  });
}

function idleExemptions(corpus: readonly CensusFile[]): string[] {
  return Object.keys(elapsedSpans).filter((key) => {
    const [fromSrc, name] = key.split(":");
    const source = corpus.find((file) => file.rel === `src/${fromSrc}`)?.source;
    const statement = source?.statements.find(
      (node) => ts.isFunctionDeclaration(node) && node.name?.text === name,
    );
    return (
      !statement ||
      findingsIn(fromSrc ?? key, statement.getText(source)).length === 0
    );
  });
}

const judged = (file: CensusFile) =>
  file.area === "src" &&
  /\.tsx?$/.test(file.path) &&
  !/\.test\.tsx?$/.test(file.path) &&
  file.rel !== OWNER;

export const rule: CensusRule = {
  name: "calendar-days-zone",
  corpus: (tree) => tree.files.filter(judged),
  floors: (corpus) =>
    corpus.filter((file) => file.text.includes("calendarDaysBetween("))
      .length >= 5
      ? []
      : ["fewer than 5 callers of calendarDaysBetween were read"],
  across: (corpus) => [
    ...corpus.flatMap(statementFindings),
    ...idleExemptions(corpus).map(
      (key) => `${key}: the elapsed-span exemption covers no floor any more`,
    ),
  ],
  advice:
    "Count calendar days with calendarDaysBetween and the zone the date is " +
    "drawn in. A floor that counts elapsed time earns an entry in " +
    "elapsedSpans with its reason.",
};
