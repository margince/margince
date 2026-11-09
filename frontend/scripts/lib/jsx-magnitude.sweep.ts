// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The typed half of src/format/jsx-magnitude.test.ts, which states the rule.
// The test calls it through inWorker, so it takes and returns plain data.

import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

import ts from "typescript";

import { plantedProgram, programOver } from "./source-tree.ts";

const frontendRoot = fileURLToPath(new URL("../..", import.meta.url));
const srcRoot = join(frontendRoot, "src");

export type Finding = Readonly<{ file: string; line: number; text: string }>;

// A site the checker could not read is kept apart from a clean one. Folded
// in, a census that read less than the tree would pass as clean.
type Verdict = Readonly<{ findings: Finding[]; unjudged: Finding[] }>;

const NUMBER_LIKE =
  ts.TypeFlags.Number |
  ts.TypeFlags.NumberLiteral |
  ts.TypeFlags.Enum |
  ts.TypeFlags.EnumLiteral;

const TEXT_LIKE = ts.TypeFlags.String | ts.TypeFlags.StringLiteral;

const NULLISH = ts.TypeFlags.Undefined | ts.TypeFlags.Null | ts.TypeFlags.Void;

function constituents(type: ts.Type): ts.Type[] {
  const parts = type.isUnion() ? type.types : [type];
  return parts.filter((part) => (part.flags & NULLISH) === 0);
}

// Every constituent must be numeric: `number | undefined` is a magnitude,
// `0 | Element` is not a figure a formatter could take.
function isMagnitude(type: ts.Type): boolean {
  const parts = constituents(type);
  return (
    parts.length > 0 && parts.every((part) => (part.flags & NUMBER_LIKE) !== 0)
  );
}

// `never` and the error type are how an unresolved import comes back. `any`
// joins them because the tree bans it, so it has the same source here.
function isUnreadable(type: ts.Type): boolean {
  return constituents(type).some(
    (part) => (part.flags & (ts.TypeFlags.Never | ts.TypeFlags.Any)) !== 0,
  );
}

// Whether a prop declared with this type is drawn as text and only as text.
function isTextSlot(type: ts.Type): boolean {
  const parts = constituents(type);
  return (
    parts.length > 0 && parts.every((part) => (part.flags & TEXT_LIKE) !== 0)
  );
}

// Whether every value of this object is text, as a catalog sentence's
// parameters are. Asked of the signature, so every translator is in scope.
function isTextTable(type: ts.Type, checker: ts.TypeChecker): boolean {
  // The parameter is optional, so a call site's contextual type is the table
  // or undefined. A union has no index signature, so the nullable part goes.
  const index = checker.getIndexInfoOfType(
    checker.getNonNullableType(type),
    ts.IndexKind.String,
  );
  return index !== undefined && isTextSlot(index.type);
}

// React's own name for drawn content. A number passed to such a prop is drawn
// bare, and only the caller can still rule it.
function isRenderedContent(type: ts.Type): boolean {
  return type.aliasSymbol?.getName() === "ReactNode";
}

// Geometry on an intrinsic tag. One of our components may take a prop named
// `values` and draw it as words, so the name alone is not enough.
function isDrawnGeometry(
  node: ts.JsxAttribute,
  geometry: Set<string>,
): boolean {
  const tag = node.parent.parent.tagName;
  const intrinsic = ts.isIdentifier(tag) && /^[a-z]/.test(tag.text);
  return intrinsic && geometry.has(node.name.getText());
}

// Where one file draws a value for a reader. That is a JSX child, a catalog
// parameter, or an attribute declared as text or drawn content.
// A prop declared `number` is left to the component's own render site.
function renderedPositions(
  parsed: ts.SourceFile,
  checker: ts.TypeChecker,
  geometry: Set<string>,
): ts.Expression[] {
  const found: ts.Expression[] = [];
  const walk = (node: ts.Node): void => {
    if (
      ts.isJsxExpression(node) &&
      node.expression !== undefined &&
      (ts.isJsxElement(node.parent) || ts.isJsxFragment(node.parent))
    ) {
      found.push(node.expression);
    }
    // Judged on the object's contextual type, not on the callee. So a params
    // object built a line before its `t()` call is still seen.
    if (
      ts.isPropertyAssignment(node) &&
      ts.isObjectLiteralExpression(node.parent)
    ) {
      const table = checker.getContextualType(node.parent);
      if (table !== undefined && isTextTable(table, checker)) {
        found.push(node.initializer);
      }
    }
    if (
      ts.isJsxAttribute(node) &&
      node.initializer !== undefined &&
      ts.isJsxExpression(node.initializer) &&
      node.initializer.expression !== undefined
    ) {
      const declared = checker.getContextualType(node.initializer.expression);
      if (
        declared !== undefined &&
        (isTextSlot(declared) || isRenderedContent(declared)) &&
        !isDrawnGeometry(node, geometry)
      ) {
        found.push(node.initializer.expression);
      }
    }
    node.forEachChild(walk);
  };
  walk(parsed);
  return found;
}

