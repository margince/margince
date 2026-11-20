// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A date reaches the reader through a formatter in `format/`, never as the
// string the server sent.
//
// The wire carries a date as `2026-11-16` and a moment as an ISO instant.
// Rendered as is, the first reads month-first beside "16/11/2026" one row up,
// and the second shows UTC with a `T` in it. Both look fine to an author
// whose own locale is close to ISO.
//
// Two arms:
//
//   - A wire field named like a date (`*_date`, `*_at`), declared in the
//     generated `api/schema.d.ts`, placed where it is shown: a JSX child, a
//     shown attribute (`title`, `label`, `aria-label`, `placeholder`, `alt`),
//     or the whole answer of a table column's `render`. The declaration is
//     asked of the type checker, so a local field a caller already formatted
//     passes. `dateTime={row.created_at}` is a machine attribute and passes,
//     and so does a field handed to a component as a prop, which formats it.
//   - In Settings, a day goes through `formatDate` or `formatDateTime`, the
//     same numeric form its date inputs take. `formatDateAbbrev` there is a
//     second style on one page.
//
// Not seen: a date field copied into a local first, or built into a template
// string.

import ts from "typescript";
import { stable } from "../../src/format/collate";
import type { CensusFile, CensusRule } from "../tree";

const DATE_FIELD = /_(date|at)$/;

// The generated contract types, where every wire field is declared.
const WIRE_TYPES = "src/api/schema.d.ts";

const SHOWN_ATTRIBUTES = new Set([
  "title",
  "label",
  "aria-label",
  "placeholder",
  "alt",
]);

const SETTINGS = /^src\/features\/settings\//;

// The other style of day, kept out of Settings.
const SECOND_DAY_STYLE = "formatDateAbbrev";

function isWireDate(
  node: ts.Node,
  checker: ts.TypeChecker,
): node is ts.PropertyAccessExpression {
  if (
    !ts.isPropertyAccessExpression(node) ||
    !DATE_FIELD.test(node.name.text)
  ) {
    return false;
  }
  const declared = checker.getSymbolAtLocation(node.name)?.declarations ?? [];
  return declared.some((declaration) =>
    declaration.getSourceFile().fileName.endsWith(WIRE_TYPES),
  );
}

function shownAttribute(expression: ts.JsxExpression): boolean {
  const attribute = expression.parent;
  return (
    ts.isJsxAttribute(attribute) &&
    SHOWN_ATTRIBUTES.has(attribute.name.getText())
  );
}

// Where a date field is put in front of the reader, if this node is one.
function shownAt(node: ts.PropertyAccessExpression): string | undefined {
  const parent = node.parent;
  if (ts.isJsxExpression(parent)) {
    if (ts.isJsxElement(parent.parent) || ts.isJsxFragment(parent.parent)) {
      return "a JSX child";
    }
    return shownAttribute(parent) ? "a shown attribute" : undefined;
  }
  if (
    ts.isArrowFunction(parent) &&
    parent.body === node &&
    ts.isPropertyAssignment(parent.parent) &&
    parent.parent.name.getText() === "render"
  ) {
    return "a column's render";
  }
  return undefined;
}

/** Every raw date shown, and every second day style in Settings, in one file. */
function rawDatesIn(
  source: ts.SourceFile,
  rel: string,
  checker: ts.TypeChecker,
): string[] {
  const found: string[] = [];
  const settings = SETTINGS.test(rel);
  const at = (node: ts.Node, text: string) => {
    const { line } = source.getLineAndCharacterOfPosition(
      node.getStart(source),
    );
    found.push(`${rel}:${line + 1}: ${text}`);
  };
  const visit = (node: ts.Node): void => {
    if (isWireDate(node, checker)) {
      const where = shownAt(node);
      if (where !== undefined) {
        at(node, `${node.getText(source)} is shown raw in ${where}`);
      }
    }
    if (
      settings &&
      ts.isCallExpression(node) &&
      ts.isIdentifier(node.expression) &&
      node.expression.text === SECOND_DAY_STYLE
    ) {
      at(node, `${SECOND_DAY_STYLE} is a second day style in Settings`);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

const judged = (file: CensusFile) =>
  file.area === "src" &&
  file.source !== undefined &&
  /\.tsx$/.test(file.rel) &&
  !/\.(test|stories)\.tsx$/.test(file.rel);

export const rule: CensusRule = {
  name: "raw-date-render",
  corpus: (tree) => tree.files.filter(judged),
  floors: (corpus) => [
    ...(corpus.length > 100 ? [] : ["fewer than 100 screens were read"]),
    ...(corpus.some((file) => SETTINGS.test(file.rel))
      ? []
      : ["no Settings screen was read"]),
  ],
  across: (corpus, tree) => {
    const checker = tree.checked().getTypeChecker();
    return corpus
      .flatMap((file) =>
        file.source ? rawDatesIn(file.source, file.rel, checker) : [],
      )
      .sort(stable);
  },
  advice:
    "Show a date through format/: formatDate for a day, formatDateTime for a " +
    "moment, in the reader's locale and zone. Settings keeps to those two, " +
    "the form its date inputs take.",
};
