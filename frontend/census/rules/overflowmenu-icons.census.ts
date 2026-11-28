// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Overflow-menu gate: an item in a menu is a line of words, never a glyph.
// A seldom-used verb's glyph is a guess; the menu line has room for its words.
// Icon-only lives outside the menu: `IconAction` in design-system/README.md.
//
// It reads every wrapper that passes its children into an `<OverflowMenu>`, but
// not an item's own component (`<ArchiveAction />`), which its story covers.

import ts from "typescript";
import { mayHoldLiteral } from "../../scripts/lib/literal-text";
import { parseSource } from "../../scripts/lib/source-tree";
import type { CensusFile, CensusRule } from "../tree";

// The menu this gate is about, named once. overflowmenu-icons.test.ts checks
// it against the design system, so a rename fails instead of emptying the
// census.
export const MENU = "OverflowMenu";
const GLYPH_PACKAGE = "lucide-react";

/** Every local name in this file that resolves to a glyph from the icon set. */
function glyphImports(source: ts.SourceFile): Set<string> {
  const names = new Set<string>();
  for (const statement of source.statements) {
    if (
      !ts.isImportDeclaration(statement) ||
      !ts.isStringLiteral(statement.moduleSpecifier) ||
      statement.moduleSpecifier.text !== GLYPH_PACKAGE
    ) {
      continue;
    }
    const clause = statement.importClause;
    if (!clause) {
      continue;
    }
    // `import Mail from` and `import * as Icons from` both bind a glyph to one
    // name. The tag reads `Mail` or `Icons.Mail`, and the root of a qualified tag
    // is matched below, so one set holds both.
    if (clause.name) {
      names.add(clause.name.text);
    }
    if (clause.namedBindings) {
      if (ts.isNamespaceImport(clause.namedBindings)) {
        names.add(clause.namedBindings.name.text);
      } else {
        for (const element of clause.namedBindings.elements) {
          names.add(element.name.text);
        }
      }
    }
  }
  return names;
}

function isElement(
  node: ts.Node,
): node is ts.JsxElement | ts.JsxSelfClosingElement {
  return ts.isJsxElement(node) || ts.isJsxSelfClosingElement(node);
}

function tagOf(
  node: ts.JsxElement | ts.JsxSelfClosingElement,
  source: ts.SourceFile,
): string {
  const tag = ts.isJsxElement(node)
    ? node.openingElement.tagName
    : node.tagName;
  return tag.getText(source);
}

/**
 * Why this tag is a glyph rather than a word, or undefined when it is neither.
 *
 * The three answers are the three ways a glyph reaches the markup. Each is
 * named in the finding, because each sends the author to a different line.
 */
function glyphReason(tag: string, glyphs: ReadonlySet<string>): string {
  // `Icons.Mail` is the namespace form; the root is what the import bound.
  const root = tag.split(".")[0];
  const leaf = tag.split(".").at(-1) ?? tag;
  if (glyphs.has(root)) {
    return `\`${tag}\` is a glyph from ${GLYPH_PACKAGE}`;
  }
  if (tag === "svg") {
    return "an `<svg>` is drawn here";
  }
  if (/Icon$/.test(leaf) && leaf !== "Icon") {
    return `\`${tag}\` is named as a glyph`;
  }
  return "";
}

type Finding = Readonly<{ file: string; line: number; why: string }>;

type ModuleText = Readonly<{ path: string; text: string }>;

type ComponentFunction =
  | ts.FunctionDeclaration
  | ts.ArrowFunction
  | ts.FunctionExpression;

type Component = Readonly<{ name: string; fn: ComponentFunction }>;

