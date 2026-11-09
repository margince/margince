// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Layout is a class, not a style attribute, in every rendered module.
//
// A margin written at a call site belongs to that one element, so a spacing
// decision gets as many homes as elements wearing it. A class is one home, and
// `Stack` / `Row` serve a unit with no stylesheet. Every file is held at zero.
//
// Only a static value is a rule. An identifier, a call, a template with a
// substitution, a conditional or a member access is a value the render
// computed. No stylesheet can hold it.
//
// A wrapper is no exemption. The rule counts through a ternary, a guard, a
// cast, a spread, a nullish default and a hoisted name. It also counts through
// a props spread and the `createElement` call JSX compiles to.

import ts from "typescript";
import { MODULE_FILE } from "../../scripts/lib/source-tree";
import { declarationOf } from "../../src/testing/classnames";
import type { CensusFile, CensusRule } from "../tree";

/**
 * A style property that lays something out, by family: the box's own space,
 * how it flows, and where it sits. Matched as a prefix, so `marginInlineStart`
 * and `overflowX` need no list of their own. The logical sizes name themselves.
 *
 * A static `transform` or `translate` moves the box. `rotate`, `scale` and
 * `opacity` change how a box looks without moving anything.
 */
const LAYOUT_PROPERTY =
  /^(margin|padding|gap|rowGap|columnGap|display|flex|grid|justify|align|place|inset|top|right|bottom|left|width|minWidth|maxWidth|height|minHeight|maxHeight|position|overflow|float|order|boxSizing|aspectRatio|transform|translate|zIndex|inlineSize|blockSize|minInlineSize|maxInlineSize|minBlockSize|maxBlockSize)/;

/** A type annotation that says an object is a style, wherever it is written. */
const STYLE_ANNOTATION = /\bCSSProperties\b/;

/** What the fix is, said the same way at every finding. */
const FIX =
  "a class in the screen's sheet, or `Stack`/`Row` from design-system/stack.tsx";

/** Files that are not the rendered product: their own suite and their own docs. */
const NOT_THE_PRODUCT = new RegExp(`\\.(test|stories)${MODULE_FILE.source}`);

/** `x as const`, `x satisfies T`, `(x)`, `x!`: wrappers that change no value. */
function unwrap(node: ts.Expression): ts.Expression {
  let current = node;
  while (
    ts.isAsExpression(current) ||
    ts.isSatisfiesExpression(current) ||
    ts.isParenthesizedExpression(current) ||
    ts.isTypeAssertionExpression(current) ||
    ts.isNonNullExpression(current)
  ) {
    current = current.expression;
  }
  return current;
}

/**
 * Whether this value is a rule rather than a reading. A no-substitution
 * template is the literal the quotes would have made, and the parser hands
 * `-4` over as an operator and a number.
 */
function isStatic(value: ts.Expression): boolean {
  const bare = unwrap(value);
  if (
    ts.isPrefixUnaryExpression(bare) &&
    (bare.operator === ts.SyntaxKind.MinusToken ||
      bare.operator === ts.SyntaxKind.PlusToken)
  ) {
    return ts.isNumericLiteral(bare.operand);
  }
  return (
    ts.isStringLiteral(bare) ||
    ts.isNumericLiteral(bare) ||
    ts.isNoSubstitutionTemplateLiteral(bare)
  );
}

/** The property this assignment names, including the computed constant form. */
function propertyName(name: ts.PropertyName): string | undefined {
  if (ts.isIdentifier(name) || ts.isStringLiteral(name)) return name.text;
  if (!ts.isComputedPropertyName(name)) return undefined;
  const key = unwrap(name.expression);
  return ts.isStringLiteral(key) || ts.isNoSubstitutionTemplateLiteral(key)
    ? key.text
    : undefined;
}

/**
 * Every object literal an expression can hand to an element, however it is
 * wrapped, including a name the expression's scope binds.
 *
 * It reads through a call, so `style={merge({ marginTop: "4px" })}` is a
 * finding: the literal is a layout rule whatever the function does with it.
 */
function styleObjectsUnder(root: ts.Expression): ts.ObjectLiteralExpression[] {
  const found: ts.ObjectLiteralExpression[] = [];
  const resolved = new Set<ts.Node>();
  const visit = (node: ts.Node): void => {
    // An object is walked by hand, so a property's name is never resolved
    // against a module constant of the same name.
    if (ts.isObjectLiteralExpression(node)) {
      found.push(node);
      for (const property of node.properties) {
        if (ts.isPropertyAssignment(property)) visit(property.initializer);
        else if (ts.isSpreadAssignment(property)) visit(property.expression);
      }
      return;
    }
    if (ts.isPropertyAccessExpression(node)) {
      visit(node.expression);
      return;
    }
    if (ts.isIdentifier(node)) {
      const declared = declarationOf(node);
      if (!declared || resolved.has(declared)) return;
      resolved.add(declared);
      if (ts.isVariableDeclaration(declared) && declared.initializer) {
        visit(declared.initializer);
      }
      return;
    }
    ts.forEachChild(node, visit);
  };
  visit(root);
  return found;
}

