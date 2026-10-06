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
    // A LOCAL BINDING IS FOLLOWED. `const classes = [...].join(" ")` and then
    // `className={classes}` is the ordinary way a component with three or four
    // conditional classes is written, and a reader that stopped at the
    // identifier recorded nothing for it — an orphan in one of those was
    // invisible. Followed ONCE per name, because a binding that refers to
    // itself would otherwise be walked forever.
    if (ts.isIdentifier(node)) {
      const initializer = read.bound.get(node.text);
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

/** The values a name the expression reads can hold, where the caller knows
 * them: a component's props as one call site hands them. */
export type Given = (
  name: string,
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
  const bound = bindingsIn(source);
  const handed = (n: ts.Node) =>
    ts.isIdentifier(n) && !bound.has(n.text) ? given?.(n.text) : undefined;
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
    n: ts.Identifier,
    read: (e: ts.Node) => T[],
    none: T[],
    value: (v: string) => T,
  ) => {
    const values = handed(n);
    if (values) return values.map((v) => value(v ?? ""));
    const initializer = bound.get(n.text);
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
    if (ts.isIdentifier(n)) return followed(n, texts, [CUT], (v) => v);
    if (ts.isConditionalExpression(n)) return chosen(n).flatMap(texts);
    if (ts.isParenthesizedExpression(n)) return texts(n.expression);
    return ts.isBinaryExpression(n) ? binaryTexts(n) : listTexts(n);
  };
  // An empty list may be a call that computes a string (`look(state)`): unread.
  const listTexts = (n: ts.Node): string[] =>
    ts.isCallExpression(n) || ts.isArrayLiteralExpression(n)
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
    if (ts.isIdentifier(n)) return followed(n, lists, [[]], words);
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
  return { bound, texts, lists };
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

/**
 * Every `const` in the module, by name, with what it was assigned.
 *
 * Read once per file rather than resolved through the checker: what a class
 * list is assembled from is nearly always a literal in the same module, and a
 * name shadowed in an inner scope resolves to whichever the walk saw last —
 * which over-reads rather than under-reads, the direction a census is allowed
 * to be wrong in.
 */
const boundIn = new WeakMap<ts.SourceFile, Map<string, ts.Expression>>();
function bindingsIn(source: ts.SourceFile): Map<string, ts.Expression> {
  const known = boundIn.get(source);
  if (known) return known;
  const out = new Map<string, ts.Expression>();
  boundIn.set(source, out);
  const visit = (node: ts.Node) => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.initializer
    ) {
      out.set(node.name.text, node.initializer);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return out;
}
