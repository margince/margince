// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Action-row gate: two buttons side by side sit in a row that says so.
//
// The spacing gates read stylesheets, and a stylesheet cannot see the one
// failure that hurts most — a row with NO rule at all. `.modal-actions`,
// `.pn-actions` and `.worklist-manager-control` were spelled in ten places in
// the markup and in no stylesheet: ten rows of buttons with no gap, no
// alignment and no margin, rendering as bare inline elements against each
// other. Every gate in the tree passed them, because absent CSS is not wrong
// CSS — there is nothing there to be wrong. The class name is the join, and
// only a gate that reads both sides can follow it.
//
// So this one reads the markup and asks the stylesheets what the markup gets:
//
//   an ACTION ROW is a JSX container whose element children are two or more
//   buttons and nothing else, and it takes `gap: var(--gapActions)` — from a
//   class it names, or from its own inline style.
//
// "and nothing else" is what keeps it from firing on correct code. A container
// holding a label, a field and a button has a general layout gap, and the space
// between its buttons is incidental to it; demanding the action role there
// would retune a row that is not one. `.lead-line` is the worked example — a
// labelled line that in one of its three uses happens to hold only verbs.
//
// WHAT IT DOES NOT JUDGE, and why:
//   - a container that is a COMPONENT (`<OverflowMenu>`, `<PanelBody>`). The
//     component owns its own layout, and the class it renders is its business.
//   - a fragment whose element ancestor is not in the same expression: `<>{a}
//     {b}</>` handed to an `actions` prop is spaced by whoever receives it, and
//     the AST cannot say who that is.
// Both are stated rather than silent, because a skip nobody wrote down reads as
// coverage.
//
// A row with a reason to differ is waived in line, on the element or the line
// above it: `{/* ds:ignore <reason> */}`.

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  extensionLayers,
  filesMatching,
  filesUnder,
  scriptKindFor,
} from "../../scripts/lib/source-tree";

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const srcDir = join(frontendRoot, "src");
const designSystem = join(srcDir, "design-system");
const extensionsDir = join(frontendRoot, "..", "extensions");

// The role this gate demands, and the file that owns it. Read rather than
// assumed: renaming the token in tokens.css without renaming it here would
// leave every comparison below asking for a property that resolves to nothing,
// and a gate demanding a value nobody can spell fails on correct code.
const ROLE = "--gapActions";
const ROLE_VALUE = `var(${ROLE})`;

// The controls that ARE buttons, named rather than derived: what makes a
// component a button is that it renders one, and nothing in the tree states
// that in a form this gate could read. The names are checked against the design
// system below, so a rename fails here loudly instead of quietly emptying the
// set — which is the way this gate would otherwise go dark.
const BUTTONS = new Set(["Button", "IconAction"]);

const GAP = /(?:^|;)\s*(?:column-)?gap\s*:\s*([^;]+)/;
const STYLESHEET = /\.css$/;