/** The call JSX compiles to, and the one a file may write by hand. */
const RENDER_CALL = /(^|\.)(createElement|jsx|jsxs|jsxDEV)$/;

/** The `style` a props object carries, however that object is written. */
function styleOfProps(props: ts.Expression): ts.Expression[] {
  return styleObjectsUnder(props).flatMap((object) =>
    object.properties.flatMap((property) =>
      ts.isPropertyAssignment(property) &&
      propertyName(property.name) === "style"
        ? [property.initializer]
        : [],
    ),
  );
}

/**
 * Every expression handed to an element as its `style`: the attribute, a
 * spread of a props object, and the call both compile to.
 */
function styleValues(node: ts.Node): ts.Expression[] {
  if (ts.isJsxAttribute(node)) {
    if (!ts.isIdentifier(node.name) || node.name.text !== "style") return [];
    const value = node.initializer;
    if (value === undefined || !ts.isJsxExpression(value)) return [];
    return value.expression === undefined ? [] : [value.expression];
  }
  if (ts.isJsxSpreadAttribute(node)) return styleOfProps(node.expression);
  if (
    ts.isCallExpression(node) &&
    RENDER_CALL.test(node.expression.getText())
  ) {
    const props = node.arguments[1];
    return props === undefined ? [] : styleOfProps(props);
  }
  return [];
}

/**
 * A style object written apart from any element, such as
 * `const row: CSSProperties = …`. It is declared to be a style, so it counts
 * even in a `.ts` module with no JSX.
 */
function annotatedStyles(source: ts.SourceFile): ts.ObjectLiteralExpression[] {
  const found: ts.ObjectLiteralExpression[] = [];
  const visit = (node: ts.Node) => {
    if (
      ts.isVariableDeclaration(node) &&
      node.type !== undefined &&
      node.initializer !== undefined &&
      STYLE_ANNOTATION.test(node.type.getText())
    ) {
      found.push(...styleObjectsUnder(node.initializer));
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

export type InlineLayout = { where: string; line: number; property: string };

/** Every static layout rule written inline in one parsed module, by line. */
export function inlineLayoutIn(
  where: string,
  source: ts.SourceFile,
): InlineLayout[] {
  // A set, because one literal is reachable from the element that wears it
  // and from its own annotated declaration.
  const objects = new Set<ts.ObjectLiteralExpression>();
  const visit = (node: ts.Node) => {
    for (const style of styleValues(node)) {
      for (const object of styleObjectsUnder(style)) objects.add(object);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  for (const object of annotatedStyles(source)) objects.add(object);

  const found: InlineLayout[] = [];
  for (const object of objects) {
    for (const property of object.properties) {
      if (!ts.isPropertyAssignment(property)) continue;
      const property_name = propertyName(property.name);
      if (property_name === undefined) continue;
      if (!LAYOUT_PROPERTY.test(property_name)) continue;
      if (!isStatic(property.initializer)) continue;
      found.push({
        where,
        line:
          source.getLineAndCharacterOfPosition(property.getStart()).line + 1,
        property: property_name,
      });
    }
  }
  return found.sort((a, b) => a.line - b.line);
}

// Everything under src/ plus each unit's own layer, in every dialect. A `.ts`
// module can hold a `CSSProperties` object. The Storybook config ships in no app.
const rendered = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.source !== undefined &&
  !NOT_THE_PRODUCT.test(file.path);

// One file of each shape: flat, nested, outside `features/`, a `.ts` module
// with no JSX, and a unit's own layer.
const SHAPES = [
  "src/features/companies/company360.tsx",
  "src/features/deals/deal360/dealactions.tsx",
  "src/design-system/trust.tsx",
  "src/design-system/anchoredpopup.ts",
  "../extensions/openchannel/frontend/screen.tsx",
];

export const rule: CensusRule = {
  name: "inlinelayout",
  corpus: (tree) => tree.files.filter(rendered),
  // A census over a smaller tree passes too, so the floor sits under the real
  // count and one file of each shape is named.
  floors: (corpus) => {
    const read = new Set(corpus.map((file) => file.rel));
    return [
      ...(corpus.length > 900 ? [] : ["fewer than 900 modules were read"]),
      ...SHAPES.filter((rel) => !read.has(rel)).map(
        (rel) => `${rel} is not in the corpus`,
      ),
    ];
  },
  visit: (file) =>
    file.source
      ? inlineLayoutIn(file.rel, file.source).map(
          (one) =>
            `${one.where}:${one.line}: \`${one.property}\` is laid out inline, ${FIX}`,
        )
      : [],
  advice: `Lay nothing out inline. Use ${FIX}.`,
};