// `String(count)` and `count.toFixed(2)` satisfy a text type and still print
// US grouping, so a conversion is judged by what it converts.
function carriesMagnitude(
  node: ts.Expression,
  checker: ts.TypeChecker,
  blind: Set<string>,
): boolean {
  if (isMagnitude(checker.getTypeAtLocation(node))) {
    return true;
  }
  if (
    ts.isCallExpression(node) &&
    ts.isIdentifier(node.expression) &&
    node.expression.text === "String" &&
    node.arguments.length === 1
  ) {
    return isMagnitude(checker.getTypeAtLocation(node.arguments[0]));
  }
  if (
    ts.isCallExpression(node) &&
    (ts.isPropertyAccessExpression(node.expression) ||
      ts.isElementAccessExpression(node.expression))
  ) {
    const member = ts.isPropertyAccessExpression(node.expression)
      ? node.expression.name.text
      : ts.isStringLiteralLike(node.expression.argumentExpression)
        ? node.expression.argumentExpression.text
        : undefined;
    return (
      member !== undefined &&
      blind.has(member) &&
      isMagnitude(checker.getTypeAtLocation(node.expression.expression))
    );
  }
  if (ts.isTemplateExpression(node)) {
    return node.templateSpans.some((span) =>
      isMagnitude(checker.getTypeAtLocation(span.expression)),
    );
  }
  // A conditional draws whichever arm it picks, so each arm is judged alone.
  if (ts.isConditionalExpression(node)) {
    return (
      carriesMagnitude(node.whenTrue, checker, blind) ||
      carriesMagnitude(node.whenFalse, checker, blind)
    );
  }
  if (ts.isParenthesizedExpression(node)) {
    return carriesMagnitude(node.expression, checker, blind);
  }
  return false;
}

function findingsIn(
  parsed: ts.SourceFile,
  checker: ts.TypeChecker,
  blind: Set<string>,
  geometry: Set<string>,
  label: string,
): Verdict {
  const at = (node: ts.Expression): Finding => ({
    file: label,
    line: parsed.getLineAndCharacterOfPosition(node.getStart()).line + 1,
    text: node.getText().replace(/\s+/g, " ").slice(0, 80),
  });
  const findings: Finding[] = [];
  const unjudged: Finding[] = [];
  for (const node of renderedPositions(parsed, checker, geometry)) {
    if (isUnreadable(checker.getTypeAtLocation(node))) {
      unjudged.push(at(node));
    } else if (carriesMagnitude(node, checker, blind)) {
      findings.push(at(node));
    }
  }
  return { findings, unjudged };
}

/** The verdict over `files`, typed in one program, with the files it missed. */
export function sweep(
  files: string[],
  blind: Set<string>,
  geometry: Set<string>,
): Readonly<{ unloaded: string[]; findings: Finding[]; unjudged: Finding[] }> {
  const program = programOver(files, frontendRoot);
  const checker = program.getTypeChecker();
  const verdicts = files.flatMap((path) => {
    const parsed = program.getSourceFile(path);
    return parsed === undefined
      ? []
      : [
          findingsIn(
            parsed,
            checker,
            blind,
            geometry,
            relative(frontendRoot, path),
          ),
        ];
  });
  return {
    unloaded: files.filter((path) => program.getSourceFile(path) === undefined),
    findings: verdicts.flatMap((verdict) => verdict.findings),
    unjudged: verdicts.flatMap((verdict) => verdict.unjudged),
  };
}

// Judges planted lines, compiled against the real declarations, since every
// arm turns on a type. The preamble is one line, so a finding's line is its
// case's own index.
export function plant(
  lines: string[],
  blind: Set<string>,
  geometry: Set<string>,
): Finding[] {
  const fixtureName = join(srcRoot, "jsx-magnitude-fixture.tsx");
  const preamble = [
    'import type { ReactNode } from "react";',
    'import { formatNumber, identifierNumber, ordinalNumber } from "./format/format";',
    "declare const count: number;",
    "declare const maybeCount: number | undefined;",
    "declare const limit: number;",
    "declare const label: string;",
    "declare const other: string;",
    "declare const tag: string;",
    "declare const ready: boolean;",
    'declare const loc: "en";',
    "declare function t(key: string, params?: Record<string, string>): string;",
    "declare function Stat(props: { value: string }): ReactNode;",
    "declare function Slot(props: { body: ReactNode }): ReactNode;",
    "declare function Icon(props: { size: number }): ReactNode;",
    "declare function Chart(props: { values: string }): ReactNode;",
    "void [formatNumber, identifierNumber, ordinalNumber, t];",
  ].join(" ");
  const source = [preamble, ...lines.map((line) => `void (${line});`)].join(
    "\n",
  );
  const program = plantedProgram(fixtureName, source, frontendRoot);
  const parsed = program.getSourceFile(fixtureName);
  if (parsed === undefined) {
    throw new Error("the planted fixture did not compile");
  }
  const verdict = findingsIn(
    parsed,
    program.getTypeChecker(),
    blind,
    geometry,
    "fixture",
  );
  // A fixture line the checker cannot read is a broken fixture. Passing it
  // would turn a planted case green.
  if (verdict.unjudged.length > 0) {
    throw new Error(
      `the planted fixture did not type: lines ${verdict.unjudged
        .map((site) => site.line - 1)
        .join(", ")}`,
    );
  }
  return verdict.findings.map((finding) => ({
    ...finding,
    line: finding.line - 1,
  }));
}
