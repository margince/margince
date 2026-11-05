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
    return isMember(n) ? given?.(n) : undefined;
  };
  // Which way a test can go: both, unless it names a handed value.
  const ways = (test: ts.Expression) => {
    const values = handed(test);
    if (!values) return { yes: true, no: true };
    return { yes: values.some(Boolean), no: values.some((v) => !v) };
  };
  const following = new Set<ts.Node>();
  // Two branches that draw the same list are one branch.
  const capped = <T>(all: T[]): T[] => {
    if (all.length <= cap) return all;
    const unique = [
      ...new Map(all.map((a) => [JSON.stringify(a), a])).values(),
    ];
    if (unique.length <= cap) return unique;
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
    n: ts.Identifier | Member,
    read: (e: ts.Node) => T[],
    none: T[],
    value: (v: string) => T,
  ) => {
    const values = handed(n);
    if (values) return values.map((v) => value(v ?? ""));
    const initializer = ts.isIdentifier(n) ? boundTo(n) : undefined;
    if (!initializer || following.has(initializer)) return none;
    if (ts.isIdentifier(n) && changedAfter(n)) return none;
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
    if (ts.isIdentifier(n) || isMember(n)) {
      return followed(n, texts, [CUT], (v) => v);
    }
    if (ts.isConditionalExpression(n)) return chosen(n).flatMap(texts);
    if (ts.isParenthesizedExpression(n)) return texts(n.expression);
    return ts.isBinaryExpression(n) ? binaryTexts(n) : listTexts(n);
  };
  // An empty list may be a call that computes a string (`look(state)`): unread.
  // Not a raw array: it renders comma-joined, which is no class list.
  const listTexts = (n: ts.Node): string[] => {
    if (!ts.isCallExpression(n)) return [CUT];
    const returned = returnedTexts(n);
    if (returned.some((t) => t.replaceAll(CUT, "").trim())) return returned;
    return lists(n).map((l) => l.join(" ").replaceAll("*", CUT) || CUT);
  };
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
    if (ts.isIdentifier(n) || isMember(n)) {
      return followed(n, lists, [[]], words);
    }
    return ts.isBinaryExpression(n) ? binary(n) : compound(n);
  };
  const compound = (n: ts.Node): string[][] => {
    if (ts.isConditionalExpression(n)) return capped(chosen(n).flatMap(lists));
    if (ts.isCallExpression(n) || ts.isArrayLiteralExpression(n)) {
      const returned = returnedBy(n);
      if (returned.some((l) => l.length > 0)) return returned;
      return product(partsOf(n).map(lists), join, []);
    }
    const branches = capped(partsOf(n).flatMap(lists));
    return branches.length > 0 ? branches : [[]];
  };
  // A call of a function the file defines reads as what it returns, so
  // `cls()` and `useMemo(() => tone)` keep their classes. Its arguments are its
  // inputs, read as classes only when its returns name none (`join(...parts)`).
  const throughBody = <T>(n: ts.Node, read: (e: ts.Node) => T[]): T[] => {
    const fn = ts.isCallExpression(n) ? calledBody(n) : undefined;
    if (!fn || following.has(fn)) return [];
    following.add(fn);
    const out = capped(returnsOf(fn).flatMap(read));
    following.delete(fn);
    return out;
  };
  const returnedBy = (n: ts.Node) => throughBody(n, lists);
  // In a template the returned text keeps its spaces: `a${look()}` is two
  // classes when `look` returns " b".
  const returnedTexts = (n: ts.Node) => throughBody(n, texts);
  return { texts, lists };
}

type Member = ts.PropertyAccessExpression | ts.ElementAccessExpression;
// `props.tone`, or `props["tone"]` spelled with a literal key.
const isMember = (n: ts.Node): n is Member =>
  ts.isPropertyAccessExpression(n) ||
  (ts.isElementAccessExpression(n) &&
    ts.isStringLiteralLike(n.argumentExpression));

type Body = ts.ArrowFunction | ts.FunctionExpression | ts.FunctionDeclaration;
// The function a call runs: one the file binds by name, or the one `useMemo`
// is handed. An import or a parameter has no body here to read.
function calledBody(call: ts.CallExpression): Body | undefined {
  const callee = call.expression;
  const hook = ts.isPropertyAccessExpression(callee) ? callee.name : callee;
  if (ts.isIdentifier(hook) && hook.text === "useMemo") {
    const [fn] = call.arguments;
    return fn && isBody(fn) ? fn : undefined;
  }
  const declared = ts.isIdentifier(callee) ? declarationOf(callee) : undefined;
  if (!declared || ts.isFunctionDeclaration(declared)) return declared;
  if (ts.isFunctionExpression(declared)) return declared;
  const init = declared.initializer;
  return init && isBody(init) ? init : undefined;
}
const isBody = (n: ts.Node): n is ts.ArrowFunction | ts.FunctionExpression =>
  ts.isArrowFunction(n) || ts.isFunctionExpression(n);

// The expressions a function returns, not those of a function nested in it.
function returnsOf(fn: Body): ts.Expression[] {
  if (!fn.body) return [];
  if (!ts.isBlock(fn.body)) return [fn.body];
  const out: ts.Expression[] = [];
  const visit = (n: ts.Node) => {
    if (ts.isReturnStatement(n) && n.expression) out.push(n.expression);
    if (!ts.isFunctionLike(n)) ts.forEachChild(n, visit);
  };
  ts.forEachChild(fn.body, visit);
  return out;
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

const MUTATORS = new Set(["push", "unshift", "splice", "fill"]);
// Whether the variable a name reads is written again after its declaration,
// so its initializer is not all it holds: `classes.push(…)` or `cls += …`.
function changedAfter(name: ts.Identifier): boolean {
  const declared = declarationOf(name);
  if (!declared || !ts.isVariableDeclaration(declared)) return false;
  const same = (e: ts.Expression) =>
    ts.isIdentifier(e) && e.text === name.text && declarationOf(e) === declared;
  const writes = (n: ts.Node): boolean => {
    const target = writtenBy(n);
    if (target && same(target)) return true;
    return ts.forEachChild(n, writes) ?? false;
  };
  return writes(declared.parent.parent.parent);
}

// The variable a write lands on: the list a mutator is called on, or the left
// side of an assignment.
function writtenBy(n: ts.Node): ts.Expression | undefined {
  if (ts.isCallExpression(n) && ts.isPropertyAccessExpression(n.expression)) {
    const { expression, name } = n.expression;
    return MUTATORS.has(name.text) ? expression : undefined;
  }
  const assigns =
    ts.isBinaryExpression(n) &&
    n.operatorToken.kind >= ts.SyntaxKind.FirstAssignment &&
    n.operatorToken.kind <= ts.SyntaxKind.LastAssignment;
  if (!assigns) return undefined;
  return ts.isElementAccessExpression(n.left) ? n.left.expression : n.left;
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
