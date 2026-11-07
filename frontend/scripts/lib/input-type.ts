// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import ts from "typescript";
import { declarationOf } from "../../src/testing/classnames";
import {
  declarations,
  definedName,
  definitionOf,
  descendants,
  elementsIn,
  fnOf,
  givenTo,
  keyOf,
  opening,
  spreads,
} from "./dialoglayout.tags";
import { resolveRelative, sourceFileAt } from "./source-tree";

const fallbacks = new Set([
  ts.SyntaxKind.QuestionQuestionToken,
  ts.SyntaxKind.BarBarToken,
  ts.SyntaxKind.AmpersandAmpersandToken,
]);

const isConst = (declared: ts.VariableDeclaration) =>
  (ts.getCombinedNodeFlags(declared) & ts.NodeFlags.Const) !== 0;

/** The expressions a value can evaluate to, through the shapes that pass one
 * along. A name is followed only to the `const` it reads in its own file. */
export function leavesOf(
  node: ts.Expression,
  seen = new Set<ts.Node>(),
): ts.Expression[] {
  if (ts.isConditionalExpression(node)) {
    return [
      ...leavesOf(node.whenTrue, seen),
      ...leavesOf(node.whenFalse, seen),
    ];
  }
  if (ts.isBinaryExpression(node) && fallbacks.has(node.operatorToken.kind)) {
    return [...leavesOf(node.left, seen), ...leavesOf(node.right, seen)];
  }
  if (
    ts.isParenthesizedExpression(node) ||
    ts.isAsExpression(node) ||
    ts.isSatisfiesExpression(node) ||
    ts.isNonNullExpression(node)
  ) {
    return leavesOf(node.expression, seen);
  }
  const declared = ts.isIdentifier(node) ? declarationOf(node) : undefined;
  const init =
    declared && ts.isVariableDeclaration(declared) && isConst(declared)
      ? declared.initializer
      : undefined;
  if (!init) return [node];
  if (seen.has(init)) return [];
  seen.add(init);
  return leavesOf(init, seen);
}

export const literalsOf = (node: ts.Expression): string[] =>
  leavesOf(node).flatMap((leaf) =>
    ts.isStringLiteral(leaf) || ts.isNoSubstitutionTemplateLiteral(leaf)
      ? [leaf.text]
      : [],
  );

const namedType = (name: ts.PropertyName | undefined) =>
  name !== undefined &&
  (ts.isIdentifier(name) || ts.isStringLiteral(name)) &&
  name.text === "type";

/** The `type` values an object can carry. A spread of anything other than an
 * object literal, or a `const` holding one, is not judged. */
export function typeInObject(node: ts.Expression): string[] {
  const own = (property: ts.ObjectLiteralElementLike): string[] => {
    if (ts.isSpreadAssignment(property)) {
      return typeInObject(property.expression);
    }
    if (!namedType(property.name)) return [];
    if (ts.isShorthandPropertyAssignment(property)) {
      return literalsOf(property.name);
    }
    return ts.isPropertyAssignment(property)
      ? literalsOf(property.initializer)
      : [];
  };
  return leavesOf(node).flatMap((leaf) =>
    ts.isObjectLiteralExpression(leaf) ? leaf.properties.flatMap(own) : [],
  );
}

/** The values one JSX attribute list gives `type`. */
export function typeValues(attributes: ts.JsxAttributes): string[] {
  return attributes.properties.flatMap((attribute) => {
    if (ts.isJsxSpreadAttribute(attribute)) {
      return typeInObject(attribute.expression);
    }
    const init = attribute.initializer;
    if (!ts.isIdentifier(attribute.name) || attribute.name.text !== "type") {
      return [];
    }
    if (init && ts.isStringLiteral(init)) return [init.text];
    return init && ts.isJsxExpression(init) && init.expression
      ? literalsOf(init.expression)
      : [];
  });
}

/** The name a tag or a factory's first argument renders: `ns.TextInput` is
 * `TextInput`. */
export const renderedName = (node: ts.Node): string | undefined => {
  if (ts.isIdentifier(node)) return node.text;
  return ts.isPropertyAccessExpression(node) ? node.name.text : undefined;
};

type Source = "props" | "own" | "opaque";

/** What an expression reads, following each local name to the value it is
 * declared with. A later assignment to a `let` is not followed. */
function readsOf(node: ts.Node, seen = new Set<ts.Node>()): ts.Node[] {
  return [node, ...descendants(node)].flatMap((read) => {
    const declared = ts.isIdentifier(read) ? declarationOf(read) : undefined;
    const init =
      declared && ts.isVariableDeclaration(declared)
        ? declared.initializer
        : undefined;
    if (!init || seen.has(init)) return [read];
    seen.add(init);
    return [read, ...readsOf(init, seen)];
  });
}

const readsTypeProp = (node: ts.Node, fn: ts.Node) =>
  readsOf(node).some((read) => givenTo(fn)(read, "type"));

