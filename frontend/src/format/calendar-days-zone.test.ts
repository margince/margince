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
// `calendarDaysBetween` takes the zone as a required third argument, so tsc
// already fails a caller that forgets it. This gate covers the two ways to
// count in UTC that tsc cannot see. One is passing the literal "UTC". The other
// is flooring a millisecond span to days by hand (`Math.floor(x / 86_400_000)`)
// outside the one module that owns the rule. Like zone-by-purpose.test.ts, it reads every
// file and parses every file whose text can hold a finding. A new screen is gated the day it lands.

const srcRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const owner = join(srcRoot, "format", "format.dates.ts");
// Files whose day floor counts elapsed time, not a calendar. Each
// entry must still contain a floor, so one cannot outlive its reason.
const elapsedSpans: Record<string, string> = {
  "format/format.ts":
    "formatDuration shows an idle or SLA span already computed as milliseconds from two instants; it is a duration, not a count of dates.",
};

const DAY_LITERAL = "86400000";

function isUtcLiteral(node: ts.Expression): boolean {
  return ts.isStringLiteralLike(node) && node.text === "UTC";
}

function isDayDivisor(node: ts.Expression): boolean {
  if (ts.isNumericLiteral(node)) {
    return node.text.replaceAll("_", "") === DAY_LITERAL;
  }
  return ts.isIdentifier(node) && node.text === "DAY_MS";
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
    expect(callers.length).toBeGreaterThanOrEqual(10);
  });

  it("finds no count in UTC beside a zoned date", () => {
    const findings = sourcesUnderTest()
      .filter((path) => !(relative(srcRoot, path) in elapsedSpans))
      .flatMap((path) => findingsIn(path, readFileSync(path, "utf8")));
    expect(findings).toEqual([]);
  });

  it("keeps each elapsed-span exemption live", () => {
    for (const file of Object.keys(elapsedSpans)) {
      expect(
        zonelessDayCounts(parsed(join(srcRoot, file))).length,
      ).toBeGreaterThan(0);
    }
  });

  it.each([
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
