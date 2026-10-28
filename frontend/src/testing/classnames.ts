// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One reader of what a `className` produces for every markup gate: a second,
// narrower one would read a smaller tree and still report PASS.

import ts from "typescript";

/** One class name a module puts on an element, and the node that carries it. */
export type ClassName = { name: string; at: ts.Node };

/** Class names the module's `className`s can produce, in source order, on `on`
 * alone when given; a token with a computed part (`tone-${level}`) is dropped. */
export function classNamesOn(source: ts.SourceFile, on?: string): ClassName[] {
  const out: ClassName[] = [];
  const read = readerFor(source);
  const following = new Set<ts.Node>();
  const add = (text: string, node: ts.Node) => {
    for (const name of text.split(/\s+/).filter(Boolean)) {
      out.push({ name, at: node });
    }
  };
  const value = (node: ts.Node): void => {
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
      add(node.text, node);
      return;
    }
    const concatenated =
      ts.isBinaryExpression(node) &&
      node.operatorToken.kind === ts.SyntaxKind.PlusToken;
    if (ts.isTemplateExpression(node) || concatenated) {
      const names = read.texts(node).flatMap((t) => tokensOf(t, false));
      add([...new Set(names)].join(" "), node);
      return;
    }
    // Stopping at a bound name would hide an orphan in it; an initializer is
    // not re-entered while read, so a self-reference ends.
    if (ts.isIdentifier(node)) {
      const initializer = boundTo(node);
      if (initializer && !following.has(initializer)) {
        following.add(initializer);
        value(initializer);
        following.delete(initializer);
      }
      return;
    }
    for (const part of partsOf(node)) {
      value(part);
    }
  };
  const visit = (node: ts.Node) => {
    if (
      ts.isJsxAttribute(node) &&
      node.name.getText() === "className" &&
      node.initializer &&
      (on === undefined || tagOf(node) === on)
    ) {
      value(node.initializer);
      return;
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return out;
}

/** The values a prop the expression reads (`size`, `props.size`) can hold,
 * where the caller knows them: a component's props as one call hands them. */
export type Given = (
  read: ts.Expression,
) => readonly (string | undefined)[] | undefined;

/** One class list per branch; a token with an unread part comes back as a
 * pattern (`ds-gap-*`). Throws past `cap` branches. */
export function classVariants(
  source: ts.SourceFile,
  node: ts.Node | undefined,
  cap = 16,
  given?: Given,
): string[][] {
  if (!node) return [[]];
  const lists = readerFor(source, node, cap, given).lists(node);
  return [...new Map(lists.map((v) => [v.join(" "), v])).values()];
}

// Where an interpolation could not be read, in a template's rendered text.
const CUT = "\0";

function tokensOf(text: string, keepCut: boolean): string[] {
  return text.split(/\s+/).flatMap((token) => {
    if (!token.includes(CUT)) return token ? [token] : [];
    const pattern = token.replaceAll(/\0+/g, "*");
    return keepCut && pattern !== "*" ? [pattern] : [];
  });
}

// A class expression read for both entry points above: as the class lists it
// can produce, and, inside a template, as the texts it renders.
function readerFor(
  source: ts.SourceFile,
  root?: ts.Node,
  cap = Infinity,
  given?: Given,
) {
  const handed = (n: ts.Node) => {
    if (ts.isIdentifier(n)) return boundTo(n) ? undefined : given?.(n);
    return ts.isPropertyAccessExpression(n) ? given?.(n) : undefined;
  };
  // Which way a test can go: both, unless it names a handed value.
  const ways = (test: ts.Expression) => {
    const values = handed(test);
    if (!values) return { yes: true, no: true };
    return { yes: values.some(Boolean), no: values.some((v) => !v) };
  };
  const following = new Set<ts.Node>();
  const capped = <T>(all: T[]): T[] => {
    if (all.length <= cap) return all;
    const at = root ?? source;
    const { line } = source.getLineAndCharacterOfPosition(at.getStart(source));
    throw new Error(
      `${source.fileName}:${line + 1}: \`${at.getText(source)}\` has more than ${cap} class branches; name the branches as consts and join one`,
    );
  };
  const product = <T>(parts: T[][], join: (a: T, b: T) => T, unit: T) =>
    parts.reduce<T[]>(
      (all, part) => capped(all.flatMap((a) => part.map((b) => join(a, b)))),
      [unit],
    );
  const followed = <T>(
    n: ts.Identifier | ts.PropertyAccessExpression,
    read: (e: ts.Node) => T[],
    none: T[],
    value: (v: string) => T,
  ) => {
    const values = handed(n);
    if (values) return values.map((v) => value(v ?? ""));
    const initializer = ts.isIdentifier(n) ? boundTo(n) : undefined;
    if (!initializer || following.has(initializer)) return none;
    following.add(initializer);
    const out = read(initializer);
    following.delete(initializer);
    return out;
  };
  const texts = (n: ts.Node): string[] => {
    if (ts.isStringLiteral(n) || ts.isNoSubstitutionTemplateLiteral(n)) {
      return [n.text];
    }
    if (ts.isTemplateExpression(n)) {
      const spans = n.templateSpans.flatMap((s) => [
        texts(s.expression),
        [s.literal.text],
      ]);
      return product([[n.head.text], ...spans], (a, b) => a + b, "");
    }
    if (ts.isIdentifier(n) || ts.isPropertyAccessExpression(n)) {
      return followed(n, texts, [CUT], (v) => v);
    }
    if (ts.isConditionalExpression(n)) return chosen(n).flatMap(texts);
    if (ts.isParenthesizedExpression(n)) return texts(n.expression);
    return ts.isBinaryExpression(n) ? binaryTexts(n) : listTexts(n);
  };
  // An empty list may be a call that computes a string (`look(state)`): unread.
  // Not a raw array: it renders comma-joined, which is no class list.
  const listTexts = (n: ts.Node): string[] =>
    ts.isCallExpression(n)
      ? lists(n).map((l) => l.join(" ").replaceAll("*", CUT) || CUT)
      : [CUT];
  const binaryTexts = (n: ts.BinaryExpression): string[] => {
    const kind = n.operatorToken.kind;
    if (kind === ts.SyntaxKind.AmpersandAmpersandToken) {
      const { yes, no } = ways(n.left);
      return [...(no ? [""] : []), ...(yes ? texts(n.right) : [])];
    }
    if (kind === ts.SyntaxKind.PlusToken) {
      return product([texts(n.left), texts(n.right)], (a, b) => a + b, "");
    }
    return joins(kind) ? [...texts(n.left), ...texts(n.right)] : [CUT];
  };
  const chosen = (n: ts.ConditionalExpression) => {
    const { yes, no } = ways(n.condition);
    return [...(yes ? [n.whenTrue] : []), ...(no ? [n.whenFalse] : [])];
  };
  const join = (a: string[], b: string[]) => [...a, ...b];
  const binary = (n: ts.BinaryExpression): string[][] => {
    const kind = n.operatorToken.kind;
    if (kind === ts.SyntaxKind.AmpersandAmpersandToken) {
      const { yes, no } = ways(n.left);
      return capped([...(no ? [[]] : []), ...(yes ? lists(n.right) : [])]);
    }
    if (kind === ts.SyntaxKind.PlusToken) {
      return texts(n).map((t) => tokensOf(t, true));
    }
    return joins(kind) ? capped([...lists(n.left), ...lists(n.right)]) : [[]];
  };
  // A conditional is the union of its branches, as is a wrapper of one part.
  const words = (v: string) => v.split(/\s+/).filter(Boolean);
  const lists = (n: ts.Node): string[][] => {
    if (ts.isStringLiteral(n) || ts.isNoSubstitutionTemplateLiteral(n)) {
      return [words(n.text)];
    }
    if (ts.isTemplateExpression(n)) {
      return texts(n).map((t) => tokensOf(t, true));
    }
    if (ts.isIdentifier(n) || ts.isPropertyAccessExpression(n)) {
      return followed(n, lists, [[]], words);
    }
    return ts.isBinaryExpression(n) ? binary(n) : compound(n);
  };
  const compound = (n: ts.Node): string[][] => {
    if (ts.isConditionalExpression(n)) return capped(chosen(n).flatMap(lists));
    if (ts.isCallExpression(n) || ts.isArrayLiteralExpression(n)) {
      return product(partsOf(n).map(lists), join, []);
    }
    const branches = capped(partsOf(n).flatMap(lists));
    return branches.length > 0 ? branches : [[]];
  };
  return { texts, lists };
}

/**
 * The sub-expressions of a class list that are themselves class lists.
 *
 * A comparison contributes nothing: in `state === "asc" ? " up" : ""`, `asc`
 * is a column state rather than a class anybody wrote.
 */
function partsOf(node: ts.Node): readonly ts.Node[] {
  if (ts.isParenthesizedExpression(node) || ts.isAsExpression(node)) {
    return [node.expression];
  }
  if (ts.isJsxExpression(node)) {
    return node.expression ? [node.expression] : [];
  }
  if (ts.isConditionalExpression(node)) {
    return [node.whenTrue, node.whenFalse];
  }
  if (ts.isBinaryExpression(node)) {
    return joins(node.operatorToken.kind) ? [node.left, node.right] : [];
  }
  if (ts.isCallExpression(node)) {
    // `[…].filter(Boolean).join(" ")` — the list is the callee's SUBJECT rather
    // than an argument, and it is the list that holds the names.
    return ts.isPropertyAccessExpression(node.expression)
      ? [...node.arguments, node.expression.expression]
      : node.arguments;
  }
  if (ts.isArrayLiteralExpression(node)) {
    return node.elements;
  }
  return [];
}

/** An operator that joins two class lists rather than comparing two values. */
function joins(kind: ts.SyntaxKind): boolean {
  return (
    kind === ts.SyntaxKind.AmpersandAmpersandToken ||
    kind === ts.SyntaxKind.BarBarToken ||
    kind === ts.SyntaxKind.QuestionQuestionToken ||
    kind === ts.SyntaxKind.PlusToken
  );
}

/**
 * The element an attribute stands on. An attribute's parent is the attribute
 * LIST and its parent is the tag, so the element a class carries is two steps
 * up rather than one.
 */
function tagOf(attribute: ts.JsxAttribute): string | undefined {
  const tag = attribute.parent.parent;
  return ts.isJsxOpeningElement(tag) || ts.isJsxSelfClosingElement(tag)
    ? tag.tagName.getText()
    : undefined;
}

function boundTo(name: ts.Identifier): ts.Expression | undefined {
  const declared = declarationOf(name);
  return declared && ts.isVariableDeclaration(declared)
    ? declared.initializer
    : undefined;
}

type Declared =
  | ts.VariableDeclaration
  | ts.ParameterDeclaration
  | ts.FunctionDeclaration
  | ts.FunctionExpression
  | ts.ClassDeclaration
  | ts.EnumDeclaration;

/** The variable or function an identifier reads in its own scope. Nothing when
 * its nearest binding is a parameter, a destructured name, a class or an enum. */
export function declarationOf(
  name: ts.Identifier,
):
  | ts.VariableDeclaration
  | ts.FunctionDeclaration
  | ts.FunctionExpression
  | undefined {
  const found = bindingOf(name);
  if (!found) return undefined;
  if (ts.isVariableDeclaration(found)) {
    return ts.isIdentifier(found.name) ? found : undefined;
  }
  const fn = ts.isFunctionDeclaration(found) || ts.isFunctionExpression(found);
  return fn ? found : undefined;
}

/** The nearest binding of an identifier, of any kind. Nothing means the file
 * declares no such name, so it is an import or a global. */
export function bindingOf(name: ts.Identifier): Declared | undefined {
  for (let scope = name.parent; scope; scope = scope.parent) {
    const found = bindingsOf(scope).get(name.text);
    if (found) return found;
  }
  return undefined;
}

const scopes = new WeakMap<ts.Node, Map<string, Declared>>();
function bindingsOf(scope: ts.Node): Map<string, Declared> {
  const known = scopes.get(scope);
  if (known) return known;
  const out = new Map<string, Declared>();
  for (const declared of declaredIn(scope)) {
    for (const name of declared.name ? namesOf(declared.name) : []) {
      if (!out.has(name)) out.set(name, declared);
    }
  }
  scopes.set(scope, out);
  return out;
}

// Parameters first: one sharing a function expression's name shadows it.
function declaredIn(scope: ts.Node): readonly Declared[] {
  if (ts.isFunctionLike(scope)) {
    const named = ts.isFunctionExpression(scope) && scope.name ? [scope] : [];
    return [...scope.parameters, ...named];
  }
  if (ts.isBlock(scope) || ts.isSourceFile(scope) || ts.isModuleBlock(scope)) {
    return scope.statements.flatMap(declaredBy);
  }
  if (ts.isCaseBlock(scope)) {
    return scope.clauses.flatMap((c) => c.statements.flatMap(declaredBy));
  }
  if (ts.isCatchClause(scope)) {
    return scope.variableDeclaration ? [scope.variableDeclaration] : [];
  }
  const loop =
    ts.isForStatement(scope) ||
    ts.isForInStatement(scope) ||
    ts.isForOfStatement(scope);
  const head = loop ? scope.initializer : undefined;
  return head && ts.isVariableDeclarationList(head) ? head.declarations : [];
}

function declaredBy(statement: ts.Statement): readonly Declared[] {
  if (ts.isVariableStatement(statement)) {
    return statement.declarationList.declarations;
  }
  const named =
    ts.isFunctionDeclaration(statement) ||
    ts.isClassDeclaration(statement) ||
    ts.isEnumDeclaration(statement);
  return named ? [statement] : [];
}

function namesOf(binding: ts.BindingName): string[] {
  if (ts.isIdentifier(binding)) return [binding.text];
  return binding.elements.flatMap((e) =>
    ts.isBindingElement(e) ? namesOf(e.name) : [],
  );
}
