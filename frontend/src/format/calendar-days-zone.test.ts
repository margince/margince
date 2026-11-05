// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { filesUnder, parseSource } from "../../scripts/lib/source-tree";

// A day count is reckoned in the zone the date beside it is drawn in.
//
// `calendarDaysBetween` requires the zone, so tsc catches a caller that forgets it.
// This gate covers what tsc cannot see: passing the literal "UTC", and flooring a
// millisecond span by hand (`Math.floor(x / 86_400_000)`) outside the owning module.
// It parses every file whose text can hold a finding, so a new screen is gated at once.

const srcRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const owner = join(srcRoot, "format", "format.dates.ts");
// Files whose day floor counts elapsed time, not a calendar. Each
// entry must still contain a floor, so one cannot outlive its reason.
const elapsedSpans: Record<string, string> = {
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

function sourcesUnderTest(): string[] {
  return filesUnder(srcRoot).filter(
    (path) =>
      /\.tsx?$/.test(path) &&
      !/\.test\.tsx?$/.test(path) &&
      path !== owner &&
      !path.endsWith("calendar-days-zone.test.ts"),
  );
}

// Every finding has `Math.floor` or `calendarDaysBetween` in its raw text. A `\u`
// escape can spell the call name, and numeric literals take many forms, so no number form is matched.
function mayHoldFinding(text: string): boolean {
  return (
    text.includes("Math.floor") ||
    text.includes("calendarDaysBetween") ||
    text.includes("\\u")
  );
}

function findingsIn(path: string, text: string): string[] {
  return mayHoldFinding(text) ? zonelessDayCounts(parseSource(path, text)) : [];
}

function parsed(path: string): ts.SourceFile {
  return parseSource(path, readFileSync(path, "utf8"));
}

describe("a day count names its zone", () => {
  it("reads the callers it exists to protect", () => {
    const callers = sourcesUnderTest().filter((path) =>
      readFileSync(path, "utf8").includes("calendarDaysBetween("),
    );
    expect(callers.length).toBeGreaterThanOrEqual(5);
  });

  it("finds no count in UTC beside a zoned date", () => {
    const findings = sourcesUnderTest().flatMap((path) => {
      const text = readFileSync(path, "utf8");
      // A statement's text is part of its file's, so a file that cannot hold a finding is not parsed.
      if (!mayHoldFinding(text)) return [];
      const file = parseSource(path, text);
      return file.statements.flatMap((statement) => {
        const key = `${relative(srcRoot, path)}:${ts.isFunctionDeclaration(statement) ? statement.name?.text : ""}`;
        const found = findingsIn(path, statement.getText(file));
        return key in elapsedSpans ? [] : found;
      });
    });
    expect(findings).toEqual([]);
  });

  it("keeps each elapsed-span exemption live", () => {
    for (const key of Object.keys(elapsedSpans)) {
      const [path, name] = key.split(":");
      const file = parsed(join(srcRoot, path));
      const statement = file.statements.find(
        (node) => ts.isFunctionDeclaration(node) && node.name?.text === name,
      );
      expect(statement, key).toBeDefined();
      expect(
        findingsIn(path, statement?.getText(file) ?? "").length,
        key,
      ).toBeGreaterThan(0);
    }
  });

  it.each([
    ["hours floor", "Math.floor(hours / 24);"],
    ["named hours floor", "Math.floor(hours / HOURS_PER_DAY);"],
    ["no zone", "calendarDaysBetween(a, b);"],
    ["a UTC literal", 'calendarDaysBetween(a, b, "UTC");'],
    ["a hand floor", "Math.floor(a.getTime() / 86_400_000);"],
    ["a named hand floor", "Math.floor(a / DAY_MS);"],
    ["a separated hand floor", "Math.floor(a / 86_400_000);"],
    ["an exponent floor", "Math.floor(a / 8.64e7);"],
    ["a hex floor", "Math.floor(a / 0x5265C00);"],
    ["a binary floor", "Math.floor(a / 0b101001001100101110000000000);"],
    ["an escaped call name", "calendar\\u0044aysBetween(a, b);"],
    ["an escaped day constant", "Math.floor(a / DAY\\u005fMS);"],
  ])("sees %s", (_name, body) => {
    expect(findingsIn("seed.ts", body)).toHaveLength(1);
  });

  it("skips text that holds no place for a finding", () => {
    expect(mayHoldFinding("const days = 7;")).toBe(false);
  });

  it("passes a zoned call", () => {
    expect(findingsIn("seed.ts", "calendarDaysBetween(a, b, zone);")).toEqual(
      [],
    );
  });
});