// A local object whose `type` reads the caller's `type` prop.
const objectPassesType = (spread: ts.Expression, fn: ts.Node) =>
  readsOf(spread).some(
    (node) =>
      ts.isObjectLiteralExpression(node) &&
      node.properties.some((property) => {
        if (!namedType(property.name)) return false;
        if (ts.isShorthandPropertyAssignment(property)) {
          return readsTypeProp(property.name, fn);
        }
        return (
          ts.isPropertyAssignment(property) &&
          readsTypeProp(property.initializer, fn)
        );
      }),
  );

function spreadSource(spread: ts.Expression, fn: ts.Node, before: Source) {
  if (spreads(fn, spread, "type") || objectPassesType(spread, fn)) {
    return "props";
  }
  if (typeInObject(spread).length > 0) return "own";
  const known = leavesOf(spread).every(ts.isObjectLiteralExpression);
  return known || before !== "own" ? before : "opaque";
}

// Decided by the last attribute that can write `type`: the caller's `type`
// prop, a value of the component's own, or a spread of unknown shape.
function typeSource(tag: ts.JsxOpeningLikeElement, fn: ts.Node): Source {
  let source: Source = "own";
  for (const attribute of tag.attributes.properties) {
    if (ts.isJsxSpreadAttribute(attribute)) {
      source = spreadSource(attribute.expression, fn, source);
    } else if (attribute.name.getText() === "type") {
      const init = attribute.initializer;
      source = init && readsTypeProp(init, fn) ? "props" : "own";
    }
  }
  return source;
}

type Component = Readonly<{
  key: string;
  name: string;
  fn: ts.Node;
  tags: ts.JsxOpeningLikeElement[];
}>;

const isFunction = (node: ts.Node) =>
  ts.isFunctionDeclaration(node) ||
  ts.isFunctionExpression(node) ||
  ts.isArrowFunction(node);

function componentsIn(file: string): Component[] {
  return [...declarations(sourceFileAt(file)).values()].flatMap((def) => {
    const init = fnOf(def);
    const fn = ts.isCallExpression(init)
      ? init.arguments.find(isFunction)
      : init;
    const name = definedName(def);
    if (!fn || !isFunction(fn) || !/^[A-Z]/.test(name)) return [];
    const tags = elementsIn(fn).flatMap((element) => opening(element) ?? []);
    return [{ key: keyOf(def), name, fn, tags }];
  });
}

/** The modules a re-export file publishes from, as files on disk. */
export const publishedFrom = (file: string): string[] =>
  sourceFileAt(file).statements.flatMap((statement) => {
    const spec = ts.isExportDeclaration(statement)
      ? statement.moduleSpecifier
      : undefined;
    const module =
      spec && ts.isStringLiteral(spec)
        ? resolveRelative(file, spec.text)
        : null;
    return module ? [module] : [];
  });

export type DateHosts = Readonly<{
  /** Every host's own name, which also matches `ns.Name`. */
  names: ReadonlySet<string>;
  /** Every host's definition, which matches a renamed import of it. */
  keys: ReadonlySet<string>;
  /** Names in `notHosts` the derivation still finds. */
  exempted: ReadonlySet<string>;
  /** Components whose input takes `type` from a spread of unknown shape. */
  opaque: readonly string[];
}>;

/** Whether a tag, or a factory's first argument, renders a host: the native
 * input, a host by name, or a renamed import of one. */
export function namesHost(
  hosts: Pick<DateHosts, "names" | "keys">,
  node: ts.Node,
): boolean {
  const name = renderedName(node);
  if (name === undefined || !/^[A-Z]/.test(name)) {
    return name === "input" && ts.isIdentifier(node);
  }
  if (hosts.names.has(name)) return true;
  const def = ts.isIdentifier(node) ? definitionOf(node) : undefined;
  return def !== undefined && hosts.keys.has(keyOf(def));
}

/** Components whose own `type` prop reaches an input's `type` at any depth, also
 * through a spread or a local name or object. `notHosts` are cleared by reading. */
export function deriveDateHosts(
  files: readonly string[],
  notHosts: ReadonlySet<string>,
): DateHosts {
  const components = [...new Set(files)].flatMap(componentsIn);
  const keys = new Set<string>();
  const names = new Set<string>();
  const exempted = new Set<string>();
  const reaches = (component: Component, source: Source) =>
    component.tags.some(
      (tag) =>
        namesHost({ names, keys }, tag.tagName) &&
        typeSource(tag, component.fn) === source,
    );
  for (let grew = true; grew; ) {
    grew = false;
    for (const component of components) {
      const { key, name } = component;
      if (keys.has(key) || exempted.has(name) || !reaches(component, "props")) {
        continue;
      }
      if (notHosts.has(name)) {
        exempted.add(name);
      } else {
        keys.add(key);
        names.add(name);
      }
      grew = true;
    }
  }
  const opaque = components
    .filter(
      (component) =>
        !keys.has(component.key) &&
        !exempted.has(component.name) &&
        reaches(component, "opaque"),
    )
    .map((component) => component.name);
  return { names, keys, exempted, opaque };
}
