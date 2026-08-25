// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";

// A calendar day cut out of an ISO string is UTC's day, whoever is reading.
//
// `new Date().toISOString().slice(0, 10)` looks like "today" and is not: east
// of UTC in the small hours it names yesterday, west of UTC in the evening it
// names tomorrow. The screens that carried it asked the AI-usage endpoint for
// the wrong day's budget band and seeded FX effective dates a day off — both
// invisible to whoever wrote them, because a machine on UTC agrees.
//
// The same cut at `slice(0, 7)` is the month, and that is the half worth
// naming: the census that found the day sites grepped for the day-length slice
// and so could not see the month one at all. A gate that knew only the length
// it was written for would have the same blind spot, so this one matches the
// CUT rather than a length.
//
// `format/calendarday.ts` is where the question is answered — in a named zone,
// from the parts — and `viewerZone()` / `useRecordZone()` say whose calendar the
// answer belongs to.
//
// WHAT IT CANNOT SEE: an ISO string sliced through a variable
// (`const iso = at.toISOString(); iso.slice(0, 10)`), or a day assembled from
// `getUTCFullYear()` and friends. Both are real respellings; this is a net
// under the one shape the tree actually reaches for, not a proof.

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(here, "..");
const owner = join(here, "calendarday.ts");

// Sites that render a UTC day ON PURPOSE, keyed by file and the function that
// holds them so a waiver ratifies the instance rather than the file. Each says
// why UTC is the right clock there — an entry that stops matching fails, so a
// waiver cannot outlive the code it was written about.
const deliberateUtcDays: Record<string, string> = {
  "screens/aiusage.tsx#adjacentMonth":
    "the two boundaries are read back off dates this function just built with Date.UTC(...) from a month it was given, so the round trip never touches a clock or a zone. Naming the month is the zoned question, and only the seed asks it",
  "format/timezone.ts#shifted":
    "the same round trip as adjacentMonth's: the date is built with Date.UTC(...) from a day this function was GIVEN, and the components read back are the ones just written. It is arithmetic on a named day, not a reading of the clock, and the zoned question is answered by startOfDayInstant on the next line",
  "mcp-apps/bridge.ts#day":
    "the answer beside this date says whether a promise is overdue, and the server judged that in UTC. A day rendered in the reader's zone could print tomorrow's date beside the word overdue — one clock, one day",
};

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" || entry.name === "dist"
        ? []
        : sourceFiles(path);
    }
    if (path === fileURLToPath(import.meta.url) || path === owner) {
      // This gate spells the pattern it hunts, and the owner is the permitted
      // site. A sweep that read either would vouch for itself.
      return [];
    }
    return /\.tsx?$/.test(entry.name) ? [path] : [];
  });
}

// enclosingName is the key a finding is reported and waived under: the nearest
// named function, method or variable-bound arrow above the offending call.
// Names rather than line numbers, because a line moves under an edit above it
// and a waiver keyed to one silently starts covering different code.
function enclosingName(node: ts.Node): string {
  for (let at: ts.Node | undefined = node; at; at = at.parent) {
    if (
      (ts.isFunctionDeclaration(at) || ts.isMethodDeclaration(at)) &&
      at.name
    ) {
      return at.name.getText();
    }
    if (ts.isVariableDeclaration(at) && ts.isIdentifier(at.name)) {
      return at.name.getText();
    }
  }
  return "<top level>";
}

type Finding = { key: string; line: number };

// `<something>.toISOString().slice(...)`, read off the syntax rather than the
// text: the cut and the call it cuts have to be the same expression, or a
// `slice` on an unrelated string one line away would match a pattern.
//
// The LENGTH is deliberately not part of the shape. A gate that knew only
// `slice(0, 10)` would repeat the blind spot that let the month cut survive the
// census this exists because of.
function isIsoSlice(node: ts.Node): node is ts.CallExpression {
  if (
    !ts.isCallExpression(node) ||
    !ts.isPropertyAccessExpression(node.expression)
  ) {
    return false;
  }
  if (node.expression.name.getText() !== "slice") {
    return false;
  }
  const sliced = node.expression.expression;
  if (
    !ts.isCallExpression(sliced) ||
    !ts.isPropertyAccessExpression(sliced.expression) ||
    sliced.expression.name.getText() !== "toISOString"
  ) {
    return false;
  }
  // Only a cut that STARTS at 0 — the date half. `slice(11, 16)` takes the
  // time, which is a different question with a different answer, and a gate
  // that reported it would be turned off for the noise. The END is not part of
  // the shape on purpose: knowing only `slice(0, 10)` is the blind spot that
  // let the month cut survive the census this exists because of.
  const [from] = node.arguments;
  return from !== undefined && ts.isNumericLiteral(from) && from.text === "0";
}

