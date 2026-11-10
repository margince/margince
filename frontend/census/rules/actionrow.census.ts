// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Action-row gate: two buttons side by side sit in a row that says so.
//
// A stylesheet gate cannot see a row with no rule at all, and absent CSS is not
// wrong CSS. The class name is the join, so this rule reads the markup and asks
// the stylesheets what each row gets.
//
// An action row is a JSX element whose children are two or more buttons and
// nothing else. It takes `gap: var(--gapActions)`, from a class it names or its
// own inline style. A container that also holds a label or a field has a layout
// gap, and is not judged.
//
// Not judged: a component container, which owns its layout, and a bare
// fragment, whose box belongs to whoever receives it. A fragment inside an
// element draws no box, so its buttons count towards that element.
// A row with a reason to differ is waived in line: `{/* ds:ignore <reason> */}`.

import { join } from "node:path";
import ts from "typescript";
import { selectorList } from "../../scripts/lib/css-rules";
import { parseSource } from "../../scripts/lib/source-tree";
import { type CensusFile, type CensusRule, srcDir } from "../tree";

// The role this gate demands. The floors check tokens.css still defines it.
export const ROLE = "--gapActions";
export const ROLE_VALUE = `var(${ROLE})`;

// The controls that are buttons, named because nothing in the tree states it.
// The floors check the design system still exports each one.
export const BUTTONS = new Set(["Button", "IconAction"]);

const GAP = /(?:^|;)\s*(?:column-)?gap\s*:\s*([^;]+)/;

