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
//   - A wire field the contract declares as a date or a moment (its
//     `Format: date` or `date-time` in the generated `api/schema.d.ts`),
//     placed where it is shown: a JSX child, a shown attribute (`title`,
//     `label`, `aria-label`, `placeholder`, `alt`), or the answer of a table
//     column's `render`. A `??`, `||`, conditional, `!` or parentheses around
//     the field do not hide it. `dateTime={row.created_at}` is a machine
//     attribute and passes, and so does a field handed to a component as a
//     prop, which formats it.
//   - In Settings, a day goes through `formatDate` or `formatDateTime`, the
//     same numeric form its date inputs take. `formatDateAbbrev` there is a
//     second style on one page.
//
// Not seen: a date field copied into a local first, or built into a template
// string.

import ts from "typescript";
import { stable } from "../../src/format/collate";
import type { CensusFile, CensusRule } from "../tree";

// The generated contract types, where every wire field is declared.
const WIRE_TYPES = "src/api/schema.d.ts";

// The comment openapi-typescript writes over a date or date-time field.
const DATE_FORMAT = /Format: date(?:-time)?\b/;

// Fewer wire-date reads than this means the declaration lookup broke, and the
// rule would pass by seeing none.
const WIRE_DATE_FLOOR = 500;

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

function declaresADate(declaration: ts.Declaration): boolean {
  const file = declaration.getSourceFile();
  if (!file.fileName.endsWith(WIRE_TYPES)) {
    return false;
  }
  return (ts.getLeadingCommentRanges(file.text, declaration.pos) ?? []).some(
    (range) => DATE_FORMAT.test(file.text.slice(range.pos, range.end)),
  );
}

function isWireDate(
  node: ts.Node,
  checker: ts.TypeChecker,
): node is ts.PropertyAccessExpression {
  if (!ts.isPropertyAccessExpression(node)) {
    return false;
  }
  const declared = checker.getSymbolAtLocation(node.name)?.declarations ?? [];
  return declared.some(declaresADate);
}

// The expression whose value the field becomes, past the operators that
// pass it through unchanged.
function carriedTo(node: ts.Node): ts.Node {
  let at = node;
  for (;;) {
    const parent = at.parent;
    const passesThrough =
      ts.isParenthesizedExpression(parent) ||
      ts.isNonNullExpression(parent) ||
      (ts.isBinaryExpression(parent) &&
        (parent.operatorToken.kind === ts.SyntaxKind.QuestionQuestionToken ||
          parent.operatorToken.kind === ts.SyntaxKind.BarBarToken)) ||
      (ts.isConditionalExpression(parent) && parent.condition !== at);
    if (!passesThrough) {
      return at;
    }
    at = parent;
  }
}

function shownAttribute(expression: ts.JsxExpression): boolean {
  const attribute = expression.parent;
  return (
    ts.isJsxAttribute(attribute) &&
    SHOWN_ATTRIBUTES.has(attribute.name.getText())
  );
}

function isRender(fn: ts.Node): boolean {
  return (
    (ts.isArrowFunction(fn) || ts.isFunctionExpression(fn)) &&
    ts.isPropertyAssignment(fn.parent) &&
    fn.parent.name.getText() === "render"
  );
}

// The function a return statement answers for.
function returnedFrom(statement: ts.ReturnStatement): ts.Node | undefined {
  for (let at: ts.Node = statement.parent; at; at = at.parent) {
    if (ts.isFunctionLike(at)) {
      return at;
    }
  }
  return undefined;
}

// Where a date field is put in front of the reader, if this node is one.
function shownAt(node: ts.Node): string | undefined {
  const value = carriedTo(node);
  const parent = value.parent;
  if (ts.isJsxExpression(parent)) {
    if (ts.isJsxElement(parent.parent) || ts.isJsxFragment(parent.parent)) {
      return "a JSX child";
    }
    return shownAttribute(parent) ? "a shown attribute" : undefined;
  }
  const answers =
    (ts.isArrowFunction(parent) && parent.body === value && isRender(parent)) ||
    (ts.isReturnStatement(parent) && isRender(returnedFrom(parent) ?? parent));
  return answers ? "a column's render" : undefined;
}

type FileReading = { findings: string[]; wireDates: number };

/** Every raw date shown, and every second day style in Settings, in one file. */
function rawDatesIn(
  source: ts.SourceFile,
  rel: string,
  checker: ts.TypeChecker,
): FileReading {
  const reading: FileReading = { findings: [], wireDates: 0 };
  const settings = SETTINGS.test(rel);
  const at = (node: ts.Node, text: string) => {
    const { line } = source.getLineAndCharacterOfPosition(
      node.getStart(source),
    );
    reading.findings.push(`${rel}:${line + 1}: ${text}`);
  };
  const visit = (node: ts.Node): void => {
    if (isWireDate(node, checker)) {
      reading.wireDates++;
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
  return reading;
}

const judged = (file: CensusFile) =>
  file.area === "src" &&
  file.source !== undefined &&
  /\.tsx$/.test(file.rel) &&
  !/\.(test|stories)\.tsx$/.test(file.rel);

/** The findings over a corpus, and how many wire-date reads it saw. */
export function readingOf(
  corpus: readonly CensusFile[],
  checker: ts.TypeChecker,
): FileReading {
  const readings = corpus.flatMap((file) =>
    file.source ? [rawDatesIn(file.source, file.rel, checker)] : [],
  );
  return {
    findings: readings.flatMap((r) => r.findings).sort(stable),
    wireDates: readings.reduce((sum, r) => sum + r.wireDates, 0),
  };
}

export const rule: CensusRule = {
  name: "raw-date-render",
  corpus: (tree) => tree.files.filter(judged),
  floors: (corpus, tree) => {
    const seen = readingOf(corpus, tree.checked().getTypeChecker()).wireDates;
    return [
      ...(corpus.length > 100 ? [] : ["fewer than 100 screens were read"]),
      ...(corpus.some((file) => SETTINGS.test(file.rel))
        ? []
        : ["no Settings screen was read"]),
      ...(seen >= WIRE_DATE_FLOOR
        ? []
        : [
            `only ${seen} wire-date reads were seen, want at least ${WIRE_DATE_FLOOR}`,
          ]),
    ];
  },
  across: (corpus, tree) =>
    readingOf(corpus, tree.checked().getTypeChecker()).findings,
  advice:
    "Show a date through format/: formatDate for a day, formatDateTime for a " +
    "moment, in the reader's locale and zone. Settings keeps to those two, " +
    "the form its date inputs take.",
};
