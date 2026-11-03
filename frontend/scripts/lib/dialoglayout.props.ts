// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a component's props are, read from its own signature and body: which
// prop an expression reads, its default, and the values its type admits.

import ts from "typescript";
import { bindingOf } from "../../src/testing/classnames";
import { declarations } from "./dialoglayout.names";

export const fnOf = (def: ts.Node) =>
  ts.isVariableDeclaration(def) && def.initializer ? def.initializer : def;

const propsOf = (def: ts.Node) => {
  const fn = fnOf(def);
  return ts.isFunctionLike(fn) ? fn.parameters[0]?.name : undefined;
};

// A destructured prop by the name a call hands it: `{ tone: t }` is `tone`.
const propKey = (b: ts.BindingElement) => (b.propertyName ?? b.name).getText();

// The name a member or attribute is spelled with, not a value read:
// `cfg.intent`, `{ intent: x }`, `intent="…"`, a method or field.
const spelled = (e: ts.Identifier) => {
  const p = e.parent;
  return (
    (ts.isPropertyAccessExpression(p) ||
      ts.isPropertyAssignment(p) ||
      ts.isJsxAttribute(p) ||
      ts.isMethodDeclaration(p) ||
      ts.isPropertyDeclaration(p) ||
      ts.isPropertySignature(p)) &&
    p.name === e
  );
};

// The prop a name unpacked from the props object in the body stands for:
// `className` in `const { className } = props`.
function unpacked(e: ts.Identifier, props: string): string | undefined {
  const declared = bindingOf(e);
  if (!declared || !ts.isVariableDeclaration(declared)) return undefined;
  const from = declared.initializer;
  if (!from || !ts.isIdentifier(from) || from.text !== props) return undefined;
  const source = bindingOf(from);
  if (!source || !ts.isParameter(source)) return undefined;
  if (!ts.isObjectBindingPattern(declared.name)) return undefined;
  const b = declared.name.elements.find((x) => x.name.getText() === e.text);
  return b && !b.dotDotDotToken ? propKey(b) : undefined;
}

const readers = new WeakMap<ts.Node, (e: ts.Node) => string | undefined>();

/** The prop of `def` an expression reads: a destructured binding by its local
 * name, `children` always, or `props.x`. */
export function propReadBy(def: ts.Node) {
  const known = readers.get(def);
  if (known) return known;
  const first = propsOf(def);
  const locals = new Map([["children", "children"]]);
  if (first && ts.isObjectBindingPattern(first)) {
    for (const b of first.elements) locals.set(b.name.getText(), propKey(b));
  }
  const props = first && ts.isIdentifier(first) ? first.text : undefined;
  const read = (e: ts.Node) => {
    if (ts.isIdentifier(e)) {
      if (spelled(e)) return undefined;
      return locals.get(e.text) ?? (props && unpacked(e, props));
    }
    if (ts.isPropertyAccessExpression(e)) {
      return e.expression.getText() === props ? e.name.text : undefined;
    }
    const keyed =
      ts.isElementAccessExpression(e) &&
      ts.isStringLiteralLike(e.argumentExpression) &&
      e.expression.getText() === props;
    return keyed ? e.argumentExpression.text : undefined;
  };
  readers.set(def, read);
  return read;
}

// Whether an expression reads a prop `def` is handed, or that prop `name`.
export function givenTo(def: ts.Node) {
  const read = propReadBy(def);
  return (e: ts.Node, name?: string) => {
    const prop = read(e);
    return prop !== undefined && (name === undefined || prop === name);
  };
}

/** Whether spreading `x` hands on what `def` is handed as `name`: its props
 * object, or the rest it destructures `name` into. */
export function spreads(def: ts.Node, x: ts.Node, name?: string): boolean {
  const first = propsOf(def);
  if (!first || !ts.isIdentifier(x)) return false;
  if (ts.isIdentifier(first)) return first.text === x.text;
  if (!ts.isObjectBindingPattern(first)) return false;
  const rest = first.elements.find((b) => !!b.dotDotDotToken);
  if (!rest || rest.name.getText() !== x.text) return false;
  return !first.elements.some((b) => b !== rest && propKey(b) === name);
}

/** The literal a component's destructured prop defaults to. */
export function presetOf(def: ts.Node, name: string): string | undefined {
  const first = propsOf(def);
  if (!first || !ts.isObjectBindingPattern(first)) return undefined;
  const b = first.elements.find((e) => propKey(e) === name);
  return b?.initializer && ts.isStringLiteral(b.initializer)
    ? b.initializer.text
    : undefined;
}

// The string literals a type admits: a union of them, a local alias of one,
// or `(typeof LIST)[number]` over a local `as const` list.
export function literalTypes(
  type: ts.TypeNode | undefined,
): string[] | undefined {
  if (!type) return undefined;
  if (ts.isParenthesizedTypeNode(type)) return literalTypes(type.type);
  if (ts.isLiteralTypeNode(type) && ts.isStringLiteral(type.literal)) {
    return [type.literal.text];
  }
  if (ts.isUnionTypeNode(type)) {
    const each = type.types.map(literalTypes);
    return each.every((t) => t) ? each.flatMap((t) => t ?? []) : undefined;
  }
  if (ts.isTypeReferenceNode(type)) {
    const alias = type
      .getSourceFile()
      .statements.find(
        (s): s is ts.TypeAliasDeclaration =>
          ts.isTypeAliasDeclaration(s) &&
          s.name.text === type.typeName.getText(),
      );
    return alias && literalTypes(alias.type);
  }
  return ts.isIndexedAccessTypeNode(type) ? listed(type) : undefined;
}

function listed(type: ts.IndexedAccessTypeNode): string[] | undefined {
  const list =
    type.indexType.getText() === "number"
      ? /^\(?typeof (\w+)\)?$/.exec(type.objectType.getText())?.[1]
      : undefined;
  const held = list ? declarations(type.getSourceFile()).get(list) : undefined;
  const init =
    held && ts.isVariableDeclaration(held) ? held.initializer : undefined;
  const array = init && ts.isAsExpression(init) ? init.expression : init;
  if (!array || !ts.isArrayLiteralExpression(array)) return undefined;
  const items = array.elements.filter(ts.isStringLiteral).map((e) => e.text);
  return items.length === array.elements.length ? items : undefined;
}

/** The values a component's prop is typed to take, where its type spells them. */
export function domainOf(def: ts.Node, name: string): string[] | undefined {
  const fn = fnOf(def);
  let type = ts.isFunctionLike(fn) ? fn.parameters[0]?.type : undefined;
  while (type && ts.isTypeReferenceNode(type) && type.typeArguments?.length) {
    type = type.typeArguments[0];
  }
  const member =
    type && ts.isTypeLiteralNode(type)
      ? type.members.find(
          (m): m is ts.PropertySignature =>
            ts.isPropertySignature(m) && m.name.getText() === name,
        )
      : undefined;
  return literalTypes(member?.type);
}
