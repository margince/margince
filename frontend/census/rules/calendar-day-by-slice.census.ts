// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A calendar day cut out of an ISO string is UTC's day, whoever is reading.
//
// `new Date().toISOString().slice(0, 10)` names yesterday east of UTC in the
// small hours, and tomorrow west of UTC in the evening. The same cut at
// `slice(0, 7)` is the month, so this matches the cut rather than a length.
//
// `format/calendarday.ts` answers the question in a named zone, from the
// parts. `browserZone()` and `useCompanyZone()` say whose calendar it is.
//
// Not seen: an ISO string sliced through a variable, or a day assembled from
// `getUTCFullYear()` and friends.

import ts from "typescript";
import type { CensusFile, CensusRule } from "../tree";

// Sites meant to render a UTC day, keyed by file and enclosing function.
// `sites` is part of what is ratified: a third cut in a function ratified for
// two is a finding again.
const deliberateUtcDays: Readonly<
  Record<string, { sites: number; why: string }>
> = {
  "features/ai/aiusage.tsx#monthAround": {
    sites: 2,
    why: "the two boundaries are read back off dates this function just built with Date.UTC(...) from the month it was given, so the round trip never touches a clock or a zone. Naming the month is the zoned question, and only currentMonth asks it",
  },
  "format/calendarday.ts#shifted": {
    sites: 1,
    why: "the carrier is built at UTC noon from a day this function was GIVEN, and the day read back is the one just written plus whole days. It is arithmetic on a named day, not a reading of the clock; the caller answers the zoned question",
  },
  "mcp-apps/values.ts#day": {
    sites: 1,
    why: "the answer beside this date says whether a promise is overdue, and the server judged that in UTC. A day rendered in the reader's zone could print tomorrow's date beside the word overdue — one clock, one day",
  },
};

// The key a finding is reported and waived under: the nearest named function,
// method or variable-bound arrow. A line number moves under an edit above it.
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

// `<something>.toISOString().slice(0, …)`, read off the syntax. Only a cut
// that starts at 0 takes the date half; `slice(11, 16)` takes the time.
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
  const [from] = node.arguments;
  return from !== undefined && ts.isNumericLiteral(from) && from.text === "0";
}

/** Every ISO day cut in one parsed file, keyed by its path from `src/`. */
export function isoSliceSites(
  source: ts.SourceFile,
  fromSrc: string,
): Finding[] {
  const found: Finding[] = [];
  const visit = (node: ts.Node): void => {
    if (isIsoSlice(node)) {
      found.push({
        key: `${fromSrc}#${enclosingName(node)}`,
        line: source.getLineAndCharacterOfPosition(node.getStart()).line + 1,
      });
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

// The detector compares the `toISOString` identifier, so a file without that
// text cannot match.
export function couldSliceAnIso(source: string): boolean {
  return source.includes("toISOString");
}

// Nothing is skipped: the owner is where an unnoticed cut would do the most
// damage, and a waiver for it goes in deliberateUtcDays.
const judged = (file: CensusFile) =>
  file.area === "src" && /\.tsx?$/.test(file.path);

export const rule: CensusRule = {
  name: "calendar-day-by-slice",
  corpus: (tree) => tree.files.filter(judged),
  floors: (corpus) =>
    corpus.length > 100 ? [] : ["fewer than 100 source files were read"],
  across: (corpus) => {
    const findings = corpus.flatMap((file) =>
      file.source && couldSliceAnIso(file.text)
        ? isoSliceSites(file.source, file.rel.replace(/^src\//, ""))
        : [],
    );
    const perKey = new Map<string, number>();
    for (const finding of findings) {
      perKey.set(finding.key, (perKey.get(finding.key) ?? 0) + 1);
    }
    // Both ways a waiver goes wrong: a cut added to a ratified function, and
    // a waiver whose sites are gone.
    const miscounted = Object.entries(deliberateUtcDays)
      .filter(([key, waiver]) => waiver.sites !== (perKey.get(key) ?? 0))
      .map(
        ([key, waiver]) =>
          `${key}: ratified ${waiver.sites}, found ${perKey.get(key) ?? 0}`,
      );
    const offenders = findings
      .filter((finding) => !(finding.key in deliberateUtcDays))
      .map(({ key, line }) => `${key}:${line}`);
    return [...offenders, ...miscounted];
  },
  advice:
    "A calendar day cut out of toISOString() is UTC's day. Ask " +
    "format/calendarday.ts in the zone the date belongs to, or ratify the " +
    "site in deliberateUtcDays with the reason UTC is right there.",
};
