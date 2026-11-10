// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A failure said on one line under a control has one spelling: `ErrorLine`.
// A copy drifts on its own, one keeping the ink and losing the announcement,
// until a refusal reads like a caption. Six arms, one per shape of the copy:
//
//   inline  - a JSX `style` object whose `color` is `var(--dangerText)`.
//   class   - a `className` carrying `t-danger` or `form-error`.
//             `field-error` is Field's own slot and is not this line.
//   sheet   - a `className` carrying a screen class whose every rule sets
//             only the danger ink and margins, derived from the screen and
//             extension sheets. A design-system sheet owns its tone classes.
//   message - an intrinsic element whose only child is one
//             `{problemMessageOf(…)}`. A line inside an element with a literal
//             `role="alert"` is read out with it and is skipped. A computed
//             role (a danger `Callout`) does not skip, so it takes a waiver.
//   alert   - an intrinsic element with `role="alert"` around a line: text
//             and expressions only, no element child.
//   name    - any identifier `RefusalLine` outside an import.
//
// A figure or a glyph drawn in the ink is not a line, and carries a reasoned
// `ds:ignore <reason>` on its line or the line above. A bare marker is a
// finding.

import { relative } from "node:path";
import ts from "typescript";
import { selectorList } from "../../scripts/lib/css-rules";
import { rulesIn } from "../../src/testing/css";
import {
  type CensusFile,
  type CensusRule,
  isAppSheet,
  repoRoot,
} from "../tree";

/** The one module allowed to wear the danger ink as a message line. */
export const HOME_MODULE = "frontend/src/design-system/errorline.tsx";

const advice = "use ErrorLine from design-system/errorline.tsx";

/**
 * The design-system modules that own an alerting line of their own, each with
 * why. Screens take a reasoned `ds:ignore` instead.
 */
export const ALERT_OWNERS = new Map([
  [
    "frontend/src/design-system/field.tsx",
    "Field's `field-error` slot, wired to its control's aria-describedby",
  ],
]);

function fromRepo(path: string): string {
  return relative(repoRoot, path).replaceAll("\\", "/");
}

type Arm = "inline" | "class" | "sheet" | "message" | "alert" | "name";

type Finding = { arm: Arm; line: number; says: string };

type Waiver = "none" | "reasoned" | "bare";

/** The marker on a line, and whether it gives a reason. */
function waiverOn(text: string): Waiver {
  const marker = /ds:ignore\b(.*)$/.exec(text);
  if (!marker) {
    return "none";
  }
  const reason = marker[1].replaceAll(/\*\/|\}/g, "").trim();
  return reason.length > 0 ? "reasoned" : "bare";
}

/** The waiver that sits with a finding: its own line first, then the one above. */
function waiverFor(lines: readonly string[], line: number): Waiver {
  const own = waiverOn(lines[line - 1] ?? "");
  return own === "none" ? waiverOn(lines[line - 2] ?? "") : own;
}

/** The literal text a node can evaluate to: strings and template pieces. */
function fragmentsOf(node: ts.Node): string[] {
  const out: string[] = [];
  const collect = (child: ts.Node) => {
    if (ts.isStringLiteral(child) || ts.isTemplateLiteralToken(child)) {
      out.push(child.text);
    }
    ts.forEachChild(child, collect);
  };
  collect(node);
  return out;
}

const dangerClass = /^(t-danger|form-error)$/;