/** The gap an inline `style={{ … }}` sets, if it sets one. */
function inlineGap(style: string): string | undefined {
  const found = /\bgap\s*:\s*("[^"]*"|'[^']*'|`[^`]*`|[^,}]+)/.exec(style);
  if (!found) return undefined;
  return found[1].trim().replace(/^["'`]|["'`]$/g, "");
}

/** Every gap value the tree gives a class, from any stylesheet in it. */
function gapsByClass(sheets: string[]): Map<string, Set<string>> {
  const gaps = new Map<string, Set<string>>();
  for (const sheet of sheets) {
    const css = readFileSync(sheet, "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
    for (const rule of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
      const declared = GAP.exec(rule[2]);
      if (!declared) continue;
      // The class a rule STYLES is the last one in it: `.board > .actions`
      // spaces the row, not the board it sits in.
      for (const part of rule[1].split(/,(?![^(]*\))/)) {
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

type Row = Readonly<{
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
 * The tags an embedded expression can render, unwrapping the three shapes a
 * conditional child takes. An expression that resolves to anything else — a
 * call, a map, a variable — yields nothing here, and the caller reads that as
 * content rather than as a button.
 */
function yieldedTags(
  expression: ts.Expression | undefined,
  source: ts.SourceFile,
): readonly string[] {
  if (!expression) return [];
  if (ts.isParenthesizedExpression(expression)) {
    return yieldedTags(expression.expression, source);
  }
  if (
    ts.isBinaryExpression(expression) &&
    (expression.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken ||
      expression.operatorToken.kind === ts.SyntaxKind.QuestionQuestionToken)
  ) {
    return yieldedTags(expression.right, source);
  }
  if (ts.isConditionalExpression(expression)) {
    return [
      ...yieldedTags(expression.whenTrue, source),
      ...yieldedTags(expression.whenFalse, source),
    ];
  }
  if (isElement(expression)) return [tagOf(expression, source)];
  // `{null}` and `{undefined}` render nothing, so they claim nothing.
  if (
    expression.kind === ts.SyntaxKind.NullKeyword ||
    (ts.isIdentifier(expression) && expression.text === "undefined")
  ) {
    return [];
  }
  return ["\u0000not-an-element"];
}

/** True when this container holds two or more buttons and nothing else. */
function isActionRow(
  node: ts.JsxElement | ts.JsxFragment,
  source: ts.SourceFile,
): boolean {
  let buttons = 0;
  for (const child of node.children) {
    if (ts.isJsxText(child)) {
      if (child.getText(source).trim() !== "") return false;
      continue;
    }
    if (isElement(child)) {
      if (!BUTTONS.has(tagOf(child, source))) return false;
      buttons++;
      continue;
    }
    // `{cond && <Button/>}` is a button the row may or may not draw; anything
    // else in an expression is content, and content makes this a layout row.
    //
    // Judged by what the expression RESOLVES to, never by whether a button
    // appears anywhere in its text: `{expanded && <Card>…</Card>}` has buttons
    // deep inside it, and reading the text called a disclosure row a row of two
    // buttons.
    if (ts.isJsxExpression(child)) {
      const yielded = yieldedTags(child.expression, source);
      if (yielded.length === 0) continue;
      if (!yielded.every((tag) => BUTTONS.has(tag))) return false;
      buttons++;
    }
  }
  return buttons >= 2;
}

/** The element that draws the box — a fragment draws none, so walk out of it. */
function hostOf(
  node: ts.JsxElement | ts.JsxFragment,
): ts.JsxElement | undefined {
  let here: ts.Node = node;
  while (ts.isJsxFragment(here)) {
    const parent = here.parent?.parent;
    if (!parent || !(ts.isJsxElement(parent) || ts.isJsxFragment(parent))) {
      return undefined;
    }
    here = parent;
  }
  return ts.isJsxElement(here) ? here : undefined;
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

function actionRows(
  file: string,
  text: string,
  gaps: Map<string, Set<string>>,
): readonly Row[] {
  const source = ts.createSourceFile(
    file,
    text,
    ts.ScriptTarget.Latest,
    true,
    scriptKindFor(file),
  );
  const rows: Row[] = [];

  const visit = (node: ts.Node): void => {
    if (
      (ts.isJsxElement(node) || ts.isJsxFragment(node)) &&
      isActionRow(node, source)
    ) {
      const host = hostOf(node);
      const open = host?.openingElement;
      const tag = open ? open.tagName.getText(source) : "";
      // A component owns its own layout; an unresolvable fragment has no box.
      if (
        open &&
        !/^[A-Z]/.test(tag) &&
        !waived(source, node.getStart(source))
      ) {
        const className = attributeText(open, "className", source);
        const inline = attributeText(open, "style", source);
        const named = [...className.matchAll(/[A-Za-z][A-Za-z0-9_-]*/g)].map(
          (match) => match[0],
        );
        const spelledInline = inlineGap(inline);
        const declared = [
          ...named.flatMap((name) => [...(gaps.get(name) ?? [])]),
          ...(spelledInline === undefined ? [] : [spelledInline]),
        ].filter((value) => value !== "");
        const { line } = source.getLineAndCharacterOfPosition(
          node.getStart(source),
        );
        rows.push({
          file,
          line: line + 1,
          found:
            declared.length === 0
              ? `${className || "<no class>"} — nothing in the tree gives it a gap`
              : declared.join(" / "),
          ok:
            declared.length > 0 &&
            declared.every((value) => value === ROLE_VALUE),
        });
      }
    }
    ts.forEachChild(node, visit);
  };

  visit(source);
  return rows;
}

describe("two buttons side by side sit in a row that says so", () => {
  // The stylesheets, from the same walk the surfaces come from: a class named
  // in a unit's screen may be styled in the core's sheet and the other way
  // round, so both trees feed both sides.
  const sheets = [
    ...filesMatching(srcDir, STYLESHEET),
    ...extensionLayers(extensionsDir).flatMap((layer) =>
      filesMatching(layer, STYLESHEET),
    ),
  ];
  const surfaces = [
    ...filesUnder(srcDir),
    ...extensionFrontendFiles(extensionsDir),
  ].filter((f) => /\.tsx$/.test(f) && !/\.(test|stories)\.tsx$/.test(f));

  it("demands a role the token layer defines", () => {
    // The gate's own subject, checked against its owner: without this, renaming
    // the token leaves every row below asking for a property that resolves to
    // nothing, and the gate fails the whole tree for a reason that is its own.
    const tokens = readFileSync(join(designSystem, "tokens.css"), "utf8");
    expect(
      new RegExp(`^\\s*${ROLE}\\s*:`, "m").test(tokens),
      `${ROLE} is not defined in tokens.css — rename it in both places`,
    ).toBe(true);
  });

  it("names controls the design system still exports", () => {
    // The button set cannot be derived — what makes a component a button is
    // that it renders one — so it is named. Named lists go stale silently: a
    // renamed control simply stops being counted, every row holding two of them
    // stops being an action row, and the gate reports a clean tree. This is the
    // assertion that turns that into a failure.
    const catalog = filesUnder(designSystem)
      .filter((f) => /\.tsx?$/.test(f) && !/\.(test|stories)\./.test(f))
      .map((f) => readFileSync(f, "utf8"))
      .join("\n");
    for (const control of BUTTONS) {
      expect(
        new RegExp(`export (function|const) ${control}\\b`).test(catalog),
        `${control} is not exported from src/design-system/ — this gate counts a control that no longer exists`,
      ).toBe(true);
    }
  });

  it("reads both trees, and finds rows in them", () => {
    // Three floors, because each can fall while the others hold. A census that
    // judged nothing certifies nothing, and the shape that fails short here is
    // "the detector matched no rows" — which prints exactly like a clean tree.
    expect(surfaces.length).toBeGreaterThan(100);
    expect(
      surfaces.some((f) => f.startsWith(`${extensionsDir}/`)),
      "the census covered frontend/src but no extension frontend layer",
    ).toBe(true);
    expect(sheets.length).toBeGreaterThan(10);
    const gaps = gapsByClass(sheets);
    const rows = surfaces.flatMap((file) =>
      actionRows(file, readFileSync(file, "utf8"), gaps),
    );
    expect(
      rows.length,
      "no action row was recognised anywhere in the tree — the detector is dark",
    ).toBeGreaterThan(20);
  });

  it("gives every action row the action gap", { timeout: 60_000 }, () => {
    const gaps = gapsByClass(sheets);
    const wrong = surfaces
      .flatMap((file) => actionRows(file, readFileSync(file, "utf8"), gaps))
      .filter((row) => !row.ok)
      .map(
        (row) =>
          `${relative(frontendRoot, row.file)}:${row.line}: ${row.found}`,
      );

    expect(
      wrong,
      `Two buttons side by side take ${ROLE_VALUE}. Put them in a row that ` +
        `declares it — .form-actions inside a form, .actions inside a Modal, ` +
        `.card-actions under a card's body — or give the row's own class that ` +
        `gap. A row with a reason to differ is waived in line: ` +
        `{/* ds:ignore <reason> */}.`,
    ).toEqual([]);
  });
});

// The rule itself, on sources written here. Everything above asserts that the
// TREE is clean, and a clean tree is exactly what a rule that stopped checking
// anything also reports — so each case below states one half of the rule and
// fails when that half goes missing.
describe("what counts as an action row", () => {
  const gaps = new Map<string, Set<string>>([
    ["verbs", new Set([ROLE_VALUE])],
    ["loose-verbs", new Set(["var(--space-3)"])],
  ]);
  const rowsIn = (markup: string) =>
    actionRows("fixture.tsx", `export const F = () => (${markup});\n`, gaps);

  it("passes a row whose class carries the action gap", () => {
    expect(
      rowsIn('<div className="verbs"><Button /><Button /></div>').map(
        (r) => r.ok,
      ),
    ).toEqual([true]);
  });

  it("passes a row that spells the gap inline", () => {
    expect(
      rowsIn(
        '<div style={{ display: "flex", gap: "var(--gapActions)" }}><Button /><Button /></div>',
      ).map((r) => r.ok),
    ).toEqual([true]);
  });

  it("reports a row that carries some other rung", () => {
    expect(
      rowsIn('<div className="loose-verbs"><Button /><Button /></div>').map(
        (r) => r.ok,
      ),
    ).toEqual([false]);
  });

  it("reports a row nothing in the tree styles at all", () => {
    const [row] = rowsIn('<div className="nowhere"><Button /><Button /></div>');
    expect(row?.ok).toBe(false);
    expect(row?.found).toContain("nothing in the tree gives it a gap");
  });

  it("is not a row when the container holds anything else", () => {
    // A label and its verbs is a LAYOUT row: its gap governs the label too, and
    // demanding the action role there retunes something that is not an action
    // row. This is the half that keeps the gate off correct code.
    expect(
      rowsIn(
        '<div className="nowhere"><span>Owner</span><Button /><Button /></div>',
      ),
    ).toEqual([]);
  });

  it("reads a conditional child by what it renders, not by its text", () => {
    // `{expanded && <Card>…<Button/></Card>}` has a button deep inside it. Read
    // as text, a disclosure row counted as a row of two buttons.
    expect(
      rowsIn(
        '<div className="nowhere"><Button />{expanded && <Card><Button /></Card>}</div>',
      ),
    ).toEqual([]);
    expect(
      rowsIn(
        '<div className="verbs"><Button />{canEdit && <Button />}</div>',
      ).map((r) => r.ok),
    ).toEqual([true]);
  });

  it("leaves a component container to the component", () => {
    expect(rowsIn("<OverflowMenu><Button /><Button /></OverflowMenu>")).toEqual(
      [],
    );
  });

  it("honours a waiver on the row or the line above it", () => {
    expect(
      rowsIn(
        '<div className="nowhere" /* ds:ignore a reason */><Button /><Button /></div>',
      ),
    ).toEqual([]);
  });
});
