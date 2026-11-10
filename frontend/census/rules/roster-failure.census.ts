// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import ts from "typescript";
import type { CensusFile, CensusRule } from "../tree";

// A failed walk leaves `data` undefined, and `data ?? []` makes an empty picker
// that never says why. So a `useRoster` result is also read for its failure,
// or handed whole to a reader of it such as `RosterFailedLine`.

// A read that only names ids may waive this with `// roster-failure: <reason>`.

const FAILURE_FIELDS = new Set(["isError", "isSuccess", "status", "error"]);
const CALL = /\buseRoster\(/g;
const DEFINITION = /\bfunction useRoster\(/g;
const WAIVER = /\/\/\s*roster-failure:\s*\S/;

function isUseRosterCall(node: ts.Node): node is ts.CallExpression {
  return (
    ts.isCallExpression(node) &&
    ts.isIdentifier(node.expression) &&
    node.expression.text === "useRoster"
  );
}

function enclosingScope(node: ts.Node): ts.Node {
  let at = node.parent;
  while (at && !ts.isFunctionLike(at) && !ts.isSourceFile(at)) {
    at = at.parent;
  }
  return at;
}

// Whether one reference to the binding reads more than `data`: a failure
// field, or the whole query passed on to a reader that decides for itself.
function readsFailure(ref: ts.Identifier): boolean {
  const parent = ref.parent;
  if (ts.isPropertyAccessExpression(parent) && parent.expression === ref) {
    return FAILURE_FIELDS.has(parent.name.text);
  }
  return true;
}

function referencesTo(scope: ts.Node, name: string, own: ts.Node) {
  const refs: ts.Identifier[] = [];
  const visit = (node: ts.Node) => {
    if (ts.isIdentifier(node) && node.text === name && node !== own) {
      refs.push(node);
    }
    ts.forEachChild(node, visit);
  };
  visit(scope);
  return refs;
}

function waived(call: ts.CallExpression, text: string): boolean {
  let statement: ts.Node = call;
  while (!ts.isVariableStatement(statement) && !ts.isSourceFile(statement)) {
    statement = statement.parent;
  }
  return (ts.getLeadingCommentRanges(text, statement.pos) ?? []).some((range) =>
    WAIVER.test(text.slice(range.pos, range.end)),
  );
}

// Why this call site ignores a failed read, or null when it does not.
export function offence(call: ts.CallExpression, text: string): string | null {
  if (waived(call, text)) {
    return null;
  }
  const parent = call.parent;
  if (ts.isVariableDeclaration(parent) && ts.isIdentifier(parent.name)) {
    const name = parent.name.text;
    const refs = referencesTo(enclosingScope(call), name, parent.name);
    return refs.some(readsFailure)
      ? null
      : `\`${name}\` is read only for its data`;
  }
  if (
    ts.isVariableDeclaration(parent) &&
    ts.isObjectBindingPattern(parent.name)
  ) {
    const taken = parent.name.elements.map((element) =>
      (element.propertyName ?? element.name).getText(),
    );
    return taken.some((field) => FAILURE_FIELDS.has(field))
      ? null
      : "the destructuring takes no failure field";
  }
  if (ts.isPropertyAccessExpression(parent)) {
    return FAILURE_FIELDS.has(parent.name.text)
      ? null
      : `the call is read only for \`.${parent.name.text}\``;
  }
  return null;
}

type Site = { line: number; reason: string | null };

/** Each `useRoster` call in one module, with why it ignores a failed read. */
function sitesIn(file: CensusFile): Site[] {
  if (!file.text.includes("useRoster(") || file.source === undefined) {
    return [];
  }
  const source = file.source;
  const sites: Site[] = [];
  const visit = (node: ts.Node) => {
    if (isUseRosterCall(node)) {
      sites.push({
        line:
          source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1,
        reason: offence(node, file.text),
      });
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return sites;
}

/**
 * The calls that ignore a failed read. A call the text spells and the parse
 * misses is a finding too: a comment names the hook without its parenthesis.
 */
export function rosterFindings(file: CensusFile): string[] {
  const sites = sitesIn(file);
  const written =
    (file.text.match(CALL)?.length ?? 0) -
    (file.text.match(DEFINITION)?.length ?? 0);
  return [
    ...(sites.length === written
      ? []
      : [
          `${file.rel}: spells useRoster( ${written} times, parses ${sites.length}`,
        ]),
    ...sites.flatMap(({ line, reason }) =>
      reason === null ? [] : [`${file.rel}:${line}: ${reason}`],
    ),
  ];
}

const judged = (file: CensusFile) =>
  file.area === "src" &&
  file.source !== undefined &&
  !/\.(test|stories)\.tsx?$/.test(file.rel);

export const rule: CensusRule = {
  name: "roster-failure",
  corpus: (tree) => tree.files.filter(judged),
  floors: (corpus) =>
    corpus.some((file) => sitesIn(file).length > 0)
      ? []
      : ["no useRoster call was found"],
  visit: rosterFindings,
  advice:
    "Read a useRoster result for its failure (isError, status, error) or hand " +
    "it whole to RosterFailedLine. A read that only names ids may waive this " +
    "with `// roster-failure: <reason>`.",
};