function inlineDanger(attribute: ts.JsxAttribute): boolean {
  const value = attribute.initializer;
  if (!value || !ts.isJsxExpression(value) || !value.expression) {
    return false;
  }
  const style = value.expression;
  return (
    ts.isObjectLiteralExpression(style) &&
    style.properties.some(
      (member) =>
        ts.isPropertyAssignment(member) &&
        member.name.getText().replaceAll(/["']/g, "") === "color" &&
        ts.isStringLiteralLike(member.initializer) &&
        member.initializer.text.trim() === "var(--dangerText)",
    )
  );
}

function classTokens(attribute: ts.JsxAttribute): string[] {
  return attribute.initializer
    ? fragmentsOf(attribute.initializer).flatMap((fragment) =>
        fragment.split(/\s+/),
      )
    : [];
}

function tagOf(attribute: ts.JsxAttribute): string {
  return attribute.parent.parent.tagName.getText();
}

function dangerClassOn(attribute: ts.JsxAttribute): string | undefined {
  const token = classTokens(attribute).find((name) => dangerClass.test(name));
  return token ? `<${tagOf(attribute)}> carries the ${token} class` : undefined;
}

function sheetClassOn(
  attribute: ts.JsxAttribute,
  sheetClasses: ReadonlySet<string>,
): string | undefined {
  const token = classTokens(attribute).find((name) => sheetClasses.has(name));
  return token
    ? `<${tagOf(attribute)}> carries .${token}, a rule that only sets the danger ink`
    : undefined;
}

const dangerInk = /^color\s*:\s*var\(--dangerText\b[^)]*\)\s*(!important)?$/;

const margin = /^margin(-[a-z]+)*\s*:/;

export type Sheet = { where: string; text: string };

const designSystemSheet = /^frontend\/src\/design-system\//;

/**
 * The classes a sheet spells as the danger line. Every rule whose selector is
 * that class alone sets the danger ink and margins, and nothing else. One rule
 * that also draws a border or a layout makes the class something else.
 */
export function dangerOnlyClasses(sheets: readonly Sheet[]): Set<string> {
  const inked = new Set<string>();
  const other = new Set<string>();
  const spelledBy = sheets.filter(
    ({ where }) => !designSystemSheet.test(where),
  );
  for (const rule of spelledBy.flatMap(({ text }) => rulesIn(text))) {
    if (rule.parents.length > 0) {
      continue;
    }
    const declarations = rule.body
      .split(";")
      .map((declaration) => declaration.trim())
      .filter((declaration) => declaration.length > 0);
    for (const selector of selectorList(rule.selector)) {
      const alone = /^\.([\w-]+)$/.exec(selector);
      if (!alone || dangerClass.test(alone[1]) || alone[1] === "field-error") {
        continue;
      }
      // A margin-only rule, a breakpoint's spacing say, leaves the class unchanged.
      if (declarations.every((declaration) => margin.test(declaration))) {
        continue;
      }
      const onlyInk = declarations.every(
        (declaration) =>
          dangerInk.test(declaration) || margin.test(declaration),
      );
      (onlyInk ? inked : other).add(alone[1]);
    }
  }
  return new Set([...inked].filter((name) => !other.has(name)));
}

type Opening = ts.JsxOpeningElement | ts.JsxSelfClosingElement;

/** A DOM element rather than a component: a lower-case bare tag name. */
function isIntrinsic(element: Opening): boolean {
  return (
    ts.isIdentifier(element.tagName) && /^[a-z]/.test(element.tagName.text)
  );
}

function carriesAlert(element: Opening): boolean {
  return element.attributes.properties.some((property) => {
    if (!ts.isJsxAttribute(property) || property.name.getText() !== "role") {
      return false;
    }
    const value = property.initializer;
    const literal =
      value && ts.isJsxExpression(value) ? value.expression : value;
    return (
      literal !== undefined &&
      ts.isStringLiteralLike(literal) &&
      literal.text === "alert"
    );
  });
}

/** Whether a JSX element above this one is an alert, which owns its cause. */
function insideAlert(element: ts.JsxElement): boolean {
  for (let at = element.parent; !ts.isSourceFile(at); at = at.parent) {
    if (ts.isJsxElement(at) && carriesAlert(at.openingElement)) {
      return true;
    }
  }
  return false;
}

/** A line: something to read, and no element inside it. */
function isLine(element: ts.JsxElement): boolean {
  const children = ownChildren(element);
  return (
    children.length > 0 &&
    !children.some(
      (child) =>
        ts.isJsxElement(child) ||
        ts.isJsxSelfClosingElement(child) ||
        ts.isJsxFragment(child),
    )
  );
}

function callsProblemMessage(node: ts.Node): boolean {
  if (!ts.isCallExpression(node)) {
    return false;
  }
  const callee = node.expression;
  const name = ts.isPropertyAccessExpression(callee)
    ? callee.name.text
    : callee.getText();
  return name === "problemMessageOf";
}

/** Children that carry something: whitespace between tags is not a child. */
function ownChildren(element: ts.JsxElement): ts.JsxChild[] {
  return element.children.filter(
    (child) => !ts.isJsxText(child) || child.text.trim().length > 0,
  );
}

/** Whether a problemMessageOf call appears anywhere under a node. */
function containsProblemMessage(node: ts.Node): boolean {
  return (
    callsProblemMessage(node) ||
    ts.forEachChild(node, containsProblemMessage) === true
  );
}

/**
 * The names a module gives a problem message: a variable initialised from a
 * call, or a prop handed one. A message usually reaches its element that way,
 * one hop from the call, and a reader of the call alone would miss it.
 */
function messageNames(source: ts.SourceFile): Set<string> {
  const names = new Set<string>();
  const visit = (node: ts.Node): void => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.initializer &&
      containsProblemMessage(node.initializer)
    ) {
      names.add(node.name.text);
    }
    if (
      ts.isJsxAttribute(node) &&
      node.initializer &&
      containsProblemMessage(node.initializer)
    ) {
      names.add(node.name.getText());
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return names;
}

function saysOnlyAProblem(element: ts.JsxElement, names: Set<string>): boolean {
  const children = ownChildren(element);
  if (children.length !== 1 || !ts.isJsxExpression(children[0])) {
    return false;
  }
  const said = children[0].expression;
  return (
    said !== undefined &&
    (callsProblemMessage(said) ||
      (ts.isIdentifier(said) && names.has(said.text)))
  );
}

function insideImport(node: ts.Node): boolean {
  for (let at: ts.Node | undefined = node; at; at = at.parent) {
    if (ts.isImportDeclaration(at)) {
      return true;
    }
  }
  return false;
}

/** Every finding in one module, waived or not, each with its waiver. */
export function findingsIn(
  source: ts.SourceFile,
  where: string,
  sheetClasses: ReadonlySet<string> = new Set(),
): (Finding & { waiver: Waiver })[] {
  const lines = source.text.split("\n");
  const out: (Finding & { waiver: Waiver })[] = [];
  const names = messageNames(source);
  const push = (arm: Arm, node: ts.Node, says: string) => {
    const line =
      source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;
    out.push({ arm, line, says, waiver: waiverFor(lines, line) });
  };
  const visit = (node: ts.Node): void => {
    if (ts.isJsxAttribute(node)) {
      const name = node.name.getText();
      if (name === "style" && inlineDanger(node)) {
        push("inline", node, "style colours the text var(--dangerText)");
      }
      const classed = name === "className" && dangerClassOn(node);
      if (classed) {
        push("class", node, classed);
      }
      const sheeted = name === "className" && sheetClassOn(node, sheetClasses);
      if (sheeted) {
        push("sheet", node, sheeted);
      }
    }
    if (
      ts.isJsxElement(node) &&
      isIntrinsic(node.openingElement) &&
      saysOnlyAProblem(node, names) &&
      !insideAlert(node)
    ) {
      push(
        "message",
        node,
        `<${node.openingElement.tagName.getText()}> says a failure message outside ErrorLine`,
      );
    }
    if (
      ts.isJsxElement(node) &&
      isIntrinsic(node.openingElement) &&
      carriesAlert(node.openingElement) &&
      isLine(node) &&
      !ALERT_OWNERS.has(where)
    ) {
      push(
        "alert",
        node,
        `<${node.openingElement.tagName.getText()}> hand-rolls an alerting line`,
      );
    }
    if (
      ts.isIdentifier(node) &&
      node.text === "RefusalLine" &&
      !insideImport(node)
    ) {
      push("name", node, "RefusalLine is the retired spelling");
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return out;
}

type Located = Finding & { where: string; waiver: Waiver };

function shown(finding: Located): string {
  return `${finding.where}:${finding.line}  ${finding.says} — ${advice}`;
}

// Every shipped TSX module, core and extension tier alike, but the home.
const isModule = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.path.endsWith(".tsx") &&
  !/\.(test|spec|stories)\.tsx$/.test(file.path) &&
  fromRepo(file.path) !== HOME_MODULE;

const sheetsOf = (files: readonly CensusFile[]): Sheet[] =>
  files
    .filter(isAppSheet)
    .map((file) => ({ where: fromRepo(file.path), text: file.text }));

// Every finding that is not waived with a reason, and every bare waiver.
function spellings(corpus: readonly CensusFile[], sheets: readonly Sheet[]) {
  const sheetClasses = dangerOnlyClasses(sheets);
  const all: Located[] = corpus.flatMap((file) => {
    const where = fromRepo(file.path);
    return file.source
      ? findingsIn(file.source, where, sheetClasses).map((finding) => ({
          where,
          ...finding,
        }))
      : [];
  });
  return [
    ...all.filter((finding) => finding.waiver !== "reasoned").map(shown),
    ...all
      .filter((finding) => finding.waiver === "bare")
      .map(
        (finding) =>
          `${shown(finding)} (ds:ignore says a line was skipped and not why; write the reason after it)`,
      ),
  ];
}

export const rule: CensusRule = {
  name: "errorline-spelling",
  corpus: (tree) => tree.files.filter(isModule),
  floors: (corpus, tree) => {
    const home = tree.files.find((file) => fromRepo(file.path) === HOME_MODULE);
    const homeClasses = home?.source
      ? findingsIn(home.source, HOME_MODULE).filter(
          (finding) => finding.arm === "class",
        )
      : [];
    const owners = [...ALERT_OWNERS.keys()].filter(
      (owner) =>
        !tree.files.some(
          (file) =>
            fromRepo(file.path) === owner && file.text.includes('role="alert"'),
        ),
    );
    return [
      ...(corpus.length > 400 ? [] : ["400 or fewer modules were read"]),
      ...(sheetsOf(tree.files).length > 100
        ? []
        : ["100 or fewer stylesheets were read"]),
      ...(homeClasses.length > 0
        ? []
        : ["errorline.tsx must wear t-danger, or the class arm reads nothing"]),
      ...owners.map((owner) => `${owner} owns no role="alert"; drop it`),
    ];
  },
  across: (corpus, tree) => spellings(corpus, sheetsOf(tree.files)),
  advice:
    "Each of these spells a failure line by hand. A failure said on one line under a control is ErrorLine (standing when it is not news).",
};