function componentsOf(source: ts.SourceFile): Component[] {
  const found: Component[] = [];
  const visit = (node: ts.Node) => {
    if (ts.isFunctionDeclaration(node) && node.name) {
      found.push({ name: node.name.text, fn: node });
    }
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.initializer &&
      (ts.isArrowFunction(node.initializer) ||
        ts.isFunctionExpression(node.initializer))
    ) {
      found.push({ name: node.name.text, fn: node.initializer });
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

/** The names a component reads its children by: `children`, and any rename. */
function childrenNames(fn: ComponentFunction): Set<string> {
  const names = new Set(["children"]);
  const props = fn.parameters[0]?.name;
  if (props && ts.isObjectBindingPattern(props)) {
    for (const element of props.elements) {
      const key = element.propertyName ?? element.name;
      if (
        ts.isIdentifier(key) &&
        key.text === "children" &&
        ts.isIdentifier(element.name)
      ) {
        names.add(element.name.text);
      }
    }
  }
  return names;
}

function rendersChildrenInto(
  component: Component,
  source: ts.SourceFile,
  menus: ReadonlySet<string>,
): boolean {
  const names = childrenNames(component.fn);
  const passed = (node: ts.Node): boolean =>
    (ts.isIdentifier(node) && names.has(node.text)) ||
    (ts.isPropertyAccessExpression(node) && node.name.text === "children") ||
    ts.forEachChild(node, passed) === true;
  const inMenu = (node: ts.Node): boolean =>
    (ts.isJsxElement(node) &&
      menus.has(tagOf(node, source)) &&
      node.children.some(passed)) ||
    ts.forEachChild(node, inMenu) === true;
  return component.fn.body !== undefined && inMenu(component.fn.body);
}

/** The menu's tag and every component that renders its children into it,
 *  wrappers of wrappers included, read from the modules so none is missed. */
export function menuTags(modules: readonly ModuleText[]): Set<string> {
  const read = new Map<
    ModuleText,
    { component: Component; source: ts.SourceFile }[]
  >();
  const componentsIn = (module: ModuleText) => {
    const known = read.get(module);
    if (known) return known;
    const source = parseSource(module.path, module.text);
    const found = componentsOf(source).map((component) => ({
      component,
      source,
    }));
    read.set(module, found);
    return found;
  };
  const menus = new Set([MENU]);
  let grew = true;
  while (grew) {
    grew = false;
    // A component renders into a menu only where its module spells that menu.
    const spelled = spellingOf(menus);
    for (const module of modules.filter((each) =>
      mayHoldLiteral(each, spelled),
    )) {
      for (const { component, source } of componentsIn(module)) {
        if (
          !menus.has(component.name) &&
          rendersChildrenInto(component, source, menus)
        ) {
          menus.add(component.name);
          grew = true;
        }
      }
    }
  }
  return menus;
}

const spellingOf = (menus: ReadonlySet<string>) =>
  new RegExp([...menus].map((name) => name.replaceAll("$", "\\$")).join("|"));

/**
 * Every glyph written inside an overflow menu in one file.
 *
 * Taken as text, not read from the path, so the planted cases run the same
 * function the tree does.
 */
export function glyphsInMenus(
  file: string,
  text: string,
  menus: ReadonlySet<string> = new Set([MENU]),
): Finding[] {
  if (!mayHoldLiteral({ text }, spellingOf(menus))) return [];
  const source = parseSource(file, text);
  const glyphs = glyphImports(source);
  const findings: Finding[] = [];

  const inspect = (node: ts.Node) => {
    if (isElement(node)) {
      const why = glyphReason(tagOf(node, source), glyphs);
      if (why) {
        const { line } = source.getLineAndCharacterOfPosition(
          node.getStart(source),
        );
        findings.push({ file, line: line + 1, why });
      }
    }
    ts.forEachChild(node, inspect);
  };

  const visit = (node: ts.Node) => {
    if (ts.isJsxElement(node) && menus.has(tagOf(node, source))) {
      // The children only: the menu's trigger draws the ellipsis, the one glyph that
      // is the control itself.
      for (const child of node.children) {
        inspect(child);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return findings;
}

// Stories and tests included. A menu in a story is a menu a
// reader is shown. A unit's screen is shipped UI in the same bundle, so the
// extension layers are read too.
const surfaces = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.path.endsWith(".tsx");

export const rule: CensusRule = {
  name: "overflowmenu-icons",
  corpus: (tree) => tree.files.filter(surfaces),
  // Two floors, because either can fall while the other holds. An empty walk and
  // a detector that matches no menu both pass a clean tree.
  floors: (corpus) => [
    ...(corpus.length > 100 ? [] : ["fewer than 100 .tsx files were read"]),
    ...(corpus.some((file) => file.area === "extensions")
      ? []
      : ["the census reached no extension frontend layer"]),
    ...(corpus.filter((file) => file.text.includes(`<${MENU}`)).length > 5
      ? []
      : [`no <${MENU}> was found anywhere under src/ — the detector is dark`]),
  ],
  across: (corpus) => {
    const menus = menuTags(corpus);
    return corpus.flatMap((file) =>
      glyphsInMenus(file.path, file.text, menus).map(
        (finding) => `${file.rel}:${finding.line}: ${finding.why}`,
      ),
    );
  },
  advice:
    `Items in an overflow menu carry words, not glyphs. Drop the glyph and ` +
    `keep the label; a verb that needs a glyph to be understood needs a ` +
    `sentence more, and the menu line is where it has room for one. ` +
    `Icon-only lives outside the menu, and only where the glyph IS the ` +
    `verb — see IconAction in design-system/README.md.`,
};
