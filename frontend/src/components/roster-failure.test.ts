// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { filesUnder, parseSource } from "../../scripts/lib/source-tree";

// A failed walk leaves `data` undefined, and `data ?? []` makes an empty picker
// that never says why. So a `useRoster` result is also read for its failure,
// or handed whole to a reader of it such as `RosterFailedLine`.

// A read that only names ids may waive this with `// roster-failure: <reason>`.

const frontendRoot = resolve(__dirname, "..", "..");
const srcDir = join(frontendRoot, "src");

const FAILURE_FIELDS = new Set(["isError", "isSuccess", "status", "error"]);
const CALL = /\buseRoster\(/g;
const DEFINITION = /\bfunction useRoster\(/g;
const WAIVER = /\/\/\s*roster-failure:\s*\S/;

type Site = { file: string; line: number; reason: string | null };

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
function offence(call: ts.CallExpression, text: string): string | null {
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

function rosterSites(): { sites: Site[]; written: number } {
  const sites: Site[] = [];
  let written = 0;
  for (const file of filesUnder(srcDir)) {
    if (/\.(test|stories)\.tsx?$/.test(file)) {
      continue;
    }
    const text = readFileSync(file, "utf8");
    written +=
      (text.match(CALL)?.length ?? 0) - (text.match(DEFINITION)?.length ?? 0);
    if (!text.includes("useRoster(")) {
      continue;
    }
    const source = parseSource(file, text);
    const visit = (node: ts.Node) => {
      if (isUseRosterCall(node)) {
        const { line } = source.getLineAndCharacterOfPosition(node.getStart());
        sites.push({
          file: relative(frontendRoot, file),
          line: line + 1,
          reason: offence(node, text),
        });
      }
      ts.forEachChild(node, visit);
    };
    visit(source);
  }
  return { sites, written };
}

describe("roster failure", () => {
  const { sites, written } = rosterSites();

  it("finds every useRoster call the source spells", () => {
    // A comment that spells the call counts in `written` and not in the
    // parse, so it fails here too; name the hook without its parenthesis.
    expect(sites.length).toBeGreaterThan(0);
    expect(sites.length).toBe(written);
  });

  it("reads every roster for its failure as well as its data", () => {
    const ignored = sites
      .filter((site) => site.reason !== null)
      .map((site) => `${site.file}:${site.line}: ${site.reason}`);
    expect(ignored).toEqual([]);
  });
});