/** The gap an inline `style={{ … }}` sets, if it sets one. */
function inlineGap(style: string): string | undefined {
  const found = /\bgap\s*:\s*("[^"]*"|'[^']*'|`[^`]*`|[^,}]+)/.exec(style);
  if (!found) return undefined;
  return found[1].trim().replace(/^["'`]|["'`]$/g, "");
}

/** Every gap value the given stylesheets give a class. */
export function gapsByClass(
  sheets: readonly string[],
): Map<string, Set<string>> {
  const gaps = new Map<string, Set<string>>();
  for (const sheet of sheets) {
    const css = sheet.replace(/\/\*[\s\S]*?\*\//g, "");
    for (const rule of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
      const declared = GAP.exec(rule[2]);
      if (!declared) continue;
      // The class a rule styles is the last one in it: `.board > .actions`
      // spaces the row, not the board.
      for (const part of selectorList(rule[1])) {
        const named = [...part.matchAll(/\.([A-Za-z0-9_-]+)/g)];
        const subject = named.at(-1)?.[1];
        if (!subject) continue;
        const values = gaps.get(subject) ?? new Set<string>();
        values.add(declared[1].trim());
        gaps.set(subject, values);
      }
    }
  }
  return gaps;
}

export type Row = Readonly<{
  file: string;
  line: number;
  /** What the row gets today, for the failure message. */
  found: string;
  ok: boolean;
}>;

function isElement(
  node: ts.Node,
): node is ts.JsxElement | ts.JsxSelfClosingElement {
  return ts.isJsxElement(node) || ts.isJsxSelfClosingElement(node);
}

function tagOf(node: ts.Node, source: ts.SourceFile): string {
  if (ts.isJsxElement(node)) return node.openingElement.tagName.getText(source);
  if (ts.isJsxSelfClosingElement(node)) return node.tagName.getText(source);
  return "";
}

/**
 * How many buttons an embedded expression renders, or `undefined` when it can
 * render anything else. A fragment's children sum, and a conditional is judged
 * by its widest branch. A guard renders its right side or nothing.
 */
function yieldedButtons(
  expression: ts.Expression | undefined,
  source: ts.SourceFile,
): number | undefined {
  if (!expression) return 0;
  if (ts.isParenthesizedExpression(expression)) {
    return yieldedButtons(expression.expression, source);
  }
  if (
    ts.isBinaryExpression(expression) &&
    (expression.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken ||
      expression.operatorToken.kind === ts.SyntaxKind.QuestionQuestionToken)
  ) {
    return yieldedButtons(expression.right, source);
  }
  if (ts.isConditionalExpression(expression)) {
    const whenTrue = yieldedButtons(expression.whenTrue, source);
    const whenFalse = yieldedButtons(expression.whenFalse, source);
    if (whenTrue === undefined || whenFalse === undefined) return undefined;
    return Math.max(whenTrue, whenFalse);
  }
  if (ts.isJsxFragment(expression)) return buttonsIn(expression, source);
  if (isElement(expression)) {
    return BUTTONS.has(tagOf(expression, source)) ? 1 : undefined;
  }
  if (
    expression.kind === ts.SyntaxKind.NullKeyword ||
    (ts.isIdentifier(expression) && expression.text === "undefined")
  ) {
    return 0;
  }
  return undefined;
}

function buttonsIn(
  node: ts.JsxElement | ts.JsxFragment,
  source: ts.SourceFile,
): number | undefined {
  let buttons = 0;
  for (const child of node.children) {
    if (ts.isJsxText(child)) {
      if (child.getText(source).trim() !== "") return undefined;
      continue;
    }
    if (ts.isJsxFragment(child)) {
      const nested = buttonsIn(child, source);
      if (nested === undefined) return undefined;
      buttons += nested;
      continue;
    }
    if (isElement(child)) {
      if (!BUTTONS.has(tagOf(child, source))) return undefined;
      buttons++;
      continue;
    }
    // Judged by what the expression resolves to, not by its text: a button deep
    // inside `{expanded && <Card>…</Card>}` is not a button of this row.
    if (ts.isJsxExpression(child)) {
      const yielded = yieldedButtons(child.expression, source);
      if (yielded === undefined) return undefined;
      buttons += yielded;
    }
  }
  return buttons;
}

function attributeText(
  open: ts.JsxOpeningElement,
  name: string,
  source: ts.SourceFile,
): string {
  for (const attribute of open.attributes.properties) {
    if (!ts.isJsxAttribute(attribute)) continue;
    if (attribute.name.getText(source) !== name) continue;
    return attribute.initializer?.getText(source) ?? "";
  }
  return "";
}

function waived(source: ts.SourceFile, position: number): boolean {
  const { line } = source.getLineAndCharacterOfPosition(position);
  const lines = source.getFullText().split("\n");
  return [lines[line - 1], lines[line]].some((text) =>
    (text ?? "").includes("ds:ignore"),
  );
}

function rowAt(
  node: ts.JsxElement,
  file: string,
  source: ts.SourceFile,
  gaps: Map<string, Set<string>>,
): Row {
  const open = node.openingElement;
  const className = attributeText(open, "className", source);
  const named = [...className.matchAll(/[A-Za-z][A-Za-z0-9_-]*/g)].map(
    (match) => match[0],
  );
  const spelledInline = inlineGap(attributeText(open, "style", source));
  const declared = [
    ...named.flatMap((name) => [...(gaps.get(name) ?? [])]),
    ...(spelledInline === undefined ? [] : [spelledInline]),
  ].filter((value) => value !== "");
  const { line } = source.getLineAndCharacterOfPosition(node.getStart(source));
  return {
    file,
    line: line + 1,
    found:
      declared.length === 0
        ? `${className || "<no class>"} — nothing in the tree gives it a gap`
        : declared.join(" / "),
    ok: declared.length > 0 && declared.every((value) => value === ROLE_VALUE),
  };
}

/** Every action row in one source text. */
export function actionRows(
  file: string,
  text: string,
  gaps: Map<string, Set<string>>,
): readonly Row[] {
  const source = parseSource(file, text);
  const rows: Row[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isJsxElement(node) &&
      (buttonsIn(node, source) ?? 0) >= 2 &&
      // A component owns its own layout.
      !/^[A-Z]/.test(node.openingElement.tagName.getText(source)) &&
      !waived(source, node.getStart(source))
    ) {
      rows.push(rowAt(node, file, source, gaps));
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return rows;
}

const inLayers = (file: CensusFile) =>
  file.area === "src" || file.area === "extensions";
const isSurface = (file: CensusFile) =>
  inLayers(file) &&
  file.rel.endsWith(".tsx") &&
  !/\.(test|stories)\.tsx$/.test(file.rel);

// A class named in a unit's screen may be styled in the core's sheet and the
// other way round, so both trees feed both sides.
const treeGaps = (files: readonly CensusFile[]) =>
  gapsByClass(
    files
      .filter((file) => inLayers(file) && file.rel.endsWith(".css"))
      .map((file) => file.text),
  );

const designSystem = join(srcDir, "design-system");

export const rule: CensusRule = {
  name: "actionrow",
  corpus: (tree) => tree.files.filter(isSurface),
  // Each floor can fall while the others hold, and a dark detector prints like
  // a clean tree.
  floors: (corpus, tree) => {
    const gaps = treeGaps(tree.files);
    const rows = corpus.flatMap((file) =>
      actionRows(file.path, file.text, gaps),
    );
    const tokens = tree.at(join(designSystem, "tokens.css"))?.text ?? "";
    const catalog = tree.files
      .filter(
        (file) =>
          file.path.startsWith(`${designSystem}/`) &&
          /\.tsx?$/.test(file.rel) &&
          !/\.(test|stories)\./.test(file.rel),
      )
      .map((file) => file.text)
      .join("\n");
    return [
      ...(corpus.length > 100 ? [] : ["fewer than 100 surfaces were read"]),
      ...(corpus.some((file) => file.area === "extensions")
        ? []
        : ["the census covered frontend/src but no extension frontend layer"]),
      ...(rows.length > 20
        ? []
        : ["no action row was recognised anywhere in the tree"]),
      ...(new RegExp(`^\\s*${ROLE}\\s*:`, "m").test(tokens)
        ? []
        : [`${ROLE} is not defined in tokens.css; rename it in both places`]),
      ...[...BUTTONS]
        .filter(
          (control) =>
            !new RegExp(`export (function|const) ${control}\\b`).test(catalog),
        )
        .map(
          (control) =>
            `${control} is not exported from src/design-system/; this gate counts a control that no longer exists`,
        ),
    ];
  },
  across: (corpus, tree) => {
    const gaps = treeGaps(tree.files);
    return corpus
      .flatMap((file) =>
        actionRows(file.path, file.text, gaps).map((row) => ({ file, row })),
      )
      .filter(({ row }) => !row.ok)
      .map(({ file, row }) => `${file.rel}:${row.line}: ${row.found}`);
  },
  advice:
    `Two buttons side by side take ${ROLE_VALUE}. Put them in a row that ` +
    `declares it — .form-actions inside a form, .actions inside a Modal, ` +
    `.card-actions under a card's body — or give the row's own class that ` +
    `gap. A row with a reason to differ is waived in line: ` +
    `{/* ds:ignore <reason> */}.`,
};