function isoSliceSites(path: string, source: string): Finding[] {
  const parsed = ts.createSourceFile(
    path,
    source,
    ts.ScriptTarget.Latest,
    true,
    path.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
  );
  const found: Finding[] = [];
  const visit = (node: ts.Node): void => {
    if (isIsoSlice(node)) {
      const file = relative(srcRoot, path).split("\\").join("/");
      found.push({
        key: `${file}#${enclosingName(node)}`,
        line: parsed.getLineAndCharacterOfPosition(node.getStart()).line + 1,
      });
    }
    ts.forEachChild(node, visit);
  };
  visit(parsed);
  return found;
}

// The census parses with the TypeScript compiler, which costs more per file
// than the whole rest of this suite, so it is asked only of files that COULD
// hold a finding. The filter is `toISOString` and nothing else: the detector
// compares that identifier's text, so a file the filter drops provably cannot
// match — the prefilter is strictly wider than what it stands in front of,
// which is the direction a skip may go and the only one.
function couldSliceAnIso(source: string): boolean {
  return source.includes("toISOString");
}

describe("a calendar day is never cut out of an ISO string", () => {
  const files = sourceFiles(srcRoot);

  // Swept once and shared. Two tests reading the tree separately is the same
  // walk paid for twice, and the second one is what pushed this suite past its
  // timeout under a loaded runner.
  const findings = files.flatMap((path) => {
    const source = readFileSync(path, "utf8");
    return couldSliceAnIso(source) ? isoSliceSites(path, source) : [];
  });

  it("walks a tree that holds source", () => {
    // A census passes by finding nothing, which is also what it does over an
    // empty walk. The floor is what tells the two apart.
    expect(files.length).toBeGreaterThan(100);
  });

  it("does not drop a file the detector would have matched", () => {
    // The prefilter is where a census goes blind, so it is asked directly:
    // every shape the detector recognises must survive it.
    expect(
      couldSliceAnIso("const day = new Date().toISOString().slice(0, 10);"),
    ).toBe(true);
    expect(couldSliceAnIso("const initials = name.slice(0, 2);")).toBe(false);
  });

  it("recognises the shape it was written for", () => {
    const planted = isoSliceSites(
      join(srcRoot, "screens", "planted.tsx"),
      "export function today() { return new Date().toISOString().slice(0, 10); }",
    );
    expect(planted.map((f) => f.key)).toEqual(["screens/planted.tsx#today"]);

    // The month cut, which the census that found the day sites could not see.
    const month = isoSliceSites(
      join(srcRoot, "screens", "planted.tsx"),
      "export function thisMonth() { return new Date().toISOString().slice(0, 7); }",
    );
    expect(month.map((f) => f.key)).toEqual(["screens/planted.tsx#thisMonth"]);

    // And the near misses: something that is not an ISO string at all, and the
    // TIME half of one, which asks a different question.
    const other = isoSliceSites(
      join(srcRoot, "screens", "planted.tsx"),
      "export function initials(name: string) { return name.slice(0, 2); }",
    );
    expect(other).toEqual([]);
    const timeHalf = isoSliceSites(
      join(srcRoot, "screens", "planted.tsx"),
      "export function at(d: Date) { return d.toISOString().slice(11, 16); }",
    );
    expect(timeHalf).toEqual([]);
  });

  it("finds no unratified site", () => {
    const offenders = findings.filter(
      (finding) => !(finding.key in deliberateUtcDays),
    );
    expect(offenders).toEqual([]);
  });

  it("keeps no waiver that has stopped covering a site", () => {
    const live = new Set(findings.map((f) => f.key));
    const stale = Object.keys(deliberateUtcDays).filter(
      (key) => !live.has(key),
    );
    expect(stale).toEqual([]);
  });
});
