// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * What a `className` attribute PRODUCES, for the gates that census the markup.
 *
 * There are two of them now — classcoverage.test.ts asks whether every class an
 * element carries is declared by a sheet, controlheight.test.ts asks which
 * classes stand on a `<button>` — and both have to answer the same question
 * first: given the expression in a `className`, which class names can it be
 * shown to produce? A second answer to that is two readers that drift, and a
 * reader whose reach is narrower reads a smaller tree and reports the same
 * word, PASS. Same reason `./css.ts` exists.
 */

import ts from "typescript";

/** One class name a module puts on an element, and the node that carries it. */
export type ClassName = { name: string; at: ts.Node };

/**
 * The class names the `className` attributes in one module can be shown to
 * produce, in source order. `on` narrows to one intrinsic element — `"button"`
 * reads the classes that stand on a button and nothing else; omitted, every
 * className in the module is read.
 *
 * Read as VALUES rather than as every string in the subtree, which is the
 * difference between auditing a class list and auditing the code around it.
 * `` `lt-arrow${state === "asc" ? " up" : ""}` `` produces `lt-arrow` and
 * sometimes `up`; `asc` is a column state being compared, and a gate that
 * counted it would report a class nobody wrote. So a conditional contributes
 * its two branches and not its question, a comparison contributes nothing, and
 * a template's interpolations are not descended into at all.
 *
 * What IS read: literals, both branches of a conditional, both sides of `&&`,
 * `||`, `??` and `+`, every argument of a call (`cx("row", open && "row-open")`)
 * and every element of an array that is joined into one. The ordinary dynamic
 * class list is covered rather than waved past.
 *
 * THE ONE BLIND SPOT is a name whose own text is computed — the `tone-` of
 * `` `tone-${level}` ``. It is not a class this can look up, and guessing at the
 * variants would make a gate report names that exist and miss names that do
 * not. So the token touching an interpolation is dropped rather than half-read,
 * and what remains of such a template — every whole token in it — is still
 * read. The base class of a variant pair is nearly always one of those, so the
 * shape the blind spot hides is a suffix on a base the caller has seen.
 */
export function classNamesOn(source: ts.SourceFile, on?: string): ClassName[] {
  const out: ClassName[] = [];
  const bound = bindingsIn(source);
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
    if (ts.isTemplateExpression(node)) {
      // `head` runs up to the first `${`, and each span's literal runs from one
      // interpolation to the next. A piece flush against an interpolation ends
      // in a PREFIX rather than a name, or begins with a suffix — both are
      // dropped, leaving the whole tokens between them.
      add(whole(node.head.text, false, true), node.head);
      node.templateSpans.forEach((span, index) => {
        const last = index === node.templateSpans.length - 1;
        add(whole(span.literal.text, true, !last), span.literal);
      });
      return;
    }
    // A LOCAL BINDING IS FOLLOWED. `const classes = [...].join(" ")` and then
    // `className={classes}` is the ordinary way a component with three or four
    // conditional classes is written, and a reader that stopped at the
    // identifier recorded nothing for it — an orphan in one of those was
    // invisible. Followed ONCE per name, because a binding that refers to
    // itself would otherwise be walked forever.
    if (ts.isIdentifier(node)) {
      const initializer = bound.get(node.text);
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

/**
 * The sub-expressions of a class list that are themselves class lists.
 *
 * A comparison contributes nothing and an interpolation is not descended into:
 * `` `lt-arrow${state === "asc" ? " up" : ""}` `` produces `lt-arrow` and
 * sometimes `up`, and `asc` is a column state rather than a class anybody
 * wrote.
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
function bindingsIn(source: ts.SourceFile): Map<string, ts.Expression> {
  const out = new Map<string, ts.Expression>();
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

/** A template piece with the partial token at either flush end removed. */
function whole(text: string, dropFirst: boolean, dropLast: boolean): string {
  const tokens = text.split(/\s+/);
  if (dropFirst && !/^\s/.test(text)) {
    tokens.shift();
  }
  if (dropLast && !/\s$/.test(text)) {
    tokens.pop();
  }
  return tokens.join(" ");
}
